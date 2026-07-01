package ragagent

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func newRefusalEvalAgent(t *testing.T, store VectorStore) *Agent {
	t.Helper()
	a, err := New(Config{
		ChatModel: "gpt-4o-mini",
		Runtime: RuntimeComponents{
			ChatModel: stubRuntimeChatModel{},
			Embedder:  stubRuntimeEmbedder{},
		},
		Storage:          StorageComponents{VectorStore: store},
		TopK:             1,
		ChunkSize:        64,
		ChunkOverlap:     0,
		MaxHistoryRounds: 8,
		RequestTimeout:   time.Second,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() {
		if cerr := a.Close(); cerr != nil {
			t.Fatalf("Close() error = %v", cerr)
		}
	})
	return a
}

func TestRunEvalSuiteExpectRefusalPasses(t *testing.T) {
	t.Parallel()

	// 空命中 store -> 触发证据不足拒答。
	a := newRefusalEvalAgent(t, stubStorageVectorStore{})

	summary, results, err := RunEvalSuite(context.Background(), a, []EvalCase{
		{Name: "unanswerable", SessionID: "r1", Query: "知识库没有的问题", ExpectRefusal: true},
	})
	if err != nil {
		t.Fatalf("RunEvalSuite() error = %v", err)
	}
	if len(results) != 1 || !results[0].Refused || !results[0].Passed {
		t.Fatalf("expected refused+passed, got %+v", results[0])
	}
	if summary.RefusalAccuracy != 1 {
		t.Fatalf("RefusalAccuracy = %v, want 1", summary.RefusalAccuracy)
	}
	if summary.PassedCases != 1 {
		t.Fatalf("PassedCases = %d, want 1", summary.PassedCases)
	}
}

func TestRunEvalSuiteExpectRefusalButAnsweredFails(t *testing.T) {
	t.Parallel()

	// 有命中 store -> 系统会作答，期望拒答的用例应判为失败。
	store := &injectedVectorStoreStub{
		searchHits: []SearchHit{
			{
				Chunk: ChunkRecord{
					ChunkID:    "doc:0",
					ParentID:   "doc",
					SourcePath: "/tmp/doc.md",
					Title:      "doc",
					Text:       "可用证据内容",
					StartRune:  0,
					EndRune:    18,
				},
				Score: 0.99,
			},
		},
	}
	a := newRefusalEvalAgent(t, store)

	summary, results, err := RunEvalSuite(context.Background(), a, []EvalCase{
		{Name: "should-refuse-but-answers", SessionID: "r2", Query: "有证据的问题", ExpectRefusal: true},
	})
	if err != nil {
		t.Fatalf("RunEvalSuite() error = %v", err)
	}
	if len(results) != 1 || results[0].Refused || results[0].Passed {
		t.Fatalf("expected not-refused+failed, got %+v", results[0])
	}
	if summary.RefusalAccuracy != 0 {
		t.Fatalf("RefusalAccuracy = %v, want 0", summary.RefusalAccuracy)
	}
}

func TestRunEvalSuiteNoRefusalCasesKeepsBackwardBehavior(t *testing.T) {
	t.Parallel()

	store := &injectedVectorStoreStub{
		searchHits: []SearchHit{
			{
				Chunk: ChunkRecord{
					ChunkID: "doc:0", ParentID: "doc", SourcePath: "/tmp/doc.md",
					Title: "doc", Text: "ok evidence", StartRune: 0, EndRune: 11,
				},
				Score: 0.99,
			},
		},
	}
	a := newRefusalEvalAgent(t, store)

	summary, _, err := RunEvalSuite(context.Background(), a, []EvalCase{
		{Name: "answerable", SessionID: "n1", Query: "q", WantAnswerContains: []string{"ok"}},
	})
	if err != nil {
		t.Fatalf("RunEvalSuite() error = %v", err)
	}
	// 无期望拒答用例时拒答正确率定为 1。
	if summary.RefusalAccuracy != 1 {
		t.Fatalf("RefusalAccuracy = %v, want 1", summary.RefusalAccuracy)
	}
}

func TestEvalReportJSONRoundTripPreservesRefusal(t *testing.T) {
	t.Parallel()

	report := EvalReport{
		Summary: EvalSummary{TotalCases: 2, PassedCases: 2, RefusalAccuracy: 1},
		Results: []EvalResult{
			{Name: "answerable", AnswerMatch: true, Passed: true},
			{Name: "unanswerable", Refused: true, Passed: true, Err: ErrEvidenceInsufficient},
		},
	}
	path := filepath.Join(t.TempDir(), "report.json")
	if err := WriteEvalReportJSON(path, report); err != nil {
		t.Fatalf("WriteEvalReportJSON() error = %v", err)
	}
	got, err := ReadEvalReportJSON(path)
	if err != nil {
		t.Fatalf("ReadEvalReportJSON() error = %v", err)
	}
	if got.Summary.RefusalAccuracy != 1 {
		t.Fatalf("RefusalAccuracy = %v, want 1", got.Summary.RefusalAccuracy)
	}
	if len(got.Results) != 2 || !got.Results[1].Refused {
		t.Fatalf("round-trip lost Refused flag: %+v", got.Results)
	}
}
