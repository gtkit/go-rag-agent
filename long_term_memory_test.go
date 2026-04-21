package ragagent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gtkit/go-rag-agent/internal/graph"
	"github.com/gtkit/go-rag-agent/internal/storage"
)

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
