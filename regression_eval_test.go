package ragagent

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/gtkit/go-rag-agent/internal/graph"
	"github.com/gtkit/go-rag-agent/internal/storage"
	"github.com/gtkit/go-rag-agent/internal/tools"
	"github.com/gtkit/go-rag-agent/internal/websearch"
)

func TestRuntimeRegressionSuite(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		run  func(*testing.T)
	}{
		{
			name: "runtime injection skips default provider config",
			run: func(t *testing.T) {
				t.Helper()

				cfg := Config{
					Runtime: RuntimeComponents{
						ChatModel: stubRuntimeChatModel{},
						Embedder:  stubRuntimeEmbedder{},
					},
				}
				if err := cfg.Validate(); err != nil {
					t.Fatalf("Validate() error = %v", err)
				}
			},
		},
		{
			name: "sync ask returns trace",
			run: func(t *testing.T) {
				t.Helper()

				a := &Agent{
					cfg: Config{
						ChatModel:           "regression-model",
						TopK:                5,
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
									Text:       "regression sync evidence",
									StartRune:  0,
									EndRune:    24,
								},
								Score: 0.99,
							},
						},
					},
					embedder: &fakeEmbedder{defaultVec: []float32{1, 0}},
					runner:   &fakeRunner{answer: "regression answer"},
					sessions: make(map[string]*Session),
				}

				answer, err := a.GetSession("regression-sync").Ask(context.Background(), "what changed?")
				if err != nil {
					t.Fatalf("Ask() error = %v", err)
				}
				if answer.Trace == nil {
					t.Fatal("Answer.Trace = nil")
				}
				if answer.Trace.Model.OutputChars != len("regression answer") {
					t.Fatalf("trace model output chars = %d, want %d", answer.Trace.Model.OutputChars, len("regression answer"))
				}
			},
		},
		{
			name: "stream ask returns done trace",
			run: func(t *testing.T) {
				t.Helper()

				a := &Agent{
					cfg: Config{
						ChatModel:           "regression-stream-model",
						TopK:                5,
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
									Text:       "regression stream evidence",
									StartRune:  0,
									EndRune:    26,
								},
								Score: 0.99,
							},
						},
					},
					embedder: &fakeEmbedder{defaultVec: []float32{1, 0}},
					runner: &fakeStreamingRunner{
						askStreamFn: func(_ context.Context, _ graph.Request, emit graph.StreamEmitter) error {
							if err := emit(graph.Event{Type: graph.EventAnswerChunk, Content: "stream "}); err != nil {
								return err
							}
							if err := emit(graph.Event{Type: graph.EventAnswerChunk, Content: "answer"}); err != nil {
								return err
							}
							return emit(graph.Event{Type: graph.EventDone, Step: 2})
						},
					},
					sessions: make(map[string]*Session),
				}

				var doneTrace *ExecutionTrace
				err := a.GetSession("regression-stream").AskStream(context.Background(), "show stream trace", func(event StreamEvent) error {
					if event.Type == EventDone {
						doneTrace = event.Trace
					}
					return nil
				})
				if err != nil {
					t.Fatalf("AskStream() error = %v", err)
				}
				if doneTrace == nil {
					t.Fatal("done trace = nil")
				}
				if !doneTrace.Stream {
					t.Fatal("done trace Stream = false, want true")
				}
			},
		},
		{
			name: "web fallback trace captures search tool",
			run: func(t *testing.T) {
				t.Helper()

				runner, err := graph.NewChatRunner(
					&fakeTraceChatModel{answer: "web-backed answer"},
					tools.NewWebSearchTool(&fakeSearcher{
						results: []websearch.Result{
							{
								Title:   "fresh result",
								URL:     "https://example.com/fresh",
								Content: "latest web evidence",
							},
						},
					}),
				)
				if err != nil {
					t.Fatalf("NewChatRunner() error = %v", err)
				}

				a := &Agent{
					cfg: Config{
						TopK:                5,
						SimilarityThreshold: 0.5,
						MaxHistoryRounds:    8,
						EnableWebSearch:     true,
					},
					store:    &fakeStore{searchHits: nil},
					embedder: &fakeEmbedder{defaultVec: []float32{1, 0}},
					runner:   runner,
					sessions: make(map[string]*Session),
				}

				answer, err := a.GetSession("regression-web").Ask(context.Background(), "latest news")
				if err != nil {
					t.Fatalf("Ask() error = %v", err)
				}
				if answer.Trace == nil {
					t.Fatal("Answer.Trace = nil")
				}

				names := make([]string, 0, len(answer.Trace.ToolCalls))
				for _, toolCall := range answer.Trace.ToolCalls {
					names = append(names, toolCall.Name)
				}
				if !slices.Contains(names, "search_web") {
					t.Fatalf("trace tool names = %v, want contains %q", names, "search_web")
				}
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.run(t)
		})
	}
}

func TestRuntimeRegressionSuiteDeterministic(t *testing.T) {
	t.Parallel()

	var chunks strings.Builder
	a := &Agent{
		cfg: Config{
			ChatModel:           "deterministic-model",
			TopK:                5,
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
						Text:       "deterministic evidence",
						StartRune:  0,
						EndRune:    22,
					},
					Score: 0.99,
				},
			},
		},
		embedder: &fakeEmbedder{defaultVec: []float32{1, 0}},
		runner: &fakeStreamingRunner{
			askStreamFn: func(_ context.Context, _ graph.Request, emit graph.StreamEmitter) error {
				if err := emit(graph.Event{Type: graph.EventAnswerChunk, Content: "stable "}); err != nil {
					return err
				}
				if err := emit(graph.Event{Type: graph.EventAnswerChunk, Content: "output"}); err != nil {
					return err
				}
				return emit(graph.Event{Type: graph.EventDone, Step: 2})
			},
		},
		sessions: make(map[string]*Session),
	}

	err := a.GetSession("deterministic").AskStream(context.Background(), "show deterministic trace", func(event StreamEvent) error {
		if event.Type == EventAnswerChunk {
			chunks.WriteString(event.Content)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("AskStream() error = %v", err)
	}
	if chunks.String() != "stable output" {
		t.Fatalf("stream output = %q, want %q", chunks.String(), "stable output")
	}
}
