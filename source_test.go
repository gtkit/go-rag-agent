package ragagent

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestFileAndDirSourceResolve(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		prepare   func(t *testing.T) (KnowledgeSource, string)
		wantFiles []KnowledgeFile
		wantErr   error
	}{
		{
			name: "FileSource on a .txt file returns 1 file",
			prepare: func(t *testing.T) (KnowledgeSource, string) {
				t.Helper()
				root := t.TempDir()
				path := filepath.Join(root, "one.txt")
				writeTestFile(t, path, "hello")
				return FileSource(path), root
			},
			wantFiles: []KnowledgeFile{
				{Path: "one.txt", Title: "one"},
			},
		},
		{
			name: "FileSource on a .md file returns 1 file",
			prepare: func(t *testing.T) (KnowledgeSource, string) {
				t.Helper()
				root := t.TempDir()
				path := filepath.Join(root, "readme.md")
				writeTestFile(t, path, "hello")
				return FileSource(path), root
			},
			wantFiles: []KnowledgeFile{
				{Path: "readme.md", Title: "readme"},
			},
		},
		{
			name: "DirSource recursively picks up .txt and .md files and returns deterministic order",
			prepare: func(t *testing.T) (KnowledgeSource, string) {
				t.Helper()
				root := t.TempDir()
				writeTestFile(t, filepath.Join(root, "z.md"), "z")
				writeTestFile(t, filepath.Join(root, "nested", "a.txt"), "a")
				writeTestFile(t, filepath.Join(root, "skip.log"), "c")
				return DirSource(root), root
			},
			wantFiles: []KnowledgeFile{
				{Path: filepath.Join("nested", "a.txt"), Title: "a"},
				{Path: "z.md", Title: "z"},
			},
		},
		{
			name: "DirSource called with a file path returns error",
			prepare: func(t *testing.T) (KnowledgeSource, string) {
				t.Helper()
				root := t.TempDir()
				path := filepath.Join(root, "just-a-file.txt")
				writeTestFile(t, path, "hello")
				return DirSource(path), root
			},
			wantErr: ErrUnsupportedSource,
		},
		{
			name: "FileSource on unsupported extension returns error",
			prepare: func(t *testing.T) (KnowledgeSource, string) {
				t.Helper()
				root := t.TempDir()
				path := filepath.Join(root, "bad.pdf")
				writeTestFile(t, path, "hello")
				return FileSource(path), root
			},
			wantErr: ErrUnsupportedSource,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			src, root := tc.prepare(t)
			files, err := src.Resolve(t.Context())
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("Resolve() error = %v, want errors.Is(..., %v)", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Resolve() unexpected error: %v", err)
			}
			if got := len(files); got != len(tc.wantFiles) {
				t.Fatalf("Resolve() file count = %d, want %d", got, len(tc.wantFiles))
			}
			gotPaths := make([]string, 0, len(files))
			gotTitles := make([]string, 0, len(files))
			for _, file := range files {
				rel, relErr := filepath.Rel(root, file.Path)
				if relErr != nil {
					t.Fatalf("filepath.Rel(%q, %q): %v", root, file.Path, relErr)
				}
				gotPaths = append(gotPaths, rel)
				gotTitles = append(gotTitles, file.Title)
			}
			wantPaths := make([]string, 0, len(tc.wantFiles))
			wantTitles := make([]string, 0, len(tc.wantFiles))
			for _, want := range tc.wantFiles {
				wantPaths = append(wantPaths, want.Path)
				wantTitles = append(wantTitles, want.Title)
			}
			if !slices.Equal(gotPaths, wantPaths) {
				t.Fatalf("Resolve() paths = %v, want %v", gotPaths, wantPaths)
			}
			if !slices.Equal(gotTitles, wantTitles) {
				t.Fatalf("Resolve() titles = %v, want %v", gotTitles, wantTitles)
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
