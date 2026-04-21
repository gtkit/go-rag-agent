package ragagent

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jung-kurt/gofpdf"

	"github.com/gtkit/go-rag-agent/internal/graph"
	"github.com/gtkit/go-rag-agent/internal/llm"
	"github.com/gtkit/go-rag-agent/internal/memory"
	"github.com/gtkit/go-rag-agent/internal/rag"
	"github.com/gtkit/go-rag-agent/internal/storage"
	"github.com/gtkit/go-rag-agent/internal/telemetry"
	"github.com/gtkit/go-rag-agent/internal/tools"
	"github.com/gtkit/go-rag-agent/internal/websearch"
)

type fakeStore struct {
	mu            sync.Mutex
	searchHits    []storage.SearchHit
	searchErr     error
	searchCalls   int
	lastFilter    storage.SearchFilter
	upsertBatches [][]storage.ChunkRecord
	deleteCalls   [][]string
}

func (f *fakeStore) Upsert(_ context.Context, chunks []storage.ChunkRecord) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.upsertBatches = append(f.upsertBatches, slices.Clone(chunks))
	return nil
}

func (f *fakeStore) Search(_ context.Context, _ []float32, topK int, threshold float32) ([]storage.SearchHit, error) {
	return f.SearchWithFilter(context.Background(), nil, topK, threshold, storage.SearchFilter{})
}

func (f *fakeStore) SearchWithFilter(_ context.Context, _ []float32, topK int, threshold float32, filter storage.SearchFilter) ([]storage.SearchHit, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.searchCalls++
	f.lastFilter = filter
	if f.searchErr != nil {
		return nil, f.searchErr
	}
	return filterHitsForTest(f.searchHits, filter, topK, threshold), nil
}

func (f *fakeStore) DeleteBySourcePaths(_ context.Context, sourcePaths []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.deleteCalls = append(f.deleteCalls, slices.Clone(sourcePaths))
	if len(sourcePaths) == 0 {
		return nil
	}
	sourceSet := make(map[string]struct{}, len(sourcePaths))
	for _, sourcePath := range sourcePaths {
		sourceSet[sourcePath] = struct{}{}
	}

	filteredHits := make([]storage.SearchHit, 0, len(f.searchHits))
	for _, hit := range f.searchHits {
		if _, ok := sourceSet[hit.Chunk.SourcePath]; ok {
			continue
		}
		filteredHits = append(filteredHits, hit)
	}
	f.searchHits = filteredHits
	return nil
}

func (f *fakeStore) Close() error { return nil }

type capturingRootVectorStoreStub struct {
	mu      sync.Mutex
	upserts [][]ChunkRecord
	hits    []SearchHit
}

func (s *capturingRootVectorStoreStub) Upsert(_ context.Context, chunks []ChunkRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.upserts = append(s.upserts, slices.Clone(chunks))
	return nil
}

func (s *capturingRootVectorStoreStub) Search(_ context.Context, _ []float32, topK int, threshold float32) ([]SearchHit, error) {
	return s.SearchWithFilter(context.Background(), nil, topK, threshold, SearchFilter{})
}

func (s *capturingRootVectorStoreStub) SearchWithFilter(_ context.Context, _ []float32, topK int, threshold float32, filter SearchFilter) ([]SearchHit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	filtered := make([]SearchHit, 0, len(s.hits))
	for _, hit := range s.hits {
		if hit.Score < threshold {
			continue
		}
		if len(filter.SourcePaths) > 0 && !slices.Contains(filter.SourcePaths, hit.Chunk.SourcePath) {
			continue
		}
		matched := true
		for key, value := range filter.Metadata {
			if hit.Chunk.Metadata[key] != value {
				matched = false
				break
			}
		}
		if !matched {
			continue
		}
		filtered = append(filtered, hit)
	}
	if topK > 0 && len(filtered) > topK {
		filtered = filtered[:topK]
	}
	return filtered, nil
}

func (s *capturingRootVectorStoreStub) DeleteBySourcePaths(_ context.Context, _ []string) error {
	return nil
}

func (s *capturingRootVectorStoreStub) Close() error { return nil }

func filterHitsForTest(hits []storage.SearchHit, filter storage.SearchFilter, topK int, threshold float32) []storage.SearchHit {
	if len(hits) == 0 {
		return nil
	}

	filtered := make([]storage.SearchHit, 0, len(hits))
	for _, hit := range hits {
		if hit.Score < threshold {
			continue
		}
		if !matchesSearchFilterForTest(hit.Chunk, filter) {
			continue
		}
		filtered = append(filtered, hit)
	}
	if topK > 0 && len(filtered) > topK {
		filtered = filtered[:topK]
	}
	return filtered
}

func matchesSearchFilterForTest(chunk storage.ChunkRecord, filter storage.SearchFilter) bool {
	if len(filter.SourcePaths) > 0 && !slices.Contains(filter.SourcePaths, chunk.SourcePath) {
		return false
	}
	if len(filter.SourcePrefixes) > 0 {
		matchedPrefix := false
		for _, prefix := range filter.SourcePrefixes {
			if strings.HasPrefix(chunk.SourcePath, prefix) {
				matchedPrefix = true
				break
			}
		}
		if !matchedPrefix {
			return false
		}
	}
	for key, value := range filter.Metadata {
		if chunk.Metadata[key] != value {
			return false
		}
	}
	return true
}

type fakeEmbedder struct {
	mu         sync.Mutex
	vectors    map[string][]float32
	defaultVec []float32
	err        error
	lastTexts  []string
}

func (f *fakeEmbedder) EmbedTexts(_ context.Context, texts []string) ([][]float32, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastTexts = slices.Clone(texts)
	if f.err != nil {
		return nil, f.err
	}
	out := make([][]float32, 0, len(texts))
	for _, text := range texts {
		if vec, ok := f.vectors[text]; ok {
			out = append(out, slices.Clone(vec))
			continue
		}
		out = append(out, slices.Clone(f.defaultVec))
	}
	return out, nil
}

type fakeRunner struct {
	mu      sync.Mutex
	answer  string
	err     error
	lastReq graph.Request
}

type fakeRootRetriever struct {
	hits []SearchHit
	err  error
	reqs []RetrieverRequest
	mu   sync.Mutex
}

func (f *fakeRootRetriever) Search(ctx context.Context, req RetrieverRequest) ([]SearchHit, error) {
	hits, _, _, err := f.SearchDetailed(ctx, req)
	return hits, err
}

func (f *fakeRootRetriever) SearchDetailed(_ context.Context, req RetrieverRequest) ([]SearchHit, RetrievalMetrics, []FallbackEvent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reqs = append(f.reqs, req)
	return slices.Clone(f.hits), RetrievalMetrics{}, nil, f.err
}

func TestAskUsesInjectedRetriever(t *testing.T) {
	t.Parallel()

	customRetriever := &fakeRootRetriever{
		hits: []SearchHit{
			{
				Chunk: ChunkRecord{
					ChunkID:    "custom:0",
					ParentID:   "custom",
					SourcePath: "/custom/doc.md",
					Title:      "custom",
					Text:       "custom retriever evidence",
					StartRune:  0,
					EndRune:    25,
				},
				Score: 0.99,
			},
		},
	}
	a, err := New(Config{
		ChatModel: "gpt-4o-mini",
		Runtime: RuntimeComponents{
			ChatModel: stubRuntimeChatModel{},
			Embedder:  stubRuntimeEmbedder{},
		},
		Retrieval: RetrievalComponents{
			Retriever: customRetriever,
		},
		TopK:             1,
		ChunkSize:        64,
		ChunkOverlap:     0,
		MaxHistoryRounds: 8,
		RequestTimeout:   time.Second,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() {
		if cerr := a.Close(); cerr != nil {
			t.Fatalf("Close() error = %v", cerr)
		}
	})

	answer, err := a.GetSession("custom-retriever").Ask(context.Background(), "what changed")
	if err != nil {
		t.Fatalf("Ask() error = %v", err)
	}
	if answer.Text == "" {
		t.Fatal("Ask() returned empty text")
	}

	customRetriever.mu.Lock()
	defer customRetriever.mu.Unlock()
	if len(customRetriever.reqs) != 1 {
		t.Fatalf("custom retriever requests = %d, want 1", len(customRetriever.reqs))
	}
	if customRetriever.reqs[0].Query == "" {
		t.Fatal("custom retriever query is empty")
	}
}

func TestAskTraceCapturesPromptCacheHit(t *testing.T) {
	t.Parallel()

	cache := NewInMemoryPromptCache()
	a, err := New(Config{
		ChatModel: "gpt-4o-mini",
		Runtime: RuntimeComponents{
			ChatModel: stubRuntimeChatModel{},
			Embedder:  stubRuntimeEmbedder{},
		},
		Storage: StorageComponents{
			VectorStore: &injectedVectorStoreStub{
				searchHits: []SearchHit{
					{
						Chunk: ChunkRecord{
							ChunkID:    "doc:0",
							ParentID:   "doc",
							SourcePath: "/tmp/doc.md",
							Title:      "doc",
							Text:       "prompt cache evidence",
							StartRune:  0,
							EndRune:    21,
						},
						Score: 0.99,
					},
				},
			},
		},
		PromptCache:      cache,
		TopK:             1,
		ChunkSize:        64,
		ChunkOverlap:     0,
		MaxHistoryRounds: 8,
		RequestTimeout:   time.Second,
		TraceRecorder:    &traceRecorderStub{},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() {
		if cerr := a.Close(); cerr != nil {
			t.Fatalf("Close() error = %v", cerr)
		}
	})

	session := a.GetSession("prompt-cache")
	if _, err := session.Ask(context.Background(), "same question"); err != nil {
		t.Fatalf("first Ask() error = %v", err)
	}
	if err := session.ClearHistory(context.Background()); err != nil {
		t.Fatalf("ClearHistory() error = %v", err)
	}
	answer, err := session.Ask(context.Background(), "same question")
	if err != nil {
		t.Fatalf("second Ask() error = %v", err)
	}
	if answer.Trace == nil {
		t.Fatal("Answer.Trace = nil")
	}
	if !answer.Trace.PromptCacheHit {
		t.Fatal("Answer.Trace.PromptCacheHit = false, want true")
	}
}

func TestAddKnowledgeAppliesAccessBoundaryNamespace(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := root + "/knowledge.md"
	if err := os.WriteFile(path, []byte("# Doc\n\nnamespaced content"), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}

	store := &capturingRootVectorStoreStub{}
	a, err := New(Config{
		Runtime: RuntimeComponents{
			ChatModel: stubRuntimeChatModel{},
			Embedder:  stubRuntimeEmbedder{},
		},
		Storage: StorageComponents{
			VectorStore: store,
		},
		AccessBoundary: AccessBoundaryConfig{
			Namespace: "tenant-a",
		},
		TopK:             1,
		ChunkSize:        64,
		ChunkOverlap:     0,
		MaxHistoryRounds: 8,
		RequestTimeout:   time.Second,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() {
		if cerr := a.Close(); cerr != nil {
			t.Fatalf("Close() error = %v", cerr)
		}
	})

	if err := a.AddKnowledge(context.Background(), FileSource(path)); err != nil {
		t.Fatalf("AddKnowledge() error = %v", err)
	}
	if len(store.upserts) == 0 || len(store.upserts[0]) == 0 {
		t.Fatal("upsert batches are empty")
	}
	if got := store.upserts[0][0].Metadata[accessBoundaryNamespaceKey]; got != "tenant-a" {
		t.Fatalf("stored namespace metadata = %q, want %q", got, "tenant-a")
	}
}

func TestAskAppliesAccessBoundarySourcePaths(t *testing.T) {
	t.Parallel()

	a, err := New(Config{
		ChatModel: "gpt-4o-mini",
		Runtime: RuntimeComponents{
			ChatModel: stubRuntimeChatModel{},
			Embedder:  stubRuntimeEmbedder{},
		},
		Storage: StorageComponents{
			VectorStore: &capturingRootVectorStoreStub{
				hits: []SearchHit{
					{
						Chunk: ChunkRecord{
							ChunkID:    "doc:0",
							ParentID:   "doc",
							SourcePath: "/tenant-a/doc.md",
							Title:      "doc",
							Text:       "tenant evidence",
							StartRune:  0,
							EndRune:    15,
							Metadata:   map[string]string{accessBoundaryNamespaceKey: "tenant-a"},
						},
						Score: 0.99,
					},
				},
			},
		},
		AccessBoundary: AccessBoundaryConfig{
			Namespace:          "tenant-a",
			AllowedSourcePaths: []string{"/tenant-a/doc.md"},
		},
		TopK:             1,
		ChunkSize:        64,
		ChunkOverlap:     0,
		MaxHistoryRounds: 8,
		RequestTimeout:   time.Second,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() {
		if cerr := a.Close(); cerr != nil {
			t.Fatalf("Close() error = %v", cerr)
		}
	})

	_, err = a.GetSession("tenant-boundary").AskWithOptions(context.Background(), "question", QueryOptions{
		Filter: RetrievalFilter{
			SourcePaths: []string{"/tenant-b/doc.md"},
		},
	})
	if !errors.Is(err, ErrEvidenceInsufficient) {
		t.Fatalf("AskWithOptions() error = %v, want errors.Is(..., %v)", err, ErrEvidenceInsufficient)
	}
}

func (f *fakeRunner) Ask(_ context.Context, req graph.Request) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastReq = req
	if f.err != nil {
		return "", f.err
	}
	return f.answer, nil
}

func (f *fakeRunner) AskStream(context.Context, graph.Request, graph.StreamEmitter) error {
	return fmt.Errorf("not implemented in task 3.4")
}

type blockingBudgetRunner struct {
	started chan struct{}
}

func (r *blockingBudgetRunner) Ask(ctx context.Context, req graph.Request) (string, error) {
	select {
	case r.started <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return "", ctx.Err()
}

func (r *blockingBudgetRunner) AskStream(context.Context, graph.Request, graph.StreamEmitter) error {
	return fmt.Errorf("not implemented")
}

type callbackRecorder struct {
	mu     sync.Mutex
	events []string
}

func (r *callbackRecorder) OnRetrieveStart(context.Context, string) {
	r.add("retrieve_start")
}

func (r *callbackRecorder) OnRetrieveEnd(context.Context, int, error) {
	r.add("retrieve_end")
}

func (r *callbackRecorder) OnToolStart(context.Context, string) {
	r.add("tool_start")
}

func (r *callbackRecorder) OnToolEnd(context.Context, string, error) {
	r.add("tool_end")
}

func (r *callbackRecorder) OnModelStart(context.Context, string) {
	r.add("model_start")
}

func (r *callbackRecorder) OnModelEnd(context.Context, string, error) {
	r.add("model_end")
}

func (r *callbackRecorder) add(event string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

func (r *callbackRecorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.events)
}

type detailedCallbackRecorder struct {
	callbackRecorder
	mu              sync.Mutex
	retrieveMetrics []RetrievalMetrics
	modelMetrics    []ModelMetrics
	fallbacks       []FallbackEvent
}

type traceRecorderStub struct {
	mu     sync.Mutex
	traces []ExecutionTrace
}

func (r *traceRecorderStub) OnExecutionTrace(_ context.Context, trace ExecutionTrace) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.traces = append(r.traces, trace)
}

func (r *traceRecorderStub) snapshot() []ExecutionTrace {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.traces)
}

type loggerEntry struct {
	level string
	msg   string
	kv    []any
}

type loggerStub struct {
	mu      sync.Mutex
	entries []loggerEntry
}

func (l *loggerStub) Debug(msg string, kv ...any) { l.add("debug", msg, kv...) }
func (l *loggerStub) Info(msg string, kv ...any)  { l.add("info", msg, kv...) }
func (l *loggerStub) Warn(msg string, kv ...any)  { l.add("warn", msg, kv...) }
func (l *loggerStub) Error(msg string, kv ...any) { l.add("error", msg, kv...) }

func (l *loggerStub) add(level string, msg string, kv ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, loggerEntry{
		level: level,
		msg:   msg,
		kv:    slices.Clone(kv),
	})
}

func (l *loggerStub) snapshot() []loggerEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.entries)
}

func (r *detailedCallbackRecorder) OnRetrieveMetrics(_ context.Context, metrics RetrievalMetrics) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.retrieveMetrics = append(r.retrieveMetrics, metrics)
}

func (r *detailedCallbackRecorder) OnModelMetrics(_ context.Context, metrics ModelMetrics) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.modelMetrics = append(r.modelMetrics, metrics)
}

func (r *detailedCallbackRecorder) OnFallback(_ context.Context, event FallbackEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fallbacks = append(r.fallbacks, event)
}

func (r *detailedCallbackRecorder) snapshotMetrics() ([]RetrievalMetrics, []ModelMetrics, []FallbackEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.retrieveMetrics), slices.Clone(r.modelMetrics), slices.Clone(r.fallbacks)
}

type panicCallback struct {
	panicOn string
}

func (c *panicCallback) OnRetrieveStart(context.Context, string) {
	if c.panicOn == "retrieve_start" {
		panic("telemetry panic")
	}
}

func (c *panicCallback) OnRetrieveEnd(context.Context, int, error) {
	if c.panicOn == "retrieve_end" {
		panic("telemetry panic")
	}
}

func (c *panicCallback) OnToolStart(context.Context, string) {
	if c.panicOn == "tool_start" {
		panic("telemetry panic")
	}
}

func (c *panicCallback) OnToolEnd(context.Context, string, error) {
	if c.panicOn == "tool_end" {
		panic("telemetry panic")
	}
}

func (c *panicCallback) OnModelStart(context.Context, string) {
	if c.panicOn == "model_start" {
		panic("telemetry panic")
	}
}

func (c *panicCallback) OnModelEnd(context.Context, string, error) {
	if c.panicOn == "model_end" {
		panic("telemetry panic")
	}
}

type blockingSource struct {
	started chan struct{}
	release chan struct{}
	files   []KnowledgeFile
}

type fakeTraceChatModel struct {
	answer string
}

func (f *fakeTraceChatModel) Generate(_ context.Context, _ []llm.Message) (llm.Message, error) {
	return llm.Message{
		Role:    llm.RoleAssistant,
		Content: f.answer,
	}, nil
}

func (f *fakeTraceChatModel) Stream(_ context.Context, _ []llm.Message, emit func(string) error) error {
	if emit == nil {
		return nil
	}
	return emit(f.answer)
}

type fakeSearcher struct {
	results []websearch.Result
	err     error
}

func (f *fakeSearcher) Search(context.Context, string) ([]websearch.Result, error) {
	if f.err != nil {
		return nil, f.err
	}
	return slices.Clone(f.results), nil
}

type injectedVectorStoreStub struct {
	mu          sync.Mutex
	upsertCalls int
	searchCalls int
	searchHits  []SearchHit
}

func (s *injectedVectorStoreStub) Upsert(context.Context, []ChunkRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.upsertCalls++
	return nil
}

func (s *injectedVectorStoreStub) Search(context.Context, []float32, int, float32) ([]SearchHit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.searchCalls++
	return slices.Clone(s.searchHits), nil
}

func (s *injectedVectorStoreStub) SearchWithFilter(context.Context, []float32, int, float32, SearchFilter) ([]SearchHit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.searchCalls++
	return slices.Clone(s.searchHits), nil
}

func (s *injectedVectorStoreStub) DeleteBySourcePaths(context.Context, []string) error { return nil }

func (s *injectedVectorStoreStub) Close() error { return nil }

func (s *injectedVectorStoreStub) snapshot() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.upsertCalls, s.searchCalls
}

type injectedDocumentLoaderStub struct {
	mu     sync.Mutex
	called bool
}

func (l *injectedDocumentLoaderStub) Load(context.Context, string, string, map[string]string, DocumentLoadOptions) (Document, error) {
	l.mu.Lock()
	l.called = true
	l.mu.Unlock()
	return Document{
		ID:         "doc",
		SourcePath: "/tmp/doc.md",
		Title:      "doc",
		Metadata:   map[string]string{"tag": "api"},
		Content:    "gateway api exact match",
	}, nil
}

func (l *injectedDocumentLoaderStub) wasCalled() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.called
}

type injectedRerankerStub struct {
	mu     sync.Mutex
	called bool
}

func (r *injectedRerankerStub) Rerank(context.Context, string, []SearchHit, RerankOptions) ([]SearchHit, error) {
	r.mu.Lock()
	r.called = true
	r.mu.Unlock()
	return []SearchHit{
		{
			Chunk: ChunkRecord{
				ChunkID:    "doc:0",
				ParentID:   "doc",
				SourcePath: "/tmp/doc.md",
				Title:      "doc",
				Text:       "gateway api exact match",
				StartRune:  0,
				EndRune:    23,
			},
			Score: 1,
		},
	}, nil
}

func (r *injectedRerankerStub) wasCalled() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.called
}

func TestStorageBoundaryTypesCompile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "root vector store interface accepts store stub",
			run: func(t *testing.T) {
				t.Helper()
				var store VectorStore = stubStorageVectorStore{}
				if err := store.Close(); err != nil {
					t.Fatalf("store.Close() error = %v", err)
				}
			},
		},
		{
			name: "root document loader interface accepts loader stub",
			run: func(t *testing.T) {
				t.Helper()
				var loader DocumentLoader = stubStorageDocumentLoader{}
				doc, err := loader.Load(context.Background(), "/tmp/doc.md", "Doc", nil, DocumentLoadOptions{})
				if err != nil {
					t.Fatalf("loader.Load() error = %v", err)
				}
				if doc.ID != "" || doc.SourcePath != "" || doc.Title != "" || len(doc.Metadata) != 0 || doc.Content != "" {
					t.Fatalf("loader.Load() doc = %#v, want zero document", doc)
				}
			},
		},
		{
			name: "root reranker interface accepts reranker stub",
			run: func(t *testing.T) {
				t.Helper()
				var reranker Reranker = stubStorageReranker{}
				hits, err := reranker.Rerank(context.Background(), "q", nil, RerankOptions{})
				if err != nil {
					t.Fatalf("reranker.Rerank() error = %v", err)
				}
				if hits != nil {
					t.Fatalf("reranker.Rerank() hits = %#v, want nil", hits)
				}
			},
		},
		{
			name: "root types are constructible",
			run: func(t *testing.T) {
				t.Helper()
				doc := Document{
					ID:         "doc",
					SourcePath: "/tmp/doc.md",
					Title:      "Doc",
					Metadata:   map[string]string{"tag": "api"},
					Content:    "body",
				}
				chunk := Chunk{
					ChunkID:    "doc:0",
					ParentID:   "doc",
					SourcePath: doc.SourcePath,
					Title:      doc.Title,
					Metadata:   maps.Clone(doc.Metadata),
					Text:       doc.Content,
					StartRune:  0,
					EndRune:    4,
				}
				hit := SearchHit{
					Chunk: ChunkRecord{
						ChunkID:    chunk.ChunkID,
						ParentID:   chunk.ParentID,
						SourcePath: chunk.SourcePath,
						Title:      chunk.Title,
						Text:       chunk.Text,
						StartRune:  chunk.StartRune,
						EndRune:    chunk.EndRune,
						Metadata:   maps.Clone(chunk.Metadata),
						Embedding:  []float32{1, 0},
					},
					Score: 0.99,
				}
				filter := SearchFilter{
					SourcePaths:    []string{doc.SourcePath},
					SourcePrefixes: []string{"/tmp"},
					Metadata:       map[string]string{"tag": "api"},
				}
				loadOpts := DocumentLoadOptions{}
				rerankOpts := RerankOptions{ShortlistSize: 4, TopK: 2}

				if hit.Chunk.SourcePath != filter.SourcePaths[0] {
					t.Fatalf("hit.Chunk.SourcePath = %q, want %q", hit.Chunk.SourcePath, filter.SourcePaths[0])
				}
				if loadOpts.MinDirectTextRunes != 0 {
					t.Fatalf("loadOpts.MinDirectTextRunes = %d, want 0", loadOpts.MinDirectTextRunes)
				}
				if rerankOpts.TopK != 2 {
					t.Fatalf("rerankOpts.TopK = %d, want 2", rerankOpts.TopK)
				}
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.run(t)
		})
	}
}

func TestDefaultStorageAdapters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "chromem vector store constructor returns usable store",
			run: func(t *testing.T) {
				t.Helper()

				store, err := NewChromemVectorStore(ChromemVectorStoreConfig{})
				if err != nil {
					t.Fatalf("NewChromemVectorStore() error = %v", err)
				}
				t.Cleanup(func() {
					if cerr := store.Close(); cerr != nil {
						t.Fatalf("Close() error = %v", cerr)
					}
				})

				err = store.Upsert(context.Background(), []ChunkRecord{
					{
						ChunkID:    "doc:0",
						ParentID:   "doc",
						SourcePath: "/tmp/doc.md",
						Title:      "Doc",
						Text:       "hello world",
						StartRune:  0,
						EndRune:    11,
						Embedding:  []float32{1, 0},
					},
				})
				if err != nil {
					t.Fatalf("Upsert() error = %v", err)
				}

				hits, err := store.Search(context.Background(), []float32{1, 0}, 1, 0)
				if err != nil {
					t.Fatalf("Search() error = %v", err)
				}
				if len(hits) != 1 {
					t.Fatalf("Search() len = %d, want 1", len(hits))
				}
			},
		},
		{
			name: "file document loader preserves markdown metadata behavior",
			run: func(t *testing.T) {
				t.Helper()

				root := t.TempDir()
				path := filepath.Join(root, "doc.md")
				if err := os.WriteFile(path, []byte("---\ntag: api\nlang: en\n---\nbody"), 0o600); err != nil {
					t.Fatalf("WriteFile(%q) error = %v", path, err)
				}

				loader := NewFileDocumentLoader()
				doc, err := loader.Load(context.Background(), path, "Doc", map[string]string{"source": "test"}, DocumentLoadOptions{})
				if err != nil {
					t.Fatalf("Load() error = %v", err)
				}
				if doc.Metadata["tag"] != "api" {
					t.Fatalf("doc.Metadata[tag] = %q, want %q", doc.Metadata["tag"], "api")
				}
				if doc.Metadata["source"] != "test" {
					t.Fatalf("doc.Metadata[source] = %q, want %q", doc.Metadata["source"], "test")
				}
				if doc.Content != "body" {
					t.Fatalf("doc.Content = %q, want %q", doc.Content, "body")
				}
			},
		},
		{
			name: "rule based reranker reorders shortlist",
			run: func(t *testing.T) {
				t.Helper()

				reranker := NewRuleBasedReranker()
				got, err := reranker.Rerank(context.Background(), "gateway api", []SearchHit{
					{
						Chunk: ChunkRecord{
							ChunkID: "a",
							Title:   "Overview",
							Text:    "semantic only",
						},
						Score: 0.99,
					},
					{
						Chunk: ChunkRecord{
							ChunkID: "b",
							Title:   "Gateway API",
							Text:    "gateway api exact match",
						},
						Score: 0.80,
					},
				}, RerankOptions{
					ShortlistSize: 2,
					TopK:          1,
				})
				if err != nil {
					t.Fatalf("Rerank() error = %v", err)
				}
				if len(got) != 1 {
					t.Fatalf("Rerank() len = %d, want 1", len(got))
				}
				if got[0].Chunk.ChunkID != "b" {
					t.Fatalf("Rerank() top id = %q, want %q", got[0].Chunk.ChunkID, "b")
				}
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.run(t)
		})
	}
}

func TestAgentUsesInjectedStorageComponents(t *testing.T) {
	t.Parallel()

	store := &injectedVectorStoreStub{
		searchHits: []SearchHit{
			{
				Chunk: ChunkRecord{
					ChunkID:    "doc:0",
					ParentID:   "doc",
					SourcePath: "/tmp/doc.md",
					Title:      "Doc",
					Text:       "gateway api exact match",
					StartRune:  0,
					EndRune:    23,
				},
				Score: 0.99,
			},
		},
	}
	loader := &injectedDocumentLoaderStub{}
	reranker := &injectedRerankerStub{}

	agent, err := New(Config{
		Runtime: RuntimeComponents{
			ChatModel: stubRuntimeChatModel{},
			Embedder:  stubRuntimeEmbedder{},
		},
		Storage: StorageComponents{
			VectorStore:    store,
			DocumentLoader: loader,
			Reranker:       reranker,
		},
		TopK:                      1,
		ChunkSize:                 32,
		ChunkOverlap:              0,
		MaxHistoryRounds:          8,
		RequestTimeout:            time.Second,
		EnableHybridSearch:        true,
		EnableRerank:              true,
		HybridRRFK:                60,
		HybridCandidateMultiplier: 1,
		RerankShortlistMultiplier: 1,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() {
		if cerr := agent.Close(); cerr != nil {
			t.Fatalf("Close() error = %v", cerr)
		}
	})

	root := t.TempDir()
	path := filepath.Join(root, "knowledge.md")
	if err := os.WriteFile(path, []byte("ignored by injected loader"), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}

	if err := agent.AddKnowledge(context.Background(), FileSource(path)); err != nil {
		t.Fatalf("AddKnowledge() error = %v", err)
	}
	answer, err := agent.GetSession("injected-storage").Ask(context.Background(), "gateway api")
	if err != nil {
		t.Fatalf("Ask() error = %v", err)
	}
	if answer.Text == "" {
		t.Fatal("Ask() returned empty text")
	}

	upsertCalls, searchCalls := store.snapshot()
	if upsertCalls == 0 {
		t.Fatal("injected vector store Upsert() was not called")
	}
	if searchCalls == 0 {
		t.Fatal("injected vector store SearchWithFilter() was not called")
	}
	if !loader.wasCalled() {
		t.Fatal("injected document loader was not called")
	}
	if !reranker.wasCalled() {
		t.Fatal("injected reranker was not called")
	}
}

func TestAskFailsWhenMaxToolCallsExceeded(t *testing.T) {
	t.Parallel()

	registry := NewToolRegistry()
	if err := registry.Register(registryTestTool{
		name:        "search_web",
		description: "web",
		result:      "web result",
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	a, err := New(Config{
		Runtime: RuntimeComponents{
			ChatModel: stubRuntimeChatModel{},
			Embedder:  stubRuntimeEmbedder{},
		},
		Storage: StorageComponents{
			VectorStore: &injectedVectorStoreStub{},
		},
		TopK:               1,
		ChunkSize:          32,
		ChunkOverlap:       0,
		MaxHistoryRounds:   8,
		MaxToolCalls:       1,
		RequestTimeout:     time.Second,
		EnableHybridSearch: false,
		EnableRerank:       false,
		EnableWebSearch:    true,
		WebSearch: WebSearchConfig{
			APIKey:      "dummy",
			MaxResults:  5,
			SearchDepth: "basic",
			Topic:       "general",
		},
		ToolRegistry: registry,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() {
		if cerr := a.Close(); cerr != nil {
			t.Fatalf("Close() error = %v", cerr)
		}
	})

	_, err = a.GetSession("tool-limit").Ask(context.Background(), "latest news")
	if err == nil {
		t.Fatal("Ask() error = nil, want non-nil")
	}
	if !errors.Is(err, ErrToolCallLimitExceeded) {
		t.Fatalf("Ask() error = %v, want errors.Is(..., %v)", err, ErrToolCallLimitExceeded)
	}
}

func TestAskUsesCustomFallbackToolWithoutEnableWebSearch(t *testing.T) {
	t.Parallel()

	registry := NewToolRegistry()
	customTool := registryTestTool{
		name:        "search_internal",
		description: "internal fallback",
		result:      "internal fallback result",
	}
	if err := registry.Register(customTool); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	a, err := New(Config{
		Runtime: RuntimeComponents{
			ChatModel: stubRuntimeChatModel{},
			Embedder:  stubRuntimeEmbedder{},
		},
		Storage: StorageComponents{
			VectorStore: &injectedVectorStoreStub{},
		},
		TopK:             1,
		ChunkSize:        32,
		ChunkOverlap:     0,
		MaxHistoryRounds: 8,
		MaxToolCalls:     2,
		RequestTimeout:   time.Second,
		ToolRegistry:     registry,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() {
		if cerr := a.Close(); cerr != nil {
			t.Fatalf("Close() error = %v", cerr)
		}
	})

	answer, err := a.GetSession("custom-fallback").Ask(context.Background(), "latest news")
	if err != nil {
		t.Fatalf("Ask() error = %v", err)
	}
	if answer.Text == "" {
		t.Fatal("Ask() text = empty")
	}
	toolNames := make([]string, 0, len(answer.Trace.ToolCalls))
	for _, toolCall := range answer.Trace.ToolCalls {
		toolNames = append(toolNames, toolCall.Name)
	}
	if !slices.Contains(toolNames, "search_internal") {
		t.Fatalf("trace tool names = %v, want contains %q", toolNames, "search_internal")
	}
}

func TestAskUsesConfiguredSearchWebOverride(t *testing.T) {
	t.Parallel()

	registry := NewToolRegistry()
	if err := registry.Register(registryTestTool{
		name:        "search_web",
		description: "override web",
		result:      "override web result",
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	a, err := New(Config{
		Runtime: RuntimeComponents{
			ChatModel: stubRuntimeChatModel{},
			Embedder:  stubRuntimeEmbedder{},
		},
		Storage: StorageComponents{
			VectorStore: &injectedVectorStoreStub{},
		},
		TopK:             1,
		ChunkSize:        32,
		ChunkOverlap:     0,
		MaxHistoryRounds: 8,
		MaxToolCalls:     2,
		RequestTimeout:   time.Second,
		EnableWebSearch:  true,
		WebSearch: WebSearchConfig{
			APIKey:      "dummy",
			BaseURL:     "http://127.0.0.1:1",
			MaxResults:  5,
			SearchDepth: "basic",
			Topic:       "general",
		},
		ToolRegistry: registry,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() {
		if cerr := a.Close(); cerr != nil {
			t.Fatalf("Close() error = %v", cerr)
		}
	})

	answer, err := a.GetSession("override-search-web").Ask(context.Background(), "latest news")
	if err != nil {
		t.Fatalf("Ask() error = %v", err)
	}
	if answer.Text == "" {
		t.Fatal("Ask() text = empty")
	}
	toolNames := make([]string, 0, len(answer.Trace.ToolCalls))
	for _, toolCall := range answer.Trace.ToolCalls {
		toolNames = append(toolNames, toolCall.Name)
	}
	if !slices.Contains(toolNames, "search_web") {
		t.Fatalf("trace tool names = %v, want contains %q", toolNames, "search_web")
	}
}

type cleanupSource struct {
	closed bool
}

func (s blockingSource) Resolve(ctx context.Context) ([]KnowledgeFile, error) {
	select {
	case s.started <- struct{}{}:
	default:
	}

	select {
	case <-s.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return slices.Clone(s.files), nil
}

func (s *cleanupSource) Resolve(context.Context) ([]KnowledgeFile, error) {
	return nil, nil
}

func (s *cleanupSource) Close() error {
	s.closed = true
	return nil
}

func TestGetSessionReusesSameID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		sessionID string
	}{
		{
			name:      "same id returns same pointer",
			sessionID: "alice",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			a := &Agent{
				cfg:      Config{MaxHistoryRounds: 4},
				sessions: make(map[string]*Session),
			}
			s1 := a.GetSession(tc.sessionID)
			s2 := a.GetSession(tc.sessionID)
			if s1 != s2 {
				t.Fatal("GetSession() should return the same session for the same id")
			}
		})
	}
}

func TestSessionAskExecutionTrace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		runnerErr       error
		wantErr         error
		wantAnswerTrace bool
		wantTraceErr    bool
	}{
		{
			name:            "successful ask returns trace and records it",
			runnerErr:       nil,
			wantErr:         nil,
			wantAnswerTrace: true,
			wantTraceErr:    false,
		},
		{
			name:            "failed ask still records trace",
			runnerErr:       errors.New("runner boom"),
			wantErr:         errors.New("runner boom"),
			wantAnswerTrace: false,
			wantTraceErr:    true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store := &fakeStore{
				searchHits: []storage.SearchHit{
					{
						Chunk: storage.ChunkRecord{
							ChunkID:    "doc:0",
							SourcePath: "/tmp/doc.md",
							Title:      "doc",
							Text:       "retrieved evidence for trace",
							StartRune:  0,
							EndRune:    28,
						},
						Score: 0.99,
					},
				},
			}
			embedder := &fakeEmbedder{defaultVec: []float32{1, 2, 3}}
			runner := &fakeRunner{
				answer: "answer text",
				err:    tc.runnerErr,
			}
			recorder := &traceRecorderStub{}
			logger := &loggerStub{}
			a := &Agent{
				cfg: Config{
					ChatModel:           "trace-model",
					TopK:                5,
					SimilarityThreshold: 0.5,
					MaxHistoryRounds:    8,
					Logger:              logger,
					TraceRecorder:       recorder,
				},
				store:    store,
				embedder: embedder,
				runner:   runner,
				sessions: make(map[string]*Session),
			}

			answer, err := a.GetSession("trace-sync").Ask(context.Background(), "what is trace?")
			if tc.wantErr != nil {
				if !containsErr(err, tc.wantErr) {
					t.Fatalf("Ask() error = %v, want contains %v", err, tc.wantErr)
				}
			} else if err != nil {
				t.Fatalf("Ask() error = %v", err)
			}

			if tc.wantAnswerTrace {
				if answer.Trace == nil {
					t.Fatal("Answer.Trace = nil, want non-nil")
				}
				if answer.Trace.RewrittenQuery != "what is trace?" {
					t.Fatalf("Answer.Trace.RewrittenQuery = %q, want %q", answer.Trace.RewrittenQuery, "what is trace?")
				}
				if answer.Trace.Retrieval.FinalHitCount != 1 {
					t.Fatalf("Answer.Trace.Retrieval.FinalHitCount = %d, want %d", answer.Trace.Retrieval.FinalHitCount, 1)
				}
				if answer.Trace.Model.OutputChars != len("answer text") {
					t.Fatalf("Answer.Trace.Model.OutputChars = %d, want %d", answer.Trace.Model.OutputChars, len("answer text"))
				}
			}

			recorded := recorder.snapshot()
			if len(recorded) != 1 {
				t.Fatalf("recorded traces len = %d, want %d", len(recorded), 1)
			}
			if recorded[0].Success == tc.wantTraceErr {
				t.Fatalf("recorded trace success = %v, want inverse of wantTraceErr=%v", recorded[0].Success, tc.wantTraceErr)
			}
			if tc.wantTraceErr && recorded[0].Err == nil {
				t.Fatal("recorded trace Err = nil, want non-nil")
			}
			if !tc.wantTraceErr && recorded[0].Err != nil {
				t.Fatalf("recorded trace Err = %v, want nil", recorded[0].Err)
			}

			logs := logger.snapshot()
			if len(logs) == 0 {
				t.Fatal("logger entries = 0, want at least one summary log")
			}
		})
	}
}

func TestAskRespectsMaxExecutionDuration(t *testing.T) {
	t.Parallel()

	runner := &blockingBudgetRunner{started: make(chan struct{}, 1)}
	a := &Agent{
		cfg: Config{
			ChatModel:            "budget-model",
			TopK:                 1,
			SimilarityThreshold:  0.5,
			MaxHistoryRounds:     8,
			MaxExecutionDuration: 20 * time.Millisecond,
		},
		store: &fakeStore{
			searchHits: []storage.SearchHit{
				{
					Chunk: storage.ChunkRecord{
						ChunkID:    "doc:0",
						SourcePath: "/tmp/doc.md",
						Title:      "doc",
						Text:       "budget evidence",
						StartRune:  0,
						EndRune:    15,
					},
					Score: 0.99,
				},
			},
		},
		embedder: &fakeEmbedder{defaultVec: []float32{1, 0}},
		runner:   runner,
		sessions: make(map[string]*Session),
	}

	_, err := a.GetSession("execution-budget").Ask(context.Background(), "budget query")
	if !errors.Is(err, ErrExecutionBudgetExceeded) {
		t.Fatalf("Ask() error = %v, want errors.Is(..., %v)", err, ErrExecutionBudgetExceeded)
	}
}

func TestAskExecutionTraceCapturesWebSearchTool(t *testing.T) {
	t.Parallel()

	runner, err := graph.NewChatRunner(
		&fakeTraceChatModel{answer: "web-backed answer"},
		tools.NewWebSearchTool(&fakeSearcher{
			results: []websearch.Result{
				{
					Title:   "fresh result",
					URL:     "https://example.com/fresh",
					Content: "latest web evidence",
				},
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewChatRunner() error = %v", err)
	}

	a := &Agent{
		cfg: Config{
			TopK:                5,
			SimilarityThreshold: 0.5,
			MaxHistoryRounds:    8,
			EnableWebSearch:     true,
		},
		store:    &fakeStore{searchHits: nil},
		embedder: &fakeEmbedder{defaultVec: []float32{1, 0}},
		runner:   runner,
		sessions: make(map[string]*Session),
	}

	answer, err := a.GetSession("trace-web").Ask(context.Background(), "latest news")
	if err != nil {
		t.Fatalf("Ask() error = %v", err)
	}
	if answer.Trace == nil {
		t.Fatal("Answer.Trace = nil, want non-nil")
	}

	toolNames := make([]string, 0, len(answer.Trace.ToolCalls))
	for _, toolCall := range answer.Trace.ToolCalls {
		toolNames = append(toolNames, toolCall.Name)
	}
	if !slices.Contains(toolNames, "search_web") {
		t.Fatalf("trace tool names = %v, want contains %q", toolNames, "search_web")
	}
}

func TestSessionClearHistory(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		before   []memory.Turn
		wantSize int
	}{
		{
			name: "clear removes all turns",
			before: []memory.Turn{
				{User: "q1", Assistant: "a1"},
				{User: "q2", Assistant: "a2"},
			},
			wantSize: 0,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			history := memory.NewHistory(8)
			for _, turn := range tc.before {
				history.Append(turn.User, turn.Assistant)
			}
			s := &Session{
				history: history,
			}
			if err := s.ClearHistory(context.Background()); err != nil {
				t.Fatalf("ClearHistory() error = %v", err)
			}
			if got := len(s.history.Turns()); got != tc.wantSize {
				t.Fatalf("len(history) = %d, want %d", got, tc.wantSize)
			}
		})
	}
}

func TestSessionAskReturnsAnswerAndCitations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		query            string
		history          []memory.Turn
		searchHits       []storage.SearchHit
		runnerAnswer     string
		wantAnswer       string
		wantCitations    []Citation
		wantRewrittenQry string
		wantErr          error
	}{
		{
			name:  "ask returns answer text and citations",
			query: "how does it work?",
			history: []memory.Turn{
				{User: "rag pipeline architecture", Assistant: "it has retrieval and generation"},
			},
			searchHits: []storage.SearchHit{
				{
					Chunk: storage.ChunkRecord{
						ChunkID:    "doc-1:0",
						SourcePath: "/tmp/one.md",
						Title:      "one",
						Text:       "retrieval augments generation",
						StartRune:  0,
						EndRune:    29,
					},
					Score: 0.9,
				},
			},
			runnerAnswer: "RAG uses retrieval before generation.",
			wantAnswer:   "RAG uses retrieval before generation.",
			wantCitations: []Citation{
				{
					SourcePath: "/tmp/one.md",
					Title:      "one",
					ChunkID:    "doc-1:0",
					StartRune:  0,
					EndRune:    29,
				},
			},
			wantRewrittenQry: "rag pipeline architecture how does it work?",
			wantErr:          nil,
		},
		{
			name:             "ask returns insufficient evidence error when no context",
			query:            "what is missing?",
			history:          nil,
			searchHits:       nil,
			runnerAnswer:     "",
			wantAnswer:       "",
			wantCitations:    nil,
			wantRewrittenQry: rag.RewriteFollowUp("what is missing?", nil),
			wantErr:          ErrEvidenceInsufficient,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store := &fakeStore{searchHits: tc.searchHits}
			embedder := &fakeEmbedder{
				vectors: map[string][]float32{
					tc.wantRewrittenQry: {0.1, 0.2, 0.3},
				},
				defaultVec: []float32{0.1, 0.2, 0.3},
			}
			runner := &fakeRunner{answer: tc.runnerAnswer}

			a := &Agent{
				cfg: Config{
					TopK:                5,
					SimilarityThreshold: 0.5,
					ChatModel:           "chat-test",
					MaxHistoryRounds:    8,
				},
				store:    store,
				embedder: embedder,
				runner:   runner,
				sessions: make(map[string]*Session),
			}

			s := a.GetSession("ask-test")
			for _, turn := range tc.history {
				s.history.Append(turn.User, turn.Assistant)
			}

			got, err := s.Ask(context.Background(), tc.query)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("Ask() error = %v, want errors.Is(..., %v)", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Ask() error = %v", err)
			}
			if got.Text != tc.wantAnswer {
				t.Fatalf("Ask() text = %q, want %q", got.Text, tc.wantAnswer)
			}
			if !slices.Equal(got.Citations, tc.wantCitations) {
				t.Fatalf("Ask() citations = %#v, want %#v", got.Citations, tc.wantCitations)
			}

			embedder.mu.Lock()
			lastTexts := slices.Clone(embedder.lastTexts)
			embedder.mu.Unlock()
			if len(lastTexts) != 1 || lastTexts[0] != tc.wantRewrittenQry {
				t.Fatalf("EmbedTexts query = %v, want [%q]", lastTexts, tc.wantRewrittenQry)
			}

			runner.mu.Lock()
			lastReq := runner.lastReq
			runner.mu.Unlock()
			if lastReq.Query != tc.wantRewrittenQry {
				t.Fatalf("runner query = %q, want %q", lastReq.Query, tc.wantRewrittenQry)
			}
			wantHistoryLen := len(tc.history)
			if gotHistoryLen := len(lastReq.History); gotHistoryLen != wantHistoryLen {
				t.Fatalf("runner history len = %d, want %d", gotHistoryLen, wantHistoryLen)
			}

			turns := s.history.Turns()
			if len(turns) != len(tc.history)+1 {
				t.Fatalf("history length after ask = %d, want %d", len(turns), len(tc.history)+1)
			}
			lastTurn := turns[len(turns)-1]
			if lastTurn.User != tc.query || lastTurn.Assistant != tc.wantAnswer {
				t.Fatalf("last turn = %#v, want user=%q assistant=%q", lastTurn, tc.query, tc.wantAnswer)
			}
		})
	}
}

func TestSessionAskWithOptionsFiltersRetrieval(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		options             QueryOptions
		wantCitations       []Citation
		wantFilter          storage.SearchFilter
		wantErr             error
		wantSearchCallCount int
	}{
		{
			name: "source prefix filter keeps only matching citations",
			options: QueryOptions{
				Filter: RetrievalFilter{
					SourcePrefixes: []string{"/kb/project-a"},
				},
			},
			wantCitations: []Citation{
				{
					SourcePath: "/kb/project-a/api.md",
					Title:      "API",
					ChunkID:    "alpha:0",
					StartRune:  0,
					EndRune:    9,
				},
			},
			wantFilter: storage.SearchFilter{
				SourcePrefixes: []string{"/kb/project-a"},
			},
			wantSearchCallCount: 1,
		},
		{
			name: "metadata filter can make evidence insufficient",
			options: QueryOptions{
				Filter: RetrievalFilter{
					Metadata: map[string]string{"team": "missing"},
				},
			},
			wantErr: ErrEvidenceInsufficient,
			wantFilter: storage.SearchFilter{
				Metadata: map[string]string{"team": "missing"},
			},
			wantSearchCallCount: 1,
		},
	}

	baseHits := []storage.SearchHit{
		{
			Chunk: storage.ChunkRecord{
				ChunkID:    "alpha:0",
				SourcePath: "/kb/project-a/api.md",
				Title:      "API",
				Text:       "alpha api",
				StartRune:  0,
				EndRune:    9,
				Metadata:   map[string]string{"team": "alpha"},
			},
			Score: 0.95,
		},
		{
			Chunk: storage.ChunkRecord{
				ChunkID:    "beta:0",
				SourcePath: "/kb/project-b/api.md",
				Title:      "Beta API",
				Text:       "beta api",
				StartRune:  0,
				EndRune:    8,
				Metadata:   map[string]string{"team": "beta"},
			},
			Score: 0.94,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store := &fakeStore{searchHits: baseHits}
			embedder := &fakeEmbedder{defaultVec: []float32{1, 0}}
			runner := &fakeRunner{answer: "filtered answer"}
			a := &Agent{
				cfg: Config{
					TopK:                5,
					SimilarityThreshold: 0.5,
					ChatModel:           "chat-test",
					MaxHistoryRounds:    8,
				},
				store:    store,
				embedder: embedder,
				runner:   runner,
				sessions: make(map[string]*Session),
			}

			answer, err := a.GetSession("ask-with-options").AskWithOptions(context.Background(), "show docs", tc.options)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("AskWithOptions() error = %v, want errors.Is(..., %v)", err, tc.wantErr)
				}
			} else if err != nil {
				t.Fatalf("AskWithOptions() error = %v", err)
			}

			store.mu.Lock()
			gotFilter := store.lastFilter
			gotSearchCalls := store.searchCalls
			store.mu.Unlock()
			if gotSearchCalls != tc.wantSearchCallCount {
				t.Fatalf("search call count = %d, want %d", gotSearchCalls, tc.wantSearchCallCount)
			}
			if !slices.Equal(gotFilter.SourcePaths, tc.wantFilter.SourcePaths) {
				t.Fatalf("search filter source paths = %v, want %v", gotFilter.SourcePaths, tc.wantFilter.SourcePaths)
			}
			if !slices.Equal(gotFilter.SourcePrefixes, tc.wantFilter.SourcePrefixes) {
				t.Fatalf("search filter source prefixes = %v, want %v", gotFilter.SourcePrefixes, tc.wantFilter.SourcePrefixes)
			}
			if !maps.Equal(gotFilter.Metadata, tc.wantFilter.Metadata) {
				t.Fatalf("search filter metadata = %v, want %v", gotFilter.Metadata, tc.wantFilter.Metadata)
			}

			if tc.wantErr == nil && !slices.Equal(answer.Citations, tc.wantCitations) {
				t.Fatalf("AskWithOptions() citations = %#v, want %#v", answer.Citations, tc.wantCitations)
			}
		})
	}
}

func TestSessionAskWithHybridRetrievalPromotesLexicalHit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		cfg         Config
		wantChunkID string
		wantTitle   string
	}{
		{
			name: "vector only keeps original ranking",
			cfg: Config{
				TopK:                1,
				SimilarityThreshold: 0.5,
				ChatModel:           "chat-test",
				MaxHistoryRounds:    8,
			},
			wantChunkID: "semantic:0",
			wantTitle:   "Overview",
		},
		{
			name: "hybrid search promotes lexical exact match",
			cfg: Config{
				TopK:                1,
				SimilarityThreshold: 0.5,
				ChatModel:           "chat-test",
				MaxHistoryRounds:    8,
				EnableHybridSearch:  true,
			},
			wantChunkID: "gateway:0",
			wantTitle:   "Gateway API",
		},
		{
			name: "rerank keeps lexical exact match at top of shortlist",
			cfg: Config{
				TopK:                1,
				SimilarityThreshold: 0.5,
				ChatModel:           "chat-test",
				MaxHistoryRounds:    8,
				EnableHybridSearch:  true,
				EnableRerank:        true,
			},
			wantChunkID: "gateway:0",
			wantTitle:   "Gateway API",
		},
	}

	searchHits := []storage.SearchHit{
		{
			Chunk: storage.ChunkRecord{
				ChunkID:   "semantic:0",
				Title:     "Overview",
				Text:      "semantic overview without exact tokens",
				StartRune: 0,
				EndRune:   36,
			},
			Score: 0.99,
		},
		{
			Chunk: storage.ChunkRecord{
				ChunkID:   "gateway:0",
				Title:     "Gateway API",
				Text:      "gateway api exact match terms appear here",
				StartRune: 0,
				EndRune:   40,
			},
			Score: 0.80,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store := &fakeStore{searchHits: searchHits}
			embedder := &fakeEmbedder{defaultVec: []float32{1, 0}}
			runner := &fakeRunner{answer: "answer"}
			a := &Agent{
				cfg:      tc.cfg,
				store:    store,
				embedder: embedder,
				runner:   runner,
				sessions: make(map[string]*Session),
			}

			answer, err := a.GetSession("hybrid").Ask(context.Background(), "gateway api")
			if err != nil {
				t.Fatalf("Ask() error = %v", err)
			}
			if len(answer.Citations) != 1 {
				t.Fatalf("Ask() citations len = %d, want 1", len(answer.Citations))
			}
			if answer.Citations[0].ChunkID != tc.wantChunkID {
				t.Fatalf("Ask() citation chunk = %q, want %q", answer.Citations[0].ChunkID, tc.wantChunkID)
			}
			if answer.Citations[0].Title != tc.wantTitle {
				t.Fatalf("Ask() citation title = %q, want %q", answer.Citations[0].Title, tc.wantTitle)
			}
		})
	}
}

func TestSessionAskWithHybridCandidateMultiplierAffectsPromotion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		cfg         Config
		wantChunkID string
	}{
		{
			name: "candidate multiplier one keeps vector winner on tie",
			cfg: Config{
				TopK:                      1,
				SimilarityThreshold:       0.5,
				ChatModel:                 "chat-test",
				MaxHistoryRounds:          8,
				EnableHybridSearch:        true,
				HybridCandidateMultiplier: 1,
			},
			wantChunkID: "semantic:0",
		},
		{
			name: "candidate multiplier two gives lexical hit enough room to win",
			cfg: Config{
				TopK:                      1,
				SimilarityThreshold:       0.5,
				ChatModel:                 "chat-test",
				MaxHistoryRounds:          8,
				EnableHybridSearch:        true,
				HybridCandidateMultiplier: 2,
			},
			wantChunkID: "gateway:0",
		},
	}

	searchHits := []storage.SearchHit{
		{
			Chunk: storage.ChunkRecord{
				ChunkID:   "semantic:0",
				Title:     "Overview",
				Text:      "semantic overview without exact tokens",
				StartRune: 0,
				EndRune:   36,
			},
			Score: 0.99,
		},
		{
			Chunk: storage.ChunkRecord{
				ChunkID:   "gateway:0",
				Title:     "Gateway API",
				Text:      "gateway api exact match terms appear here",
				StartRune: 0,
				EndRune:   40,
			},
			Score: 0.80,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store := &fakeStore{searchHits: searchHits}
			embedder := &fakeEmbedder{defaultVec: []float32{1, 0}}
			runner := &fakeRunner{answer: "answer"}
			a := &Agent{
				cfg:      tc.cfg,
				store:    store,
				embedder: embedder,
				runner:   runner,
				sessions: make(map[string]*Session),
			}

			answer, err := a.GetSession("hybrid-multiplier").Ask(context.Background(), "gateway api")
			if err != nil {
				t.Fatalf("Ask() error = %v", err)
			}
			if len(answer.Citations) != 1 {
				t.Fatalf("Ask() citations len = %d, want 1", len(answer.Citations))
			}
			if answer.Citations[0].ChunkID != tc.wantChunkID {
				t.Fatalf("Ask() citation chunk = %q, want %q", answer.Citations[0].ChunkID, tc.wantChunkID)
			}
		})
	}
}

func TestAskEmitsDetailedMetricsAndFallbacks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		cfg                Config
		wantChunkID        string
		wantFallbackStage  string
		wantFallbackTarget string
	}{
		{
			name: "hybrid metrics emitted without fallback",
			cfg: Config{
				TopK:                1,
				SimilarityThreshold: 0.5,
				ChatModel:           "chat-test",
				MaxHistoryRounds:    8,
				EnableHybridSearch:  true,
			},
			wantChunkID: "gateway:0",
		},
		{
			name: "invalid hybrid tuning falls back to vector only",
			cfg: Config{
				TopK:                      1,
				SimilarityThreshold:       0.5,
				ChatModel:                 "chat-test",
				MaxHistoryRounds:          8,
				EnableHybridSearch:        true,
				HybridCandidateMultiplier: -1,
			},
			wantChunkID:        "semantic:0",
			wantFallbackStage:  FallbackStageHybrid,
			wantFallbackTarget: FallbackTargetVectorOnly,
		},
		{
			name: "invalid rerank tuning falls back to hybrid",
			cfg: Config{
				TopK:                      1,
				SimilarityThreshold:       0.5,
				ChatModel:                 "chat-test",
				MaxHistoryRounds:          8,
				EnableHybridSearch:        true,
				EnableRerank:              true,
				RerankShortlistMultiplier: -1,
			},
			wantChunkID:        "gateway:0",
			wantFallbackStage:  FallbackStageRerank,
			wantFallbackTarget: FallbackTargetHybrid,
		},
	}

	searchHits := []storage.SearchHit{
		{
			Chunk: storage.ChunkRecord{
				ChunkID:   "semantic:0",
				Title:     "Overview",
				Text:      "semantic overview without exact tokens",
				StartRune: 0,
				EndRune:   36,
			},
			Score: 0.99,
		},
		{
			Chunk: storage.ChunkRecord{
				ChunkID:   "gateway:0",
				Title:     "Gateway API",
				Text:      "gateway api exact match terms appear here",
				StartRune: 0,
				EndRune:   40,
			},
			Score: 0.80,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			recorder := &detailedCallbackRecorder{}
			a := &Agent{
				cfg:      tc.cfg,
				store:    &fakeStore{searchHits: searchHits},
				embedder: &fakeEmbedder{defaultVec: []float32{1, 0}},
				runner:   &fakeRunner{answer: "answer"},
				dispatcher: telemetry.NewDispatcher([]telemetry.Callback{
					recorder,
				}),
				callbacks: []Callback{recorder},
				sessions:  make(map[string]*Session),
			}

			answer, err := a.GetSession("metrics").Ask(context.Background(), "gateway api")
			if err != nil {
				t.Fatalf("Ask() error = %v", err)
			}
			if len(answer.Citations) != 1 {
				t.Fatalf("Ask() citations len = %d, want 1", len(answer.Citations))
			}
			if answer.Citations[0].ChunkID != tc.wantChunkID {
				t.Fatalf("Ask() citation chunk = %q, want %q", answer.Citations[0].ChunkID, tc.wantChunkID)
			}

			retrieveMetrics, modelMetrics, fallbacks := recorder.snapshotMetrics()
			if len(retrieveMetrics) != 1 {
				t.Fatalf("retrieve metrics len = %d, want 1", len(retrieveMetrics))
			}
			if retrieveMetrics[0].FinalHitCount != 1 {
				t.Fatalf("retrieve FinalHitCount = %d, want 1", retrieveMetrics[0].FinalHitCount)
			}
			if retrieveMetrics[0].Duration <= 0 {
				t.Fatalf("retrieve Duration = %v, want positive", retrieveMetrics[0].Duration)
			}
			if len(modelMetrics) != 1 {
				t.Fatalf("model metrics len = %d, want 1", len(modelMetrics))
			}
			if modelMetrics[0].OutputChars != len("answer") {
				t.Fatalf("model OutputChars = %d, want %d", modelMetrics[0].OutputChars, len("answer"))
			}
			if modelMetrics[0].Duration <= 0 {
				t.Fatalf("model Duration = %v, want positive", modelMetrics[0].Duration)
			}

			if tc.wantFallbackStage == "" {
				if len(fallbacks) != 0 {
					t.Fatalf("fallbacks = %v, want none", fallbacks)
				}
				return
			}
			if len(fallbacks) != 1 {
				t.Fatalf("fallback len = %d, want 1", len(fallbacks))
			}
			if fallbacks[0].Stage != tc.wantFallbackStage {
				t.Fatalf("fallback stage = %q, want %q", fallbacks[0].Stage, tc.wantFallbackStage)
			}
			if fallbacks[0].FallbackTo != tc.wantFallbackTarget {
				t.Fatalf("fallback target = %q, want %q", fallbacks[0].FallbackTo, tc.wantFallbackTarget)
			}
			if fallbacks[0].Err == nil {
				t.Fatal("fallback error = nil, want non-nil")
			}
		})
	}
}

func TestAskFallsBackToWebSearchPathWhenLocalEvidenceInsufficient(t *testing.T) {
	t.Parallel()

	a := &Agent{
		cfg: Config{
			TopK:                5,
			SimilarityThreshold: 0.5,
			ChatModel:           "chat-test",
			MaxHistoryRounds:    8,
			EnableWebSearch:     true,
			WebSearch: WebSearchConfig{
				APIKey:      "tvly-test",
				MaxResults:  5,
				SearchDepth: "basic",
				Topic:       "general",
			},
		},
		store:    &fakeStore{searchHits: nil},
		embedder: &fakeEmbedder{defaultVec: []float32{1, 0}},
		runner:   &fakeRunner{answer: "web answer"},
		sessions: make(map[string]*Session),
	}

	answer, err := a.GetSession("web-search-fallback").Ask(context.Background(), "latest news")
	if err != nil {
		t.Fatalf("Ask() error = %v", err)
	}
	if answer.Text != "web answer" {
		t.Fatalf("Ask() text = %q, want %q", answer.Text, "web answer")
	}
	if len(answer.Citations) != 0 {
		t.Fatalf("Ask() citations = %#v, want no local citations", answer.Citations)
	}
}

func TestAddKnowledgeIngestsAndUpsertsChunks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		content   string
		chunkSize int
	}{
		{
			name:      "file source loads chunks and writes records",
			content:   "abcdefghij",
			chunkSize: 4,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			path := filepath.Join(root, "knowledge.md")
			writeTestFile(t, path, tc.content)

			chunker, err := rag.NewChunker(tc.chunkSize, 0)
			if err != nil {
				t.Fatalf("NewChunker() error = %v", err)
			}
			store := &fakeStore{}
			embedder := &fakeEmbedder{defaultVec: []float32{1, 2, 3}}
			a := &Agent{
				cfg:      Config{},
				store:    store,
				embedder: embedder,
				chunker:  chunker,
				sessions: make(map[string]*Session),
			}

			if err := a.AddKnowledge(context.Background(), FileSource(path)); err != nil {
				t.Fatalf("AddKnowledge() error = %v", err)
			}

			store.mu.Lock()
			defer store.mu.Unlock()
			if len(store.upsertBatches) != 1 {
				t.Fatalf("upsert call count = %d, want 1", len(store.upsertBatches))
			}
			records := store.upsertBatches[0]
			if len(records) == 0 {
				t.Fatal("upsert records should not be empty")
			}
			for _, record := range records {
				if record.ChunkID == "" {
					t.Fatal("record chunk id is empty")
				}
				if record.ParentID == "" {
					t.Fatal("record parent id is empty")
				}
				if record.SourcePath != path {
					t.Fatalf("record source path = %q, want %q", record.SourcePath, path)
				}
				if record.Title != "knowledge" {
					t.Fatalf("record title = %q, want %q", record.Title, "knowledge")
				}
				if len(record.Embedding) == 0 {
					t.Fatal("record embedding should not be empty")
				}
			}
		})
	}
}

func TestAddKnowledgePropagatesBuiltinMetadata(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "knowledge.md")
	writeTestFile(t, path, "---\ntag: api\nlang: en\n---\nhello world")
	writeTestFile(t, filepath.Join(root, "knowledge.meta.json"), `{"lang":"zh","team":"search"}`)

	store := &fakeStore{}
	embedder := &fakeEmbedder{defaultVec: []float32{1, 2, 3}}
	a := &Agent{
		cfg:      Config{},
		store:    store,
		embedder: embedder,
		chunker:  mustNewChunkerForTest(t, 64, 0),
		sessions: make(map[string]*Session),
	}

	if err := a.AddKnowledge(t.Context(), FileSource(path)); err != nil {
		t.Fatalf("AddKnowledge() error = %v", err)
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.upsertBatches) != 1 {
		t.Fatalf("upsert call count = %d, want 1", len(store.upsertBatches))
	}
	if len(store.upsertBatches[0]) != 1 {
		t.Fatalf("upsert batch len = %d, want 1", len(store.upsertBatches[0]))
	}

	record := store.upsertBatches[0][0]
	wantMetadata := map[string]string{
		"tag":  "api",
		"lang": "zh",
		"team": "search",
	}
	if !maps.Equal(record.Metadata, wantMetadata) {
		t.Fatalf("record metadata = %v, want %v", record.Metadata, wantMetadata)
	}
	if strings.Contains(record.Text, "tag: api") || strings.HasPrefix(record.Text, "---") {
		t.Fatalf("record text = %q, want stripped body content", record.Text)
	}
}

func TestAddKnowledgeRejectsNilSource(t *testing.T) {
	t.Parallel()

	a := &Agent{
		chunker:  mustNewChunkerForTest(t, 16, 4),
		store:    &fakeStore{},
		embedder: &fakeEmbedder{defaultVec: []float32{0.1, 0.2, 0.3}},
		sessions: make(map[string]*Session),
	}

	err := a.AddKnowledge(context.Background(), nil)
	if !errors.Is(err, ErrUnsupportedSource) {
		t.Fatalf("AddKnowledge(nil) error = %v, want ErrUnsupportedSource", err)
	}
}

func TestAddKnowledgeClosesBridgeSource(t *testing.T) {
	t.Parallel()

	source := &cleanupSource{}

	a := &Agent{
		chunker:  mustNewChunkerForTest(t, 16, 4),
		store:    &fakeStore{},
		embedder: &fakeEmbedder{defaultVec: []float32{0.1, 0.2, 0.3}},
		sessions: make(map[string]*Session),
	}

	err := a.AddKnowledge(context.Background(), source)
	if err != nil {
		t.Fatalf("AddKnowledge() error = %v", err)
	}
	if !source.closed {
		t.Fatal("expected source.Close() to be called")
	}
}

func TestAddKnowledgeUsesPDFOCRBridgeForBlankPDF(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	pdfPath := filepath.Join(root, "scan.pdf")
	writeBlankPDFForAgentTest(t, pdfPath)

	markerPath := filepath.Join(root, "ocr-marker.txt")
	scriptPath := filepath.Join(root, "ocr-bridge.sh")
	writeExecutableFile(t, scriptPath, fmt.Sprintf(`#!/bin/sh
printf '%%s' "$1" > %q
printf 'OCR text from bridge' > "$2"
`, markerPath))

	a := &Agent{
		cfg: Config{
			PDFOCRBridge: PDFOCRBridgeConfig{
				Command: scriptPath,
				Args:    []string{"{input}", "{output}"},
			},
		},
		chunker:  mustNewChunkerForTest(t, 64, 0),
		store:    &fakeStore{},
		embedder: &fakeEmbedder{defaultVec: []float32{0.1, 0.2, 0.3}},
		sessions: make(map[string]*Session),
	}

	if err := a.AddKnowledge(t.Context(), FileSource(pdfPath)); err != nil {
		t.Fatalf("AddKnowledge() error = %v", err)
	}

	markerData, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", markerPath, err)
	}
	if got := strings.TrimSpace(string(markerData)); got != pdfPath {
		t.Fatalf("OCR bridge input path = %q, want %q", got, pdfPath)
	}

	store := a.store.(*fakeStore)
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.upsertBatches) != 1 {
		t.Fatalf("upsert call count = %d, want 1", len(store.upsertBatches))
	}
	if len(store.upsertBatches[0]) == 0 {
		t.Fatal("upsert batch should not be empty")
	}
	if !strings.Contains(store.upsertBatches[0][0].Text, "OCR text from bridge") {
		t.Fatalf("upsert text = %q, want OCR output", store.upsertBatches[0][0].Text)
	}
}

func TestCloseWaitsInFlightAddKnowledge(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "knowledge.md")
	writeTestFile(t, path, "abcdefgh")
	chunker, err := rag.NewChunker(4, 0)
	if err != nil {
		t.Fatalf("NewChunker() error = %v", err)
	}

	a := &Agent{
		store:    &fakeStore{},
		embedder: &fakeEmbedder{defaultVec: []float32{1, 2, 3}},
		chunker:  chunker,
		sessions: make(map[string]*Session),
	}
	src := blockingSource{
		started: make(chan struct{}, 1),
		release: make(chan struct{}),
		files: []KnowledgeFile{
			{
				Path:     path,
				Title:    "knowledge",
				Metadata: map[string]string{},
			},
		},
	}

	addErrCh := make(chan error, 1)
	go func() {
		addErrCh <- a.AddKnowledge(context.Background(), src)
	}()

	select {
	case <-src.started:
	case <-time.After(2 * time.Second):
		t.Fatal("AddKnowledge() did not reach source resolve")
	}

	closeDone := make(chan error, 1)
	go func() {
		closeDone <- a.Close()
	}()

	select {
	case err := <-closeDone:
		t.Fatalf("Close() returned early before in-flight AddKnowledge completion: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(src.release)

	select {
	case err := <-addErrCh:
		if err != nil {
			t.Fatalf("AddKnowledge() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("AddKnowledge() did not return")
	}

	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close() did not wait and return")
	}
}

func TestAskRetrievalCallbackOrder(t *testing.T) {
	t.Parallel()

	recorder := &callbackRecorder{}
	a := &Agent{
		cfg: Config{
			TopK:                5,
			SimilarityThreshold: 0.5,
			ChatModel:           "chat-test",
			MaxHistoryRounds:    8,
		},
		store: &fakeStore{
			searchHits: []storage.SearchHit{
				{
					Chunk: storage.ChunkRecord{
						ChunkID:    "doc:0",
						SourcePath: "/tmp/doc.md",
						Title:      "doc",
						Text:       "alpha beta",
					},
					Score: 0.95,
				},
			},
		},
		embedder:   &fakeEmbedder{defaultVec: []float32{1, 2, 3}},
		runner:     &fakeRunner{answer: "ok"},
		dispatcher: telemetry.NewDispatcher([]telemetry.Callback{recorder}),
		sessions:   make(map[string]*Session),
	}

	s := a.GetSession("order-test")
	if _, err := s.Ask(context.Background(), "what is this?"); err != nil {
		t.Fatalf("Ask() error = %v", err)
	}

	events := recorder.snapshot()
	pos := map[string]int{}
	for i, event := range events {
		if _, ok := pos[event]; !ok {
			pos[event] = i
		}
	}

	required := []string{"retrieve_start", "tool_start", "retrieve_end", "tool_end"}
	for _, event := range required {
		if _, ok := pos[event]; !ok {
			t.Fatalf("missing callback event %q in %v", event, events)
		}
	}
	if pos["retrieve_start"] >= pos["tool_start"] ||
		pos["tool_start"] >= pos["retrieve_end"] ||
		pos["retrieve_end"] >= pos["tool_end"] {
		t.Fatalf("unexpected callback order: %v", events)
	}
}

func TestAskTelemetryPanicReturnsError(t *testing.T) {
	t.Parallel()

	a := &Agent{
		cfg: Config{
			TopK:                5,
			SimilarityThreshold: 0.5,
			ChatModel:           "chat-test",
			MaxHistoryRounds:    8,
		},
		store: &fakeStore{
			searchHits: []storage.SearchHit{
				{
					Chunk: storage.ChunkRecord{
						ChunkID:    "doc:0",
						SourcePath: "/tmp/doc.md",
						Title:      "doc",
						Text:       "alpha beta",
					},
					Score: 0.95,
				},
			},
		},
		embedder:   &fakeEmbedder{defaultVec: []float32{1, 2, 3}},
		runner:     &fakeRunner{answer: "ok"},
		dispatcher: telemetry.NewDispatcher([]telemetry.Callback{&panicCallback{panicOn: "retrieve_start"}}),
		sessions:   make(map[string]*Session),
	}

	_, err := a.GetSession("panic-telemetry").Ask(context.Background(), "what is this?")
	if err == nil || !strings.Contains(err.Error(), "callback panic") {
		t.Fatalf("Ask() error = %v, want callback panic error", err)
	}
}

func TestGetSessionAfterCloseDoesNotMutateRegistry(t *testing.T) {
	t.Parallel()

	a := &Agent{
		cfg:      Config{MaxHistoryRounds: 8},
		store:    &fakeStore{},
		sessions: make(map[string]*Session),
	}

	_ = a.GetSession("existing")
	if err := a.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if a.sessions != nil {
		t.Fatalf("sessions registry should be nil after close, got %d entries", len(a.sessions))
	}

	closedSession := a.GetSession("new-session")
	if !closedSession.closed {
		t.Fatal("GetSession() after close should return a closed session")
	}
	if a.sessions != nil {
		t.Fatalf("GetSession() after close should not mutate registry, got %d entries", len(a.sessions))
	}
}

func TestCloseDoesNotForceErrSessionClosedForAdmittedAsk(t *testing.T) {
	t.Parallel()

	store := &fakeStore{
		searchHits: []storage.SearchHit{
			{
				Chunk: storage.ChunkRecord{
					ChunkID:    "doc:0",
					SourcePath: "/tmp/doc.md",
					Title:      "doc",
					Text:       "alpha beta",
				},
				Score: 0.95,
			},
		},
	}
	a := &Agent{
		cfg: Config{
			TopK:                5,
			SimilarityThreshold: 0.5,
			ChatModel:           "chat-test",
			MaxHistoryRounds:    8,
		},
		store:    store,
		embedder: &fakeEmbedder{defaultVec: []float32{1, 2, 3}},
		runner:   &fakeRunner{answer: "ok"},
		sessions: make(map[string]*Session),
	}
	s := a.GetSession("race-session")

	admitted := make(chan struct{}, 1)
	release := make(chan struct{})
	s.beforeAskLock = func() {
		select {
		case admitted <- struct{}{}:
		default:
		}
		<-release
	}

	askResult := make(chan struct {
		answer Answer
		err    error
	}, 1)
	go func() {
		answer, err := s.Ask(context.Background(), "what is this?")
		askResult <- struct {
			answer Answer
			err    error
		}{
			answer: answer,
			err:    err,
		}
	}()

	select {
	case <-admitted:
	case <-time.After(2 * time.Second):
		t.Fatal("Ask() was not admitted before timeout")
	}

	closeDone := make(chan error, 1)
	go func() {
		closeDone <- a.Close()
	}()

	select {
	case err := <-closeDone:
		t.Fatalf("Close() returned before admitted Ask() drained: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(release)

	select {
	case got := <-askResult:
		if got.err != nil {
			t.Fatalf("Ask() error = %v, want nil", got.err)
		}
		if got.answer.Text != "ok" {
			t.Fatalf("Ask() text = %q, want %q", got.answer.Text, "ok")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Ask() did not complete after unblocking")
	}

	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close() did not return after Ask() drained")
	}
}

func mustNewChunkerForTest(t *testing.T, size, overlap int) *rag.Chunker {
	t.Helper()

	chunker, err := rag.NewChunker(size, overlap)
	if err != nil {
		t.Fatalf("NewChunker() error = %v", err)
	}
	return chunker
}

func writeBlankPDFForAgentTest(t *testing.T, path string) {
	t.Helper()

	p := gofpdf.New("P", "mm", "A4", "")
	p.AddPage()
	if err := p.OutputFileAndClose(path); err != nil {
		t.Fatalf("OutputFileAndClose(%q): %v", path, err)
	}
}
