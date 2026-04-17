package ragagent

import (
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestFileAndDirSourceResolve(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		prepare    func(t *testing.T) (KnowledgeSource, string)
		wantFiles  []KnowledgeFile
		wantMetas  []map[string]string
		wantErr    error
		wantAnyErr bool
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
			name: "FileSource on a .pdf file returns 1 file",
			prepare: func(t *testing.T) (KnowledgeSource, string) {
				t.Helper()
				root := t.TempDir()
				path := filepath.Join(root, "paper.pdf")
				writeTestFile(t, path, "pdf-bytes-placeholder")
				return FileSource(path), root
			},
			wantFiles: []KnowledgeFile{
				{Path: "paper.pdf", Title: "paper"},
			},
		},
		{
			name: "FileSource loads sidecar metadata",
			prepare: func(t *testing.T) (KnowledgeSource, string) {
				t.Helper()
				root := t.TempDir()
				path := filepath.Join(root, "note.md")
				writeTestFile(t, path, "hello")
				writeTestFile(t, filepath.Join(root, "note.meta.json"), `{"tag":"api","lang":"en"}`)
				return FileSource(path), root
			},
			wantFiles: []KnowledgeFile{
				{Path: "note.md", Title: "note"},
			},
			wantMetas: []map[string]string{
				{"tag": "api", "lang": "en"},
			},
		},
		{
			name: "DirSource recursively picks up .txt .md and .pdf files and returns deterministic order",
			prepare: func(t *testing.T) (KnowledgeSource, string) {
				t.Helper()
				root := t.TempDir()
				writeTestFile(t, filepath.Join(root, "z.md"), "z")
				writeTestFile(t, filepath.Join(root, "nested", "b.pdf"), "pdf")
				writeTestFile(t, filepath.Join(root, "nested", "a.txt"), "a")
				writeTestFile(t, filepath.Join(root, "skip.log"), "c")
				return DirSource(root), root
			},
			wantFiles: []KnowledgeFile{
				{Path: filepath.Join("nested", "a.txt"), Title: "a"},
				{Path: filepath.Join("nested", "b.pdf"), Title: "b"},
				{Path: "z.md", Title: "z"},
			},
		},
		{
			name: "DirSource skips sidecar files and attaches sidecar metadata",
			prepare: func(t *testing.T) (KnowledgeSource, string) {
				t.Helper()
				root := t.TempDir()
				writeTestFile(t, filepath.Join(root, "docs", "a.md"), "hello")
				writeTestFile(t, filepath.Join(root, "docs", "a.meta.yaml"), "tag: api\nlang: zh\n")
				writeTestFile(t, filepath.Join(root, "docs", "a.meta.json"), `{"ignored":"because yaml wins by priority order"}`)
				writeTestFile(t, filepath.Join(root, "docs", "b.txt"), "world")
				writeTestFile(t, filepath.Join(root, "docs", "b.meta.json"), `{"team":"search"}`)
				return DirSource(filepath.Join(root, "docs")), filepath.Join(root, "docs")
			},
			wantFiles: []KnowledgeFile{
				{Path: "a.md", Title: "a"},
				{Path: "b.txt", Title: "b"},
			},
			wantMetas: []map[string]string{
				{"tag": "api", "lang": "zh"},
				{"team": "search"},
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
			name: "DirSource on symlinked directory returns error",
			prepare: func(t *testing.T) (KnowledgeSource, string) {
				t.Helper()
				root := t.TempDir()
				targetDir := filepath.Join(root, "real")
				if err := os.MkdirAll(targetDir, 0o755); err != nil {
					t.Fatalf("MkdirAll(%q): %v", targetDir, err)
				}
				linkDir := filepath.Join(root, "link")
				if err := os.Symlink(targetDir, linkDir); err != nil {
					t.Fatalf("Symlink(%q, %q): %v", targetDir, linkDir, err)
				}
				return DirSource(linkDir), root
			},
			wantErr: ErrUnsupportedSource,
		},
		{
			name: "FileSource on unsupported extension returns error",
			prepare: func(t *testing.T) (KnowledgeSource, string) {
				t.Helper()
				root := t.TempDir()
				path := filepath.Join(root, "bad.docx")
				writeTestFile(t, path, "hello")
				return FileSource(path), root
			},
			wantErr: ErrUnsupportedSource,
		},
		{
			name: "FileSource on symlink returns error",
			prepare: func(t *testing.T) (KnowledgeSource, string) {
				t.Helper()
				root := t.TempDir()
				target := filepath.Join(root, "target.md")
				link := filepath.Join(root, "link.md")
				writeTestFile(t, target, "hello")
				if err := os.Symlink(target, link); err != nil {
					t.Fatalf("Symlink(%q, %q): %v", target, link, err)
				}
				return FileSource(link), root
			},
			wantErr: ErrUnsupportedSource,
		},
		{
			name: "DirSource skips symlinked files",
			prepare: func(t *testing.T) (KnowledgeSource, string) {
				t.Helper()
				root := t.TempDir()
				writeTestFile(t, filepath.Join(root, "z.md"), "z")
				outside := filepath.Join(t.TempDir(), "outside.md")
				writeTestFile(t, outside, "outside")
				if err := os.Symlink(outside, filepath.Join(root, "linked.md")); err != nil {
					t.Fatalf("Symlink(%q, %q): %v", outside, filepath.Join(root, "linked.md"), err)
				}
				return DirSource(root), root
			},
			wantFiles: []KnowledgeFile{
				{Path: "z.md", Title: "z"},
			},
		},
		{
			name: "Youdao bridge export resolves supported files",
			prepare: func(t *testing.T) (KnowledgeSource, string) {
				t.Helper()
				root := t.TempDir()
				script := filepath.Join(root, "youdao-export.sh")
				writeExecutableFile(t, script, "#!/bin/sh\nout=\"$1\"\nmkdir -p \"$out\"\nprintf '# note\\ncontent' > \"$out/note.md\"\n")
				return YoudaoNoteSource(YoudaoNoteBridgeConfig{
					Command: script,
					Args:    []string{"{output}"},
				}), root
			},
			wantFiles: []KnowledgeFile{
				{Path: "note.md", Title: "note"},
			},
		},
		{
			name: "Youdao bridge missing command returns error",
			prepare: func(t *testing.T) (KnowledgeSource, string) {
				t.Helper()
				root := t.TempDir()
				return YoudaoNoteSource(YoudaoNoteBridgeConfig{
					Command: "missing-youdaonote-command",
					Args:    []string{"{output}"},
				}), root
			},
			wantErr: ErrUnsupportedSource,
		},
		{
			name: "Youdao bridge command failure returns error",
			prepare: func(t *testing.T) (KnowledgeSource, string) {
				t.Helper()
				root := t.TempDir()
				script := filepath.Join(root, "youdao-fail.sh")
				writeExecutableFile(t, script, "#!/bin/sh\necho fail >&2\nexit 2\n")
				return YoudaoNoteSource(YoudaoNoteBridgeConfig{
					Command: script,
					Args:    []string{"{output}"},
				}), root
			},
			wantAnyErr: true,
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
			if tc.wantAnyErr {
				if err == nil {
					t.Fatal("Resolve() error = nil, want non-nil")
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
				compareRoot := root
				if _, ok := src.(*youdaoNoteSource); ok {
					compareRoot = filepath.Dir(file.Path)
				}
				rel, relErr := filepath.Rel(compareRoot, file.Path)
				if relErr != nil {
					t.Fatalf("filepath.Rel(%q, %q): %v", compareRoot, file.Path, relErr)
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
			if len(tc.wantMetas) > 0 {
				gotMetas := make([]map[string]string, 0, len(files))
				for _, file := range files {
					gotMetas = append(gotMetas, file.Metadata)
				}
				for i := range tc.wantMetas {
					if !maps.Equal(gotMetas[i], tc.wantMetas[i]) {
						t.Fatalf("Resolve() metadata[%d] = %v, want %v", i, gotMetas[i], tc.wantMetas[i])
					}
				}
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

func writeExecutableFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}
