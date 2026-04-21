package ragagent

import (
	"context"
	"errors"
	"testing"

	"github.com/gtkit/go-rag-agent/internal/storage"
)

type structuredSummary struct {
	Summary string `json:"summary"`
	Score   int    `json:"score"`
}

func TestSessionAskStructuredPopulatesTarget(t *testing.T) {
	t.Parallel()

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
						Text:       "gateway api exact match",
						StartRune:  0,
						EndRune:    23,
					},
					Score: 0.99,
				},
			},
		},
		embedder: &fakeEmbedder{defaultVec: []float32{1, 0}},
		runner:   &fakeRunner{answer: `{"summary":"gateway ready","score":2}`},
		sessions: make(map[string]*Session),
	}

	var target structuredSummary
	got, err := a.GetSession("structured").AskStructured(context.Background(), "summarize", &target)
	if err != nil {
		t.Fatalf("AskStructured() error = %v", err)
	}
	if target.Summary != "gateway ready" || target.Score != 2 {
		t.Fatalf("target = %#v, want summary=%q score=%d", target, "gateway ready", 2)
	}
	if got.RawJSON != `{"summary":"gateway ready","score":2}` {
		t.Fatalf("RawJSON = %q, want %q", got.RawJSON, `{"summary":"gateway ready","score":2}`)
	}
	if got.Answer.Trace == nil {
		t.Fatal("Answer.Trace = nil")
	}

	runner := a.runner.(*fakeRunner)
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if runner.lastReq.ResponseFormatInstruction == "" {
		t.Fatal("ResponseFormatInstruction = empty, want non-empty")
	}
}

func TestSessionAskStructuredExtractsJSONFromCodeFence(t *testing.T) {
	t.Parallel()

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
						Text:       "gateway api exact match",
						StartRune:  0,
						EndRune:    23,
					},
					Score: 0.99,
				},
			},
		},
		embedder: &fakeEmbedder{defaultVec: []float32{1, 0}},
		runner:   &fakeRunner{answer: "```json\n{\"summary\":\"gateway ready\",\"score\":2}\n```"},
		sessions: make(map[string]*Session),
	}

	var target structuredSummary
	got, err := a.GetSession("structured-fence").AskStructured(context.Background(), "summarize", &target)
	if err != nil {
		t.Fatalf("AskStructured() error = %v", err)
	}
	if target.Summary != "gateway ready" || target.Score != 2 {
		t.Fatalf("target = %#v, want summary=%q score=%d", target, "gateway ready", 2)
	}
	if got.RawJSON != `{"summary":"gateway ready","score":2}` {
		t.Fatalf("RawJSON = %q, want %q", got.RawJSON, `{"summary":"gateway ready","score":2}`)
	}
}

func TestSessionAskStructuredRejectsInvalidTarget(t *testing.T) {
	t.Parallel()

	a := &Agent{
		cfg:      Config{},
		sessions: make(map[string]*Session),
	}

	tests := []struct {
		name   string
		target any
	}{
		{
			name:   "non pointer target",
			target: structuredSummary{},
		},
		{
			name:   "nil pointer target",
			target: (*structuredSummary)(nil),
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := a.GetSession("invalid-target").AskStructured(context.Background(), "summarize", tc.target)
			if !errors.Is(err, ErrStructuredOutputTarget) {
				t.Fatalf("AskStructured() error = %v, want errors.Is(..., %v)", err, ErrStructuredOutputTarget)
			}
		})
	}
}

func TestSessionAskStructuredReturnsParseError(t *testing.T) {
	t.Parallel()

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
						Text:       "gateway api exact match",
						StartRune:  0,
						EndRune:    23,
					},
					Score: 0.99,
				},
			},
		},
		embedder: &fakeEmbedder{defaultVec: []float32{1, 0}},
		runner:   &fakeRunner{answer: "not valid json"},
		sessions: make(map[string]*Session),
	}

	var target structuredSummary
	got, err := a.GetSession("structured-invalid-json").AskStructured(context.Background(), "summarize", &target)
	if !errors.Is(err, ErrStructuredOutputInvalid) {
		t.Fatalf("AskStructured() error = %v, want errors.Is(..., %v)", err, ErrStructuredOutputInvalid)
	}
	if got.Answer.Text != "not valid json" {
		t.Fatalf("Answer.Text = %q, want %q", got.Answer.Text, "not valid json")
	}
}

func TestExtractStructuredJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "raw object",
			input: `{"summary":"ok"}`,
			want:  `{"summary":"ok"}`,
		},
		{
			name:  "fenced json",
			input: "```json\n{\"summary\":\"ok\"}\n```",
			want:  `{"summary":"ok"}`,
		},
		{
			name:  "surrounded object",
			input: "result:\n{\"summary\":\"ok\"}\nthanks",
			want:  `{"summary":"ok"}`,
		},
		{
			name:    "missing json",
			input:   "plain text only",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := extractStructuredJSON(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatal("extractStructuredJSON() error = nil, want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("extractStructuredJSON() error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("extractStructuredJSON() = %q, want %q", got, tc.want)
			}
		})
	}
}
