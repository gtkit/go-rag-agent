package storage

import "context"

// ChunkRecord is the storage-layer shape for one indexed chunk.
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

// SearchHit is one vector search result.
type SearchHit struct {
	Chunk ChunkRecord
	Score float32
}

// VectorStore defines the minimal vector storage contract for the RAG pipeline.
type VectorStore interface {
	Upsert(ctx context.Context, chunks []ChunkRecord) error
	Search(ctx context.Context, queryEmbedding []float32, topK int, threshold float32) ([]SearchHit, error)
	Close() error
}

// Config configures the vector store backend.
type Config struct {
	DataDir    string
	Collection string
}
