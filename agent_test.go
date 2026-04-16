package ragagent

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"my-gtkit-package/go-rag-agent/internal/graph"
	"my-gtkit-package/go-rag-agent/internal/memory"
	"my-gtkit-package/go-rag-agent/internal/rag"
	"my-gtkit-package/go-rag-agent/internal/storage"
)

type fakeStore struct {
	mu            sync.Mutex
	searchHits    []storage.SearchHit
	searchErr     error
	searchCalls   int
	upsertBatches [][]storage.ChunkRecord
}

func (f *fakeStore) Upsert(_ context.Context, chunks []storage.ChunkRecord) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.upsertBatches = append(f.upsertBatches, slices.Clone(chunks))
	return nil
}

func (f *fakeStore) Search(_ context.Context, _ []float32, _ int, _ float32) ([]storage.SearchHit, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.searchCalls++
	return slices.Clone(f.searchHits), f.searchErr
}

func (f *fakeStore) Close() error { return nil }

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
