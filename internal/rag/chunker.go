package rag

import (
	"fmt"
)

// Chunk 是 RAG 层自包含的文本分块模型。
type Chunk struct {
	ChunkID    string
	ParentID   string
	SourcePath string
	Title      string
	Metadata   map[string]string
	Text       string
	StartRune  int
	EndRune    int
}

// Chunker 按 rune 长度和 overlap 规则切分文档。
type Chunker struct {
	size    int
	overlap int
}

func NewChunker(size int, overlap int) (*Chunker, error) {
	switch {
	case size <= 0:
		return nil, fmt.Errorf("invalid chunk size %d", size)
	case overlap < 0:
		return nil, fmt.Errorf("invalid overlap %d", overlap)
	case overlap >= size:
		return nil, fmt.Errorf("overlap %d must be less than size %d", overlap, size)
	default:
		return &Chunker{
			size:    size,
			overlap: overlap,
		}, nil
	}
}

func (c *Chunker) Split(doc Document) []Chunk {
	runes := []rune(doc.Content)
	if len(runes) == 0 {
		return nil
	}

	step := c.size - c.overlap
	chunks := make([]Chunk, 0, len(runes)/step+1)
	for start := 0; start < len(runes); start += step {
		end := start + c.size
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, Chunk{
			ChunkID:    fmt.Sprintf("%s:%d", doc.ID, len(chunks)),
			ParentID:   doc.ID,
			SourcePath: doc.SourcePath,
			Title:      doc.Title,
			Metadata:   cloneMetadata(doc.Metadata),
			Text:       string(runes[start:end]),
			StartRune:  start,
			EndRune:    end,
		})
		if end == len(runes) {
			break
		}
	}
	return chunks
}
