package storage

import (
	"context"
	"slices"
	"testing"
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
