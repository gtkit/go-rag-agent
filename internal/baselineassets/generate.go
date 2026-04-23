package baselineassets

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	ragagent "github.com/gtkit/go-rag-agent"
)

const (
	stableFixtureSource = "testdata/baseline/overview.md"
	evalReportFileName  = "eval-report.json"
	traceFileName       = "trace-summary.json"
)

// GenerateSDKPositioningBaseline 生成仓库内提交的定位基线样例结果。
func GenerateSDKPositioningBaseline(ctx context.Context, outDir string) error {
	if ctx == nil {
		ctx = context.Background()
	}

	repoRoot, err := findRepoRoot()
	if err != nil {
		return err
	}
	fixturePath := filepath.Join(repoRoot, filepath.FromSlash(stableFixtureSource))
	if _, err := os.Stat(fixturePath); err != nil {
		return fmt.Errorf("stat baseline fixture %q: %w", fixturePath, err)
	}

	recorder := &traceCollector{}
	agent, err := ragagent.New(ragagent.Config{
		ChatModel: "demo-chat",
		Runtime: ragagent.RuntimeComponents{
			ChatModel: demoChatModel{},
			Embedder:  demoEmbedder{},
		},
		Storage: ragagent.StorageComponents{
			VectorStore: newInMemoryVectorStore(),
		},
		TopK:             1,
		ChunkSize:        256,
		ChunkOverlap:     0,
		MaxHistoryRounds: 4,
		RequestTimeout:   time.Second,
		TraceRecorder:    recorder,
	})
	if err != nil {
		return fmt.Errorf("new baseline agent: %w", err)
	}
	defer func() {
		_ = agent.Close()
	}()

	if err := agent.AddKnowledge(ctx, ragagent.FileSource(fixturePath)); err != nil {
		return fmt.Errorf("add baseline knowledge: %w", err)
	}

	summary, results, err := ragagent.RunEvalSuite(ctx, agent, []ragagent.EvalCase{
		{
			Name:                   "positioning-baseline",
			SessionID:              "baseline",
			Query:                  "summarize the positioning baseline",
			WantCitationSources:    []string{fixturePath},
			WantAnswerContains:     []string{"ok"},
			WantGroundedSubstrings: []string{"ok"},
		},
	})
	if err != nil {
		return fmt.Errorf("run baseline eval suite: %w", err)
	}

	report := normalizeReport(ragagent.EvalReport{
		Summary: summary,
		Results: results,
	}, fixturePath, stableFixtureSource)
	if err := ragagent.WriteEvalReportJSON(filepath.Join(outDir, evalReportFileName), report); err != nil {
		return fmt.Errorf("write baseline eval report: %w", err)
	}

	trace, ok := recorder.last()
	if !ok {
		return errors.New("baseline trace recorder captured no trace")
	}
	traceSummary := ragagent.SummarizeExecutionTrace(trace)
	traceSummary.Duration = 0
	if err := writeJSON(filepath.Join(outDir, traceFileName), traceSummary); err != nil {
		return fmt.Errorf("write baseline trace summary: %w", err)
	}
	return nil
}

type traceCollector struct {
	mu    sync.Mutex
	trace ragagent.ExecutionTrace
	ok    bool
}

func (c *traceCollector) OnExecutionTrace(_ context.Context, trace ragagent.ExecutionTrace) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.trace = trace
	c.ok = true
}

func (c *traceCollector) last() (ragagent.ExecutionTrace, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.trace, c.ok
}

type demoChatModel struct{}

func (demoChatModel) Generate(context.Context, []ragagent.Message) (ragagent.Message, error) {
	return ragagent.Message{Role: ragagent.RoleAssistant, Content: "ok"}, nil
}

func (demoChatModel) Stream(_ context.Context, _ []ragagent.Message, emit func(string) error) error {
	return emit("ok")
}

type demoEmbedder struct{}

func (demoEmbedder) EmbedTexts(_ context.Context, texts []string) ([][]float32, error) {
	rows := make([][]float32, 0, len(texts))
	for range texts {
		rows = append(rows, []float32{1})
	}
	return rows, nil
}

type inMemoryVectorStore struct {
	mu     sync.RWMutex
	chunks []ragagent.ChunkRecord
}

func newInMemoryVectorStore() *inMemoryVectorStore {
	return &inMemoryVectorStore{}
}

func (s *inMemoryVectorStore) Upsert(_ context.Context, chunks []ragagent.ChunkRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, chunk := range chunks {
		s.chunks = slices.DeleteFunc(s.chunks, func(existing ragagent.ChunkRecord) bool {
			return existing.ChunkID == chunk.ChunkID
		})
		s.chunks = append(s.chunks, cloneChunk(chunk))
	}
	slices.SortFunc(s.chunks, func(a, b ragagent.ChunkRecord) int {
		if a.SourcePath != b.SourcePath {
			return strings.Compare(a.SourcePath, b.SourcePath)
		}
		if a.StartRune != b.StartRune {
			if a.StartRune < b.StartRune {
				return -1
			}
			return 1
		}
		return strings.Compare(a.ChunkID, b.ChunkID)
	})
	return nil
}

func (s *inMemoryVectorStore) Search(ctx context.Context, _ []float32, topK int, _ float32) ([]ragagent.SearchHit, error) {
	return s.search(ctx, topK, ragagent.SearchFilter{})
}

func (s *inMemoryVectorStore) SearchWithFilter(ctx context.Context, _ []float32, topK int, _ float32, filter ragagent.SearchFilter) ([]ragagent.SearchHit, error) {
	return s.search(ctx, topK, filter)
}

func (s *inMemoryVectorStore) DeleteBySourcePaths(_ context.Context, sourcePaths []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.chunks = slices.DeleteFunc(s.chunks, func(chunk ragagent.ChunkRecord) bool {
		return slices.Contains(sourcePaths, chunk.SourcePath)
	})
	return nil
}

func (s *inMemoryVectorStore) Close() error {
	return nil
}

func (s *inMemoryVectorStore) search(ctx context.Context, topK int, filter ragagent.SearchFilter) ([]ragagent.SearchHit, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	hits := make([]ragagent.SearchHit, 0, len(s.chunks))
	for _, chunk := range s.chunks {
		if !matchesFilter(chunk, filter) {
			continue
		}
		hits = append(hits, ragagent.SearchHit{
			Chunk: cloneChunk(chunk),
			Score: 1,
		})
	}
	if topK > 0 && len(hits) > topK {
		hits = hits[:topK]
	}
	return hits, nil
}

func matchesFilter(chunk ragagent.ChunkRecord, filter ragagent.SearchFilter) bool {
	if len(filter.SourcePaths) > 0 && !slices.Contains(filter.SourcePaths, chunk.SourcePath) {
		return false
	}
	if len(filter.SourcePrefixes) > 0 {
		matched := false
		for _, prefix := range filter.SourcePrefixes {
			if strings.HasPrefix(chunk.SourcePath, prefix) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	for key, want := range filter.Metadata {
		if chunk.Metadata[key] != want {
			return false
		}
	}
	return true
}

func cloneChunk(chunk ragagent.ChunkRecord) ragagent.ChunkRecord {
	out := chunk
	if len(chunk.Metadata) > 0 {
		out.Metadata = make(map[string]string, len(chunk.Metadata))
		for key, value := range chunk.Metadata {
			out.Metadata[key] = value
		}
	}
	if len(chunk.Embedding) > 0 {
		out.Embedding = slices.Clone(chunk.Embedding)
	}
	return out
}

func normalizeReport(report ragagent.EvalReport, actualSourcePath string, stableSourcePath string) ragagent.EvalReport {
	out := report
	out.Results = make([]ragagent.EvalResult, 0, len(report.Results))
	for _, result := range report.Results {
		next := result
		next.Answer.Citations = normalizeCitations(result.Answer.Citations, actualSourcePath, stableSourcePath)
		out.Results = append(out.Results, next)
	}
	return out
}

func normalizeCitations(in []ragagent.Citation, actualSourcePath string, stableSourcePath string) []ragagent.Citation {
	if len(in) == 0 {
		return nil
	}

	cleanActual := filepath.Clean(actualSourcePath)
	out := make([]ragagent.Citation, 0, len(in))
	for _, citation := range in {
		next := citation
		if filepath.Clean(next.SourcePath) == cleanActual {
			next.SourcePath = stableSourcePath
		}
		if strings.HasPrefix(next.ChunkID, cleanActual+":") {
			next.ChunkID = stableSourcePath + strings.TrimPrefix(next.ChunkID, cleanActual)
		}
		out = append(out, next)
	}
	return out
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal json %q: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir json dir %q: %w", path, err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write json %q: %w", path, err)
	}
	return nil
}

func findRepoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	wd, err = filepath.Abs(wd)
	if err != nil {
		return "", fmt.Errorf("abs working directory: %w", err)
	}

	for current := wd; ; current = filepath.Dir(current) {
		if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("find repo root from %q: go.mod not found", wd)
		}
	}
}
