package retrieval

import "cmp"

// Options 定义 hybrid retrieval 与 rerank 的内部调优参数。
type Options struct {
	CandidateMultiplier int
	RRFK                float64
	RerankMultiplier    int
}

func (o Options) Normalize() Options {
	o.CandidateMultiplier = cmp.Or(o.CandidateMultiplier, 4)
	if o.RRFK == 0 {
		o.RRFK = 60
	}
	o.RerankMultiplier = cmp.Or(o.RerankMultiplier, 2)
	return o
}

func (o Options) CandidateLimit(topK int) int {
	o = o.Normalize()
	return max(topK, topK*o.CandidateMultiplier)
}

func (o Options) RerankShortlistSize(topK int, candidateCount int) int {
	o = o.Normalize()
	shortlistSize := max(topK, topK*o.RerankMultiplier)
	if shortlistSize > candidateCount {
		shortlistSize = candidateCount
	}
	return shortlistSize
}
