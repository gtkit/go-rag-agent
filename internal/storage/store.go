package storage

import "context"

// ChunkRecord 表示存储层里的一个已索引分块。
type ChunkRecord struct {
	ChunkID    string
	ParentID   string
	SourcePath string
	Title      string
	Text       string
	StartRune  int
	EndRune    int
	Metadata   map[string]string
	Embedding  []float32
}

// SearchHit 表示一次向量检索结果。
type SearchHit struct {
	Chunk ChunkRecord
	Score float32
}

// VectorStore 定义 RAG 管线所需的最小向量存储契约。
type VectorStore interface {
	Upsert(ctx context.Context, chunks []ChunkRecord) error
	Search(ctx context.Context, queryEmbedding []float32, topK int, threshold float32) ([]SearchHit, error)
	Close() error
}

// Config 定义向量存储后端配置。
type Config struct {
	DataDir    string
	Collection string
}
