package ragagent

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gtkit/go-rag-agent/internal/graph"
	"github.com/gtkit/go-rag-agent/internal/storage"
)

type memoryVectorStoreStub struct {
	mu      sync.Mutex
	chunks  map[string]ChunkRecord
	deletes []string
}

func newMemoryVectorStoreStub() *memoryVectorStoreStub {
	return &memoryVectorStoreStub{
		chunks: make(map[string]ChunkRecord),
	}
}

func (s *memoryVectorStoreStub) Upsert(_ context.Context, chunks []ChunkRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, chunk := range chunks {
		s.chunks[chunk.ChunkID] = chunk
	}
	return nil
}

func (s *memoryVectorStoreStub) Search(_ context.Context, queryEmbedding []float32, topK int, threshold float32) ([]SearchHit, error) {
	return s.SearchWithFilter(context.Background(), queryEmbedding, topK, threshold, SearchFilter{})
}

func (s *memoryVectorStoreStub) SearchWithFilter(_ context.Context, queryEmbedding []float32, topK int, threshold float32, filter SearchFilter) ([]SearchHit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	hits := make([]SearchHit, 0)
	for _, chunk := range s.chunks {
		if len(filter.SourcePaths) > 0 && !strings.Contains(strings.Join(filter.SourcePaths, ","), chunk.SourcePath) {
			continue
		}
		score, ok := cosineSimilarity(queryEmbedding, chunk.Embedding)
		if !ok || score < threshold {
			continue
		}
		hits = append(hits, SearchHit{
			Chunk: chunk,
			Score: score,
		})
	}
	if len(hits) > topK {
		hits = hits[:topK]
	}
	return hits, nil
}

func (s *memoryVectorStoreStub) DeleteBySourcePaths(_ context.Context, sourcePaths []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deletes = append(s.deletes, sourcePaths...)
	for id, chunk := range s.chunks {
		for _, path := range sourcePaths {
			if chunk.SourcePath == path {
				delete(s.chunks, id)
			}
		}
	}
	return nil
}

func (s *memoryVectorStoreStub) Close() error { return nil }

func TestInMemoryLongTermMemoryStore(t *testing.T) {
	t.Parallel()

	store := NewInMemoryLongTermMemoryStore()
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	err := store.Store(context.Background(), []LongTermMemoryRecord{
		{
			ID:        "m1",
			SessionID: "session-a",
			User:      "what is rag",
			Assistant: "rag is retrieval augmented generation",
			Embedding: []float32{1, 0},
			CreatedAt: time.Now(),
		},
		{
			ID:        "m2",
			SessionID: "session-b",
			User:      "what is redis",
			Assistant: "redis is a cache",
			Embedding: []float32{0, 1},
			CreatedAt: time.Now(),
		},
	})
	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}

	hits, err := store.Search(context.Background(), "session-a", []float32{1, 0}, 3, 0)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("Search() len = %d, want 1", len(hits))
	}
	if hits[0].Memory.ID != "m1" {
		t.Fatalf("Search() top memory id = %q, want %q", hits[0].Memory.ID, "m1")
	}

	if err := store.ClearSession(context.Background(), "session-a"); err != nil {
		t.Fatalf("ClearSession() error = %v", err)
	}
	hits, err = store.Search(context.Background(), "session-a", []float32{1, 0}, 3, 0)
	if err != nil {
		t.Fatalf("Search() after clear error = %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("Search() after clear len = %d, want 0", len(hits))
	}
}

func TestInMemoryLongTermMemoryStoreSkipsExpiredRecords(t *testing.T) {
	t.Parallel()

	store := NewInMemoryLongTermMemoryStore()
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	err := store.Store(context.Background(), []LongTermMemoryRecord{
		{
			ID:        "expired",
			SessionID: "session-a",
			User:      "old",
			Assistant: "expired answer",
			Embedding: []float32{1, 0},
			CreatedAt: time.Now().Add(-2 * time.Hour),
			ExpiresAt: time.Now().Add(-time.Hour),
		},
		{
			ID:        "active",
			SessionID: "session-a",
			User:      "new",
			Assistant: "active answer",
			Embedding: []float32{1, 0},
			CreatedAt: time.Now(),
			ExpiresAt: time.Now().Add(time.Hour),
		},
	})
	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}

	hits, err := store.Search(context.Background(), "session-a", []float32{1, 0}, 5, 0)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("Search() len = %d, want 1", len(hits))
	}
	if hits[0].Memory.ID != "active" {
		t.Fatalf("Search() top memory id = %q, want %q", hits[0].Memory.ID, "active")
	}
}

func TestInMemoryLongTermMemoryStorePruneExpired(t *testing.T) {
	t.Parallel()

	store := NewInMemoryLongTermMemoryStore()
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	now := time.Now()
	err := store.Store(context.Background(), []LongTermMemoryRecord{
		{
			ID:        "expired",
			SessionID: "session-a",
			User:      "old",
			Assistant: "expired answer",
			Embedding: []float32{1, 0},
			CreatedAt: now.Add(-2 * time.Hour),
			ExpiresAt: now.Add(-time.Hour),
		},
		{
			ID:        "active",
			SessionID: "session-a",
			User:      "new",
			Assistant: "active answer",
			Embedding: []float32{1, 0},
			CreatedAt: now,
			ExpiresAt: now.Add(time.Hour),
		},
	})
	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}

	maint, ok := store.(LongTermMemoryMaintenance)
	if !ok {
		t.Fatal("store does not implement LongTermMemoryMaintenance")
	}
	pruned, err := maint.PruneExpired(context.Background(), now)
	if err != nil {
		t.Fatalf("PruneExpired() error = %v", err)
	}
	if pruned != 1 {
		t.Fatalf("PruneExpired() = %d, want 1", pruned)
	}
}

func TestVectorLongTermMemoryStore(t *testing.T) {
	t.Parallel()

	now := time.Now()
	tests := []struct {
		name string
		run  func(*testing.T, LongTermMemoryStore, *memoryVectorStoreStub)
	}{
		{
			name: "store and search scoped to session",
			run: func(t *testing.T, store LongTermMemoryStore, backing *memoryVectorStoreStub) {
				t.Helper()

				err := store.Store(context.Background(), []LongTermMemoryRecord{
					{
						ID:        "m1",
						SessionID: "session-a",
						User:      "what is rag",
						Assistant: "rag is retrieval augmented generation",
						Embedding: []float32{1, 0},
						CreatedAt: now,
					},
					{
						ID:        "m2",
						SessionID: "session-b",
						User:      "what is redis",
						Assistant: "redis is a cache",
						Embedding: []float32{0, 1},
						CreatedAt: now,
					},
				})
				if err != nil {
					t.Fatalf("Store() error = %v", err)
				}

				hits, err := store.Search(context.Background(), "session-a", []float32{1, 0}, 3, 0)
				if err != nil {
					t.Fatalf("Search() error = %v", err)
				}
				if len(hits) != 1 {
					t.Fatalf("Search() len = %d, want 1", len(hits))
				}
				if hits[0].Memory.ID != "m1" {
					t.Fatalf("Search() top memory id = %q, want %q", hits[0].Memory.ID, "m1")
				}

				backing.mu.Lock()
				chunk := backing.chunks["m1"]
				backing.mu.Unlock()
				if chunk.SourcePath != "/ragagent/memory/session-a" {
					t.Fatalf("stored chunk source path = %q, want %q", chunk.SourcePath, "/ragagent/memory/session-a")
				}
			},
		},
		{
			name: "clear session deletes memory source path",
			run: func(t *testing.T, store LongTermMemoryStore, backing *memoryVectorStoreStub) {
				t.Helper()

				err := store.Store(context.Background(), []LongTermMemoryRecord{
					{
						ID:        "m1",
						SessionID: "session-a",
						User:      "what is rag",
						Assistant: "rag is retrieval augmented generation",
						Embedding: []float32{1, 0},
						CreatedAt: now,
					},
				})
				if err != nil {
					t.Fatalf("Store() error = %v", err)
				}
				if err := store.ClearSession(context.Background(), "session-a"); err != nil {
					t.Fatalf("ClearSession() error = %v", err)
				}
				backing.mu.Lock()
				defer backing.mu.Unlock()
				if len(backing.deletes) != 1 {
					t.Fatalf("DeleteBySourcePaths() calls = %d, want 1", len(backing.deletes))
				}
				if backing.deletes[0] != "/ragagent/memory/session-a" {
					t.Fatalf("DeleteBySourcePaths() path = %q, want %q", backing.deletes[0], "/ragagent/memory/session-a")
				}
				if len(backing.chunks) != 0 {
					t.Fatalf("backing chunks len = %d, want 0", len(backing.chunks))
				}
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			backing := newMemoryVectorStoreStub()
			store := NewVectorLongTermMemoryStore(backing)
			t.Cleanup(func() {
				if err := store.Close(); err != nil {
					t.Fatalf("Close() error = %v", err)
				}
			})

			tc.run(t, store, backing)
		})
	}
}

func TestAskUsesLongTermMemoryAfterShortTermHistoryTrim(t *testing.T) {
	t.Parallel()

	store := NewInMemoryLongTermMemoryStore()
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	embedder := &fakeEmbedder{
		defaultVec: []float32{0, 0},
		vectors: map[string][]float32{
			"remember architecture":               {1, 0},
			"remember architecture\nfirst answer": {1, 0},
			"what did we decide":                  {1, 0},
			"what did we decide\nsecond answer":   {0, 1},
		},
	}
	runner := &fakeRunner{answer: "first answer"}
	a := &Agent{
		cfg: Config{
			ChatModel:               "memory-model",
			TopK:                    1,
			SimilarityThreshold:     0.5,
			MaxHistoryRounds:        1,
			LongTermMemoryTopK:      2,
			LongTermMemoryThreshold: 0,
			MaxMemoryTokens:         256,
		},
		store: &fakeStore{
			searchHits: []storage.SearchHit{
				{
					Chunk: storage.ChunkRecord{
						ChunkID:    "doc:0",
						SourcePath: "/tmp/doc.md",
						Title:      "doc",
						Text:       "architecture context",
						StartRune:  0,
						EndRune:    20,
						Metadata:   map[string]string{accessBoundaryNamespaceKey: "tenant-a"},
					},
					Score: 0.99,
				},
			},
		},
		embedder:       embedder,
		runner:         runner,
		longTermMemory: store,
		sessions:       make(map[string]*Session),
	}

	session := a.GetSession("memory-session")
	if _, err := session.Ask(context.Background(), "remember architecture"); err != nil {
		t.Fatalf("first Ask() error = %v", err)
	}
	if err := session.ClearHistory(context.Background()); err != nil {
		t.Fatalf("ClearHistory() error = %v", err)
	}

	runner.mu.Lock()
	runner.answer = "second answer"
	runner.mu.Unlock()
	if _, err := session.Ask(context.Background(), "what did we decide"); err != nil {
		t.Fatalf("second Ask() error = %v", err)
	}

	runner.mu.Lock()
	gotMemory := runner.lastReq.LongTermMemoryText
	runner.mu.Unlock()
	if !strings.Contains(gotMemory, "first answer") {
		t.Fatalf("LongTermMemoryText = %q, want contains %q", gotMemory, "first answer")
	}
}

func TestStoreLongTermMemoryDeduplicatesByContent(t *testing.T) {
	t.Parallel()

	store := NewInMemoryLongTermMemoryStore()
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	embedder := &fakeEmbedder{
		defaultVec: []float32{1, 0},
		vectors: map[string][]float32{
			"same query\nsame answer": {1, 0},
		},
	}
	a := &Agent{
		cfg: Config{
			LongTermMemoryMaxStoredRunes: 128,
			AccessBoundary: AccessBoundaryConfig{
				Namespace: "tenant-a",
			},
		},
		embedder:       embedder,
		longTermMemory: store,
	}

	opts := QueryOptions{
		MemoryScope: MemoryScope{
			UserID: "user-a",
		},
	}
	if err := a.storeLongTermMemory(context.Background(), "session-a", "same query", "same answer", opts); err != nil {
		t.Fatalf("first storeLongTermMemory() error = %v", err)
	}
	if err := a.storeLongTermMemory(context.Background(), "session-a", "same query", "same answer", opts); err != nil {
		t.Fatalf("second storeLongTermMemory() error = %v", err)
	}

	backing := store.(*inMemoryLongTermMemoryStore)
	backing.mu.RLock()
	defer backing.mu.RUnlock()
	if len(backing.records) != 1 {
		t.Fatalf("records len = %d, want 1", len(backing.records))
	}
	if backing.records[0].Tenant != "tenant-a" {
		t.Fatalf("record tenant = %q, want %q", backing.records[0].Tenant, "tenant-a")
	}
	if backing.records[0].UserID != "user-a" {
		t.Fatalf("record user id = %q, want %q", backing.records[0].UserID, "user-a")
	}
}

func TestStoreLongTermMemoryCompressesStoredText(t *testing.T) {
	t.Parallel()

	store := NewInMemoryLongTermMemoryStore()
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	query := strings.Repeat("query ", 40)
	answer := strings.Repeat("answer ", 40)
	embedder := &fakeEmbedder{
		defaultVec: []float32{1, 0},
		vectors: map[string][]float32{
			strings.TrimSpace(query) + "\n" + strings.TrimSpace(answer): {1, 0},
		},
	}
	a := &Agent{
		cfg: Config{
			LongTermMemoryMaxStoredRunes: 64,
		},
		embedder:       embedder,
		longTermMemory: store,
	}
	if err := a.storeLongTermMemory(context.Background(), "session-a", query, answer, QueryOptions{}); err != nil {
		t.Fatalf("storeLongTermMemory() error = %v", err)
	}

	backing := store.(*inMemoryLongTermMemoryStore)
	backing.mu.RLock()
	defer backing.mu.RUnlock()
	if len(backing.records) != 1 {
		t.Fatalf("records len = %d, want 1", len(backing.records))
	}
	combined := backing.records[0].User + backing.records[0].Assistant
	if len([]rune(combined)) > 64 {
		t.Fatalf("combined stored runes = %d, want <= 64", len([]rune(combined)))
	}
}

func TestAskUsesScopedLongTermMemory(t *testing.T) {
	t.Parallel()

	store := NewInMemoryLongTermMemoryStore()
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	embedder := &fakeEmbedder{
		defaultVec: []float32{0, 0},
		vectors: map[string][]float32{
			"remember decision\nanswer one": {1, 0},
			"remember decision":             {1, 0},
			"follow up":                     {1, 0},
			"follow up\nanswer two":         {1, 0},
			"follow up\nanswer three":       {1, 0},
		},
	}
	runner := &fakeRunner{answer: "answer one"}
	a := &Agent{
		cfg: Config{
			ChatModel:                    "memory-model",
			TopK:                         1,
			SimilarityThreshold:          0.5,
			MaxHistoryRounds:             1,
			LongTermMemoryTopK:           2,
			LongTermMemoryThreshold:      0,
			LongTermMemoryMaxStoredRunes: 128,
			AccessBoundary: AccessBoundaryConfig{
				Namespace: "tenant-a",
			},
		},
		store: &fakeStore{
			searchHits: []storage.SearchHit{
				{
					Chunk: storage.ChunkRecord{
						ChunkID:    "doc:0",
						SourcePath: "/tmp/doc.md",
						Title:      "doc",
						Text:       "architecture context",
						StartRune:  0,
						EndRune:    20,
						Metadata:   map[string]string{accessBoundaryNamespaceKey: "tenant-a"},
					},
					Score: 0.99,
				},
			},
		},
		embedder:       embedder,
		runner:         runner,
		longTermMemory: store,
		sessions:       make(map[string]*Session),
	}

	session := a.GetSession("shared-session")
	if _, err := session.AskWithOptions(context.Background(), "remember decision", QueryOptions{
		MemoryScope: MemoryScope{UserID: "user-a"},
	}); err != nil {
		t.Fatalf("first AskWithOptions() error = %v", err)
	}
	if err := session.ClearHistory(context.Background()); err != nil {
		t.Fatalf("ClearHistory() error = %v", err)
	}

	runner.mu.Lock()
	runner.answer = "answer two"
	runner.mu.Unlock()
	if _, err := session.AskWithOptions(context.Background(), "follow up", QueryOptions{
		MemoryScope: MemoryScope{UserID: "user-b"},
	}); err != nil {
		t.Fatalf("second AskWithOptions() error = %v", err)
	}
	runner.mu.Lock()
	noLeak := runner.lastReq.LongTermMemoryText
	runner.mu.Unlock()
	if strings.Contains(noLeak, "answer one") {
		t.Fatalf("LongTermMemoryText leaked across user scope: %q", noLeak)
	}

	if err := session.ClearHistory(context.Background()); err != nil {
		t.Fatalf("ClearHistory() second error = %v", err)
	}
	runner.mu.Lock()
	runner.answer = "answer three"
	runner.mu.Unlock()
	if _, err := session.AskWithOptions(context.Background(), "follow up", QueryOptions{
		MemoryScope: MemoryScope{UserID: "user-a"},
	}); err != nil {
		t.Fatalf("third AskWithOptions() error = %v", err)
	}
	runner.mu.Lock()
	gotMemory := runner.lastReq.LongTermMemoryText
	runner.mu.Unlock()
	if !strings.Contains(gotMemory, "answer one") {
		t.Fatalf("LongTermMemoryText = %q, want contains %q", gotMemory, "answer one")
	}
}

func TestAskStreamStoresLongTermMemoryAfterSuccess(t *testing.T) {
	t.Parallel()

	store := NewInMemoryLongTermMemoryStore()
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	embedder := &fakeEmbedder{
		defaultVec: []float32{0, 0},
		vectors: map[string][]float32{
			"stream query\nstream answer": {1, 0},
		},
	}
	runner := &fakeStreamingRunner{
		askStreamFn: func(_ context.Context, _ graph.Request, emit graph.StreamEmitter) error {
			if err := emit(graph.Event{Type: graph.EventAnswerChunk, Content: "stream "}); err != nil {
				return err
			}
			if err := emit(graph.Event{Type: graph.EventAnswerChunk, Content: "answer"}); err != nil {
				return err
			}
			return emit(graph.Event{Type: graph.EventDone, Step: 2})
		},
	}
	a := &Agent{
		cfg: Config{
			ChatModel:               "memory-model",
			TopK:                    1,
			SimilarityThreshold:     0.5,
			MaxHistoryRounds:        1,
			LongTermMemoryTopK:      2,
			LongTermMemoryThreshold: 0,
			MaxMemoryTokens:         256,
		},
		store: &fakeStore{
			searchHits: []storage.SearchHit{
				{
					Chunk: storage.ChunkRecord{
						ChunkID:    "doc:0",
						SourcePath: "/tmp/doc.md",
						Title:      "doc",
						Text:       "stream context",
						StartRune:  0,
						EndRune:    14,
					},
					Score: 0.99,
				},
			},
		},
		embedder:       embedder,
		runner:         runner,
		longTermMemory: store,
		sessions:       make(map[string]*Session),
	}

	err := a.GetSession("memory-stream").AskStream(context.Background(), "stream query", func(StreamEvent) error { return nil })
	if err != nil {
		t.Fatalf("AskStream() error = %v", err)
	}

	hits, err := store.Search(context.Background(), "memory-stream", []float32{1, 0}, 2, 0)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("Search() len = %d, want 1", len(hits))
	}
	if hits[0].Memory.Assistant != "stream answer" {
		t.Fatalf("Assistant memory = %q, want %q", hits[0].Memory.Assistant, "stream answer")
	}
}
