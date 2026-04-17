package ragagent

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	einotool "github.com/cloudwego/eino/components/tool"

	"github.com/gtkit/go-rag-agent/internal/graph"
	"github.com/gtkit/go-rag-agent/internal/llm"
	"github.com/gtkit/go-rag-agent/internal/memory"
	"github.com/gtkit/go-rag-agent/internal/rag"
	"github.com/gtkit/go-rag-agent/internal/retrieval"
	"github.com/gtkit/go-rag-agent/internal/storage"
	"github.com/gtkit/go-rag-agent/internal/telemetry"
	"github.com/gtkit/go-rag-agent/internal/tools"
)

const (
	defaultCollectionName = "knowledge"
	retrieveToolName      = "retrieve_context"
	maxEvidenceChars      = 4000
)

// Agent 是根运行时对象，负责知识导入、检索、会话与执行编排。
type Agent struct {
	cfg         Config
	store       storage.VectorStore
	embedder    llm.Embedder
	runner      graph.Runner
	chunker     *rag.Chunker
	dispatcher  telemetry.Dispatcher
	callbacks   []Callback
	dirSync     *directorySyncState
	dirSyncErr  error
	dirSyncOnce sync.Once

	sessionsMu sync.RWMutex
	sessions   map[string]*Session

	closed        atomic.Bool
	callbackDepth atomic.Int32
	opMu          sync.Mutex
	opCond        *sync.Cond
	inFlight      int
}

type rootRetriever struct {
	store     storage.VectorStore
	embedder  llm.Embedder
	topK      int
	threshold float32
	filter    storage.SearchFilter
	hybrid    bool
	rerank    bool
	opts      retrieval.Options
}

func newRootRetriever(store storage.VectorStore, embedder llm.Embedder, topK int, threshold float32, filter storage.SearchFilter, hybrid bool, rerank bool, opts retrieval.Options) *rootRetriever {
	return &rootRetriever{
		store:     store,
		embedder:  embedder,
		topK:      topK,
		threshold: threshold,
		filter:    filter,
		hybrid:    hybrid,
		rerank:    rerank,
		opts:      opts,
	}
}

func (r *rootRetriever) Search(ctx context.Context, query string) ([]storage.SearchHit, error) {
	hits, _, _, err := r.SearchDetailed(ctx, query)
	return hits, err
}

func (r *rootRetriever) SearchDetailed(ctx context.Context, query string) ([]storage.SearchHit, RetrievalMetrics, []FallbackEvent, error) {
	metrics := RetrievalMetrics{
		HybridEnabled: r.hybrid,
		RerankEnabled: r.rerank,
	}
	startedAt := time.Now()
	finish := func() RetrievalMetrics {
		metrics.Duration = time.Since(startedAt)
		return metrics
	}

	if strings.TrimSpace(query) == "" {
		return nil, finish(), nil, fmt.Errorf("search query is required")
	}
	rows, err := r.embedder.EmbedTexts(ctx, []string{query})
	if err != nil {
		return nil, finish(), nil, fmt.Errorf("embed search query: %w", err)
	}
	if len(rows) != 1 || len(rows[0]) == 0 {
		return nil, finish(), nil, fmt.Errorf("query embedding is empty")
	}

	vectorOnlySearch := func() ([]storage.SearchHit, error) {
		hits, err := r.store.SearchWithFilter(ctx, rows[0], r.topK, r.threshold, r.filter)
		if err != nil {
			return nil, fmt.Errorf("search vector store: %w", err)
		}
		return hits, nil
	}

	if !r.hybrid {
		hits, err := vectorOnlySearch()
		if err != nil {
			return nil, finish(), nil, err
		}
		metrics.VectorCandidateCount = len(hits)
		return hits, finish(), nil, nil
	}

	if r.opts.CandidateMultiplier < 0 || r.opts.RRFK < 0 {
		hits, err := vectorOnlySearch()
		if err != nil {
			return nil, finish(), nil, err
		}
		metrics.VectorCandidateCount = len(hits)
		return hits, finish(), []FallbackEvent{{
			Stage:      FallbackStageHybrid,
			FallbackTo: FallbackTargetVectorOnly,
			Err:        fmt.Errorf("invalid hybrid tuning options"),
		}}, nil
	}

	opts := r.opts.Normalize()
	allHits, err := r.store.SearchWithFilter(ctx, rows[0], math.MaxInt, -1, r.filter)
	if err != nil {
		hits, fallbackErr := vectorOnlySearch()
		if fallbackErr != nil {
			return nil, finish(), nil, fmt.Errorf("search vector store for hybrid retrieval: %w", err)
		}
		metrics.VectorCandidateCount = len(hits)
		return hits, finish(), []FallbackEvent{{
			Stage:      FallbackStageHybrid,
			FallbackTo: FallbackTargetVectorOnly,
			Err:        fmt.Errorf("search vector store for hybrid retrieval: %w", err),
		}}, nil
	}
	if len(allHits) == 0 {
		return nil, finish(), nil, nil
	}

	candidateLimit := opts.CandidateLimit(r.topK)
	vectorHits := make([]storage.SearchHit, 0, min(candidateLimit, len(allHits)))
	for _, hit := range allHits {
		if hit.Score < r.threshold {
			continue
		}
		vectorHits = append(vectorHits, hit)
		if len(vectorHits) == candidateLimit {
			break
		}
	}
	metrics.VectorCandidateCount = len(vectorHits)
	lexicalHits, lexicalErr := safeLexicalSearch(query, allHits, candidateLimit)
	if lexicalErr != nil {
		if len(vectorHits) > r.topK {
			vectorHits = vectorHits[:r.topK]
		}
		return vectorHits, finish(), []FallbackEvent{{
			Stage:      FallbackStageHybrid,
			FallbackTo: FallbackTargetVectorOnly,
			Err:        lexicalErr,
		}}, nil
	}
	metrics.LexicalCandidateCount = len(lexicalHits)
	fusedHits := retrieval.FuseRRFWithK(vectorHits, lexicalHits, candidateLimit, opts.RRFK)
	metrics.FusedCandidateCount = len(fusedHits)
	if r.rerank {
		if r.opts.RerankMultiplier < 0 {
			if len(fusedHits) > r.topK {
				fusedHits = fusedHits[:r.topK]
			}
			return fusedHits, finish(), []FallbackEvent{{
				Stage:      FallbackStageRerank,
				FallbackTo: FallbackTargetHybrid,
				Err:        fmt.Errorf("invalid rerank tuning options"),
			}}, nil
		}
		shortlistSize := opts.RerankShortlistSize(r.topK, len(fusedHits))
		metrics.RerankShortlistCount = shortlistSize
		rerankedHits, rerankErr := safeRerank(query, fusedHits, shortlistSize, r.topK)
		if rerankErr != nil {
			if len(fusedHits) > r.topK {
				fusedHits = fusedHits[:r.topK]
			}
			return fusedHits, finish(), []FallbackEvent{{
				Stage:      FallbackStageRerank,
				FallbackTo: FallbackTargetHybrid,
				Err:        rerankErr,
			}}, nil
		}
		return rerankedHits, finish(), nil, nil
	}
	if len(fusedHits) > r.topK {
		fusedHits = fusedHits[:r.topK]
	}
	return fusedHits, finish(), nil, nil
}

// New 创建一个 Phase 1 根 Agent，使用嵌入式存储与 OpenAI-compatible 适配器。
func New(cfg Config) (*Agent, error) {
	cfg = cfg.withDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	ctx := context.Background()
	store, err := storage.NewChromemStore(storage.Config{
		DataDir:    cfg.DataDir,
		Collection: defaultCollectionName,
	})
	if err != nil {
		return nil, fmt.Errorf("create vector store: %w", err)
	}

	embedder, err := llm.NewOpenAIEmbedder(ctx, llm.EmbeddingConfig{
		Model:   cfg.EmbeddingModel,
		BaseURL: firstNonEmpty(cfg.EmbeddingBaseURL, cfg.ChatBaseURL),
		APIKey:  firstNonEmpty(cfg.EmbeddingAPIKey, cfg.ChatAPIKey),
		Timeout: cfg.RequestTimeout,
	})
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("create embedder: %w", err)
	}

	chatModel, err := llm.NewOpenAIChatModel(ctx, llm.ChatConfig{
		Model:   cfg.ChatModel,
		BaseURL: cfg.ChatBaseURL,
		APIKey:  cfg.ChatAPIKey,
		Timeout: cfg.RequestTimeout,
	})
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("create chat model: %w", err)
	}

	chunker, err := rag.NewChunker(cfg.ChunkSize, cfg.ChunkOverlap)
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("create chunker: %w", err)
	}

	rootCallbacks := make([]Callback, 0, len(cfg.Callbacks))
	callbacks := make([]telemetry.Callback, 0, len(cfg.Callbacks))
	for _, cb := range cfg.Callbacks {
		if cb == nil {
			continue
		}
		rootCallbacks = append(rootCallbacks, cb)
		callbacks = append(callbacks, cb)
	}
	dispatcher := telemetry.NewDispatcher(callbacks)
	retrievalOptions := retrieval.Options{
		CandidateMultiplier: cfg.HybridCandidateMultiplier,
		RRFK:                cfg.HybridRRFK,
		RerankMultiplier:    cfg.RerankShortlistMultiplier,
	}
	retrievalTool := tools.NewRetrievalTool(
		newRootRetriever(store, embedder, cfg.TopK, float32(cfg.SimilarityThreshold), storage.SearchFilter{}, cfg.EnableHybridSearch, cfg.EnableRerank, retrievalOptions),
	)
	toolset := []einotool.BaseTool{retrievalTool}
	if webSearcher := newWebSearcher(cfg); webSearcher != nil {
		toolset = append(toolset, tools.NewWebSearchTool(webSearcher))
	}
	runner, err := graph.NewReactRunner(ctx, chatModel, cfg.MaxIterations, toolset...)
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("create react runner: %w", err)
	}

	dirSync, err := newDirectorySyncState(cfg.DataDir)
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("create directory sync state: %w", err)
	}

	return &Agent{
		cfg:        cfg,
		store:      store,
		embedder:   embedder,
		runner:     runner,
		chunker:    chunker,
		dispatcher: dispatcher,
		callbacks:  rootCallbacks,
		dirSync:    dirSync,
		sessions:   make(map[string]*Session),
	}, nil
}

// GetSession 为给定 ID 返回稳定复用的 Session 实例。
func (a *Agent) GetSession(id string) *Session {
	if a.isClosed() {
		return &Session{
			agent:   a,
			id:      id,
			history: memory.NewHistory(a.cfg.MaxHistoryRounds),
			closed:  true,
		}
	}

	a.sessionsMu.Lock()
	defer a.sessionsMu.Unlock()

	if a.isClosed() {
		return &Session{
			agent:   a,
			id:      id,
			history: memory.NewHistory(a.cfg.MaxHistoryRounds),
			closed:  true,
		}
	}
	if session, ok := a.sessions[id]; ok {
		return session
	}

	session := &Session{
		agent:   a,
		id:      id,
		history: memory.NewHistory(a.cfg.MaxHistoryRounds),
	}
	a.sessions[id] = session
	return session
}

// Close 释放所有会话以及底层存储资源。
func (a *Agent) Close() error {
	if a.callbackDepth.Load() > 0 {
		return fmt.Errorf("ragagent: agent close from callback is not supported")
	}
	if !a.closed.CompareAndSwap(false, true) {
		return nil
	}

	a.opMu.Lock()
	a.ensureOpCondLocked()
	for a.inFlight > 0 {
		a.opCond.Wait()
	}
	a.opMu.Unlock()

	a.sessionsMu.Lock()
	sessions := make([]*Session, 0, len(a.sessions))
	for _, session := range a.sessions {
		sessions = append(sessions, session)
	}
	a.sessions = nil
	a.sessionsMu.Unlock()

	for _, session := range sessions {
		_ = session.Close()
	}

	if a.store == nil {
		return nil
	}
	return a.store.Close()
}

func callbackPanicError(v any) error {
	return fmt.Errorf("ragagent: callback panic: %v", v)
}

func (a *Agent) ensureDirectorySync() (*directorySyncState, error) {
	a.dirSyncOnce.Do(func() {
		if a.dirSync != nil || a.dirSyncErr != nil {
			return
		}
		a.dirSync, a.dirSyncErr = newDirectorySyncState(a.cfg.DataDir)
	})
	if a.dirSyncErr != nil {
		return nil, fmt.Errorf("ensure directory sync state: %w", a.dirSyncErr)
	}
	if a.dirSync == nil {
		return nil, fmt.Errorf("directory sync state is nil")
	}
	return a.dirSync, nil
}

func (a *Agent) runTelemetryCallback(s *Session, fn func()) (err error) {
	a.callbackDepth.Add(1)
	defer a.callbackDepth.Add(-1)
	if s != nil {
		s.mu.Lock()
		s.emitting = true
		s.mu.Unlock()
		defer func() {
			s.mu.Lock()
			s.emitting = false
			s.mu.Unlock()
		}()
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = callbackPanicError(recovered)
		}
	}()
	fn()
	return nil
}

func (a *Agent) emitRetrievalMetrics(ctx context.Context, s *Session, metrics RetrievalMetrics) error {
	if len(a.callbacks) == 0 {
		return nil
	}
	return a.runTelemetryCallback(s, func() {
		for _, cb := range a.callbacks {
			if obs, ok := cb.(RetrievalMetricsCallback); ok {
				obs.OnRetrieveMetrics(ctx, metrics)
			}
		}
	})
}

func (a *Agent) emitModelMetrics(ctx context.Context, s *Session, metrics ModelMetrics) error {
	if len(a.callbacks) == 0 {
		return nil
	}
	return a.runTelemetryCallback(s, func() {
		for _, cb := range a.callbacks {
			if obs, ok := cb.(ModelMetricsCallback); ok {
				obs.OnModelMetrics(ctx, metrics)
			}
		}
	})
}

func (a *Agent) emitFallback(ctx context.Context, s *Session, event FallbackEvent) error {
	if len(a.callbacks) == 0 {
		return nil
	}
	return a.runTelemetryCallback(s, func() {
		for _, cb := range a.callbacks {
			if obs, ok := cb.(FallbackCallback); ok {
				obs.OnFallback(ctx, event)
			}
		}
	})
}

func (a *Agent) retrieve(ctx context.Context, s *Session, query string, filter storage.SearchFilter) ([]storage.SearchHit, string, error) {
	if err := a.runTelemetryCallback(s, func() { a.dispatcher.OnRetrieveStart(ctx, query) }); err != nil {
		return nil, "", err
	}
	if err := a.runTelemetryCallback(s, func() { a.dispatcher.OnToolStart(ctx, retrieveToolName) }); err != nil {
		return nil, "", err
	}

	var (
		hits      []storage.SearchHit
		keptHits  []storage.SearchHit
		runErr    error
		evidence  string
		metrics   RetrievalMetrics
		fallbacks []FallbackEvent
		retriever = newRootRetriever(a.store, a.embedder, a.cfg.TopK, float32(a.cfg.SimilarityThreshold), filter, a.cfg.EnableHybridSearch, a.cfg.EnableRerank, retrieval.Options{
			CandidateMultiplier: a.cfg.HybridCandidateMultiplier,
			RRFK:                a.cfg.HybridRRFK,
			RerankMultiplier:    a.cfg.RerankShortlistMultiplier,
		})
	)
	defer func() {
		if endErr := a.runTelemetryCallback(s, func() { a.dispatcher.OnRetrieveEnd(ctx, len(keptHits), runErr) }); endErr != nil && runErr == nil {
			runErr = endErr
		}
		if endErr := a.runTelemetryCallback(s, func() { a.dispatcher.OnToolEnd(ctx, retrieveToolName, runErr) }); endErr != nil && runErr == nil {
			runErr = endErr
		}
		metrics.FinalHitCount = len(keptHits)
		if metricsErr := a.emitRetrievalMetrics(ctx, s, metrics); metricsErr != nil && runErr == nil {
			runErr = metricsErr
		}
		for _, event := range fallbacks {
			if fallbackErr := a.emitFallback(ctx, s, event); fallbackErr != nil && runErr == nil {
				runErr = fallbackErr
			}
		}
	}()

	hits, metrics, fallbacks, runErr = retriever.SearchDetailed(ctx, query)
	if runErr != nil {
		runErr = fmt.Errorf("retrieve hits: %w", runErr)
		return nil, "", runErr
	}

	ragChunks := make([]rag.Chunk, 0, len(hits))
	for _, hit := range hits {
		ragChunks = append(ragChunks, rag.Chunk{
			ChunkID:    hit.Chunk.ChunkID,
			ParentID:   hit.Chunk.ParentID,
			SourcePath: hit.Chunk.SourcePath,
			Title:      hit.Chunk.Title,
			Metadata:   hit.Chunk.Metadata,
			Text:       hit.Chunk.Text,
			StartRune:  hit.Chunk.StartRune,
			EndRune:    hit.Chunk.EndRune,
		})
	}

	keptChunks, evidenceText, err := rag.AssembleContext(ragChunks, maxEvidenceChars)
	if err != nil {
		switch {
		case errors.Is(err, rag.ErrInvalidContextLimit):
			runErr = errors.Join(ErrContextAssembly, err)
		case errors.Is(err, rag.ErrNoContextAssembled):
			runErr = errors.Join(ErrEvidenceInsufficient, err)
		default:
			runErr = fmt.Errorf("assemble evidence context: %w", err)
		}
		return nil, "", runErr
	}

	keptSet := make(map[string]struct{}, len(keptChunks))
	for _, chunk := range keptChunks {
		keptSet[chunk.ChunkID] = struct{}{}
	}
	keptHits = make([]storage.SearchHit, 0, len(keptChunks))
	for _, hit := range hits {
		if _, ok := keptSet[hit.Chunk.ChunkID]; !ok {
			continue
		}
		keptHits = append(keptHits, hit)
	}
	evidence = evidenceText

	return keptHits, evidence, nil
}

func citationsFromHits(hits []storage.SearchHit) []Citation {
	citations := make([]Citation, 0, len(hits))
	for _, hit := range hits {
		citations = append(citations, Citation{
			SourcePath: hit.Chunk.SourcePath,
			Title:      hit.Chunk.Title,
			ChunkID:    hit.Chunk.ChunkID,
			StartRune:  hit.Chunk.StartRune,
			EndRune:    hit.Chunk.EndRune,
		})
	}
	return citations
}

func (a *Agent) askLocked(ctx context.Context, s *Session, query string, opts QueryOptions) (Answer, error) {
	if err := ctx.Err(); err != nil {
		return Answer{}, err
	}

	rewrittenQuery := rag.RewriteFollowUp(query, s.history.LastUserQueries())
	hits, evidenceText, err := a.retrieve(ctx, s, rewrittenQuery, opts.storageFilter())
	if err != nil {
		if a.cfg.EnableWebSearch && errors.Is(err, ErrEvidenceInsufficient) {
			hits = nil
			evidenceText = ""
		} else {
			return Answer{}, err
		}
	}

	modelStartedAt := time.Now()
	if err := a.runTelemetryCallback(s, func() { a.dispatcher.OnModelStart(ctx, a.cfg.ChatModel) }); err != nil {
		return Answer{}, err
	}
	answerText, err := a.runner.Ask(ctx, graph.Request{
		Query:        rewrittenQuery,
		History:      s.history.Turns(),
		EvidenceText: evidenceText,
	})
	if endErr := a.runTelemetryCallback(s, func() { a.dispatcher.OnModelEnd(ctx, a.cfg.ChatModel, err) }); endErr != nil && err == nil {
		err = endErr
	}
	if metricsErr := a.emitModelMetrics(ctx, s, ModelMetrics{
		Model:       a.cfg.ChatModel,
		Duration:    time.Since(modelStartedAt),
		Stream:      false,
		OutputChars: len(answerText),
	}); metricsErr != nil && err == nil {
		err = metricsErr
	}
	if err != nil {
		return Answer{}, fmt.Errorf("run answer generation: %w", err)
	}

	return Answer{
		Text:      answerText,
		Citations: citationsFromHits(hits),
	}, nil
}

func (a *Agent) askStreamLocked(ctx context.Context, s *Session, query string, opts QueryOptions, emit func(StreamEvent) error) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if emit == nil {
		return "", fmt.Errorf("stream emitter is required")
	}

	emitEvent := func(event StreamEvent) error {
		event.Timestamp = time.Now()
		var emitErr error
		if err := a.runTelemetryCallback(s, func() {
			emitErr = emit(event)
		}); err != nil {
			return err
		}
		return emitErr
	}
	emitError := func(runErr error) error {
		if runErr == nil {
			return nil
		}
		if err := emitEvent(StreamEvent{Type: EventError, Err: runErr}); err != nil {
			return err
		}
		return runErr
	}

	rewrittenQuery := rag.RewriteFollowUp(query, s.history.LastUserQueries())
	filter := opts.storageFilter()
	if err := emitEvent(StreamEvent{Type: EventRetrieveStart, Content: rewrittenQuery}); err != nil {
		return "", err
	}
	if err := emitEvent(StreamEvent{Type: EventToolStart, ToolName: retrieveToolName}); err != nil {
		return "", err
	}

	hits, evidenceText, err := a.retrieve(ctx, s, rewrittenQuery, filter)
	if err != nil {
		if a.cfg.EnableWebSearch && errors.Is(err, ErrEvidenceInsufficient) {
			hits = nil
			evidenceText = ""
		} else {
			if emitErr := emitEvent(StreamEvent{Type: EventRetrieveEnd, Err: err}); emitErr != nil {
				return "", emitErr
			}
			if emitErr := emitEvent(StreamEvent{Type: EventToolEnd, ToolName: retrieveToolName, Err: err}); emitErr != nil {
				return "", emitErr
			}
			return "", emitError(err)
		}
	}
	if err := emitEvent(StreamEvent{Type: EventRetrieveEnd}); err != nil {
		return "", err
	}
	if err := emitEvent(StreamEvent{Type: EventToolEnd, ToolName: retrieveToolName}); err != nil {
		return "", err
	}

	citations := citationsFromHits(hits)
	for i := range citations {
		citation := citations[i]
		if err := emitEvent(StreamEvent{Type: EventCitation, Citation: &citation}); err != nil {
			return "", err
		}
	}

	var answerBuilder strings.Builder
	var emitterErr error
	modelStartedAt := time.Now()
	if err := a.runTelemetryCallback(s, func() { a.dispatcher.OnModelStart(ctx, a.cfg.ChatModel) }); err != nil {
		return "", err
	}
	err = a.runner.AskStream(ctx, graph.Request{
		Query:        rewrittenQuery,
		History:      s.history.Turns(),
		EvidenceText: evidenceText,
	}, func(event graph.Event) error {
		switch event.Type {
		case graph.EventAnswerChunk:
			answerBuilder.WriteString(event.Content)
			emittedErr := emitEvent(StreamEvent{
				Type:    EventAnswerChunk,
				Content: event.Content,
				Step:    event.Step,
			})
			if emittedErr != nil {
				emitterErr = emittedErr
			}
			return emittedErr
		case graph.EventDone:
			emittedErr := emitEvent(StreamEvent{
				Type: EventDone,
				Step: event.Step,
			})
			if emittedErr != nil {
				emitterErr = emittedErr
			}
			return emittedErr
		default:
			return nil
		}
	})
	if endErr := a.runTelemetryCallback(s, func() { a.dispatcher.OnModelEnd(ctx, a.cfg.ChatModel, err) }); endErr != nil && err == nil {
		err = endErr
	}
	if metricsErr := a.emitModelMetrics(ctx, s, ModelMetrics{
		Model:       a.cfg.ChatModel,
		Duration:    time.Since(modelStartedAt),
		Stream:      true,
		OutputChars: answerBuilder.Len(),
	}); metricsErr != nil && err == nil {
		err = metricsErr
	}
	if err != nil {
		if emitterErr != nil && errors.Is(err, emitterErr) {
			return "", err
		}
		return "", emitError(err)
	}

	return answerBuilder.String(), nil
}

func safeLexicalSearch(query string, candidates []storage.SearchHit, topK int) (hits []storage.SearchHit, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("lexical search panic: %v", recovered)
		}
	}()
	return retrieval.LexicalSearch(query, candidates, topK), nil
}

func safeRerank(query string, candidates []storage.SearchHit, shortlistSize int, topK int) (hits []storage.SearchHit, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("rerank panic: %v", recovered)
		}
	}()
	return retrieval.RerankShortlistWithLimit(query, candidates, shortlistSize, topK), nil
}

func (a *Agent) beginOperation() error {
	if a.isClosed() {
		return ErrAgentClosed
	}

	a.opMu.Lock()
	defer a.opMu.Unlock()

	if a.isClosed() {
		return ErrAgentClosed
	}
	a.ensureOpCondLocked()
	a.inFlight++
	return nil
}

func (a *Agent) endOperation() {
	a.opMu.Lock()
	defer a.opMu.Unlock()

	if a.inFlight <= 0 {
		return
	}
	a.inFlight--
	if a.inFlight == 0 {
		a.ensureOpCondLocked()
		a.opCond.Broadcast()
	}
}

func (a *Agent) ensureOpCondLocked() {
	if a.opCond == nil {
		a.opCond = sync.NewCond(&a.opMu)
	}
}

func (a *Agent) isClosed() bool {
	return a.closed.Load()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
