package rag

import (
	"context"
	"errors"
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
				docA, err := LoadFile(context.Background(), path, "Knowledge", nil)
				if err != nil {
					t.Fatalf("LoadFile(path) error: %v", err)
				}
				docB, err := LoadFile(context.Background(), altPath, "Knowledge", nil)
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
			doc, err := LoadFile(ctx, path, title, metadata)
			tc.assertion(t, doc, err)
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
