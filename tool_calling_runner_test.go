package ragagent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gtkit/go-rag-agent/internal/graph"
	"github.com/gtkit/go-rag-agent/internal/llm"
)

type sequencedRootChatModel struct {
	responses []string
	calls     [][]Message
	err       error
}

func (m *sequencedRootChatModel) Generate(ctx context.Context, input []Message) (Message, error) {
	if err := ctx.Err(); err != nil {
		return Message{}, err
	}
	m.calls = append(m.calls, append([]Message(nil), input...))
	if m.err != nil {
		return Message{}, m.err
	}
	if len(m.responses) == 0 {
		return Message{}, errors.New("no response")
	}
	resp := m.responses[0]
	m.responses = m.responses[1:]
	return Message{Role: RoleAssistant, Content: resp}, nil
}

func (m *sequencedRootChatModel) Stream(ctx context.Context, input []Message, emit func(string) error) error {
	msg, err := m.Generate(ctx, input)
	if err != nil {
		return err
	}
	return emit(msg.Content)
}

func TestToolCallingRunnerExecutesToolAndReturnsFinalAnswer(t *testing.T) {
	t.Parallel()

	model := &sequencedRootChatModel{
		responses: []string{
			`{"tool_call":{"name":"search_docs","arguments":{"query":"rag"}}}`,
			`RAG means retrieval augmented generation.`,
		},
	}
	tool := structuredToolFunc{
		name:        "search_docs",
		description: "search docs",
		schema: ToolSchema{
			Properties: map[string]ToolParameterSchema{"query": {Type: ToolParameterString, MinLength: 1}},
			Required:   []string{"query"},
		},
		run: func(_ context.Context, args map[string]any) (ToolResult, error) {
			return ToolResult{Text: "tool evidence for " + args["query"].(string)}, nil
		},
	}
	runner, err := NewToolCallingRunner(model, NewToolRegistry(NewStructuredToolAdapter(tool)), ToolCallingRunnerConfig{
		MaxToolCalls:  2,
		MaxIterations: 3,
	})
	if err != nil {
		t.Fatalf("NewToolCallingRunner() error = %v", err)
	}

	var observed []string
	answer, err := runner.Ask(context.Background(), graph.Request{
		Query:        "what is rag?",
		EvidenceText: "local evidence",
		ToolObserver: graphToolObserver{
			onStart: func(_ context.Context, tool string) error {
				observed = append(observed, "start:"+tool)
				return nil
			},
			onEnd: func(_ context.Context, tool string, err error) error {
				observed = append(observed, "end:"+tool)
				return nil
			},
		},
	})
	if err != nil {
		t.Fatalf("Ask() error = %v", err)
	}
	if answer != "RAG means retrieval augmented generation." {
		t.Fatalf("answer = %q", answer)
	}
	if got, want := strings.Join(observed, ","), "start:search_docs,end:search_docs"; got != want {
		t.Fatalf("observed = %q, want %q", got, want)
	}
	if len(model.calls) != 2 {
		t.Fatalf("model calls = %d, want 2", len(model.calls))
	}
	if !messagesContain(model.calls[1], "tool evidence for rag") {
		t.Fatalf("second model call did not include tool result: %#v", model.calls[1])
	}
}

func TestToolCallingRunnerFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		responses []string
		cfg       ToolCallingRunnerConfig
		wantErr   error
		wantSub   string
	}{
		{
			name:      "unknown tool",
			responses: []string{`{"tool_call":{"name":"missing","arguments":{}}}`},
			cfg:       ToolCallingRunnerConfig{MaxToolCalls: 2, MaxIterations: 3},
			wantErr:   ErrToolCallInvalid,
			wantSub:   "missing",
		},
		{
			name:      "invalid arguments",
			responses: []string{`{"tool_call":{"name":"search_docs","arguments":{}}}`},
			cfg:       ToolCallingRunnerConfig{MaxToolCalls: 2, MaxIterations: 3},
			wantErr:   ErrToolArgumentInvalid,
			wantSub:   "query",
		},
		{
			name:      "tool call budget exceeded",
			responses: []string{`{"tool_call":{"name":"search_docs","arguments":{"query":"rag"}}}`},
			cfg:       ToolCallingRunnerConfig{MaxToolCalls: 0, MaxIterations: 3},
			wantErr:   ErrToolCallLimitExceeded,
			wantSub:   "limit",
		},
		{
			name:      "iteration budget exceeded",
			responses: []string{`{"tool_call":{"name":"search_docs","arguments":{"query":"rag"}}}`, `{"tool_call":{"name":"search_docs","arguments":{"query":"rag"}}}`},
			cfg:       ToolCallingRunnerConfig{MaxToolCalls: 4, MaxIterations: 1},
			wantErr:   ErrToolCallLimitExceeded,
			wantSub:   "iterations",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			model := &sequencedRootChatModel{responses: tt.responses}
			runner, err := NewToolCallingRunner(model, NewToolRegistry(NewStructuredToolAdapter(testSearchDocsTool())), tt.cfg)
			if err != nil {
				t.Fatalf("NewToolCallingRunner() error = %v", err)
			}
			_, err = runner.Ask(context.Background(), graph.Request{Query: "q", EvidenceText: "e"})
			if err == nil {
				t.Fatal("Ask() error = nil, want non-nil")
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Ask() error = %v, want errors.Is %v", err, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantSub) {
				t.Fatalf("Ask() error = %v, want containing %q", err, tt.wantSub)
			}
		})
	}
}

func TestToolCallingRunnerContextCancel(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runner, err := NewToolCallingRunner(&sequencedRootChatModel{}, NewToolRegistry(NewStructuredToolAdapter(testSearchDocsTool())), ToolCallingRunnerConfig{
		MaxToolCalls:  2,
		MaxIterations: 3,
	})
	if err != nil {
		t.Fatalf("NewToolCallingRunner() error = %v", err)
	}

	_, err = runner.Ask(ctx, graph.Request{Query: "q", EvidenceText: "e"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Ask() error = %v, want context.Canceled", err)
	}
}

func TestToolCallingRunnerAskStreamAndParseEnvelope(t *testing.T) {
	t.Parallel()

	model := &sequencedRootChatModel{responses: []string{`{"final":"streamed answer"}`}}
	runner, err := NewToolCallingRunner(model, NewToolRegistry(NewStructuredToolAdapter(testSearchDocsTool())), ToolCallingRunnerConfig{})
	if err != nil {
		t.Fatalf("NewToolCallingRunner() error = %v", err)
	}
	var events []graph.Event
	if err := runner.AskStream(context.Background(), graph.Request{Query: "q"}, func(event graph.Event) error {
		events = append(events, event)
		return nil
	}); err != nil {
		t.Fatalf("AskStream() error = %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events len = %d, want 2", len(events))
	}
	if events[0].Type != graph.EventAnswerChunk || events[0].Content != "streamed answer" {
		t.Fatalf("first event = %+v, want answer chunk", events[0])
	}
	if events[1].Type != graph.EventDone {
		t.Fatalf("second event = %+v, want done", events[1])
	}

	if err := runner.AskStream(context.Background(), graph.Request{Query: "q"}, nil); err == nil {
		t.Fatal("AskStream(nil emit) error = nil, want non-nil")
	}

	_, _, ok, err := parseToolCallMessage(`{"noop":true}`)
	if err != nil {
		t.Fatalf("parseToolCallMessage(noop) error = %v", err)
	}
	if ok {
		t.Fatal("parseToolCallMessage(noop) ok = true, want false")
	}
	_, _, _, err = parseToolCallMessage(`{"tool_call":`)
	if !errors.Is(err, ErrToolCallInvalid) {
		t.Fatalf("parseToolCallMessage(invalid) error = %v, want ErrToolCallInvalid", err)
	}
}

func TestToolCallingRunnerConstructionValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		model   ChatModel
		tools   *ToolRegistry
		cfg     ToolCallingRunnerConfig
		wantErr bool
	}{
		{name: "missing model", tools: NewToolRegistry(NewStructuredToolAdapter(testSearchDocsTool())), wantErr: true},
		{name: "missing tools", model: &sequencedRootChatModel{}, wantErr: true},
		{name: "negative iterations", model: &sequencedRootChatModel{}, tools: NewToolRegistry(NewStructuredToolAdapter(testSearchDocsTool())), cfg: ToolCallingRunnerConfig{MaxIterations: -1}, wantErr: true},
		{name: "negative tool calls", model: &sequencedRootChatModel{}, tools: NewToolRegistry(NewStructuredToolAdapter(testSearchDocsTool())), cfg: ToolCallingRunnerConfig{MaxToolCalls: -1}, wantErr: true},
		{name: "valid defaults", model: &sequencedRootChatModel{}, tools: NewToolRegistry(NewStructuredToolAdapter(testSearchDocsTool()))},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runner, err := NewToolCallingRunner(tt.model, tt.tools, tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Fatalf("NewToolCallingRunner() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && runner.cfg.MaxIterations != 3 {
				t.Fatalf("default MaxIterations = %d, want 3", runner.cfg.MaxIterations)
			}
		})
	}
}

func testSearchDocsTool() StructuredTool {
	return structuredToolFunc{
		name:        "search_docs",
		description: "search docs",
		schema: ToolSchema{
			Properties: map[string]ToolParameterSchema{"query": {Type: ToolParameterString, MinLength: 1}},
			Required:   []string{"query"},
		},
		run: func(_ context.Context, args map[string]any) (ToolResult, error) {
			return ToolResult{Text: "tool evidence for " + args["query"].(string)}, nil
		},
	}
}

func messagesContain(messages []llm.Message, needle string) bool {
	for _, msg := range messages {
		if strings.Contains(msg.Content, needle) {
			return true
		}
	}
	return false
}

var _ graph.Runner = (*ToolCallingRunner)(nil)
