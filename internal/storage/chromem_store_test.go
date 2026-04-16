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
					Metadata:   map[string]string{"topic": "alpha"},
					Embedding:  []float32{1, 0},
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
