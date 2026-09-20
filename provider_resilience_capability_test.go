package ragagent

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/gtkit/go-rag-agent/internal/llm"
)

// flakyCapableModel 前 failures 次调用返回瞬时错误，之后成功，用于验证韧性包装对扩展方法同样重试。
type flakyCapableModel struct {
	mu       sync.Mutex
	failures int
	calls    int
	lastOpts llm.GenerateOptions
}

func (m *flakyCapableModel) attempt(opts llm.GenerateOptions) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	m.lastOpts = opts
	if m.calls <= m.failures {
		return errors.New("upstream timeout")
	}
	return nil
}

func (m *flakyCapableModel) Generate(ctx context.Context, input []llm.Message) (llm.Message, error) {
	return m.GenerateWithOptions(ctx, input, llm.GenerateOptions{})
}

func (m *flakyCapableModel) Stream(ctx context.Context, input []llm.Message, emit func(string) error) error {
	_, err := m.StreamWithOptions(ctx, input, llm.GenerateOptions{}, emit)
	return err
}

func (m *flakyCapableModel) GenerateWithOptions(_ context.Context, _ []llm.Message, opts llm.GenerateOptions) (llm.Message, error) {
	if err := m.attempt(opts); err != nil {
		return llm.Message{}, err
	}
	return llm.Message{
		Role:           llm.RoleAssistant,
		Content:        "ok",
		ToolCalls:      []llm.ToolCall{{ID: "c1", Name: "echo", Arguments: "{}"}},
		GenerationInfo: map[string]any{"PromptTokens": 7, "CompletionTokens": 3, "TotalTokens": 10},
	}, nil
}

func (m *flakyCapableModel) StreamWithOptions(_ context.Context, _ []llm.Message, opts llm.GenerateOptions, emit func(string) error) (llm.Message, error) {
	if err := m.attempt(opts); err != nil {
		return llm.Message{}, err
	}
	if err := emit("ok"); err != nil {
		return llm.Message{}, err
	}
	return llm.Message{
		Role:           llm.RoleAssistant,
		Content:        "ok",
		GenerationInfo: map[string]any{"PromptTokens": 7, "CompletionTokens": 3, "TotalTokens": 10},
	}, nil
}

func (m *flakyCapableModel) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

func TestResilientChatModelPreservesToolCapability(t *testing.T) {
	t.Parallel()

	governance := ProviderGovernanceConfig{}.normalized()
	governor := newProviderGovernors(governance, nil).forProvider("openai")

	t.Run("capable inner model stays capable after wrapping", func(t *testing.T) {
		t.Parallel()
		wrapped := newResilientChatModel(&flakyCapableModel{}, governance, governor, "openai", "m")
		if _, ok := wrapped.(llm.ToolCapableChatModel); !ok {
			t.Fatalf("wrapped model type %T must implement ToolCapableChatModel", wrapped)
		}
	})

	t.Run("plain inner model is not promoted", func(t *testing.T) {
		t.Parallel()
		wrapped := newResilientChatModel(&sequencedRootChatModel{}, governance, governor, "openai", "m")
		if _, ok := wrapped.(llm.ToolCapableChatModel); ok {
			t.Fatalf("wrapped model type %T must not implement ToolCapableChatModel", wrapped)
		}
	})

	t.Run("nil inner model yields nil", func(t *testing.T) {
		t.Parallel()
		if wrapped := newResilientChatModel(nil, governance, governor, "openai", "m"); wrapped != nil {
			t.Fatalf("newResilientChatModel(nil) = %v, want nil", wrapped)
		}
	})
}

func TestResilientToolCapableChatModelRetriesAndTraces(t *testing.T) {
	t.Parallel()

	governance := ProviderGovernanceConfig{RetryMaxAttempts: 3}.normalized()
	opts := llm.GenerateOptions{
		Tools:           []llm.ToolDefinition{{Name: "echo"}},
		ResponseFormat:  &llm.ResponseFormat{Type: llm.ResponseFormatJSONObject},
		ReasoningEffort: "low",
	}

	tests := []struct {
		name          string
		failures      int
		stream        bool
		wantErr       bool
		wantCalls     int
		wantOperation string
	}{
		{name: "generate retries transient failure then succeeds", failures: 1, wantCalls: 2, wantOperation: "chat_generate"},
		{name: "generate gives up after max attempts", failures: 3, wantErr: true, wantCalls: 3, wantOperation: "chat_generate"},
		{name: "stream retries before first chunk", failures: 1, stream: true, wantCalls: 2, wantOperation: "chat_stream"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			inner := &flakyCapableModel{failures: tt.failures}
			governor := newProviderGovernors(governance, nil).forProvider("openai")
			wrapped, ok := newResilientChatModel(inner, governance, governor, "openai", "m").(llm.ToolCapableChatModel)
			if !ok {
				t.Fatal("wrapped model must be tool capable")
			}
			var traces []ProviderCallTrace
			ctx := withProviderTraceObserver(context.Background(), func(call ProviderCallTrace) {
				traces = append(traces, call)
			})

			var (
				msg llm.Message
				err error
			)
			if tt.stream {
				var chunks string
				msg, err = wrapped.StreamWithOptions(ctx, []llm.Message{{Role: llm.RoleUser, Content: "hi"}}, opts, func(chunk string) error {
					chunks += chunk
					return nil
				})
				if err == nil && chunks != "ok" {
					t.Fatalf("chunks = %q, want ok", chunks)
				}
			} else {
				msg, err = wrapped.GenerateWithOptions(ctx, []llm.Message{{Role: llm.RoleUser, Content: "hi"}}, opts)
			}
			if tt.wantErr {
				if err == nil {
					t.Fatal("error = nil, want transient failure after max attempts")
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if inner.callCount() != tt.wantCalls {
				t.Fatalf("inner calls = %d, want %d", inner.callCount(), tt.wantCalls)
			}
			if inner.lastOpts.ReasoningEffort != "low" || inner.lastOpts.ResponseFormat == nil || len(inner.lastOpts.Tools) != 1 {
				t.Fatalf("options not forwarded to inner model: %+v", inner.lastOpts)
			}
			if len(traces) != 1 || traces[0].Operation != tt.wantOperation || traces[0].Attempts != tt.wantCalls {
				t.Fatalf("provider traces = %+v", traces)
			}
			if !tt.wantErr {
				if traces[0].InputTokens != 7 || traces[0].OutputTokens != 3 || traces[0].TotalTokens != 10 {
					t.Fatalf("usage not recorded in trace: %+v", traces[0])
				}
				if !tt.stream && (len(msg.ToolCalls) != 1 || msg.ToolCalls[0].Name != "echo") {
					t.Fatalf("tool calls lost through wrapper: %+v", msg)
				}
			}
		})
	}
}

func TestResilientToolCapableChatModelStreamUsesOptionsPath(t *testing.T) {
	t.Parallel()

	inner := &flakyCapableModel{}
	governance := ProviderGovernanceConfig{}.normalized()
	wrapped := newResilientChatModel(inner, governance, newProviderGovernors(governance, nil).forProvider("openai"), "openai", "m")
	var traces []ProviderCallTrace
	ctx := withProviderTraceObserver(context.Background(), func(call ProviderCallTrace) { traces = append(traces, call) })
	if err := wrapped.Stream(ctx, []llm.Message{{Role: llm.RoleUser, Content: "hi"}}, func(string) error { return nil }); err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if len(traces) != 1 || traces[0].TotalTokens != 10 {
		t.Fatalf("plain Stream on a capable model must record usage from StreamWithOptions: %+v", traces)
	}
}
