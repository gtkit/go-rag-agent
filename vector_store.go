package ragagent

import (
	"context"

	"github.com/gtkit/go-rag-agent/internal/storage"
)

// VectorStore 定义向量存储所需的最小契约。
type VectorStore interface {
	Upsert(ctx context.Context, chunks []ChunkRecord) error
	SearchWithFilter(ctx context.Context, queryEmbedding []float32, topK int, threshold float32, filter SearchFilter) ([]SearchHit, error)
	DeleteBySourcePaths(ctx context.Context, sourcePaths []string) error
	Search(ctx context.Context, queryEmbedding []float32, topK int, threshold float32) ([]SearchHit, error)
	Close() error
}

// ChromemVectorStoreConfig 定义默认 chromem adapter 的配置。
type ChromemVectorStoreConfig struct {
	DataDir    string
	Collection string
}

type chromemVectorStore struct {
	inner storage.VectorStore
}

// NewChromemVectorStore 创建默认 chromem 向量存储 adapter。
func NewChromemVectorStore(cfg ChromemVectorStoreConfig) (VectorStore, error) {
	inner, err := storage.NewChromemStore(storage.Config{
		DataDir:    cfg.DataDir,
		Collection: cfg.Collection,
	})
	if err != nil {
		return nil, err
	}
	return &chromemVectorStore{inner: inner}, nil
}

func (s *chromemVectorStore) Upsert(ctx context.Context, chunks []ChunkRecord) error {
	return s.inner.Upsert(ctx, toInternalChunkRecords(chunks))
}

func (s *chromemVectorStore) SearchWithFilter(ctx context.Context, queryEmbedding []float32, topK int, threshold float32, filter SearchFilter) ([]SearchHit, error) {
	hits, err := s.inner.SearchWithFilter(ctx, queryEmbedding, topK, threshold, toInternalSearchFilter(filter))
	if err != nil {
		return nil, err
	}
	return fromInternalSearchHits(hits), nil
}

func (s *chromemVectorStore) DeleteBySourcePaths(ctx context.Context, sourcePaths []string) error {
	return s.inner.DeleteBySourcePaths(ctx, sourcePaths)
}

func (s *chromemVectorStore) Search(ctx context.Context, queryEmbedding []float32, topK int, threshold float32) ([]SearchHit, error) {
	hits, err := s.inner.Search(ctx, queryEmbedding, topK, threshold)
	if err != nil {
		return nil, err
	}
	return fromInternalSearchHits(hits), nil
}

func (s *chromemVectorStore) Close() error {
	return s.inner.Close()
}
