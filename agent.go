package ragagent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"my-gtkit-package/go-rag-agent/internal/graph"
	"my-gtkit-package/go-rag-agent/internal/llm"
	"my-gtkit-package/go-rag-agent/internal/memory"
	"my-gtkit-package/go-rag-agent/internal/rag"
	"my-gtkit-package/go-rag-agent/internal/storage"
	"my-gtkit-package/go-rag-agent/internal/telemetry"
	"my-gtkit-package/go-rag-agent/internal/tools"
)

const (
	defaultCollectionName = "knowledge"
	retrieveToolName      = "retrieve_context"
	maxEvidenceChars      = 4000
)

// Agent is the root runtime that owns ingestion, retrieval, sessions, and execution wiring.
type Agent struct {
	cfg        Config
	store      storage.VectorStore
	embedder   llm.Embedder
	runner     graph.Runner
	chunker    *rag.Chunker
	dispatcher telemetry.Dispatcher

	sessionsMu sync.RWMutex
	sessions   map[string]*Session

	closed   atomic.Bool
	opMu     sync.Mutex
	opCond   *sync.Cond
	inFlight int
}

type rootRetriever struct {
	store     storage.VectorStore
	embedder  llm.Embedder
	topK      int
	threshold float32
}

func newRootRetriever(store storage.VectorStore, embedder llm.Embedder, topK int, threshold float32) *rootRetriever {
	return &rootRetriever{
		store:     store,
		embedder:  embedder,
		topK:      topK,
		threshold: threshold,
	}
}

func (r *rootRetriever) Search(ctx context.Context, query string) ([]storage.SearchHit, error) {
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("search query is required")
	}
	rows, err := r.embedder.EmbedTexts(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("embed search query: %w", err)
	}
	if len(rows) != 1 || len(rows[0]) == 0 {
		return nil, fmt.Errorf("query embedding is empty")
	}
	hits, err := r.store.Search(ctx, rows[0], r.topK, r.threshold)
	if err != nil {
		return nil, fmt.Errorf("search vector store: %w", err)
	}
	return hits, nil
}

// New creates a phase-1 root agent with embedded storage and OpenAI-compatible adapters.
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

	callbacks := make([]telemetry.Callback, 0, len(cfg.Callbacks))
	for _, cb := range cfg.Callbacks {
		if cb == nil {
			continue
		}
		callbacks = append(callbacks, cb)
	}
	dispatcher := telemetry.NewDispatcher(callbacks)
	retrievalTool := tools.NewRetrievalTool(
		newRootRetriever(store, embedder, cfg.TopK, float32(cfg.SimilarityThreshold)),
	)
	runner, err := graph.NewReactRunner(ctx, chatModel, retrievalTool, cfg.MaxIterations)
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("create react runner: %w", err)
	}

	return &Agent{
		cfg:        cfg,
		store:      store,
		embedder:   embedder,
		runner:     runner,
		chunker:    chunker,
		dispatcher: dispatcher,
		sessions:   make(map[string]*Session),
	}, nil
}

// GetSession returns one stable session instance per ID.
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

// Close releases all sessions and underlying storage resources.
func (a *Agent) Close() error {
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

func (a *Agent) retrieve(ctx context.Context, query string) ([]storage.SearchHit, string, error) {
	a.dispatcher.OnRetrieveStart(ctx, query)
	a.dispatcher.OnToolStart(ctx, retrieveToolName)

	var (
		hits      []storage.SearchHit
		keptHits  []storage.SearchHit
		runErr    error
		evidence  string
		retriever = newRootRetriever(a.store, a.embedder, a.cfg.TopK, float32(a.cfg.SimilarityThreshold))
	)
	defer func() {
		a.dispatcher.OnRetrieveEnd(ctx, len(keptHits), runErr)
		a.dispatcher.OnToolEnd(ctx, retrieveToolName, runErr)
	}()

	hits, runErr = retriever.Search(ctx, query)
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

func (a *Agent) askLocked(ctx context.Context, s *Session, query string) (Answer, error) {
	if err := ctx.Err(); err != nil {
		return Answer{}, err
	}

	rewrittenQuery := rag.RewriteFollowUp(query, s.history.LastUserQueries())
	hits, evidenceText, err := a.retrieve(ctx, rewrittenQuery)
	if err != nil {
		return Answer{}, err
	}

	a.dispatcher.OnModelStart(ctx, a.cfg.ChatModel)
	answerText, err := a.runner.Ask(ctx, graph.Request{
		Query:        rewrittenQuery,
		History:      s.history.Turns(),
		EvidenceText: evidenceText,
	})
	a.dispatcher.OnModelEnd(ctx, a.cfg.ChatModel, err)
	if err != nil {
		return Answer{}, fmt.Errorf("run answer generation: %w", err)
	}

	s.history.Append(query, answerText)
	return Answer{
		Text:      answerText,
		Citations: citationsFromHits(hits),
	}, nil
}

func (a *Agent) askStreamLocked(ctx context.Context, s *Session, query string, emit func(StreamEvent) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if emit == nil {
		return fmt.Errorf("stream emitter is required")
	}

	emitEvent := func(event StreamEvent) error {
		event.Timestamp = time.Now()
		return emit(event)
	}

	rewrittenQuery := rag.RewriteFollowUp(query, s.history.LastUserQueries())
	if err := emitEvent(StreamEvent{Type: EventRetrieveStart, Content: rewrittenQuery}); err != nil {
		return err
	}
	if err := emitEvent(StreamEvent{Type: EventToolStart, ToolName: retrieveToolName}); err != nil {
		return err
	}

	hits, evidenceText, err := a.retrieve(ctx, rewrittenQuery)
	if err != nil {
		emitErr := emitEvent(StreamEvent{Type: EventError, Err: err})
		if emitErr != nil {
			return emitErr
		}
		return err
	}
	if err := emitEvent(StreamEvent{Type: EventRetrieveEnd}); err != nil {
		return err
	}
	if err := emitEvent(StreamEvent{Type: EventToolEnd, ToolName: retrieveToolName}); err != nil {
		return err
	}

	citations := citationsFromHits(hits)
	for i := range citations {
		citation := citations[i]
		if err := emitEvent(StreamEvent{Type: EventCitation, Citation: &citation}); err != nil {
			return err
		}
	}

	var answerBuilder strings.Builder
	err = a.runner.AskStream(ctx, graph.Request{
		Query:        rewrittenQuery,
		History:      s.history.Turns(),
		EvidenceText: evidenceText,
	}, func(event graph.Event) error {
		switch event.Type {
		case graph.EventAnswerChunk:
			answerBuilder.WriteString(event.Content)
			return emitEvent(StreamEvent{
				Type:    EventAnswerChunk,
				Content: event.Content,
				Step:    event.Step,
			})
		case graph.EventDone:
			return emitEvent(StreamEvent{
				Type: EventDone,
				Step: event.Step,
			})
		default:
			return nil
		}
	})
	if err != nil {
		return err
	}

	s.history.Append(query, answerBuilder.String())
	return nil
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
