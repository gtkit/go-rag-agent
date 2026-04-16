package rag

import (
	"context"
	"fmt"
	"os"
)

// Document is the self-contained RAG-layer source document model.
type Document struct {
	ID         string
	SourcePath string
	Title      string
	Metadata   map[string]string
	Content    string
}

// LoadFile reads a local file as a Document.
func LoadFile(ctx context.Context, path string, title string, metadata map[string]string) (Document, error) {
	if err := ctx.Err(); err != nil {
		return Document{}, err
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return Document{}, fmt.Errorf("read file %q: %w", path, err)
	}

	return Document{
		ID:         path,
		SourcePath: path,
		Title:      title,
		Metadata:   cloneMetadata(metadata),
		Content:    string(content),
	}, nil
}

func cloneMetadata(input map[string]string) map[string]string {
	if len(input) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}
