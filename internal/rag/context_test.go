package rag

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestAssembleContext(t *testing.T) {
	t.Parallel()

	const testEvidenceBudget = 4000

	tests := []struct {
		name          string
		chunks        []Chunk
		maxChars      int
		wantErr       bool
		wantChunkIDs  []string
		wantContext   string
		wantContextLE int
		wantErrIs     error
	}{
		{
			name: "deduplicate by chunk id and preserve first occurrence",
			chunks: []Chunk{
				{ChunkID: "c1", Text: "alpha"},
				{ChunkID: "c2", Text: "beta"},
				{ChunkID: "c1", Text: "alpha-duplicate"},
			},
			maxChars:      64,
			wantChunkIDs:  []string{"c1", "c2"},
			wantContext:   "[c1] alpha\n\n[c2] beta",
			wantContextLE: 64,
		},
		{
			name: "bounded context stops after first chunk if adding next exceeds limit",
			chunks: []Chunk{
				{ChunkID: "c1", Text: "abcd"},
				{ChunkID: "c2", Text: "efgh"},
			},
			maxChars:      11,
			wantChunkIDs:  []string{"c1"},
			wantContext:   "[c1] abcd",
			wantContextLE: 11,
		},
		{
			name: "error for invalid max chars",
			chunks: []Chunk{
				{ChunkID: "c1", Text: "alpha"},
			},
			maxChars:  0,
			wantErr:   true,
			wantErrIs: ErrInvalidContextLimit,
		},
		{
			name: "error when no chunk can fit",
			chunks: []Chunk{
				{ChunkID: "c1", Text: "long-content"},
			},
			maxChars:  3,
			wantErr:   true,
			wantErrIs: ErrNoContextAssembled,
		},
		{
			name:      "error when input chunk list is empty",
			chunks:    nil,
			maxChars:  16,
			wantErr:   true,
			wantErrIs: ErrNoContextAssembled,
		},
		{
			name: "max chars uses runes not bytes",
			chunks: []Chunk{
				{ChunkID: "c1", Text: "你好"},
				{ChunkID: "c2", Text: "世界"},
			},
			maxChars:      8,
			wantChunkIDs:  []string{"c1"},
			wantContext:   "[c1] 你好",
			wantContextLE: 8,
		},
		{
			name: "first chunk is truncated to fit evidence budget",
			chunks: []Chunk{
				{ChunkID: "c1", Text: strings.Repeat("a", testEvidenceBudget)},
			},
			maxChars:      testEvidenceBudget,
			wantChunkIDs:  []string{"c1"},
			wantContextLE: testEvidenceBudget,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			gotChunks, gotContext, err := AssembleContext(tc.chunks, tc.maxChars)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("AssembleContext() error = nil, want non-nil")
				}
				if tc.wantErrIs != nil && !errors.Is(err, tc.wantErrIs) {
					t.Fatalf("AssembleContext() error = %v, want errors.Is(..., %v)", err, tc.wantErrIs)
				}
				return
			}
			if err != nil {
				t.Fatalf("AssembleContext() unexpected error: %v", err)
			}

			gotIDs := make([]string, 0, len(gotChunks))
			for _, chunk := range gotChunks {
				gotIDs = append(gotIDs, chunk.ChunkID)
			}
			if !slices.Equal(gotIDs, tc.wantChunkIDs) {
				t.Fatalf("AssembleContext() chunk ids = %v, want %v", gotIDs, tc.wantChunkIDs)
			}
			if tc.wantContext != "" && gotContext != tc.wantContext {
				t.Fatalf("AssembleContext() context = %q, want %q", gotContext, tc.wantContext)
			}
			if got := len([]rune(gotContext)); got > tc.wantContextLE {
				t.Fatalf("AssembleContext() context rune len = %d, want <= %d", got, tc.wantContextLE)
			}
		})
	}
}
