package ragagent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gtkit/go-rag-agent/internal/storage"
)

func TestRunEvalSuite(t *testing.T) {
	t.Parallel()

	a, err := New(Config{
		ChatModel: "gpt-4o-mini",
		Runtime: RuntimeComponents{
			ChatModel: stubRuntimeChatModel{},
			Embedder:  stubRuntimeEmbedder{},
		},
		Storage: StorageComponents{
			VectorStore: &injectedVectorStoreStub{
				searchHits: []SearchHit{
					{
						Chunk: ChunkRecord{
							ChunkID:    "doc:0",
							ParentID:   "doc",
							SourcePath: "/tmp/doc.md",
							Title:      "doc",
							Text:       "prompt cache evidence",
							StartRune:  0,
							EndRune:    21,
						},
						Score: 0.99,
					},
				},
			},
		},
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

	summary, results, err := RunEvalSuite(context.Background(), a, []EvalCase{
		{
			Name:                   "basic",
			SessionID:              "eval",
			Query:                  "same question",
			WantCitationSources:    []string{"/tmp/doc.md"},
			WantAnswerContains:     []string{"ok"},
			WantGroundedSubstrings: []string{"ok"},
		},
	})
	if err != nil {
		t.Fatalf("RunEvalSuite() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results len = %d, want 1", len(results))
	}
	if !results[0].Passed {
		t.Fatalf("results[0].Passed = false, result = %+v", results[0])
	}
	if summary.PassedCases != 1 {
		t.Fatalf("summary.PassedCases = %d, want 1", summary.PassedCases)
	}
}

func TestRunStructuredEvalSuite(t *testing.T) {
	t.Parallel()

	type payload struct {
		Status string `json:"status"`
	}

	a := &Agent{
		cfg: Config{
			ChatModel:           "structured-model",
			TopK:                1,
			SimilarityThreshold: 0.5,
			MaxHistoryRounds:    8,
		},
		store: &fakeStore{
			searchHits: []storage.SearchHit{
				{
					Chunk: storage.ChunkRecord{
						ChunkID:    "doc:0",
						SourcePath: "/tmp/doc.md",
						Title:      "doc",
						Text:       "structured evidence",
						StartRune:  0,
						EndRune:    18,
					},
					Score: 0.99,
				},
			},
		},
		embedder: &fakeEmbedder{defaultVec: []float32{1, 0}},
		runner:   &fakeRunner{answer: `{"status":"ok"}`},
		sessions: make(map[string]*Session),
	}

	results, err := RunStructuredEvalSuite(context.Background(), a, []StructuredEvalCase{
		{
			Name:      "structured",
			SessionID: "structured",
			Query:     "status?",
			NewTarget: func() any { return &payload{} },
			Validate: func(target any) error {
				if target.(*payload).Status != "ok" {
					return ErrStructuredOutputInvalid
				}
				return nil
			},
		},
	})
	if err != nil {
		t.Fatalf("RunStructuredEvalSuite() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results len = %d, want 1", len(results))
	}
	if !results[0].StructuredOutputValid || !results[0].Passed {
		t.Fatalf("result = %+v, want structured output valid", results[0])
	}
}

func TestMarshalEvalReportJSON(t *testing.T) {
	t.Parallel()

	report := EvalReport{
		Summary: EvalSummary{
			TotalCases:        1,
			PassedCases:       1,
			RetrievalRecall:   1,
			CitationPrecision: 1,
			Groundedness:      1,
			AnswerMatchRate:   1,
		},
		Results: []EvalResult{
			{
				Name: "basic",
				Answer: Answer{
					Text: "ok",
				},
				Passed: true,
			},
		},
	}

	data, err := MarshalEvalReportJSON(report)
	if err != nil {
		t.Fatalf("MarshalEvalReportJSON() error = %v", err)
	}
	encoded := string(data)
	if !strings.Contains(encoded, `"summary"`) || !strings.Contains(encoded, `"basic"`) {
		t.Fatalf("encoded report = %q, want summary and case name", encoded)
	}
}

func TestCheckEvalThresholdsAndBaseline(t *testing.T) {
	t.Parallel()

	report := EvalReport{
		Summary: EvalSummary{
			TotalCases:        2,
			PassedCases:       2,
			RetrievalRecall:   1,
			CitationPrecision: 1,
			Groundedness:      1,
			AnswerMatchRate:   1,
		},
		StructuredSummary: StructuredEvalSummary{
			TotalCases:                1,
			PassedCases:               1,
			StructuredOutputValidRate: 1,
		},
	}

	if err := CheckEvalThresholds(report, EvalThresholds{
		MinRetrievalRecall:           0.9,
		MinCitationPrecision:         0.9,
		MinGroundedness:              0.9,
		MinAnswerMatchRate:           0.9,
		MinStructuredOutputValidRate: 0.9,
		MaxFailures:                  0,
	}); err != nil {
		t.Fatalf("CheckEvalThresholds() error = %v", err)
	}

	if err := CompareEvalReportWithBaseline(report, report); err != nil {
		t.Fatalf("CompareEvalReportWithBaseline() error = %v", err)
	}
}

func TestEvalReportJSONFileRoundTrip(t *testing.T) {
	t.Parallel()

	report := EvalReport{
		Summary: EvalSummary{
			TotalCases:        1,
			PassedCases:       1,
			RetrievalRecall:   1,
			CitationPrecision: 1,
			Groundedness:      1,
			AnswerMatchRate:   1,
		},
		Results: []EvalResult{
			{
				Name: "basic",
				Answer: Answer{
					Text: "ok",
					Citations: []Citation{
						{SourcePath: "/tmp/doc.md", Title: "doc", ChunkID: "doc:0"},
					},
				},
				Passed: true,
			},
		},
	}

	path := filepath.Join(t.TempDir(), "report.json")
	if err := WriteEvalReportJSON(path, report); err != nil {
		t.Fatalf("WriteEvalReportJSON() error = %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("Stat(%q) error = %v", path, err)
	}

	got, err := ReadEvalReportJSON(path)
	if err != nil {
		t.Fatalf("ReadEvalReportJSON() error = %v", err)
	}
	if got.Summary.TotalCases != report.Summary.TotalCases {
		t.Fatalf("ReadEvalReportJSON() total cases = %d, want %d", got.Summary.TotalCases, report.Summary.TotalCases)
	}
	if len(got.Results) != 1 || got.Results[0].Name != "basic" {
		t.Fatalf("ReadEvalReportJSON() results = %+v, want basic result", got.Results)
	}
}
