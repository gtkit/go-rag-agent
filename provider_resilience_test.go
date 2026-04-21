package ragagent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gtkit/go-rag-agent/internal/llm"
)

type flakyChatModel struct {
	generateCalls int
	streamCalls   int
	failTimes     int
	streamFail    bool
	answer        string
	info          map[string]any
}

func (m *flakyChatModel) Generate(context.Context, []llm.Message) (llm.Message, error) {
	m.generateCalls++
	if m.generateCalls <= m.failTimes {
		return llm.Message{}, errors.New("429 rate limit exceeded")
	}
	return llm.Message{
		Role:           llm.RoleAssistant,
		Content:        m.answer,
		GenerationInfo: m.info,
	}, nil
}

func (m *flakyChatModel) Stream(_ context.Context, _ []llm.Message, emit func(string) error) error {
	m.streamCalls++
	if m.streamFail {
		if err := emit("partial"); err != nil {
			return err
		}
		return errors.New("503 provider unavailable")
	}
	return emit(m.answer)
}

func TestProviderGovernanceConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     ProviderGovernanceConfig
		wantErr error
	}{
		{
			name:    "defaults normalize",
			cfg:     ProviderGovernanceConfig{},
			wantErr: nil,
		},
		{
			name: "invalid retry attempts",
			cfg: ProviderGovernanceConfig{
				RetryMaxAttempts: -1,
			},
			wantErr: ErrInvalidConfig,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := tc.cfg.normalized()
			err := Config{
				ChatModel:          "gpt-4o-mini",
				ChatBaseURL:        "https://api.example.com/v1",
				ChatAPIKey:         "key",
				EmbeddingModel:     "text-embedding-3-small",
				ProviderGovernance: cfg,
			}.Validate()
			if tc.wantErr == nil && err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Fatalf("Validate() error = %v, want errors.Is(..., %v)", err, tc.wantErr)
			}
		})
	}
}

func TestResilientChatModelRetriesAndAggregatesUsage(t *testing.T) {
	t.Parallel()

	model := &flakyChatModel{
		failTimes: 1,
		answer:    "answer",
		info: map[string]any{
			"PromptTokens":     100,
			"CompletionTokens": 50,
			"TotalTokens":      150,
		},
	}
	var traces []ProviderCallTrace
	ctx := withProviderTraceObserver(context.Background(), func(call ProviderCallTrace) {
		traces = append(traces, call)
	})
	wrapped := newResilientChatModel(model, ProviderGovernanceConfig{
		RetryMaxAttempts: 2,
		RetryBaseDelay:   time.Millisecond,
		RetryMaxDelay:    5 * time.Millisecond,
		Pricing: ProviderPricingConfig{
			ChatModels: map[string]TokenPricing{
				"gpt-4o-mini": {InputUSDPer1K: 0.01, OutputUSDPer1K: 0.02},
			},
		},
	}, "openai", "gpt-4o-mini")

	msg, err := wrapped.Generate(ctx, []llm.Message{{Role: llm.RoleUser, Content: "hello"}})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if msg.Content != "answer" {
		t.Fatalf("Generate() content = %q, want %q", msg.Content, "answer")
	}
	if model.generateCalls != 2 {
		t.Fatalf("generate calls = %d, want 2", model.generateCalls)
	}
	if len(traces) != 1 {
		t.Fatalf("provider traces len = %d, want 1", len(traces))
	}
	if traces[0].Attempts != 2 {
		t.Fatalf("provider trace attempts = %d, want 2", traces[0].Attempts)
	}
	if traces[0].TotalTokens != 150 {
		t.Fatalf("provider trace total tokens = %d, want 150", traces[0].TotalTokens)
	}
	if traces[0].EstimatedCostUSD <= 0 {
		t.Fatalf("provider trace estimated cost = %f, want positive", traces[0].EstimatedCostUSD)
	}
}

func TestResilientChatModelStreamDoesNotRetryAfterChunk(t *testing.T) {
	t.Parallel()

	model := &flakyChatModel{streamFail: true}
	wrapped := newResilientChatModel(model, ProviderGovernanceConfig{
		RetryMaxAttempts: 2,
		RetryBaseDelay:   time.Millisecond,
		RetryMaxDelay:    5 * time.Millisecond,
	}, "openai", "gpt-4o-mini")

	err := wrapped.Stream(context.Background(), []llm.Message{{Role: llm.RoleUser, Content: "hello"}}, func(string) error { return nil })
	if err == nil {
		t.Fatal("Stream() error = nil, want non-nil")
	}
	if model.streamCalls != 1 {
		t.Fatalf("stream calls = %d, want 1", model.streamCalls)
	}
}

func TestAskExecutionTraceIncludesProviderUsage(t *testing.T) {
	t.Parallel()

	model := &flakyChatModel{
		answer: "answer",
		info: map[string]any{
			"PromptTokens":     100,
			"CompletionTokens": 50,
			"TotalTokens":      150,
		},
	}
	a, err := New(Config{
		ChatModel: "gpt-4o-mini",
		Runtime: RuntimeComponents{
			ChatModel: model,
			Embedder:  stubRuntimeEmbedder{},
		},
		TopK:             1,
		ChunkSize:        32,
		ChunkOverlap:     0,
		MaxHistoryRounds: 8,
		RequestTimeout:   time.Second,
		ProviderGovernance: ProviderGovernanceConfig{
			RetryMaxAttempts: 1,
			RetryBaseDelay:   time.Millisecond,
			RetryMaxDelay:    5 * time.Millisecond,
			Pricing: ProviderPricingConfig{
				ChatModels: map[string]TokenPricing{
					"gpt-4o-mini": {InputUSDPer1K: 0.01, OutputUSDPer1K: 0.02},
				},
			},
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
							Text:       "gateway api exact match",
							StartRune:  0,
							EndRune:    23,
						},
						Score: 0.99,
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() {
		if cerr := a.Close(); cerr != nil {
			t.Fatalf("Close() error = %v", cerr)
		}
	})

	answer, err := a.GetSession("provider-usage").Ask(context.Background(), "gateway api")
	if err != nil {
		t.Fatalf("Ask() error = %v", err)
	}
	if answer.Trace == nil {
		t.Fatal("Answer.Trace = nil")
	}
	if len(answer.Trace.ProviderCalls) == 0 {
		t.Fatal("ProviderCalls len = 0, want > 0")
	}
	call := answer.Trace.ProviderCalls[len(answer.Trace.ProviderCalls)-1]
	if call.TotalTokens != 150 {
		t.Fatalf("ProviderCalls total tokens = %d, want 150", call.TotalTokens)
	}
	if call.EstimatedCostUSD <= 0 {
		t.Fatalf("ProviderCalls estimated cost = %f, want positive", call.EstimatedCostUSD)
	}
}
