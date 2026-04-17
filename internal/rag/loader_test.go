package rag

import (
	"context"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jung-kurt/gofpdf"
)

func TestLoadFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		prepare   func(t *testing.T) (context.Context, string, string, map[string]string)
		assertion func(t *testing.T, doc Document, err error)
	}{
		{
			name: "success",
			prepare: func(t *testing.T) (context.Context, string, string, map[string]string) {
				t.Helper()
				root := t.TempDir()
				path := filepath.Join(root, "knowledge.md")
				if err := os.WriteFile(path, []byte("hello world"), 0o644); err != nil {
					t.Fatalf("WriteFile(%q): %v", path, err)
				}
				return context.Background(), path, "Knowledge", map[string]string{"lang": "en"}
			},
			assertion: func(t *testing.T, doc Document, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("LoadFile() error = %v", err)
				}
				if doc.SourcePath == "" || doc.ID == "" {
					t.Fatalf("LoadFile() produced empty identity fields: %+v", doc)
				}
				if doc.Title != "Knowledge" {
					t.Fatalf("LoadFile() title = %q, want %q", doc.Title, "Knowledge")
				}
				if doc.Content != "hello world" {
					t.Fatalf("LoadFile() content = %q, want %q", doc.Content, "hello world")
				}
				if got := doc.Metadata["lang"]; got != "en" {
					t.Fatalf("LoadFile() metadata[lang] = %q, want %q", got, "en")
				}
			},
		},
		{
			name: "stable identity from normalized path but preserves source path spelling",
			prepare: func(t *testing.T) (context.Context, string, string, map[string]string) {
				t.Helper()
				root := t.TempDir()
				path := filepath.Join(root, "dir", "knowledge.md")
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatalf("MkdirAll(%q): %v", filepath.Dir(path), err)
				}
				if err := os.WriteFile(path, []byte("hello world"), 0o644); err != nil {
					t.Fatalf("WriteFile(%q): %v", path, err)
				}

				altPath := filepath.Join(root, "dir", "..", "dir", "knowledge.md")
				docA, err := LoadFile(context.Background(), path, "Knowledge", nil, LoadOptions{})
				if err != nil {
					t.Fatalf("LoadFile(path) error: %v", err)
				}
				docB, err := LoadFile(context.Background(), altPath, "Knowledge", nil, LoadOptions{})
				if err != nil {
					t.Fatalf("LoadFile(altPath) error: %v", err)
				}
				if docA.ID != docB.ID {
					t.Fatalf("stable ID mismatch: %q vs %q", docA.ID, docB.ID)
				}
				if docA.SourcePath != path {
					t.Fatalf("docA.SourcePath = %q, want %q", docA.SourcePath, path)
				}
				if docB.SourcePath != altPath {
					t.Fatalf("docB.SourcePath = %q, want %q", docB.SourcePath, altPath)
				}
				return context.Background(), path, "Knowledge", nil
			},
			assertion: func(t *testing.T, _ Document, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("LoadFile() error = %v", err)
				}
			},
		},
		{
			name: "pdf text extraction success",
			prepare: func(t *testing.T) (context.Context, string, string, map[string]string) {
				t.Helper()
				root := t.TempDir()
				path := filepath.Join(root, "knowledge.pdf")
				writeTestPDF(t, path, []string{"Hello PDF", "Second line"})
				return context.Background(), path, "Knowledge PDF", map[string]string{"lang": "en"}
			},
			assertion: func(t *testing.T, doc Document, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("LoadFile() error = %v", err)
				}
				if !strings.Contains(doc.Content, "Hello PDF") {
					t.Fatalf("LoadFile() content = %q, want to contain %q", doc.Content, "Hello PDF")
				}
				if !strings.Contains(doc.Content, "Second line") {
					t.Fatalf("LoadFile() content = %q, want to contain %q", doc.Content, "Second line")
				}
			},
		},
		{
			name: "markdown front matter is extracted into metadata and removed from content",
			prepare: func(t *testing.T) (context.Context, string, string, map[string]string) {
				t.Helper()
				root := t.TempDir()
				path := filepath.Join(root, "knowledge.md")
				writeTestFile(t, path, "---\ntag: api\nlang: en\n---\n# Title\n\nhello world\n")
				return context.Background(), path, "Knowledge", map[string]string{"source": "sidecar"}
			},
			assertion: func(t *testing.T, doc Document, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("LoadFile() error = %v", err)
				}
				wantMetadata := map[string]string{
					"tag":    "api",
					"lang":   "en",
					"source": "sidecar",
				}
				if !maps.Equal(doc.Metadata, wantMetadata) {
					t.Fatalf("LoadFile() metadata = %v, want %v", doc.Metadata, wantMetadata)
				}
				if strings.Contains(doc.Content, "tag: api") || strings.HasPrefix(doc.Content, "---") {
					t.Fatalf("LoadFile() content = %q, want front matter stripped", doc.Content)
				}
				if !strings.Contains(doc.Content, "hello world") {
					t.Fatalf("LoadFile() content = %q, want body text", doc.Content)
				}
			},
		},
		{
			name: "canceled context before read",
			prepare: func(t *testing.T) (context.Context, string, string, map[string]string) {
				t.Helper()
				root := t.TempDir()
				path := filepath.Join(root, "knowledge.md")
				if err := os.WriteFile(path, []byte("hello world"), 0o644); err != nil {
					t.Fatalf("WriteFile(%q): %v", path, err)
				}
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx, path, "Knowledge", nil
			},
			assertion: func(t *testing.T, _ Document, err error) {
				t.Helper()
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("LoadFile() error = %v, want errors.Is(..., context.Canceled)", err)
				}
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, path, title, metadata := tc.prepare(t)
			doc, err := LoadFile(ctx, path, title, metadata, LoadOptions{})
			tc.assertion(t, doc, err)
		})
	}
}

func TestLoadFilePDFOCRFallback(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		prepare   func(t *testing.T) (string, LoadOptions, string)
		assertion func(t *testing.T, doc Document, err error, markerPath string)
	}{
		{
			name: "text pdf uses direct extraction and skips ocr",
			prepare: func(t *testing.T) (string, LoadOptions, string) {
				t.Helper()
				root := t.TempDir()
				path := filepath.Join(root, "text.pdf")
				writeTestPDF(t, path, []string{"Hello PDF", "Second line"})
				markerPath := filepath.Join(root, "ocr-called")
				return path, LoadOptions{
					PDFOCR: func(context.Context, string) (string, error) {
						if err := os.WriteFile(markerPath, []byte("called"), 0o644); err != nil {
							t.Fatalf("WriteFile(%q): %v", markerPath, err)
						}
						return "OCR text", nil
					},
					MinDirectTextRunes: 0,
				}, markerPath
			},
			assertion: func(t *testing.T, doc Document, err error, markerPath string) {
				t.Helper()
				if err != nil {
					t.Fatalf("LoadFile() error = %v", err)
				}
				if !strings.Contains(doc.Content, "Hello PDF") {
					t.Fatalf("LoadFile() content = %q, want direct pdf text", doc.Content)
				}
				if _, statErr := os.Stat(markerPath); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatalf("OCR marker stat err = %v, want not exist", statErr)
				}
			},
		},
		{
			name: "blank pdf falls back to ocr output",
			prepare: func(t *testing.T) (string, LoadOptions, string) {
				t.Helper()
				root := t.TempDir()
				path := filepath.Join(root, "scan.pdf")
				writeBlankPDF(t, path)
				return path, LoadOptions{
					PDFOCR: func(context.Context, string) (string, error) {
						return "OCR line one\nOCR line two", nil
					},
				}, ""
			},
			assertion: func(t *testing.T, doc Document, err error, _ string) {
				t.Helper()
				if err != nil {
					t.Fatalf("LoadFile() error = %v", err)
				}
				if !strings.Contains(doc.Content, "OCR line one") {
					t.Fatalf("LoadFile() content = %q, want OCR text", doc.Content)
				}
			},
		},
		{
			name: "blank pdf without ocr returns clear error",
			prepare: func(t *testing.T) (string, LoadOptions, string) {
				t.Helper()
				root := t.TempDir()
				path := filepath.Join(root, "scan.pdf")
				writeBlankPDF(t, path)
				return path, LoadOptions{}, ""
			},
			assertion: func(t *testing.T, _ Document, err error, _ string) {
				t.Helper()
				if err == nil || !strings.Contains(err.Error(), "requires OCR") {
					t.Fatalf("LoadFile() error = %v, want clear OCR-required error", err)
				}
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			path, opts, markerPath := tc.prepare(t)
			doc, err := LoadFile(t.Context(), path, "Knowledge PDF", nil, opts)
			tc.assertion(t, doc, err, markerPath)
		})
	}
}

func writeTestPDF(t *testing.T, path string, lines []string) {
	t.Helper()

	p := gofpdf.New("P", "mm", "A4", "")
	p.AddPage()
	p.SetFont("Arial", "", 12)
	for _, line := range lines {
		p.CellFormat(0, 10, line, "", 1, "", false, 0, "")
	}
	if err := p.OutputFileAndClose(path); err != nil {
		t.Fatalf("OutputFileAndClose(%q): %v", path, err)
	}
}

func writeBlankPDF(t *testing.T, path string) {
	t.Helper()

	p := gofpdf.New("P", "mm", "A4", "")
	p.AddPage()
	if err := p.OutputFileAndClose(path); err != nil {
		t.Fatalf("OutputFileAndClose(%q): %v", path, err)
	}
}

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}
