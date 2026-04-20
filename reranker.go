package ragagent

import (
	"context"

	"github.com/gtkit/go-rag-agent/internal/retrieval"
)

// RerankOptions 定义候选重排时的控制参数。
type RerankOptions struct {
	ShortlistSize int
	TopK          int
}

// Reranker 定义候选重排能力。
type Reranker interface {
	Rerank(ctx context.Context, query string, candidates []SearchHit, opts RerankOptions) ([]SearchHit, error)
}

type ruleBasedReranker struct{}

// NewRuleBasedReranker 创建默认规则重排器。
func NewRuleBasedReranker() Reranker {
	return ruleBasedReranker{}
}

func (ruleBasedReranker) Rerank(_ context.Context, query string, candidates []SearchHit, opts RerankOptions) ([]SearchHit, error) {
	reranked := retrieval.RerankShortlistWithLimit(query, toInternalSearchHits(candidates), opts.ShortlistSize, opts.TopK)
	return fromInternalSearchHits(reranked), nil
}
