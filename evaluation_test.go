package ragagent

import (
	"context"
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
