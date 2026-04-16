package ragagent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileAndDirSourceResolve(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		prepare   func(t *testing.T) KnowledgeSource
		wantCount int
	}{
		{
			name: "FileSource on a .txt file returns 1 file",
			prepare: func(t *testing.T) KnowledgeSource {
				t.Helper()
				root := t.TempDir()
				path := filepath.Join(root, "one.txt")
				writeTestFile(t, path, "hello")
				return FileSource(path)
			},
			wantCount: 1,
		},
		{
			name: "DirSource recursively picks up .txt and .md files and returns 2 files",
			prepare: func(t *testing.T) KnowledgeSource {
				t.Helper()
				root := t.TempDir()
				writeTestFile(t, filepath.Join(root, "a.txt"), "a")
				writeTestFile(t, filepath.Join(root, "nested", "b.md"), "b")
				writeTestFile(t, filepath.Join(root, "skip.log"), "c")
				return DirSource(root)
			},
			wantCount: 2,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			src := tc.prepare(t)
			files, err := src.Resolve(t.Context())
			if err != nil {
				t.Fatalf("Resolve() unexpected error: %v", err)
			}
			if got := len(files); got != tc.wantCount {
				t.Fatalf("Resolve() file count = %d, want %d", got, tc.wantCount)
			}
		})
	}
}

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}
