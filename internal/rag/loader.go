package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	pdf "github.com/ledongthuc/pdf"
)

// Document 是 RAG 层自包含的源文档模型。
type Document struct {
	ID         string
	SourcePath string
	Title      string
	Metadata   map[string]string
	Content    string
}

// LoadOptions 定义文件加载时的可选能力。
type LoadOptions struct {
	PDFOCR             func(context.Context, string) (string, error)
	MinDirectTextRunes int
}

// LoadFile 把本地文件读取为一个 RAG 文档。
func LoadFile(ctx context.Context, path string, title string, metadata map[string]string, opts LoadOptions) (Document, error) {
	if err := ctx.Err(); err != nil {
		return Document{}, err
	}
	stableID, err := normalizeStablePath(path)
	if err != nil {
		return Document{}, fmt.Errorf("normalize path %q: %w", path, err)
	}

	content, extractedMetadata, err := readDocumentContent(ctx, path, opts)
	if err != nil {
		return Document{}, fmt.Errorf("read file %q: %w", path, err)
	}

	return Document{
		ID:         stableID,
		SourcePath: path,
		Title:      title,
		Metadata:   mergeMetadata(extractedMetadata, metadata),
		Content:    string(content),
	}, nil
}

func readDocumentContent(ctx context.Context, path string, opts LoadOptions) ([]byte, map[string]string, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".pdf":
		content, err := readPDFDocumentContent(ctx, path, opts)
		return content, map[string]string{}, err
	case ".md":
		content, metadata, err := readMarkdownDocumentContent(path)
		return content, metadata, err
	default:
		content, err := os.ReadFile(path)
		return content, map[string]string{}, err
	}
}

func readMarkdownDocumentContent(path string) ([]byte, map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	body, metadata, err := extractMarkdownFrontMatter(string(data))
	if err != nil {
		return nil, nil, err
	}
	return []byte(body), metadata, nil
}

func readPDFDocumentContent(ctx context.Context, path string, opts LoadOptions) ([]byte, error) {
	text, err := extractPDFText(path)
	if err != nil {
		return nil, err
	}
	if hasUsableDirectPDFText(text, opts.MinDirectTextRunes) {
		return []byte(text), nil
	}
	if opts.PDFOCR == nil {
		return nil, fmt.Errorf("pdf %q requires OCR bridge configuration", path)
	}

	text, err = opts.PDFOCR(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("ocr pdf: %w", err)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("ocr pdf %q produced empty text", path)
	}
	return []byte(text), nil
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

func hasUsableDirectPDFText(text string, minDirectTextRunes int) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	if minDirectTextRunes <= 0 {
		return true
	}
	return utf8.RuneCountInString(text) >= minDirectTextRunes
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

func mergeMetadata(base map[string]string, override map[string]string) map[string]string {
	if len(base) == 0 && len(override) == 0 {
		return map[string]string{}
	}

	out := make(map[string]string, len(base)+len(override))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range override {
		out[k] = v
	}
	return out
}

func stringifyMetadataMap(raw map[string]any) (map[string]string, error) {
	if len(raw) == 0 {
		return map[string]string{}, nil
	}

	out := make(map[string]string, len(raw))
	for key, value := range raw {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		text, err := stringifyMetadataValue(value)
		if err != nil {
			return nil, fmt.Errorf("stringify metadata key %q: %w", key, err)
		}
		out[key] = text
	}
	return out, nil
}

func stringifyMetadataValue(value any) (string, error) {
	switch v := value.(type) {
	case nil:
		return "", nil
	case string:
		return v, nil
	case bool:
		if v {
			return "true", nil
		}
		return "false", nil
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return fmt.Sprint(v), nil
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
}
