package ragagent

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/gtkit/go-rag-agent/internal/storage"
)

func TestAddKnowledgeDirSourceDeletesRemovedFiles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{
			name: "重导入目录时删除已移除文件的历史索引",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()
			baseDir := t.TempDir()
			dataDir := filepath.Join(baseDir, "data")
			knowledgeDir := filepath.Join(baseDir, "knowledge")
			keepPath := filepath.Join(knowledgeDir, "keep.md")
			removePath := filepath.Join(knowledgeDir, "remove.md")
			writeTestFile(t, keepPath, "alpha")
			writeTestFile(t, removePath, "beta")

			agent := newDirectorySyncTestAgent(t, dataDir)
			if err := agent.AddKnowledge(ctx, DirSource(knowledgeDir)); err != nil {
				t.Fatalf("first AddKnowledge() error = %v", err)
			}
			if err := os.Remove(removePath); err != nil {
				t.Fatalf("Remove(%q) error = %v", removePath, err)
			}
			if err := agent.AddKnowledge(ctx, DirSource(knowledgeDir)); err != nil {
				t.Fatalf("second AddKnowledge() error = %v", err)
			}

			gotPaths := collectHitSourcePaths(t, agent.store, []float32{0, 1})
			if slices.Contains(gotPaths, removePath) {
				t.Fatalf("Search() returned removed source path %q, got %v", removePath, gotPaths)
			}

			keepPaths := collectHitSourcePaths(t, agent.store, []float32{1, 0})
			if !slices.Contains(keepPaths, keepPath) {
				t.Fatalf("Search() missing kept source path %q, got %v", keepPath, keepPaths)
			}
		})
	}
}

func TestAddKnowledgeDirSourceDeletesAllStaleFilesWhenDirectoryBecomesEmpty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{
			name: "空目录重导入时清空该目录历史索引",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()
			baseDir := t.TempDir()
			dataDir := filepath.Join(baseDir, "data")
			knowledgeDir := filepath.Join(baseDir, "knowledge")
			onlyPath := filepath.Join(knowledgeDir, "only.md")
			writeTestFile(t, onlyPath, "alpha")

			agent := newDirectorySyncTestAgent(t, dataDir)
			if err := agent.AddKnowledge(ctx, DirSource(knowledgeDir)); err != nil {
				t.Fatalf("first AddKnowledge() error = %v", err)
			}
			if err := os.Remove(onlyPath); err != nil {
				t.Fatalf("Remove(%q) error = %v", onlyPath, err)
			}
			if err := agent.AddKnowledge(ctx, DirSource(knowledgeDir)); err != nil {
				t.Fatalf("second AddKnowledge() error = %v", err)
			}

			gotPaths := collectHitSourcePaths(t, agent.store, []float32{1, 0})
			if len(gotPaths) != 0 {
				t.Fatalf("Search() returned stale paths for empty directory import: %v", gotPaths)
			}
		})
	}
}

func TestAddKnowledgeDirSourcePersistsDeletionSyncAcrossRestart(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{
			name: "持久化模式重启后仍会删除目录中已移除文件",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()
			baseDir := t.TempDir()
			dataDir := filepath.Join(baseDir, "data")
			knowledgeDir := filepath.Join(baseDir, "knowledge")
			keepPath := filepath.Join(knowledgeDir, "keep.md")
			removePath := filepath.Join(knowledgeDir, "remove.md")
			writeTestFile(t, keepPath, "alpha")
			writeTestFile(t, removePath, "beta")

			firstAgent := newDirectorySyncTestAgent(t, dataDir)
			if err := firstAgent.AddKnowledge(ctx, DirSource(knowledgeDir)); err != nil {
				t.Fatalf("first AddKnowledge() error = %v", err)
			}
			if err := firstAgent.Close(); err != nil {
				t.Fatalf("first Close() error = %v", err)
			}

			if err := os.Remove(removePath); err != nil {
				t.Fatalf("Remove(%q) error = %v", removePath, err)
			}

			secondAgent := newDirectorySyncTestAgent(t, dataDir)
			if err := secondAgent.AddKnowledge(ctx, DirSource(knowledgeDir)); err != nil {
				t.Fatalf("second AddKnowledge() error = %v", err)
			}

			gotPaths := collectHitSourcePaths(t, secondAgent.store, []float32{0, 1})
			if slices.Contains(gotPaths, removePath) {
				t.Fatalf("Search() returned removed source path after restart %q, got %v", removePath, gotPaths)
			}
		})
	}
}

func newDirectorySyncTestAgent(t *testing.T, dataDir string) *Agent {
	t.Helper()

	store, err := storage.NewChromemStore(storage.Config{
		DataDir:    dataDir,
		Collection: defaultCollectionName,
	})
	if err != nil {
		t.Fatalf("NewChromemStore() error = %v", err)
	}

	agent := &Agent{
		cfg: Config{
			DataDir: dataDir,
		},
		store: store,
		embedder: &fakeEmbedder{
			vectors: map[string][]float32{
				"alpha": {1, 0},
				"beta":  {0, 1},
			},
			defaultVec: []float32{1, 0},
		},
		chunker:  mustNewChunkerForTest(t, 64, 0),
		sessions: make(map[string]*Session),
	}

	t.Cleanup(func() {
		if err := agent.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	return agent
}

func collectHitSourcePaths(t *testing.T, store storage.VectorStore, queryEmbedding []float32) []string {
	t.Helper()

	hits, err := store.Search(t.Context(), queryEmbedding, 10, -1)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	paths := make([]string, 0, len(hits))
	for _, hit := range hits {
		if !slices.Contains(paths, hit.Chunk.SourcePath) {
			paths = append(paths, hit.Chunk.SourcePath)
		}
	}
	slices.Sort(paths)
	return paths
}
