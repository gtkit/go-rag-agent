package rag

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
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
