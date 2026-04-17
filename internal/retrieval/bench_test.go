package retrieval

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gtkit/go-rag-agent/internal/storage"
)

func BenchmarkRetrievalModes(b *testing.B) {
	candidates := benchmarkCandidates(256)
	query := "gateway api"
	opts := Options{
		CandidateMultiplier: 4,
		RRFK:                60,
		RerankMultiplier:    2,
	}

	b.Run("vector_only", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			hits := candidates[:8]
			if len(hits) != 8 {
				b.Fatalf("vector hits len = %d, want 8", len(hits))
			}
		}
	})

	b.Run("hybrid", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			lexicalHits := LexicalSearch(query, candidates, opts.CandidateLimit(8))
			hits := FuseRRFWithK(candidates[:opts.CandidateLimit(8)], lexicalHits, 8, opts.RRFK)
			if len(hits) == 0 {
				b.Fatal("expected hybrid hits")
			}
		}
	})

	b.Run("hybrid_rerank", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			candidateLimit := opts.CandidateLimit(8)
			lexicalHits := LexicalSearch(query, candidates, candidateLimit)
			fusedHits := FuseRRFWithK(candidates[:candidateLimit], lexicalHits, candidateLimit, opts.RRFK)
			hits := RerankShortlistWithLimit(query, fusedHits, opts.RerankShortlistSize(8, len(fusedHits)), 8)
			if len(hits) == 0 {
				b.Fatal("expected reranked hits")
			}
		}
	})
}

func benchmarkCandidates(count int) []storage.SearchHit {
	candidates := make([]storage.SearchHit, 0, count)
	for i := 0; i < count; i++ {
		text := fmt.Sprintf("semantic candidate %d", i)
		title := fmt.Sprintf("Candidate %d", i)
		score := float32(count-i) / float32(count)
		if i%16 == 0 {
			title = fmt.Sprintf("Gateway API %d", i)
			text = "gateway api exact match terms appear here " + strings.Repeat("token ", 4)
		}
		candidates = append(candidates, storage.SearchHit{
			Chunk: storage.ChunkRecord{
				ChunkID: fmt.Sprintf("chunk-%03d", i),
				Title:   title,
				Text:    text,
			},
			Score: score,
		})
	}
	return candidates
}
