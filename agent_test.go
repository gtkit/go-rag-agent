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

	"my-gtkit-package/go-rag-agent/internal/graph"
	"my-gtkit-package/go-rag-agent/internal/memory"
	"my-gtkit-package/go-rag-agent/internal/rag"
	"my-gtkit-package/go-rag-agent/internal/storage"
	"my-gtkit-package/go-rag-agent/internal/telemetry"
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

func (f *fakeStore) Search(_ context.Context, _ []float32, _ int, _ float32) ([]storage.SearchHit, error) {
	return f.SearchWithFilter(context.Background(), nil, 0, 0, storage.SearchFilter{})
}

func (f *fakeStore) SearchWithFilter(_ context.Context, _ []float32, _ int, _ float32, filter storage.SearchFilter) ([]storage.SearchHit, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.searchCalls++
	f.lastFilter = filter
	if f.searchErr != nil {
		return nil, f.searchErr
	}
	return filterHitsForTest(f.searchHits, filter), nil
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

func filterHitsForTest(hits []storage.SearchHit, filter storage.SearchFilter) []storage.SearchHit {
	if len(hits) == 0 {
		return nil
	}

	filtered := make([]storage.SearchHit, 0, len(hits))
	for _, hit := range hits {
		if !matchesSearchFilterForTest(hit.Chunk, filter) {
			continue
		}
		filtered = append(filtered, hit)
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
	if !(pos["retrieve_start"] < pos["tool_start"] &&
		pos["tool_start"] < pos["retrieve_end"] &&
		pos["retrieve_end"] < pos["tool_end"]) {
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
