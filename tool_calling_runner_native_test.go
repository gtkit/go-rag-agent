package ragagent

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/gtkit/go-rag-agent/internal/graph"
	"github.com/gtkit/go-rag-agent/internal/memory"
)

// scriptedCapableModel 按脚本逐轮返回消息，实现 ToolCapableChatModel，记录每轮收到的消息与选项。
type scriptedCapableModel struct {
	mu     sync.Mutex
	rounds []Message
	err    error
	calls  [][]Message
	opts   []GenerateOptions
}

func (m *scriptedCapableModel) Generate(ctx context.Context, input []Message) (Message, error) {
	return m.GenerateWithOptions(ctx, input, GenerateOptions{})
}

func (m *scriptedCapableModel) Stream(ctx context.Context, input []Message, emit func(string) error) error {
	_, err := m.StreamWithOptions(ctx, input, GenerateOptions{}, emit)
	return err
}

func (m *scriptedCapableModel) GenerateWithOptions(ctx context.Context, input []Message, opts GenerateOptions) (Message, error) {
	if err := ctx.Err(); err != nil {
		return Message{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, append([]Message(nil), input...))
	m.opts = append(m.opts, opts)
	if m.err != nil {
		return Message{}, m.err
	}
	if len(m.rounds) == 0 {
		return Message{}, errors.New("scripted model exhausted")
	}
	round := m.rounds[0]
	m.rounds = m.rounds[1:]
	return round, nil
}

// StreamWithOptions 按空格切片流式发出文本，模拟逐 token 输出。
func (m *scriptedCapableModel) StreamWithOptions(ctx context.Context, input []Message, opts GenerateOptions, emit func(string) error) (Message, error) {
	msg, err := m.GenerateWithOptions(ctx, input, opts)
	if err != nil {
		return Message{}, err
	}
	for _, part := range strings.SplitAfter(msg.Content, " ") {
		if part == "" {
			continue
		}
		if err := emit(part); err != nil {
			return Message{}, err
		}
	}
	return msg, nil
}

type echoTool struct {
	mu     sync.Mutex
	inputs []string
	err    error
}

func (t *echoTool) Name() string        { return "echo" }
func (t *echoTool) Description() string { return "echo the input" }

func (t *echoTool) Run(_ context.Context, input string) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.inputs = append(t.inputs, input)
	if t.err != nil {
		return "", t.err
	}
	return "echoed:" + input, nil
}

func (t *echoTool) recorded() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]string(nil), t.inputs...)
}

type denyPolicy struct{ deny string }

func (p denyPolicy) AuthorizeToolCall(_ context.Context, call ToolInvocation) error {
	if call.Name == p.deny {
		return errors.New("not allowed")
	}
	return nil
}

type recordingObserver struct {
	mu     sync.Mutex
	events []string
	endErr error
}

func (o *recordingObserver) OnToolStart(_ context.Context, tool string) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.events = append(o.events, "start:"+tool)
	return nil
}

func (o *recordingObserver) OnToolEnd(_ context.Context, tool string, err error) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	status := "ok"
	if err != nil {
		status = "err"
	}
	o.events = append(o.events, "end:"+tool+":"+status)
	return o.endErr
}

func (o *recordingObserver) joined() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return strings.Join(o.events, ",")
}

func toolMessages(messages []Message) map[string]string {
	out := map[string]string{}
	for _, msg := range messages {
		if msg.Role == RoleTool {
			out[msg.ToolCallID] = msg.Content
		}
	}
	return out
}

func TestToolCallingRunnerNativeLoop(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		rounds        []Message
		cfg           ToolCallingRunnerConfig
		wantAnswer    string
		wantErr       error
		wantEvents    string
		wantEchoInput []string
		wantToolMsgs  map[string]string
		wantRounds    int
	}{
		{
			name: "answers directly without tools",
			rounds: []Message{
				{Role: RoleAssistant, Content: "direct answer"},
			},
			cfg:        ToolCallingRunnerConfig{MaxToolCalls: 2},
			wantAnswer: "direct answer",
			wantRounds: 1,
		},
		{
			name: "executes multiple tool calls and feeds results back as tool messages",
			rounds: []Message{
				{Role: RoleAssistant, Content: "Let me check.", ToolCalls: []ToolCall{
					{ID: "c1", Name: "search_docs", Arguments: `{"query":"rag"}`},
					{ID: "c2", Name: "echo", Arguments: `{"input":"hi"}`},
				}},
				{Role: RoleAssistant, Content: "final answer"},
			},
			cfg:           ToolCallingRunnerConfig{MaxToolCalls: 4},
			wantAnswer:    "Let me check.\nfinal answer",
			wantEvents:    "start:search_docs,end:search_docs:ok,start:echo,end:echo:ok",
			wantEchoInput: []string{"hi"},
			wantToolMsgs:  map[string]string{"c1": "tool evidence for rag", "c2": "echoed:hi"},
			wantRounds:    2,
		},
		{
			name: "unknown tool and invalid arguments are reported back to the model",
			rounds: []Message{
				{Role: RoleAssistant, ToolCalls: []ToolCall{
					{ID: "c1", Name: "missing", Arguments: `{}`},
					{ID: "c2", Name: "search_docs", Arguments: `{}`},
					{ID: "c3", Name: "search_docs", Arguments: `not json`},
				}},
				{Role: RoleAssistant, Content: "recovered"},
			},
			cfg:        ToolCallingRunnerConfig{MaxToolCalls: 4},
			wantAnswer: "recovered",
			wantEvents: "end:missing:err,end:search_docs:err,end:search_docs:err",
			wantToolMsgs: map[string]string{
				"c1": `{"error":"ragagent: tool call invalid: unknown tool \"missing\""}`,
				"c2": `{"error":"ragagent: tool argument invalid: required argument \"query\" is missing"}`,
			},
			wantRounds: 2,
		},
		{
			name: "structured tool with an input field still receives JSON arguments and its failure is sanitized",
			rounds: []Message{
				{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "c1", Name: "failing", Arguments: `{"input":"x"}`}}},
				{Role: RoleAssistant, Content: "handled"},
			},
			cfg:          ToolCallingRunnerConfig{MaxToolCalls: 4},
			wantAnswer:   "handled",
			wantEvents:   "start:failing,end:failing:err",
			wantToolMsgs: map[string]string{"c1": `{"error":"tool execution failed"}`},
			wantRounds:   2,
		},
		{
			name: "non-input arguments reach a plain tool as JSON text",
			rounds: []Message{
				{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "c1", Name: "echo", Arguments: `{"a":1,"b":"x"}`}}},
				{Role: RoleAssistant, Content: "done"},
			},
			cfg:           ToolCallingRunnerConfig{MaxToolCalls: 4},
			wantAnswer:    "done",
			wantEvents:    "start:echo,end:echo:ok",
			wantEchoInput: []string{`{"a":1,"b":"x"}`},
			wantRounds:    2,
		},
		{
			name: "policy denial aborts the run",
			rounds: []Message{
				{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "c1", Name: "echo", Arguments: `{"input":"x"}`}}},
			},
			cfg:        ToolCallingRunnerConfig{MaxToolCalls: 4, Policy: denyPolicy{deny: "echo"}},
			wantErr:    ErrToolCallDenied,
			wantEvents: "end:echo:err",
			wantRounds: 1,
		},
		{
			name: "tool call budget is enforced across one response",
			rounds: []Message{
				{Role: RoleAssistant, ToolCalls: []ToolCall{
					{ID: "c1", Name: "echo", Arguments: `{"input":"a"}`},
					{ID: "c2", Name: "echo", Arguments: `{"input":"b"}`},
				}},
			},
			cfg:           ToolCallingRunnerConfig{MaxToolCalls: 1},
			wantErr:       ErrToolCallLimitExceeded,
			wantEvents:    "start:echo,end:echo:ok",
			wantEchoInput: []string{"a"},
			wantRounds:    1,
		},
		{
			name: "iteration budget is enforced",
			rounds: []Message{
				{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "c1", Name: "echo", Arguments: `{"input":"a"}`}}},
				{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "c2", Name: "echo", Arguments: `{"input":"b"}`}}},
			},
			cfg:           ToolCallingRunnerConfig{MaxToolCalls: 4, MaxIterations: 2},
			wantErr:       ErrToolCallLimitExceeded,
			wantEvents:    "start:echo,end:echo:ok,start:echo,end:echo:ok",
			wantEchoInput: []string{"a", "b"},
			wantRounds:    2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			model := &scriptedCapableModel{rounds: tt.rounds}
			echo := &echoTool{}
			failing := structuredToolFunc{
				name: "failing", description: "always fails",
				schema: ToolSchema{Properties: map[string]ToolParameterSchema{"input": {Type: ToolParameterString}}},
				run: func(context.Context, map[string]any) (ToolResult, error) {
					return ToolResult{}, errors.New("db connection refused: 10.0.0.1")
				},
			}
			registry := NewToolRegistry(NewStructuredToolAdapter(testSearchDocsTool()), echo, NewStructuredToolAdapter(failing))
			runner, err := NewToolCallingRunner(model, registry, tt.cfg)
			if err != nil {
				t.Fatalf("NewToolCallingRunner() error = %v", err)
			}
			observer := &recordingObserver{}
			answer, err := runner.Ask(context.Background(), graph.Request{Query: "q", EvidenceText: "e", ToolObserver: observer})
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Ask() error = %v, want %v", err, tt.wantErr)
				}
			} else if err != nil {
				t.Fatalf("Ask() error = %v", err)
			}
			if answer != tt.wantAnswer {
				t.Fatalf("answer = %q, want %q", answer, tt.wantAnswer)
			}
			if got := observer.joined(); got != tt.wantEvents {
				t.Fatalf("observer events = %q, want %q", got, tt.wantEvents)
			}
			if got := echo.recorded(); strings.Join(got, "|") != strings.Join(tt.wantEchoInput, "|") {
				t.Fatalf("echo inputs = %v, want %v", got, tt.wantEchoInput)
			}
			if len(model.calls) != tt.wantRounds {
				t.Fatalf("model rounds = %d, want %d", len(model.calls), tt.wantRounds)
			}
			if tt.wantToolMsgs != nil {
				got := toolMessages(model.calls[len(model.calls)-1])
				for id, want := range tt.wantToolMsgs {
					if got[id] != want {
						t.Fatalf("tool message %s = %q, want %q", id, got[id], want)
					}
				}
			}
			for _, opts := range model.opts {
				if len(opts.Tools) != 3 || opts.Tools[1].Name != "echo" {
					t.Fatalf("tool definitions not sent: %+v", opts.Tools)
				}
			}
		})
	}
}

func TestToolCallingRunnerNativeToolDefinitions(t *testing.T) {
	t.Parallel()

	model := &scriptedCapableModel{rounds: []Message{{Role: RoleAssistant, Content: "ok"}}}
	runner, err := NewToolCallingRunner(model, NewToolRegistry(NewStructuredToolAdapter(testSearchDocsTool()), &echoTool{}), ToolCallingRunnerConfig{})
	if err != nil {
		t.Fatalf("NewToolCallingRunner() error = %v", err)
	}
	format := &ResponseFormat{Type: ResponseFormatJSONObject}
	if _, err := runner.Ask(context.Background(), graph.Request{Query: "q", ResponseFormat: format, ReasoningEffort: "high", ResponseFormatInstruction: "Return only valid JSON."}); err != nil {
		t.Fatalf("Ask() error = %v", err)
	}
	opts := model.opts[0]
	if opts.ResponseFormat != format || opts.ReasoningEffort != "high" {
		t.Fatalf("options not forwarded: %+v", opts)
	}
	structured, ok := opts.Tools[0].Parameters.(ToolSchema)
	if !ok || structured.Required[0] != "query" {
		t.Fatalf("structured tool schema = %#v", opts.Tools[0].Parameters)
	}
	plain, ok := opts.Tools[1].Parameters.(ToolSchema)
	if !ok || plain.Properties["input"].Type != ToolParameterString || plain.Required[0] != "input" {
		t.Fatalf("plain tool schema = %#v", opts.Tools[1].Parameters)
	}
	if !messagesContain(model.calls[0], "Return only valid JSON.") {
		t.Fatalf("structured instruction missing from prompt: %+v", model.calls[0])
	}
	for _, msg := range model.calls[0] {
		if strings.Contains(msg.Content, "tool_call") {
			t.Fatalf("native prompt must not describe the envelope protocol: %q", msg.Content)
		}
	}
}

func TestToolCallingRunnerNativeStream(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		rounds     func() []Message
		emitErr    error
		wantChunks string
		wantErr    bool
	}{
		{
			name: "streams every round and separates rounds with newline",
			rounds: func() []Message {
				return []Message{
					{Role: RoleAssistant, Content: "Checking now.", ToolCalls: []ToolCall{{ID: "c1", Name: "echo", Arguments: `{"input":"x"}`}}},
					{Role: RoleAssistant, Content: "done answer"},
				}
			},
			wantChunks: "Checking now.\ndone answer",
		},
		{
			name: "tool-only round emits no separator",
			rounds: func() []Message {
				return []Message{
					{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "c1", Name: "echo", Arguments: `{"input":"x"}`}}},
					{Role: RoleAssistant, Content: "final"},
				}
			},
			wantChunks: "final",
		},
		{
			name:    "emitter error stops the stream",
			rounds:  func() []Message { return []Message{{Role: RoleAssistant, Content: "partial answer"}} },
			emitErr: errors.New("client gone"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runner, err := NewToolCallingRunner(&scriptedCapableModel{rounds: tt.rounds()}, NewToolRegistry(&echoTool{}), ToolCallingRunnerConfig{MaxToolCalls: 4})
			if err != nil {
				t.Fatalf("NewToolCallingRunner() error = %v", err)
			}
			var chunks strings.Builder
			var last graph.Event
			err = runner.AskStream(context.Background(), graph.Request{Query: "q"}, func(event graph.Event) error {
				last = event
				if event.Type == graph.EventAnswerChunk {
					chunks.WriteString(event.Content)
					return tt.emitErr
				}
				return nil
			})
			if tt.wantErr {
				if !errors.Is(err, tt.emitErr) {
					t.Fatalf("AskStream() error = %v, want %v", err, tt.emitErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("AskStream() error = %v", err)
			}
			if chunks.String() != tt.wantChunks {
				t.Fatalf("chunks = %q, want %q", chunks.String(), tt.wantChunks)
			}
			if last.Type != graph.EventDone {
				t.Fatalf("last event = %+v, want done", last)
			}

			// 同步路径必须返回与流式拼接一致的答案。
			syncRunner, _ := NewToolCallingRunner(&scriptedCapableModel{rounds: tt.rounds()}, NewToolRegistry(&echoTool{}), ToolCallingRunnerConfig{MaxToolCalls: 4})
			answer, err := syncRunner.Ask(context.Background(), graph.Request{Query: "q"})
			if err != nil || answer != tt.wantChunks {
				t.Fatalf("Ask() = %q, %v; want %q", answer, err, tt.wantChunks)
			}
		})
	}
}

func TestToolCallingRunnerNativeContextAndObserverErrors(t *testing.T) {
	t.Parallel()

	t.Run("canceled context stops before the model is called", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		model := &scriptedCapableModel{rounds: []Message{{Role: RoleAssistant, Content: "never"}}}
		runner, _ := NewToolCallingRunner(model, NewToolRegistry(&echoTool{}), ToolCallingRunnerConfig{})
		if _, err := runner.Ask(ctx, graph.Request{Query: "q"}); !errors.Is(err, context.Canceled) {
			t.Fatalf("Ask() error = %v, want context.Canceled", err)
		}
		if len(model.calls) != 0 {
			t.Fatalf("model must not be called after cancel")
		}
	})

	t.Run("observer end error is fatal even for recoverable tool failures", func(t *testing.T) {
		t.Parallel()
		observerErr := errors.New("stream closed")
		model := &scriptedCapableModel{rounds: []Message{
			{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "c1", Name: "missing", Arguments: `{}`}}},
			{Role: RoleAssistant, Content: "never"},
		}}
		runner, _ := NewToolCallingRunner(model, NewToolRegistry(&echoTool{}), ToolCallingRunnerConfig{MaxToolCalls: 2})
		_, err := runner.Ask(context.Background(), graph.Request{Query: "q", ToolObserver: &recordingObserver{endErr: observerErr}})
		if !errors.Is(err, observerErr) {
			t.Fatalf("Ask() error = %v, want observer error", err)
		}
		if len(model.calls) != 1 {
			t.Fatalf("loop must stop after observer error, rounds = %d", len(model.calls))
		}
	})

	t.Run("model error is wrapped", func(t *testing.T) {
		t.Parallel()
		modelErr := errors.New("upstream 500")
		runner, _ := NewToolCallingRunner(&scriptedCapableModel{err: modelErr}, NewToolRegistry(&echoTool{}), ToolCallingRunnerConfig{})
		if _, err := runner.Ask(context.Background(), graph.Request{Query: "q"}); !errors.Is(err, modelErr) {
			t.Fatalf("Ask() error = %v, want wrapped model error", err)
		}
	})
}

func TestToolCallingRunnerEnvelopePlainToolInputAndPolicy(t *testing.T) {
	t.Parallel()

	t.Run("input field is passed verbatim to plain tools", func(t *testing.T) {
		t.Parallel()
		echo := &echoTool{}
		model := &sequencedRootChatModel{responses: []string{`{"tool_call":{"name":"echo","arguments":{"input":"plain text"}}}`, "ok"}}
		runner, _ := NewToolCallingRunner(model, NewToolRegistry(echo), ToolCallingRunnerConfig{MaxToolCalls: 2})
		answer, err := runner.Ask(context.Background(), graph.Request{Query: "q"})
		if err != nil || answer != "ok" {
			t.Fatalf("Ask() = %q, %v", answer, err)
		}
		if got := echo.recorded(); len(got) != 1 || got[0] != "plain text" {
			t.Fatalf("echo inputs = %v", got)
		}
		if !messagesContain(model.calls[0], `"input"`) {
			t.Fatalf("envelope tool description must declare the input schema: %+v", model.calls[0])
		}
	})

	t.Run("policy denial applies to the envelope protocol too", func(t *testing.T) {
		t.Parallel()
		model := &sequencedRootChatModel{responses: []string{`{"tool_call":{"name":"echo","arguments":{"input":"x"}}}`}}
		runner, _ := NewToolCallingRunner(model, NewToolRegistry(&echoTool{}), ToolCallingRunnerConfig{MaxToolCalls: 2, Policy: denyPolicy{deny: "echo"}})
		if _, err := runner.Ask(context.Background(), graph.Request{Query: "q"}); !errors.Is(err, ErrToolCallDenied) {
			t.Fatalf("Ask() error = %v, want ErrToolCallDenied", err)
		}
	})

	t.Run("history and structured instruction enter the envelope prompt", func(t *testing.T) {
		t.Parallel()
		model := &sequencedRootChatModel{responses: []string{"ok"}}
		runner, _ := NewToolCallingRunner(model, NewToolRegistry(&echoTool{}), ToolCallingRunnerConfig{})
		_, err := runner.Ask(context.Background(), graph.Request{
			Query:                     "follow up",
			History:                   []memory.Turn{{User: "earlier question", Assistant: "earlier answer"}},
			ResponseFormatInstruction: "Return only valid JSON.",
		})
		if err != nil {
			t.Fatalf("Ask() error = %v", err)
		}
		prompt := model.calls[0]
		if !messagesContain(prompt, "earlier question") || !messagesContain(prompt, "Return only valid JSON.") {
			t.Fatalf("prompt missing history or format instruction: %+v", prompt)
		}
		if prompt[len(prompt)-1].Content != "follow up" {
			t.Fatalf("query must stay the last message, got %q", prompt[len(prompt)-1].Content)
		}
	})
}
