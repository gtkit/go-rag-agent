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
	cfg          Config
	store        storage.VectorStore
	loader       DocumentLoader
	embedder     llm.Embedder
	reranker     Reranker
	toolRegistry *ToolRegistry
	runner       graph.Runner
	chunker      *rag.Chunker
	dispatcher   telemetry.Dispatcher
	callbacks    []Callback
	dirSync      *directorySyncState
	dirSyncErr   error
	dirSyncOnce  sync.Once

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
	reranker  Reranker
	topK      int
	threshold float32
	filter    storage.SearchFilter
	hybrid    bool
	rerank    bool
	opts      retrieval.Options
}

func newRootRetriever(store storage.VectorStore, embedder llm.Embedder, reranker Reranker, topK int, threshold float32, filter storage.SearchFilter, hybrid bool, rerank bool, opts retrieval.Options) *rootRetriever {
	return &rootRetriever{
		store:     store,
		embedder:  embedder,
		reranker:  reranker,
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
		rerankedHits, rerankErr := safeRerank(ctx, r.reranker, query, fusedHits, shortlistSize, r.topK)
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
	store, err := newAgentVectorStore(cfg)
	if err != nil {
		return nil, fmt.Errorf("create vector store: %w", err)
	}
	loader := cfg.Storage.DocumentLoader
	if loader == nil {
		loader = NewFileDocumentLoader()
	}
	reranker := cfg.Storage.Reranker
	if reranker == nil {
		reranker = NewRuleBasedReranker()
	}

	embedder := cfg.Runtime.Embedder
	if embedder == nil {
		embedder, err = llm.NewOpenAIEmbedder(ctx, llm.EmbeddingConfig{
			Model:   cfg.EmbeddingModel,
			BaseURL: firstNonEmpty(cfg.EmbeddingBaseURL, cfg.ChatBaseURL),
			APIKey:  firstNonEmpty(cfg.EmbeddingAPIKey, cfg.ChatAPIKey),
			Timeout: cfg.RequestTimeout,
		})
		if err != nil {
			_ = store.Close()
			return nil, fmt.Errorf("create embedder: %w", err)
		}
	}

	chatModel := cfg.Runtime.ChatModel
	if chatModel == nil {
		chatModel, err = llm.NewOpenAIChatModel(ctx, llm.ChatConfig{
			Model:   cfg.ChatModel,
			BaseURL: cfg.ChatBaseURL,
			APIKey:  cfg.ChatAPIKey,
			Timeout: cfg.RequestTimeout,
		})
		if err != nil {
			_ = store.Close()
			return nil, fmt.Errorf("create chat model: %w", err)
		}
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
		newRootRetriever(store, embedder, reranker, cfg.TopK, float32(cfg.SimilarityThreshold), storage.SearchFilter{}, cfg.EnableHybridSearch, cfg.EnableRerank, retrievalOptions),
	)
	toolRegistry := cfg.ToolRegistry.clone()
	if toolRegistry == nil {
		toolRegistry = NewToolRegistry()
	}
	if _, ok := toolRegistry.Lookup(retrieveToolName); !ok {
		_ = toolRegistry.registerOrReplace(retrievalTool)
	}
	if webSearcher := newWebSearcher(cfg); webSearcher != nil {
		if _, ok := toolRegistry.Lookup("search_web"); !ok {
			_ = toolRegistry.registerOrReplace(tools.NewWebSearchTool(webSearcher))
		}
	}
	toolset := make([]tools.Tool, 0, len(toolRegistry.Tools()))
	for _, tool := range toolRegistry.Tools() {
		toolset = append(toolset, tool)
	}
	runner, err := graph.NewChatRunner(chatModel, toolset...)
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("create chat runner: %w", err)
	}

	dirSync, err := newDirectorySyncState(cfg.DataDir)
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("create directory sync state: %w", err)
	}

	return &Agent{
		cfg:          cfg,
		store:        store,
		loader:       loader,
		embedder:     embedder,
		reranker:     reranker,
		toolRegistry: toolRegistry,
		runner:       runner,
		chunker:      chunker,
		dispatcher:   dispatcher,
		callbacks:    rootCallbacks,
		dirSync:      dirSync,
		sessions:     make(map[string]*Session),
	}, nil
}

type toolCallBudget struct {
	limit int
	used  int
}

func newToolCallBudget(limit int) *toolCallBudget {
	if limit <= 0 {
		limit = 4
	}
	return &toolCallBudget{limit: limit}
}

func (b *toolCallBudget) Acquire(tool string) error {
	if b == nil {
		return nil
	}
	if b.used >= b.limit {
		return fmt.Errorf("%w: tool=%s limit=%d", ErrToolCallLimitExceeded, tool, b.limit)
	}
	b.used++
	return nil
}

func newAgentVectorStore(cfg Config) (storage.VectorStore, error) {
	if cfg.Storage.VectorStore != nil {
		return rootToInternalVectorStore{inner: cfg.Storage.VectorStore}, nil
	}
	return storage.NewChromemStore(storage.Config{
		DataDir:    cfg.DataDir,
		Collection: defaultCollectionName,
	})
}

type rootToInternalVectorStore struct {
	inner VectorStore
}

func (s rootToInternalVectorStore) Upsert(ctx context.Context, chunks []storage.ChunkRecord) error {
	return s.inner.Upsert(ctx, fromInternalChunkRecords(chunks))
}

func (s rootToInternalVectorStore) SearchWithFilter(ctx context.Context, queryEmbedding []float32, topK int, threshold float32, filter storage.SearchFilter) ([]storage.SearchHit, error) {
	hits, err := s.inner.SearchWithFilter(ctx, queryEmbedding, topK, threshold, fromInternalSearchFilter(filter))
	if err != nil {
		return nil, err
	}
	return toInternalSearchHits(hits), nil
}

func (s rootToInternalVectorStore) DeleteBySourcePaths(ctx context.Context, sourcePaths []string) error {
	return s.inner.DeleteBySourcePaths(ctx, sourcePaths)
}

func (s rootToInternalVectorStore) Search(ctx context.Context, queryEmbedding []float32, topK int, threshold float32) ([]storage.SearchHit, error) {
	hits, err := s.inner.Search(ctx, queryEmbedding, topK, threshold)
	if err != nil {
		return nil, err
	}
	return toInternalSearchHits(hits), nil
}

func (s rootToInternalVectorStore) Close() error {
	return s.inner.Close()
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

func (a *Agent) recordExecutionTrace(ctx context.Context, s *Session, trace ExecutionTrace) error {
	if a.cfg.TraceRecorder == nil && a.cfg.Logger == nil {
		return nil
	}

	if a.cfg.TraceRecorder != nil {
		if err := a.runTelemetryCallback(s, func() {
			a.cfg.TraceRecorder.OnExecutionTrace(ctx, trace)
		}); err != nil {
			return err
		}
	}

	if a.cfg.Logger == nil {
		return nil
	}

	for _, fallback := range trace.Fallbacks {
		fallback := fallback
		if err := a.runTelemetryCallback(s, func() {
			a.cfg.Logger.Warn("ragagent fallback",
				"session_id", trace.SessionID,
				"stage", fallback.Stage,
				"fallback_to", fallback.FallbackTo,
				"error", fallback.Err,
			)
		}); err != nil {
			return err
		}
	}

	logFn := a.cfg.Logger.Info
	if trace.Err != nil {
		logFn = a.cfg.Logger.Error
	}
	return a.runTelemetryCallback(s, func() {
		logFn("ragagent execution complete",
			"session_id", trace.SessionID,
			"stream", trace.Stream,
			"success", trace.Success,
			"query", trace.Query,
			"rewritten_query", trace.RewrittenQuery,
			"duration", trace.Duration,
			"citations", len(trace.Citations),
			"retrieval_hits", trace.Retrieval.FinalHitCount,
			"error", trace.Err,
		)
	})
}

type graphToolObserver struct {
	onStart func(context.Context, string) error
	onEnd   func(context.Context, string, error) error
}

func (o graphToolObserver) OnToolStart(ctx context.Context, tool string) error {
	if o.onStart == nil {
		return nil
	}
	return o.onStart(ctx, tool)
}

func (o graphToolObserver) OnToolEnd(ctx context.Context, tool string, err error) error {
	if o.onEnd == nil {
		return nil
	}
	return o.onEnd(ctx, tool, err)
}

func (a *Agent) newGraphToolObserver(s *Session, trace *executionTraceBuilder, emit func(StreamEvent) error) graph.ToolObserver {
	return graphToolObserver{
		onStart: func(ctx context.Context, tool string) error {
			if trace != nil {
				trace.startTool(tool)
			}
			if emit != nil && tool != retrieveToolName {
				if err := emit(StreamEvent{Type: EventToolStart, ToolName: tool}); err != nil {
					return err
				}
			}
			return a.runTelemetryCallback(s, func() { a.dispatcher.OnToolStart(ctx, tool) })
		},
		onEnd: func(ctx context.Context, tool string, err error) error {
			if trace != nil {
				trace.endTool(tool, err)
			}
			if emit != nil && tool != retrieveToolName {
				if streamErr := emit(StreamEvent{Type: EventToolEnd, ToolName: tool, Err: err}); streamErr != nil {
					return streamErr
				}
			}
			return a.runTelemetryCallback(s, func() { a.dispatcher.OnToolEnd(ctx, tool, err) })
		},
	}
}

func (a *Agent) retrieve(ctx context.Context, s *Session, query string, filter storage.SearchFilter, trace *executionTraceBuilder, budget *toolCallBudget) ([]storage.SearchHit, string, error) {
	if err := budget.Acquire(retrieveToolName); err != nil {
		return nil, "", err
	}
	if err := a.runTelemetryCallback(s, func() { a.dispatcher.OnRetrieveStart(ctx, query) }); err != nil {
		return nil, "", err
	}
	if err := a.runTelemetryCallback(s, func() { a.dispatcher.OnToolStart(ctx, retrieveToolName) }); err != nil {
		return nil, "", err
	}
	if trace != nil {
		trace.startTool(retrieveToolName)
	}

	var (
		hits      []storage.SearchHit
		keptHits  []storage.SearchHit
		runErr    error
		evidence  string
		metrics   RetrievalMetrics
		fallbacks []FallbackEvent
		retriever = newRootRetriever(a.store, a.embedder, a.reranker, a.cfg.TopK, float32(a.cfg.SimilarityThreshold), filter, a.cfg.EnableHybridSearch, a.cfg.EnableRerank, retrieval.Options{
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
		if trace != nil {
			trace.endTool(retrieveToolName, runErr)
		}
		metrics.FinalHitCount = len(keptHits)
		if trace != nil {
			trace.setRetrieval(metrics, fallbacks)
		}
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

func (a *Agent) askLocked(ctx context.Context, s *Session, query string, opts QueryOptions) (answer Answer, err error) {
	return a.askWithFormatLocked(ctx, s, query, opts, "")
}

func (a *Agent) askStructuredLocked(ctx context.Context, s *Session, query string, opts QueryOptions, target any) (StructuredAnswer, error) {
	responseFormatInstruction, err := buildStructuredInstruction(target)
	if err != nil {
		return StructuredAnswer{}, err
	}
	answer, err := a.askWithFormatLocked(ctx, s, query, opts, responseFormatInstruction)
	if err != nil {
		return StructuredAnswer{Answer: answer}, err
	}
	rawJSON, err := extractStructuredJSON(answer.Text)
	if err != nil {
		return StructuredAnswer{Answer: answer}, err
	}
	if err := unmarshalStructuredJSON(rawJSON, target); err != nil {
		return StructuredAnswer{
			Answer:  answer,
			RawJSON: rawJSON,
		}, err
	}
	return StructuredAnswer{
		Answer:  answer,
		RawJSON: rawJSON,
	}, nil
}

func (a *Agent) askWithFormatLocked(ctx context.Context, s *Session, query string, opts QueryOptions, responseFormatInstruction string) (answer Answer, err error) {
	ctx, cancel := a.withExecutionBudget(ctx)
	defer cancel()
	if err = ctx.Err(); err != nil {
		return Answer{}, err
	}

	rewrittenQuery := rag.RewriteFollowUp(query, s.history.LastUserQueries())
	trace := newExecutionTraceBuilder(s.id, query, rewrittenQuery, opts.Filter, false)
	budget := newToolCallBudget(a.cfg.MaxToolCalls)
	defer func() {
		if trace == nil {
			return
		}
		if answer.Trace == nil && err == nil {
			finalTrace := trace.finish(nil)
			answer.Trace = &finalTrace
		}
		finalTrace := trace.finish(err)
		if err == nil {
			answer.Trace = &finalTrace
		}
		if recordErr := a.recordExecutionTrace(ctx, s, finalTrace); recordErr != nil {
			answer = Answer{}
			err = recordErr
		}
	}()

	hits, evidenceText, err := a.retrieve(ctx, s, rewrittenQuery, opts.storageFilter(), trace, budget)
	err = normalizeExecutionBudgetError(ctx, err)
	if err != nil {
		if a.hasFallbackTools() && errors.Is(err, ErrEvidenceInsufficient) {
			hits = nil
			evidenceText = ""
		} else {
			return Answer{}, err
		}
	}

	modelStartedAt := time.Now()
	modelName := a.cfg.chatModelName()
	if err := a.runTelemetryCallback(s, func() { a.dispatcher.OnModelStart(ctx, modelName) }); err != nil {
		return Answer{}, err
	}
	answerText, err := a.runner.Ask(ctx, graph.Request{
		Query:                     rewrittenQuery,
		History:                   s.history.Turns(),
		EvidenceText:              evidenceText,
		MaxPromptTokens:           a.cfg.MaxPromptTokens,
		MaxHistoryTokens:          a.cfg.MaxHistoryTokens,
		MaxEvidenceTokens:         a.cfg.MaxEvidenceTokens,
		MaxSummaryTokens:          a.cfg.MaxSummaryTokens,
		EnablePromptHardening:     a.cfg.EnablePromptHardening,
		ResponseFormatInstruction: responseFormatInstruction,
		ToolObserver:              a.newGraphToolObserver(s, trace, nil),
		ToolCallLimiter:           budget,
	})
	err = normalizeExecutionBudgetError(ctx, err)
	if endErr := a.runTelemetryCallback(s, func() { a.dispatcher.OnModelEnd(ctx, modelName, err) }); endErr != nil && err == nil {
		err = endErr
	}
	modelMetrics := ModelMetrics{
		Model:       modelName,
		Duration:    time.Since(modelStartedAt),
		Stream:      false,
		OutputChars: len(answerText),
	}
	if trace != nil {
		trace.setModel(modelMetrics)
	}
	if metricsErr := a.emitModelMetrics(ctx, s, modelMetrics); metricsErr != nil && err == nil {
		err = metricsErr
	}
	if err != nil {
		return Answer{}, fmt.Errorf("run answer generation: %w", err)
	}

	citations := citationsFromHits(hits)
	if trace != nil {
		trace.setCitations(citations)
	}
	return Answer{
		Text:      answerText,
		Citations: citations,
	}, nil
}

func (a *Agent) askStreamLocked(ctx context.Context, s *Session, query string, opts QueryOptions, emit func(StreamEvent) error) (string, error) {
	ctx, cancel := a.withExecutionBudget(ctx)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if emit == nil {
		return "", fmt.Errorf("stream emitter is required")
	}

	trace := newExecutionTraceBuilder(s.id, query, rag.RewriteFollowUp(query, s.history.LastUserQueries()), opts.Filter, true)
	budget := newToolCallBudget(a.cfg.MaxToolCalls)
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
		finalTrace := trace.finish(runErr)
		if err := emitEvent(StreamEvent{Type: EventError, Err: runErr, Trace: &finalTrace}); err != nil {
			return err
		}
		if err := a.recordExecutionTrace(ctx, s, finalTrace); err != nil {
			return err
		}
		return runErr
	}

	rewrittenQuery := trace.trace.RewrittenQuery
	filter := opts.storageFilter()
	if err := emitEvent(StreamEvent{Type: EventRetrieveStart, Content: rewrittenQuery}); err != nil {
		return "", err
	}
	if err := emitEvent(StreamEvent{Type: EventToolStart, ToolName: retrieveToolName}); err != nil {
		return "", err
	}

	hits, evidenceText, err := a.retrieve(ctx, s, rewrittenQuery, filter, trace, budget)
	err = normalizeExecutionBudgetError(ctx, err)
	if err != nil {
		if a.hasFallbackTools() && errors.Is(err, ErrEvidenceInsufficient) {
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
	trace.setCitations(citations)
	for i := range citations {
		citation := citations[i]
		if err := emitEvent(StreamEvent{Type: EventCitation, Citation: &citation}); err != nil {
			return "", err
		}
	}

	var answerBuilder strings.Builder
	var emitterErr error
	doneStep := 0
	modelStartedAt := time.Now()
	modelName := a.cfg.chatModelName()
	if err := a.runTelemetryCallback(s, func() { a.dispatcher.OnModelStart(ctx, modelName) }); err != nil {
		return "", err
	}
	err = a.runner.AskStream(ctx, graph.Request{
		Query:                 rewrittenQuery,
		History:               s.history.Turns(),
		EvidenceText:          evidenceText,
		MaxPromptTokens:       a.cfg.MaxPromptTokens,
		MaxHistoryTokens:      a.cfg.MaxHistoryTokens,
		MaxEvidenceTokens:     a.cfg.MaxEvidenceTokens,
		MaxSummaryTokens:      a.cfg.MaxSummaryTokens,
		EnablePromptHardening: a.cfg.EnablePromptHardening,
		ToolObserver:          a.newGraphToolObserver(s, trace, emitEvent),
		ToolCallLimiter:       budget,
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
			doneStep = event.Step
			return nil
		default:
			return nil
		}
	})
	err = normalizeExecutionBudgetError(ctx, err)
	if endErr := a.runTelemetryCallback(s, func() { a.dispatcher.OnModelEnd(ctx, modelName, err) }); endErr != nil && err == nil {
		err = endErr
	}
	modelMetrics := ModelMetrics{
		Model:       modelName,
		Duration:    time.Since(modelStartedAt),
		Stream:      true,
		OutputChars: answerBuilder.Len(),
	}
	trace.setModel(modelMetrics)
	if metricsErr := a.emitModelMetrics(ctx, s, modelMetrics); metricsErr != nil && err == nil {
		err = metricsErr
	}
	if err != nil {
		if emitterErr != nil && errors.Is(err, emitterErr) {
			return "", err
		}
		return "", emitError(err)
	}

	finalTrace := trace.finish(nil)
	if err := emitEvent(StreamEvent{
		Type:  EventDone,
		Step:  doneStep,
		Trace: &finalTrace,
	}); err != nil {
		return "", err
	}
	if err := a.recordExecutionTrace(ctx, s, finalTrace); err != nil {
		return "", err
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

func safeRerank(ctx context.Context, reranker Reranker, query string, candidates []storage.SearchHit, shortlistSize int, topK int) (hits []storage.SearchHit, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("rerank panic: %v", recovered)
		}
	}()
	if reranker == nil {
		return retrieval.RerankShortlistWithLimit(query, candidates, shortlistSize, topK), nil
	}
	reranked, err := reranker.Rerank(ctx, query, fromInternalSearchHits(candidates), RerankOptions{
		ShortlistSize: shortlistSize,
		TopK:          topK,
	})
	if err != nil {
		return nil, err
	}
	return toInternalSearchHits(reranked), nil
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

func (c Config) chatModelName() string {
	if trimmed := strings.TrimSpace(c.ChatModel); trimmed != "" {
		return trimmed
	}
	if c.Runtime.ChatModel != nil {
		return "custom-chat-model"
	}
	return "chat-model"
}

func (a *Agent) hasFallbackTools() bool {
	if a.cfg.EnableWebSearch {
		return true
	}
	if a.toolRegistry == nil {
		return false
	}
	for _, tool := range a.toolRegistry.Tools() {
		if tool == nil {
			continue
		}
		if tool.Name() == retrieveToolName {
			continue
		}
		return true
	}
	return false
}

func (a *Agent) withExecutionBudget(ctx context.Context) (context.Context, context.CancelFunc) {
	if a.cfg.MaxExecutionDuration <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeoutCause(ctx, a.cfg.MaxExecutionDuration, ErrExecutionBudgetExceeded)
}

func normalizeExecutionBudgetError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) && errors.Is(context.Cause(ctx), ErrExecutionBudgetExceeded) {
		return fmt.Errorf("%w: %w", ErrExecutionBudgetExceeded, err)
	}
	return err
}
