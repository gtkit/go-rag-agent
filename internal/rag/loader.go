package rag

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	pdf "github.com/ledongthuc/pdf"
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
	stableID, err := normalizeStablePath(path)
	if err != nil {
		return Document{}, fmt.Errorf("normalize path %q: %w", path, err)
	}

	content, err := readDocumentContent(path)
	if err != nil {
		return Document{}, fmt.Errorf("read file %q: %w", path, err)
	}

	return Document{
		ID:         stableID,
		SourcePath: path,
		Title:      title,
		Metadata:   cloneMetadata(metadata),
		Content:    string(content),
	}, nil
}

func readDocumentContent(path string) ([]byte, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".pdf":
		text, err := extractPDFText(path)
		if err != nil {
			return nil, err
		}
		return []byte(text), nil
	default:
		return os.ReadFile(path)
	}
}

func extractPDFText(path string) (string, error) {
	file, reader, err := pdf.Open(path)
	if err != nil {
		return "", fmt.Errorf("open pdf: %w", err)
	}
	defer file.Close()

	textReader, err := reader.GetPlainText()
	if err != nil {
		return "", fmt.Errorf("extract pdf text: %w", err)
	}

	data, err := io.ReadAll(textReader)
	if err != nil {
		return "", fmt.Errorf("read extracted pdf text: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

func normalizeStablePath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
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
