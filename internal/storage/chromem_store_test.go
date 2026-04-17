package storage

import (
	"context"
	"errors"
	"slices"
	"sync/atomic"
	"testing"
	"time"
)

func TestChromemStoreUpsertAndSearch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		chunks           []ChunkRecord
		queryEmbedding   []float32
		topK             int
		threshold        float32
		wantChunkOrder   []string
		wantFirstStart   int
		wantFirstEnd     int
		wantFirstTopic   string
		wantFirstNSValue string
		wantSecondParent string
	}{
		{
			name: "returns expected ordering and preserves metadata mapping",
			chunks: []ChunkRecord{
				{
					ChunkID:    "doc-a:0",
					ParentID:   "doc-a",
					SourcePath: "/kb/a.md",
					Title:      "Doc A",
					Text:       "alpha text",
					StartRune:  0,
					EndRune:    10,
					Metadata: map[string]string{
						"topic":           "alpha",
						"rag_source_path": "caller-namespace-value",
					},
					Embedding: []float32{1, 0},
				},
				{
					ChunkID:    "doc-b:0",
					ParentID:   "doc-b",
					SourcePath: "/kb/b.md",
					Title:      "Doc B",
					Text:       "beta text",
					StartRune:  5,
					EndRune:    15,
					Metadata:   map[string]string{"topic": "beta"},
					Embedding:  []float32{0.8, 0.6},
				},
				{
					ChunkID:    "doc-c:0",
					ParentID:   "doc-c",
					SourcePath: "/kb/c.md",
					Title:      "Doc C",
					Text:       "gamma text",
					StartRune:  2,
					EndRune:    12,
					Metadata:   map[string]string{"topic": "gamma"},
					Embedding:  []float32{-1, 0},
				},
			},
			queryEmbedding:   []float32{1, 0},
			topK:             5,
			threshold:        0.5,
			wantChunkOrder:   []string{"doc-a:0", "doc-b:0"},
			wantFirstStart:   0,
			wantFirstEnd:     10,
			wantFirstTopic:   "alpha",
			wantFirstNSValue: "caller-namespace-value",
			wantSecondParent: "doc-b",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store, err := NewChromemStore(Config{})
			if err != nil {
				t.Fatalf("NewChromemStore() error = %v", err)
			}
			t.Cleanup(func() {
				if cerr := store.Close(); cerr != nil {
					t.Fatalf("Close() error = %v", cerr)
				}
			})

			if err := store.Upsert(context.Background(), tc.chunks); err != nil {
				t.Fatalf("Upsert() error = %v", err)
			}

			got, err := store.Search(context.Background(), tc.queryEmbedding, tc.topK, tc.threshold)
			if err != nil {
				t.Fatalf("Search() error = %v", err)
			}

			gotOrder := make([]string, 0, len(got))
			for _, hit := range got {
				gotOrder = append(gotOrder, hit.Chunk.ChunkID)
			}
			if !slices.Equal(gotOrder, tc.wantChunkOrder) {
				t.Fatalf("Search() order = %v, want %v", gotOrder, tc.wantChunkOrder)
			}

			if len(got) < 2 {
				t.Fatalf("Search() len = %d, want at least 2", len(got))
			}
			if got[0].Chunk.StartRune != tc.wantFirstStart || got[0].Chunk.EndRune != tc.wantFirstEnd {
				t.Fatalf("Search() first rune range = [%d,%d], want [%d,%d]", got[0].Chunk.StartRune, got[0].Chunk.EndRune, tc.wantFirstStart, tc.wantFirstEnd)
			}
			if got[0].Chunk.Metadata["topic"] != tc.wantFirstTopic {
				t.Fatalf("Search() first topic = %q, want %q", got[0].Chunk.Metadata["topic"], tc.wantFirstTopic)
			}
			if got[0].Chunk.Metadata["rag_source_path"] != tc.wantFirstNSValue {
				t.Fatalf("Search() first namespaced metadata = %q, want %q", got[0].Chunk.Metadata["rag_source_path"], tc.wantFirstNSValue)
			}
			if got[1].Chunk.ParentID != tc.wantSecondParent {
				t.Fatalf("Search() second parent id = %q, want %q", got[1].Chunk.ParentID, tc.wantSecondParent)
			}
		})
	}
}

func TestChromemStoreSearchThresholdFiltering(t *testing.T) {
	t.Parallel()

	chunks := []ChunkRecord{
		{
			ChunkID:    "doc-1:0",
			ParentID:   "doc-1",
			SourcePath: "/kb/one.md",
			Title:      "One",
			Text:       "one",
			StartRune:  0,
			EndRune:    3,
			Embedding:  []float32{1, 0},
		},
		{
			ChunkID:    "doc-2:0",
			ParentID:   "doc-2",
			SourcePath: "/kb/two.md",
			Title:      "Two",
			Text:       "two",
			StartRune:  0,
			EndRune:    3,
			Embedding:  []float32{0.8, 0.6},
		},
		{
			ChunkID:    "doc-3:0",
			ParentID:   "doc-3",
			SourcePath: "/kb/three.md",
			Title:      "Three",
			Text:       "three",
			StartRune:  0,
			EndRune:    5,
			Embedding:  []float32{0, 1},
		},
	}

	tests := []struct {
		name      string
		threshold float32
		wantOrder []string
	}{
		{
			name:      "keeps all non-negative similarity results when threshold zero",
			threshold: 0,
			wantOrder: []string{"doc-1:0", "doc-2:0", "doc-3:0"},
		},
		{
			name:      "keeps only near-exact matches when threshold high",
			threshold: 0.9,
			wantOrder: []string{"doc-1:0"},
		},
		{
			name:      "drops all when threshold above max similarity",
			threshold: 1.1,
			wantOrder: nil,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store, err := NewChromemStore(Config{})
			if err != nil {
				t.Fatalf("NewChromemStore() error = %v", err)
			}
			t.Cleanup(func() {
				if cerr := store.Close(); cerr != nil {
					t.Fatalf("Close() error = %v", cerr)
				}
			})

			if err := store.Upsert(context.Background(), chunks); err != nil {
				t.Fatalf("Upsert() error = %v", err)
			}

			got, err := store.Search(context.Background(), []float32{1, 0}, len(chunks), tc.threshold)
			if err != nil {
				t.Fatalf("Search() error = %v", err)
			}

			gotOrder := make([]string, 0, len(got))
			for _, hit := range got {
				gotOrder = append(gotOrder, hit.Chunk.ChunkID)
			}
			if !slices.Equal(gotOrder, tc.wantOrder) {
				t.Fatalf("Search() order = %v, want %v", gotOrder, tc.wantOrder)
			}
		})
	}
}

func TestChromemStoreSearchWithFilter(t *testing.T) {
	t.Parallel()

	chunks := []ChunkRecord{
		{
			ChunkID:    "alpha-api:0",
			ParentID:   "alpha-api",
			SourcePath: "/kb/project-a/api.md",
			Title:      "API",
			Text:       "alpha api",
			StartRune:  0,
			EndRune:    9,
			Metadata: map[string]string{
				"tag":  "api",
				"team": "alpha",
			},
			Embedding: []float32{1, 0},
		},
		{
			ChunkID:    "alpha-guide:0",
			ParentID:   "alpha-guide",
			SourcePath: "/kb/project-a/guide.md",
			Title:      "Guide",
			Text:       "alpha guide",
			StartRune:  0,
			EndRune:    11,
			Metadata: map[string]string{
				"tag":  "guide",
				"team": "alpha",
			},
			Embedding: []float32{0.95, 0.05},
		},
		{
			ChunkID:    "beta-api:0",
			ParentID:   "beta-api",
			SourcePath: "/kb/project-b/api.md",
			Title:      "Beta API",
			Text:       "beta api",
			StartRune:  0,
			EndRune:    8,
			Metadata: map[string]string{
				"tag":  "api",
				"team": "beta",
			},
			Embedding: []float32{0.9, 0.1},
		},
	}

	tests := []struct {
		name    string
		filter  SearchFilter
		wantIDs []string
	}{
		{
			name: "filters by exact source path",
			filter: SearchFilter{
				SourcePaths: []string{"/kb/project-b/api.md"},
			},
			wantIDs: []string{"beta-api:0"},
		},
		{
			name: "filters by source prefix",
			filter: SearchFilter{
				SourcePrefixes: []string{"/kb/project-a"},
			},
			wantIDs: []string{"alpha-api:0", "alpha-guide:0"},
		},
		{
			name: "filters by exact metadata",
			filter: SearchFilter{
				Metadata: map[string]string{"team": "alpha", "tag": "guide"},
			},
			wantIDs: []string{"alpha-guide:0"},
		},
		{
			name: "combines source prefix and metadata",
			filter: SearchFilter{
				SourcePrefixes: []string{"/kb/project-a"},
				Metadata:       map[string]string{"tag": "api"},
			},
			wantIDs: []string{"alpha-api:0"},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store, err := NewChromemStore(Config{})
			if err != nil {
				t.Fatalf("NewChromemStore() error = %v", err)
			}
			t.Cleanup(func() {
				if cerr := store.Close(); cerr != nil {
					t.Fatalf("Close() error = %v", cerr)
				}
			})

			if err := store.Upsert(context.Background(), chunks); err != nil {
				t.Fatalf("Upsert() error = %v", err)
			}

			got, err := store.SearchWithFilter(context.Background(), []float32{1, 0}, 5, -1, tc.filter)
			if err != nil {
				t.Fatalf("SearchWithFilter() error = %v", err)
			}

			gotIDs := make([]string, 0, len(got))
			for _, hit := range got {
				gotIDs = append(gotIDs, hit.Chunk.ChunkID)
			}
			if !slices.Equal(gotIDs, tc.wantIDs) {
				t.Fatalf("SearchWithFilter() ids = %v, want %v", gotIDs, tc.wantIDs)
			}
		})
	}
}

func TestChromemStoreUpsertRejectsDuplicateChunkIDInBatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		chunks  []ChunkRecord
		wantErr bool
	}{
		{
			name: "duplicate chunk ids in same batch returns error",
			chunks: []ChunkRecord{
				{
					ChunkID:    "doc-a:0",
					ParentID:   "doc-a",
					SourcePath: "/kb/a.md",
					Title:      "A",
					Text:       "a",
					StartRune:  0,
					EndRune:    1,
					Embedding:  []float32{1, 0},
				},
				{
					ChunkID:    "doc-a:0",
					ParentID:   "doc-a",
					SourcePath: "/kb/a.md",
					Title:      "A",
					Text:       "a2",
					StartRune:  0,
					EndRune:    2,
					Embedding:  []float32{0.9, 0.1},
				},
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store, err := NewChromemStore(Config{})
			if err != nil {
				t.Fatalf("NewChromemStore() error = %v", err)
			}

			err = store.Upsert(context.Background(), tc.chunks)
			if tc.wantErr && err == nil {
				t.Fatalf("Upsert() error = nil, want non-nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("Upsert() error = %v, want nil", err)
			}
		})
	}
}

func TestChromemStoreUpsertReplacesParentChunksAndPreventsStaleResults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		firstIngest         []ChunkRecord
		secondIngest        []ChunkRecord
		query               []float32
		topK                int
		threshold           float32
		wantAfterSecond     []string
		wantNoLongerPresent string
	}{
		{
			name: "shorter re-ingest for same parent removes stale chunks",
			firstIngest: []ChunkRecord{
				{
					ChunkID:    "doc-a:0",
					ParentID:   "doc-a",
					SourcePath: "/kb/a.md",
					Title:      "A",
					Text:       "old 0",
					StartRune:  0,
					EndRune:    5,
					Embedding:  []float32{1, 0},
				},
				{
					ChunkID:    "doc-a:1",
					ParentID:   "doc-a",
					SourcePath: "/kb/a.md",
					Title:      "A",
					Text:       "old 1",
					StartRune:  5,
					EndRune:    10,
					Embedding:  []float32{0.95, 0.05},
				},
			},
			secondIngest: []ChunkRecord{
				{
					ChunkID:    "doc-a:0",
					ParentID:   "doc-a",
					SourcePath: "/kb/a.md",
					Title:      "A new",
					Text:       "new only",
					StartRune:  0,
					EndRune:    8,
					Embedding:  []float32{1, 0},
				},
			},
			query:               []float32{1, 0},
			topK:                5,
			threshold:           -1,
			wantAfterSecond:     []string{"doc-a:0"},
			wantNoLongerPresent: "doc-a:1",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store, err := NewChromemStore(Config{})
			if err != nil {
				t.Fatalf("NewChromemStore() error = %v", err)
			}

			if err := store.Upsert(context.Background(), tc.firstIngest); err != nil {
				t.Fatalf("Upsert(first) error = %v", err)
			}
			if err := store.Upsert(context.Background(), tc.secondIngest); err != nil {
				t.Fatalf("Upsert(second) error = %v", err)
			}

			got, err := store.Search(context.Background(), tc.query, tc.topK, tc.threshold)
			if err != nil {
				t.Fatalf("Search() error = %v", err)
			}
			gotIDs := make([]string, 0, len(got))
			for _, hit := range got {
				gotIDs = append(gotIDs, hit.Chunk.ChunkID)
			}
			if !slices.Equal(gotIDs, tc.wantAfterSecond) {
				t.Fatalf("Search() ids after second ingest = %v, want %v", gotIDs, tc.wantAfterSecond)
			}
			if slices.Contains(gotIDs, tc.wantNoLongerPresent) {
				t.Fatalf("Search() contains stale chunk id %q", tc.wantNoLongerPresent)
			}
		})
	}
}

func TestChromemStoreUpsertCanceledContextPreservesExistingData(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		existing          []ChunkRecord
		canceledUpsert    []ChunkRecord
		query             []float32
		topK              int
		wantSearchChunkID []string
	}{
		{
			name: "canceled context before upsert keeps prior chunks searchable",
			existing: []ChunkRecord{
				{
					ChunkID:    "doc-a:0",
					ParentID:   "doc-a",
					SourcePath: "/kb/a.md",
					Title:      "A old",
					Text:       "old text",
					StartRune:  0,
					EndRune:    8,
					Embedding:  []float32{1, 0},
				},
				{
					ChunkID:    "doc-a:1",
					ParentID:   "doc-a",
					SourcePath: "/kb/a.md",
					Title:      "A old",
					Text:       "old text 2",
					StartRune:  8,
					EndRune:    18,
					Embedding:  []float32{0.9, 0.1},
				},
			},
			canceledUpsert: []ChunkRecord{
				{
					ChunkID:    "doc-a:0",
					ParentID:   "doc-a",
					SourcePath: "/kb/a.md",
					Title:      "A new",
					Text:       "new text",
					StartRune:  0,
					EndRune:    8,
					Embedding:  []float32{1, 0},
				},
			},
			query:             []float32{1, 0},
			topK:              5,
			wantSearchChunkID: []string{"doc-a:0", "doc-a:1"},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store, err := NewChromemStore(Config{})
			if err != nil {
				t.Fatalf("NewChromemStore() error = %v", err)
			}

			if err := store.Upsert(context.Background(), tc.existing); err != nil {
				t.Fatalf("Upsert(existing) error = %v", err)
			}

			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			err = store.Upsert(ctx, tc.canceledUpsert)
			if err == nil {
				t.Fatalf("Upsert(canceled) error = nil, want non-nil")
			}

			got, err := store.Search(context.Background(), tc.query, tc.topK, -1)
			if err != nil {
				t.Fatalf("Search() error = %v", err)
			}

			gotIDs := make([]string, 0, len(got))
			for _, hit := range got {
				gotIDs = append(gotIDs, hit.Chunk.ChunkID)
			}
			if !slices.Equal(gotIDs, tc.wantSearchChunkID) {
				t.Fatalf("Search() ids = %v, want %v", gotIDs, tc.wantSearchChunkID)
			}
		})
	}
}

func TestChromemStoreUpsertPostAddCancellationCleansStaleAndReturnsContextError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		existing        []ChunkRecord
		reingest        []ChunkRecord
		query           []float32
		wantIDs         []string
		wantFirstTitle  string
		wantCanceledErr error
	}{
		{
			name: "cancel after add still completes cleanup then returns cancellation",
			existing: []ChunkRecord{
				{
					ChunkID:    "doc-a:0",
					ParentID:   "doc-a",
					SourcePath: "/kb/a.md",
					Title:      "A old",
					Text:       "old text",
					StartRune:  0,
					EndRune:    8,
					Embedding:  []float32{1, 0},
				},
				{
					ChunkID:    "doc-a:1",
					ParentID:   "doc-a",
					SourcePath: "/kb/a.md",
					Title:      "A old",
					Text:       "old text 2",
					StartRune:  8,
					EndRune:    18,
					Embedding:  []float32{0.9, 0.1},
				},
			},
			reingest: []ChunkRecord{
				{
					ChunkID:    "doc-a:0",
					ParentID:   "doc-a",
					SourcePath: "/kb/a.md",
					Title:      "A new",
					Text:       "new text",
					StartRune:  0,
					EndRune:    8,
					Embedding:  []float32{1, 0},
				},
			},
			query:           []float32{1, 0},
			wantIDs:         []string{"doc-a:0"},
			wantFirstTitle:  "A new",
			wantCanceledErr: context.Canceled,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store, err := NewChromemStore(Config{})
			if err != nil {
				t.Fatalf("NewChromemStore() error = %v", err)
			}

			if err := store.Upsert(context.Background(), tc.existing); err != nil {
				t.Fatalf("Upsert(existing) error = %v", err)
			}

			ctx, cancel := context.WithCancel(context.Background())
			store.afterAddHook = cancel

			err = store.Upsert(ctx, tc.reingest)
			if err == nil {
				t.Fatalf("Upsert(post-add-cancel) error = nil, want non-nil")
			}
			if !errors.Is(err, tc.wantCanceledErr) {
				t.Fatalf("Upsert(post-add-cancel) error = %v, want errors.Is(..., %v)", err, tc.wantCanceledErr)
			}

			got, err := store.Search(context.Background(), tc.query, 5, -1)
			if err != nil {
				t.Fatalf("Search() error = %v", err)
			}
			gotIDs := make([]string, 0, len(got))
			for _, hit := range got {
				gotIDs = append(gotIDs, hit.Chunk.ChunkID)
			}
			if !slices.Equal(gotIDs, tc.wantIDs) {
				t.Fatalf("Search() ids = %v, want %v", gotIDs, tc.wantIDs)
			}
			if len(got) != 1 {
				t.Fatalf("Search() len = %d, want 1", len(got))
			}
			if got[0].Chunk.Title != tc.wantFirstTitle {
				t.Fatalf("Search() title = %q, want %q", got[0].Chunk.Title, tc.wantFirstTitle)
			}
		})
	}
}

func TestChromemStoreUpsertSerializesConcurrentCalls(t *testing.T) {
	t.Parallel()

	store, err := NewChromemStore(Config{})
	if err != nil {
		t.Fatalf("NewChromemStore() error = %v", err)
	}
	t.Cleanup(func() {
		if cerr := store.Close(); cerr != nil {
			t.Fatalf("Close() error = %v", cerr)
		}
	})

	firstEntered := make(chan struct{}, 1)
	releaseFirst := make(chan struct{})
	var hookCount atomic.Int32
	store.afterAddHook = func() {
		if hookCount.Add(1) != 1 {
			return
		}
		select {
		case firstEntered <- struct{}{}:
		default:
		}
		<-releaseFirst
	}

	firstChunks := []ChunkRecord{
		{
			ChunkID:    "doc-a:0",
			ParentID:   "doc-a",
			SourcePath: "/kb/a.md",
			Title:      "A",
			Text:       "alpha",
			StartRune:  0,
			EndRune:    5,
			Embedding:  []float32{1, 0},
		},
	}
	secondChunks := []ChunkRecord{
		{
			ChunkID:    "doc-b:0",
			ParentID:   "doc-b",
			SourcePath: "/kb/b.md",
			Title:      "B",
			Text:       "beta",
			StartRune:  0,
			EndRune:    4,
			Embedding:  []float32{0, 1},
		},
	}

	firstDone := make(chan error, 1)
	go func() {
		firstDone <- store.Upsert(context.Background(), firstChunks)
	}()

	select {
	case <-firstEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("first Upsert did not reach afterAddHook")
	}

	secondDone := make(chan error, 1)
	go func() {
		secondDone <- store.Upsert(context.Background(), secondChunks)
	}()

	select {
	case err := <-secondDone:
		t.Fatalf("second Upsert finished before first released: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	close(releaseFirst)

	select {
	case err := <-firstDone:
		if err != nil {
			t.Fatalf("first Upsert() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first Upsert did not finish after release")
	}

	select {
	case err := <-secondDone:
		if err != nil {
			t.Fatalf("second Upsert() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second Upsert did not finish after first completed")
	}
}

func TestChromemStoreUpsertReplaceSameIDSemantics(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		firstIngest   []ChunkRecord
		secondIngest  []ChunkRecord
		query         []float32
		threshold     float32
		wantChunkID   string
		wantTitle     string
		wantStartRune int
		wantEndRune   int
	}{
		{
			name: "same chunk id is replaced by second ingest",
			firstIngest: []ChunkRecord{
				{
					ChunkID:    "doc-a:0",
					ParentID:   "doc-a",
					SourcePath: "/kb/a.md",
					Title:      "Old",
					Text:       "old",
					StartRune:  0,
					EndRune:    3,
					Metadata:   map[string]string{"v": "1"},
					Embedding:  []float32{1, 0},
				},
			},
			secondIngest: []ChunkRecord{
				{
					ChunkID:    "doc-a:0",
					ParentID:   "doc-a",
					SourcePath: "/kb/a.md",
					Title:      "New",
					Text:       "new-content",
					StartRune:  2,
					EndRune:    12,
					Metadata:   map[string]string{"v": "2"},
					Embedding:  []float32{1, 0},
				},
			},
			query:         []float32{1, 0},
			threshold:     -1,
			wantChunkID:   "doc-a:0",
			wantTitle:     "New",
			wantStartRune: 2,
			wantEndRune:   12,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store, err := NewChromemStore(Config{})
			if err != nil {
				t.Fatalf("NewChromemStore() error = %v", err)
			}

			if err := store.Upsert(context.Background(), tc.firstIngest); err != nil {
				t.Fatalf("Upsert(first) error = %v", err)
			}
			if err := store.Upsert(context.Background(), tc.secondIngest); err != nil {
				t.Fatalf("Upsert(second) error = %v", err)
			}

			got, err := store.Search(context.Background(), tc.query, 1, tc.threshold)
			if err != nil {
				t.Fatalf("Search() error = %v", err)
			}
			if len(got) != 1 {
				t.Fatalf("Search() len = %d, want 1", len(got))
			}
			if got[0].Chunk.ChunkID != tc.wantChunkID {
				t.Fatalf("Search() chunk id = %q, want %q", got[0].Chunk.ChunkID, tc.wantChunkID)
			}
			if got[0].Chunk.Title != tc.wantTitle {
				t.Fatalf("Search() title = %q, want %q", got[0].Chunk.Title, tc.wantTitle)
			}
			if got[0].Chunk.StartRune != tc.wantStartRune || got[0].Chunk.EndRune != tc.wantEndRune {
				t.Fatalf("Search() rune range = [%d,%d], want [%d,%d]", got[0].Chunk.StartRune, got[0].Chunk.EndRune, tc.wantStartRune, tc.wantEndRune)
			}
		})
	}
}

func TestChromemStorePersistentReopen(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		prepare  []ChunkRecord
		query    []float32
		topK     int
		wantID   string
		wantMeta string
		wantFrom int
		wantTo   int
	}{
		{
			name: "reopen persistent db returns stored chunk with metadata",
			prepare: []ChunkRecord{
				{
					ChunkID:    "doc-p:0",
					ParentID:   "doc-p",
					SourcePath: "/kb/p.md",
					Title:      "Persisted",
					Text:       "persisted text",
					StartRune:  1,
					EndRune:    14,
					Metadata:   map[string]string{"topic": "persist"},
					Embedding:  []float32{1, 0},
				},
			},
			query:    []float32{1, 0},
			topK:     1,
			wantID:   "doc-p:0",
			wantMeta: "persist",
			wantFrom: 1,
			wantTo:   14,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dataDir := t.TempDir()
			cfg := Config{
				DataDir:    dataDir,
				Collection: "persist-test",
			}

			storeA, err := NewChromemStore(cfg)
			if err != nil {
				t.Fatalf("NewChromemStore(storeA) error = %v", err)
			}
			if err := storeA.Upsert(context.Background(), tc.prepare); err != nil {
				t.Fatalf("Upsert() error = %v", err)
			}
			if err := storeA.Close(); err != nil {
				t.Fatalf("Close(storeA) error = %v", err)
			}

			storeB, err := NewChromemStore(cfg)
			if err != nil {
				t.Fatalf("NewChromemStore(storeB) error = %v", err)
			}
			t.Cleanup(func() {
				if cerr := storeB.Close(); cerr != nil {
					t.Fatalf("Close(storeB) error = %v", cerr)
				}
			})

			got, err := storeB.Search(context.Background(), tc.query, tc.topK, -1)
			if err != nil {
				t.Fatalf("Search() error = %v", err)
			}
			if len(got) != 1 {
				t.Fatalf("Search() len = %d, want 1", len(got))
			}
			if got[0].Chunk.ChunkID != tc.wantID {
				t.Fatalf("Search() chunk id = %q, want %q", got[0].Chunk.ChunkID, tc.wantID)
			}
			if got[0].Chunk.Metadata["topic"] != tc.wantMeta {
				t.Fatalf("Search() metadata topic = %q, want %q", got[0].Chunk.Metadata["topic"], tc.wantMeta)
			}
			if got[0].Chunk.StartRune != tc.wantFrom || got[0].Chunk.EndRune != tc.wantTo {
				t.Fatalf("Search() rune range = [%d,%d], want [%d,%d]", got[0].Chunk.StartRune, got[0].Chunk.EndRune, tc.wantFrom, tc.wantTo)
			}
		})
	}
}

func TestChromemStoreParseIntMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		metadata map[string]string
		key      string
		want     int
	}{
		{
			name:     "valid integer",
			metadata: map[string]string{"v": "123"},
			key:      "v",
			want:     123,
		},
		{
			name:     "missing key",
			metadata: map[string]string{"v": "1"},
			key:      "missing",
			want:     0,
		},
		{
			name:     "invalid integer",
			metadata: map[string]string{"v": "bad"},
			key:      "v",
			want:     0,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := parseIntMetadata(tc.metadata, tc.key)
			if got != tc.want {
				t.Fatalf("parseIntMetadata() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestChromemStoreErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		run     func(store *ChromemStore) error
		wantErr bool
	}{
		{
			name: "upsert rejects empty embedding",
			run: func(store *ChromemStore) error {
				return store.Upsert(context.Background(), []ChunkRecord{
					{
						ChunkID:  "doc:0",
						ParentID: "doc",
					},
				})
			},
			wantErr: true,
		},
		{
			name: "search rejects empty query embedding",
			run: func(store *ChromemStore) error {
				_, err := store.Search(context.Background(), nil, 1, 0)
				return err
			},
			wantErr: true,
		},
		{
			name: "search rejects non-positive topK",
			run: func(store *ChromemStore) error {
				_, err := store.Search(context.Background(), []float32{1, 0}, 0, 0)
				return err
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store, err := NewChromemStore(Config{})
			if err != nil {
				t.Fatalf("NewChromemStore() error = %v", err)
			}
			err = tc.run(store)
			if tc.wantErr && err == nil {
				t.Fatalf("got nil error, want non-nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("got error %v, want nil", err)
			}
		})
	}
}
