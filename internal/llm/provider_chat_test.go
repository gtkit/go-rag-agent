package llm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	provider "github.com/gtkit/go-llm-provider/v2/provider"
)

// toolCallServer 记录最近一次请求体，非流式返回 tool_calls 响应，流式按增量分片返回文本与 tool call。
type toolCallServer struct {
	mu   sync.Mutex
	last map[string]any
}

func (s *toolCallServer) lastRequest() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.last
}

func newToolCallServer(t *testing.T) (*httptest.Server, *toolCallServer) {
	t.Helper()
	recorder := &toolCallServer{}
	mux := http.NewServeMux()
	mux.HandleFunc("/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		_ = json.Unmarshal(body, &req)
		recorder.mu.Lock()
		recorder.last = req
		recorder.mu.Unlock()

		if req["stream"] == true {
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, _ := w.(http.Flusher)
			chunks := []string{
				`{"choices":[{"index":0,"delta":{"content":"Let me "}}]}`,
				`{"choices":[{"index":0,"delta":{"content":"check."}}]}`,
				`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"echo","arguments":"{\"in"}}]}}]}`,
				`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"put\":\"x\"}"}}]}}]}`,
				`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"id":"call_2","type":"function","function":{"name":"search","arguments":"{}"}}]}}]}`,
				`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
			}
			for _, chunk := range chunks {
				_, _ = w.Write([]byte("data: " + chunk + "\n\n"))
				if flusher != nil {
					flusher.Flush()
				}
			}
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"echo","arguments":"{\"input\":\"x\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15,"completion_tokens_details":{"reasoning_tokens":2}}}`))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server, recorder
}

func newTestProviderChatModel(t *testing.T, baseURL string) *ProviderChatModel {
	t.Helper()
	model, err := NewOpenAIChatModel(context.Background(), ChatConfig{Model: "test-chat", BaseURL: baseURL, APIKey: "k", Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("NewOpenAIChatModel() error = %v", err)
	}
	return model
}

func TestNewProviderChatModelAndEmbedderRejectNil(t *testing.T) {
	t.Parallel()

	if _, err := NewProviderChatModel(nil, "m"); err == nil {
		t.Fatal("NewProviderChatModel(nil) error = nil, want error")
	}
	if _, err := NewProviderEmbedder(nil); err == nil {
		t.Fatal("NewProviderEmbedder(nil) error = nil, want error")
	}
	var typed *ProviderChatModel
	if _, err := typed.GenerateWithOptions(context.Background(), nil, GenerateOptions{}); err == nil || !strings.Contains(err.Error(), "provider is nil") {
		t.Fatalf("nil receiver error = %v", err)
	}
}

func TestProviderChatModelGenerateWithOptionsSendsNativeProtocol(t *testing.T) {
	t.Parallel()

	server, recorder := newToolCallServer(t)
	model := newTestProviderChatModel(t, server.URL)

	input := []Message{
		{Role: RoleSystem, Content: "sys"},
		{Role: RoleUser, Content: "hi"},
		{Role: RoleAssistant, Content: "", ToolCalls: []ToolCall{{ID: "prev_1", Name: "echo", Arguments: `{"input":"a"}`}}},
		{Role: RoleTool, ToolCallID: "prev_1", Content: "echoed:a"},
	}
	opts := GenerateOptions{
		Tools:           []ToolDefinition{{Name: "echo", Description: "echo input", Parameters: map[string]any{"type": "object", "properties": map[string]any{"input": map[string]any{"type": "string"}}}}, {Name: "noargs"}},
		ResponseFormat:  &ResponseFormat{Type: ResponseFormatJSONObject},
		ReasoningEffort: "low",
	}
	msg, err := model.GenerateWithOptions(context.Background(), input, opts)
	if err != nil {
		t.Fatalf("GenerateWithOptions() error = %v", err)
	}
	if len(msg.ToolCalls) != 1 || msg.ToolCalls[0].ID != "call_1" || msg.ToolCalls[0].Name != "echo" || msg.ToolCalls[0].Arguments != `{"input":"x"}` {
		t.Fatalf("tool calls = %+v", msg.ToolCalls)
	}
	if msg.GenerationInfo["ReasoningTokens"] != 2 || msg.GenerationInfo["TotalTokens"] != 15 {
		t.Fatalf("generation info = %v", msg.GenerationInfo)
	}

	req := recorder.lastRequest()
	tools := req["tools"].([]any)
	if len(tools) != 2 {
		t.Fatalf("tools = %v", tools)
	}
	first := tools[0].(map[string]any)["function"].(map[string]any)
	if first["name"] != "echo" || first["description"] != "echo input" {
		t.Fatalf("first tool = %v", first)
	}
	second := tools[1].(map[string]any)["function"].(map[string]any)
	if params, _ := second["parameters"].(map[string]any); params["type"] != "object" {
		t.Fatalf("tool without parameters must send an empty object schema, got %v", second["parameters"])
	}
	if format := req["response_format"].(map[string]any); format["type"] != "json_object" {
		t.Fatalf("response_format = %v", format)
	}
	if req["reasoning_effort"] != "low" {
		t.Fatalf("reasoning_effort = %v", req["reasoning_effort"])
	}
	messages := req["messages"].([]any)
	if len(messages) != 4 {
		t.Fatalf("messages = %v", messages)
	}
	assistant := messages[2].(map[string]any)
	if assistant["role"] != "assistant" || len(assistant["tool_calls"].([]any)) != 1 {
		t.Fatalf("assistant message must carry tool_calls: %v", assistant)
	}
	tool := messages[3].(map[string]any)
	if tool["role"] != "tool" || tool["tool_call_id"] != "prev_1" {
		t.Fatalf("tool message = %v", tool)
	}
}

func TestProviderChatModelStreamWithOptionsAccumulatesToolCalls(t *testing.T) {
	t.Parallel()

	server, recorder := newToolCallServer(t)
	model := newTestProviderChatModel(t, server.URL)

	var chunks []string
	msg, err := model.StreamWithOptions(context.Background(), []Message{{Role: RoleUser, Content: "hi"}}, GenerateOptions{
		Tools:          []ToolDefinition{{Name: "echo"}},
		ResponseFormat: &ResponseFormat{Type: ResponseFormatJSONSchema, Name: "Out", Schema: map[string]any{"type": "object"}},
	}, func(chunk string) error {
		chunks = append(chunks, chunk)
		return nil
	})
	if err != nil {
		t.Fatalf("StreamWithOptions() error = %v", err)
	}
	if strings.Join(chunks, "|") != "Let me |check." || msg.Content != "Let me check." {
		t.Fatalf("chunks = %v content = %q", chunks, msg.Content)
	}
	if len(msg.ToolCalls) != 2 {
		t.Fatalf("tool calls = %+v, want 2 accumulated by index", msg.ToolCalls)
	}
	if msg.ToolCalls[0] != (ToolCall{ID: "call_1", Name: "echo", Arguments: `{"input":"x"}`}) {
		t.Fatalf("first tool call = %+v", msg.ToolCalls[0])
	}
	if msg.ToolCalls[1] != (ToolCall{ID: "call_2", Name: "search", Arguments: "{}"}) {
		t.Fatalf("second tool call = %+v", msg.ToolCalls[1])
	}
	req := recorder.lastRequest()
	format := req["response_format"].(map[string]any)
	if format["type"] != "json_schema" {
		t.Fatalf("response_format = %v", format)
	}
	if schema := format["json_schema"].(map[string]any); schema["name"] != "Out" {
		t.Fatalf("json_schema = %v", schema)
	}
}

func TestProviderChatModelStreamEmitterErrorStopsStream(t *testing.T) {
	t.Parallel()

	server, _ := newToolCallServer(t)
	model := newTestProviderChatModel(t, server.URL)
	emitErr := errors.New("client gone")
	_, err := model.StreamWithOptions(context.Background(), []Message{{Role: RoleUser, Content: "hi"}}, GenerateOptions{}, func(string) error {
		return emitErr
	})
	if !errors.Is(err, emitErr) {
		t.Fatalf("StreamWithOptions() error = %v, want emitter error returned as-is", err)
	}
	if _, err := model.StreamWithOptions(context.Background(), nil, GenerateOptions{}, nil); err == nil {
		t.Fatal("nil emitter must be rejected")
	}
}

func TestToProviderResponseFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		format   ResponseFormat
		wantType provider.ResponseFormatType
		wantName string
		wantErr  bool
	}{
		{name: "json object", format: ResponseFormat{Type: ResponseFormatJSONObject}, wantType: provider.ResponseFormatJSONObject},
		{name: "json schema with name", format: ResponseFormat{Type: ResponseFormatJSONSchema, Name: "Out", Schema: map[string]any{}}, wantType: provider.ResponseFormatJSONSchema, wantName: "Out"},
		{name: "json schema defaults name", format: ResponseFormat{Type: ResponseFormatJSONSchema, Schema: map[string]any{}}, wantType: provider.ResponseFormatJSONSchema, wantName: "response"},
		{name: "json schema without schema", format: ResponseFormat{Type: ResponseFormatJSONSchema}, wantErr: true},
		{name: "unsupported type", format: ResponseFormat{Type: "xml"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := toProviderResponseFormat(tt.format)
			if tt.wantErr {
				if err == nil {
					t.Fatal("error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Type != tt.wantType || got.Name != tt.wantName {
				t.Fatalf("format = %+v, want type %s name %q", got, tt.wantType, tt.wantName)
			}
		})
	}
}

func TestToProviderMessagesToolRoles(t *testing.T) {
	t.Parallel()

	msgs := toProviderMessages([]Message{
		{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "c1", Name: "echo", Arguments: "{}"}}},
		{Role: RoleTool, ToolCallID: "c1", Content: "result"},
	})
	if msgs[0].Role != provider.RoleAssistant || len(msgs[0].ToolCalls) != 1 || msgs[0].ToolCalls[0].Function.Name != "echo" {
		t.Fatalf("assistant message = %+v", msgs[0])
	}
	if msgs[1].Role != provider.RoleTool || msgs[1].ToolCallID != "c1" || msgs[1].Content[0].Text != "result" {
		t.Fatalf("tool message = %+v", msgs[1])
	}
	if usageGenerationInfo(provider.Usage{}) != nil {
		t.Fatal("zero usage must map to nil generation info")
	}
}
