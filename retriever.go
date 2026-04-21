package ragagent

import (
	"context"

	"github.com/gtkit/go-rag-agent/internal/retrieval"
	"github.com/gtkit/go-rag-agent/internal/storage"
)

// RetrieverRequest 描述一次检索请求。
type RetrieverRequest struct {
	Query  string
	Filter RetrievalFilter
}

// Retriever 定义根包公开的检索编排抽象。
type Retriever interface {
	Search(ctx context.Context, req RetrieverRequest) ([]SearchHit, error)
	SearchDetailed(ctx context.Context, req RetrieverRequest) ([]SearchHit, RetrievalMetrics, []FallbackEvent, error)
}

// RetrieverConfig 定义默认 Retriever 的构造参数。
type RetrieverConfig struct {
	TopK                    int
	SimilarityThreshold     float32
	EnableHybridSearch      bool
	EnableRerank            bool
	HybridCandidateMultiply int
	HybridRRFK              float64
	RerankShortlistMultiple int
}

// RetrievalComponents 定义 Agent 可选注入的检索组件。
type RetrievalComponents struct {
	Retriever Retriever
}

type defaultRetriever struct {
	inner *rootRetriever
}

// NewRetriever 创建根包默认 Retriever，实现当前 hybrid / rerank / filter 语义。
func NewRetriever(store VectorStore, embedder Embedder, reranker Reranker, cfg RetrieverConfig) Retriever {
	if store == nil || embedder == nil {
		return nil
	}
	return &defaultRetriever{
		inner: newRootRetriever(
			rootToInternalVectorStore{inner: store},
			embedder,
			reranker,
			cfg.TopK,
			cfg.SimilarityThreshold,
			cfg.EnableHybridSearch,
			cfg.EnableRerank,
			retrieval.Options{
				CandidateMultiplier: cfg.HybridCandidateMultiply,
				RRFK:                cfg.HybridRRFK,
				RerankMultiplier:    cfg.RerankShortlistMultiple,
			},
		),
	}
}

func (r *defaultRetriever) Search(ctx context.Context, req RetrieverRequest) ([]SearchHit, error) {
	hits, err := r.inner.Search(ctx, req)
	if err != nil {
		return nil, err
	}
	return fromInternalSearchHits(hits), nil
}

func (r *defaultRetriever) SearchDetailed(ctx context.Context, req RetrieverRequest) ([]SearchHit, RetrievalMetrics, []FallbackEvent, error) {
	hits, metrics, fallbacks, err := r.inner.SearchDetailed(ctx, req)
	if err != nil {
		return nil, metrics, fallbacks, err
	}
	return fromInternalSearchHits(hits), metrics, fallbacks, nil
}

type retrieverToolAdapter struct {
	retriever Retriever
}

func (a retrieverToolAdapter) Search(ctx context.Context, query string) ([]storage.SearchHit, error) {
	hits, err := a.retriever.Search(ctx, RetrieverRequest{Query: query})
	if err != nil {
		return nil, err
	}
	return toInternalSearchHits(hits), nil
}

func toInternalRetrievalFilter(filter RetrievalFilter) storage.SearchFilter {
	return storage.SearchFilter{
		SourcePaths:    append([]string(nil), filter.SourcePaths...),
		SourcePrefixes: append([]string(nil), filter.SourcePrefixes...),
		Metadata:       cloneStringMap(filter.Metadata),
	}
}

func fromInternalRetrievalFilter(filter storage.SearchFilter) RetrievalFilter {
	return RetrievalFilter{
		SourcePaths:    append([]string(nil), filter.SourcePaths...),
		SourcePrefixes: append([]string(nil), filter.SourcePrefixes...),
		Metadata:       cloneStringMap(filter.Metadata),
	}
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

var _ Retriever = (*defaultRetriever)(nil)
