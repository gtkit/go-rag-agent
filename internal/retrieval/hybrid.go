package retrieval

import (
	"cmp"
	"slices"
	"strings"
	"unicode"

	"github.com/gtkit/go-rag-agent/internal/storage"
)

// LexicalSearch 根据 query token 与候选文本的命中情况返回 lexical 排名。
func LexicalSearch(query string, candidates []storage.SearchHit, topK int) []storage.SearchHit {
	if topK <= 0 || len(candidates) == 0 {
		return nil
	}

	queryTokens := tokenizeQuery(query)
	if len(queryTokens) == 0 {
		return nil
	}

	type scored struct {
		hit   storage.SearchHit
		score float32
	}
	scoredHits := make([]scored, 0, len(candidates))
	for _, candidate := range candidates {
		score := lexicalScore(queryTokens, candidate.Chunk.Title+" "+candidate.Chunk.Text)
		if score <= 0 {
			continue
		}
		scoredHits = append(scoredHits, scored{
			hit:   candidate,
			score: score,
		})
	}

	slices.SortFunc(scoredHits, func(a, b scored) int {
		if diff := cmp.Compare(b.score, a.score); diff != 0 {
			return diff
		}
		if diff := cmp.Compare(b.hit.Score, a.hit.Score); diff != 0 {
			return diff
		}
		return strings.Compare(a.hit.Chunk.ChunkID, b.hit.Chunk.ChunkID)
	})

	if len(scoredHits) > topK {
		scoredHits = scoredHits[:topK]
	}
	out := make([]storage.SearchHit, 0, len(scoredHits))
	for _, item := range scoredHits {
		hit := item.hit
		hit.Score = item.score
		out = append(out, hit)
	}
	return out
}

// FuseRRF 融合向量召回和 lexical 召回结果。
func FuseRRF(vectorHits []storage.SearchHit, lexicalHits []storage.SearchHit, topK int) []storage.SearchHit {
	return FuseRRFWithK(vectorHits, lexicalHits, topK, Options{}.Normalize().RRFK)
}

// FuseRRFWithK 使用指定 RRF 常量融合向量召回和 lexical 召回结果。
func FuseRRFWithK(vectorHits []storage.SearchHit, lexicalHits []storage.SearchHit, topK int, rrfK float64) []storage.SearchHit {
	if topK <= 0 {
		return nil
	}

	type fused struct {
		hit      storage.SearchHit
		score    float32
		tieBreak float32
	}
	scores := make(map[string]fused, len(vectorHits)+len(lexicalHits))
	add := func(rank int, hit storage.SearchHit, vector bool) {
		item := scores[hit.Chunk.ChunkID]
		if item.hit.Chunk.ChunkID == "" {
			item.hit = hit
			if !vector {
				item.hit.Score = 0
			}
		}
		item.score += float32(1.0 / (rrfK + float64(rank+1)))
		if vector {
			item.tieBreak = hit.Score
		}
		scores[hit.Chunk.ChunkID] = item
	}

	for i, hit := range vectorHits {
		add(i, hit, true)
	}
	for i, hit := range lexicalHits {
		add(i, hit, false)
	}

	fusedHits := make([]fused, 0, len(scores))
	for _, item := range scores {
		fusedHits = append(fusedHits, item)
	}
	slices.SortFunc(fusedHits, func(a, b fused) int {
		if diff := cmp.Compare(b.score, a.score); diff != 0 {
			return diff
		}
		if diff := cmp.Compare(b.tieBreak, a.tieBreak); diff != 0 {
			return diff
		}
		return strings.Compare(a.hit.Chunk.ChunkID, b.hit.Chunk.ChunkID)
	})

	if len(fusedHits) > topK {
		fusedHits = fusedHits[:topK]
	}
	out := make([]storage.SearchHit, 0, len(fusedHits))
	for _, item := range fusedHits {
		hit := item.hit
		hit.Score = item.score
		out = append(out, hit)
	}
	return out
}

// RerankShortlist 对 shortlist 做本地规则重排。
func RerankShortlist(query string, candidates []storage.SearchHit, topK int) []storage.SearchHit {
	return RerankShortlistWithLimit(query, candidates, len(candidates), topK)
}

// RerankShortlistWithLimit 对 shortlist 做本地规则重排，并只使用前 shortlistSize 个候选。
func RerankShortlistWithLimit(query string, candidates []storage.SearchHit, shortlistSize int, topK int) []storage.SearchHit {
	if topK <= 0 || len(candidates) == 0 {
		return nil
	}
	if shortlistSize <= 0 {
		return nil
	}
	if shortlistSize < len(candidates) {
		candidates = slices.Clone(candidates[:shortlistSize])
	} else {
		candidates = slices.Clone(candidates)
	}

	queryTokens := tokenizeQuery(query)
	type scored struct {
		hit   storage.SearchHit
		score float32
	}
	scoredHits := make([]scored, 0, len(candidates))
	for _, candidate := range candidates {
		score := candidate.Score
		if len(queryTokens) > 0 {
			score += lexicalScore(queryTokens, candidate.Chunk.Title+" "+candidate.Chunk.Text)
		}
		scoredHits = append(scoredHits, scored{
			hit:   candidate,
			score: score,
		})
	}

	slices.SortFunc(scoredHits, func(a, b scored) int {
		if diff := cmp.Compare(b.score, a.score); diff != 0 {
			return diff
		}
		return strings.Compare(a.hit.Chunk.ChunkID, b.hit.Chunk.ChunkID)
	})
	if len(scoredHits) > topK {
		scoredHits = scoredHits[:topK]
	}

	out := make([]storage.SearchHit, 0, len(scoredHits))
	for _, item := range scoredHits {
		hit := item.hit
		hit.Score = item.score
		out = append(out, hit)
	}
	return out
}

func tokenizeQuery(input string) []string {
	input = strings.ToLower(strings.TrimSpace(input))
	if input == "" {
		return nil
	}
	parts := strings.FieldsFunc(input, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	parts = slices.DeleteFunc(parts, func(token string) bool {
		return strings.TrimSpace(token) == ""
	})
	if len(parts) == 0 {
		return nil
	}
	slices.Sort(parts)
	return slices.Compact(parts)
}

func lexicalScore(tokens []string, text string) float32 {
	if len(tokens) == 0 {
		return 0
	}
	normalizedText := strings.ToLower(text)
	var score float32
	for _, token := range tokens {
		if strings.Contains(normalizedText, token) {
			score += 1
		}
	}
	return score
}
