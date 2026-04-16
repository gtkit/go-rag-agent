package rag

import (
	"fmt"
	"slices"
	"testing"
)

func TestChunkerSplit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		size      int
		overlap   int
		doc       Document
		wantErr   bool
		wantTexts []string
		wantRange [][2]int
	}{
		{
			name:    "single chunk when content shorter than size",
			size:    10,
			overlap: 2,
			doc: Document{
				ID:      "d1",
				Content: "hello",
			},
			wantTexts: []string{"hello"},
			wantRange: [][2]int{{0, 5}},
		},
		{
			name:    "multi chunk with overlap by rune count",
			size:    4,
			overlap: 1,
			doc: Document{
				ID:      "d2",
				Content: "abcdefghi",
			},
			wantTexts: []string{"abcd", "defg", "ghi"},
			wantRange: [][2]int{{0, 4}, {3, 7}, {6, 9}},
		},
		{
			name:    "unicode content uses rune offsets not bytes",
			size:    3,
			overlap: 1,
			doc: Document{
				ID:      "unicode-doc-id",
				Content: "你好世界再见",
			},
			wantTexts: []string{"你好世", "世界再", "再见"},
			wantRange: [][2]int{{0, 3}, {2, 5}, {4, 6}},
		},
		{
			name:    "empty content yields no chunks",
			size:    4,
			overlap: 1,
			doc: Document{
				ID:      "d-empty",
				Content: "",
			},
			wantTexts: []string{},
			wantRange: [][2]int{},
		},
		{
			name:    "invalid settings are rejected",
			size:    4,
			overlap: 4,
			doc: Document{
				ID:      "d3",
				Content: "abcdef",
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			chunker, err := NewChunker(tc.size, tc.overlap)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("NewChunker() error = nil, want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("NewChunker() unexpected error: %v", err)
			}

			chunks := chunker.Split(tc.doc)
			gotTexts := make([]string, 0, len(chunks))
			gotRanges := make([][2]int, 0, len(chunks))
			for _, chunk := range chunks {
				gotTexts = append(gotTexts, chunk.Text)
				gotRanges = append(gotRanges, [2]int{chunk.StartRune, chunk.EndRune})
			}
			if !slices.Equal(gotTexts, tc.wantTexts) {
				t.Fatalf("Split() texts = %v, want %v", gotTexts, tc.wantTexts)
			}
			if !slices.Equal(gotRanges, tc.wantRange) {
				t.Fatalf("Split() ranges = %v, want %v", gotRanges, tc.wantRange)
			}
			if len(chunks) > 0 {
				for i, chunk := range chunks {
					wantChunkID := fmt.Sprintf("%s:%d", tc.doc.ID, i)
					if chunk.ChunkID != wantChunkID {
						t.Fatalf("Split() chunk[%d].ChunkID = %q, want %q", i, chunk.ChunkID, wantChunkID)
					}
				}
			}
		})
	}
}
