package retrieval

import (
	"slices"
	"testing"

	"github.com/gtkit/go-rag-agent/internal/storage"
)

func TestLexicalSearch(t *testing.T) {
	t.Parallel()

	candidates := []storage.SearchHit{
		{
			Chunk: storage.ChunkRecord{
				ChunkID: "a",
				Title:   "Architecture",
				Text:    "semantic overview only",
			},
			Score: 0.95,
		},
		{
			Chunk: storage.ChunkRecord{
				ChunkID: "b",
				Title:   "Gateway API",
				Text:    "gateway api exact match terms appear here",
			},
			Score: 0.70,
		},
		{
			Chunk: storage.ChunkRecord{
				ChunkID: "c",
				Title:   "Other",
				Text:    "gateway only",
			},
			Score: 0.60,
		},
	}

	got := LexicalSearch("gateway api", candidates, 2)
	gotIDs := make([]string, 0, len(got))
	for _, hit := range got {
		gotIDs = append(gotIDs, hit.Chunk.ChunkID)
	}
	wantIDs := []string{"b", "c"}
	if !slices.Equal(gotIDs, wantIDs) {
		t.Fatalf("LexicalSearch() ids = %v, want %v", gotIDs, wantIDs)
	}
}

func TestFuseRRF(t *testing.T) {
	t.Parallel()

	vectorHits := []storage.SearchHit{
		{Chunk: storage.ChunkRecord{ChunkID: "a", Text: "semantic"}, Score: 0.99},
		{Chunk: storage.ChunkRecord{ChunkID: "b", Text: "gateway api"}, Score: 0.80},
	}
	lexicalHits := []storage.SearchHit{
		{Chunk: storage.ChunkRecord{ChunkID: "b", Text: "gateway api"}, Score: 1},
		{Chunk: storage.ChunkRecord{ChunkID: "c", Text: "gateway"}, Score: 0.8},
	}

	got := FuseRRF(vectorHits, lexicalHits, 2)
	gotIDs := make([]string, 0, len(got))
	for _, hit := range got {
		gotIDs = append(gotIDs, hit.Chunk.ChunkID)
	}
	wantIDs := []string{"b", "a"}
	if !slices.Equal(gotIDs, wantIDs) {
		t.Fatalf("FuseRRF() ids = %v, want %v", gotIDs, wantIDs)
	}
}

func TestRerankShortlist(t *testing.T) {
	t.Parallel()

	candidates := []storage.SearchHit{
		{Chunk: storage.ChunkRecord{ChunkID: "a", Title: "Overview", Text: "semantic only"}, Score: 0.99},
		{Chunk: storage.ChunkRecord{ChunkID: "b", Title: "Gateway API", Text: "gateway api exact match"}, Score: 0.80},
	}

	got := RerankShortlist("gateway api", candidates, 1)
	if len(got) != 1 {
		t.Fatalf("RerankShortlist() len = %d, want 1", len(got))
	}
	if got[0].Chunk.ChunkID != "b" {
		t.Fatalf("RerankShortlist() top id = %q, want %q", got[0].Chunk.ChunkID, "b")
	}
}

func TestOptionsNormalizeAndLimits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		opts               Options
		topK               int
		candidateCount     int
		wantCandidateLimit int
		wantShortlist      int
	}{
		{
			name:               "defaults apply when zero values provided",
			opts:               Options{},
			topK:               5,
			candidateCount:     20,
			wantCandidateLimit: 20,
			wantShortlist:      10,
		},
		{
			name:               "custom multipliers are honored",
			opts:               Options{CandidateMultiplier: 3, RRFK: 10, RerankMultiplier: 4},
			topK:               4,
			candidateCount:     20,
			wantCandidateLimit: 12,
			wantShortlist:      16,
		},
		{
			name:               "shortlist is capped by candidate count",
			opts:               Options{CandidateMultiplier: 2, RerankMultiplier: 4},
			topK:               4,
			candidateCount:     6,
			wantCandidateLimit: 8,
			wantShortlist:      6,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := tc.opts.Normalize()
			if got.CandidateMultiplier <= 0 {
				t.Fatalf("normalize() CandidateMultiplier = %d, want positive", got.CandidateMultiplier)
			}
			if got.RRFK <= 0 {
				t.Fatalf("normalize() RRFK = %v, want positive", got.RRFK)
			}
			if got.RerankMultiplier <= 0 {
				t.Fatalf("normalize() RerankMultiplier = %d, want positive", got.RerankMultiplier)
			}
			if got.CandidateLimit(tc.topK) != tc.wantCandidateLimit {
				t.Fatalf("CandidateLimit() = %d, want %d", got.CandidateLimit(tc.topK), tc.wantCandidateLimit)
			}
			if got.RerankShortlistSize(tc.topK, tc.candidateCount) != tc.wantShortlist {
				t.Fatalf("RerankShortlistSize() = %d, want %d", got.RerankShortlistSize(tc.topK, tc.candidateCount), tc.wantShortlist)
			}
		})
	}
}
