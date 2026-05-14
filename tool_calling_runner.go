package ragagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gtkit/go-rag-agent/internal/graph"
	"github.com/gtkit/go-rag-agent/internal/llm"
)

// ToolCallingRunnerConfig configures the optional tool-calling runner.
type ToolCallingRunnerConfig struct {
	MaxToolCalls  int
	MaxIterations int
}

// ToolCallingRunner is a lightweight single-agent tool loop runner.
type ToolCallingRunner struct {
	model ChatModel
	tools *ToolRegistry
	cfg   ToolCallingRunnerConfig
}

type toolCallEnvelope struct {
	ToolCall *toolCallRequest `json:"tool_call"`
	Final    string           `json:"final,omitempty"`
}

type toolCallRequest struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// NewToolCallingRunner creates a bounded runner that executes registered tools requested by the model.
func NewToolCallingRunner(model ChatModel, registry *ToolRegistry, cfg ToolCallingRunnerConfig) (*ToolCallingRunner, error) {
	if model == nil {
		return nil, fmt.Errorf("chat model is required: %w", ErrInvalidConfig)
	}
	if registry == nil || len(registry.Tools()) == 0 {
		return nil, fmt.Errorf("tool registry requires at least one tool: %w", ErrInvalidConfig)
	}
	if cfg.MaxIterations == 0 {
		cfg.MaxIterations = 3
	}
	if cfg.MaxIterations < 0 {
		return nil, fmt.Errorf("max iterations must be non-negative: %w", ErrInvalidConfig)
	}
	if cfg.MaxToolCalls < 0 {
		return nil, fmt.Errorf("max tool calls must be non-negative: %w", ErrInvalidConfig)
	}
	return &ToolCallingRunner{model: model, tools: registry.clone(), cfg: cfg}, nil
}

// Ask runs a bounded tool-calling loop and returns the final answer text.
func (r *ToolCallingRunner) Ask(ctx context.Context, req graph.Request) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	messages := r.initialMessages(req)
	toolCalls := 0
	for iteration := 0; iteration < r.cfg.MaxIterations; iteration++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		msg, err := r.model.Generate(ctx, messages)
		if err != nil {
			return "", fmt.Errorf("generate tool-calling response: %w", err)
		}
		call, final, ok, err := parseToolCallMessage(msg.Content)
		if err != nil {
			return "", err
		}
		if !ok {
			return msg.Content, nil
		}
		if strings.TrimSpace(final) != "" {
			return final, nil
		}
		if toolCalls >= r.cfg.MaxToolCalls {
			return "", fmt.Errorf("%w: tool call limit=%d", ErrToolCallLimitExceeded, r.cfg.MaxToolCalls)
		}
		toolCalls++
		result, err := r.runTool(ctx, req, call)
		if err != nil {
			return "", err
		}
		messages = append(messages,
			llm.Message{Role: llm.RoleAssistant, Content: msg.Content},
			llm.Message{Role: llm.RoleUser, Content: "Tool result for " + call.Name + ":\n" + result},
		)
	}
	return "", fmt.Errorf("%w: max iterations=%d", ErrToolCallLimitExceeded, r.cfg.MaxIterations)
}

// AskStream runs Ask and emits the final answer as a single chunk.
func (r *ToolCallingRunner) AskStream(ctx context.Context, req graph.Request, emit graph.StreamEmitter) error {
	if emit == nil {
		return fmt.Errorf("stream emitter is required")
	}
	answer, err := r.Ask(ctx, req)
	if err != nil {
		return err
	}
	if strings.TrimSpace(answer) != "" {
		if err := emit(graph.Event{Type: graph.EventAnswerChunk, Content: answer, Step: 1}); err != nil {
			return err
		}
	}
	return emit(graph.Event{Type: graph.EventDone, Step: 1})
}

func (r *ToolCallingRunner) initialMessages(req graph.Request) []llm.Message {
	messages := []llm.Message{{
		Role: llm.RoleSystem,
		Content: "You may either answer directly or request one tool call. " +
			"To call a tool, output JSON exactly as {\"tool_call\":{\"name\":\"tool_name\",\"arguments\":{...}}}. " +
			"To finish with JSON, output {\"final\":\"answer\"}. Otherwise output the final answer text.",
	}}
	if strings.TrimSpace(req.EvidenceText) != "" {
		messages = append(messages, llm.Message{Role: llm.RoleUser, Content: "Relevant context:\n" + req.EvidenceText})
	}
	if strings.TrimSpace(req.LongTermMemoryText) != "" {
		messages = append(messages, llm.Message{Role: llm.RoleUser, Content: "Relevant memory:\n" + req.LongTermMemoryText})
	}
	messages = append(messages, llm.Message{Role: llm.RoleUser, Content: "Available tools:\n" + r.toolDescriptions()})
	messages = append(messages, llm.Message{Role: llm.RoleUser, Content: req.Query})
	return messages
}

func (r *ToolCallingRunner) toolDescriptions() string {
	descriptions := make([]string, 0, len(r.tools.Tools()))
	for _, tool := range r.tools.Tools() {
		if structured := structuredToolFromTool(tool); structured != nil {
			schema, _ := json.Marshal(structured.Schema())
			descriptions = append(descriptions, fmt.Sprintf("- %s: %s schema=%s", tool.Name(), tool.Description(), string(schema)))
			continue
		}
		descriptions = append(descriptions, fmt.Sprintf("- %s: %s", tool.Name(), tool.Description()))
	}
	return strings.Join(descriptions, "\n")
}

func (r *ToolCallingRunner) runTool(ctx context.Context, req graph.Request, call toolCallRequest) (string, error) {
	name := strings.TrimSpace(call.Name)
	if name == "" {
		return "", fmt.Errorf("%w: tool name is required", ErrToolCallInvalid)
	}
	tool, ok := r.tools.Lookup(name)
	if !ok {
		err := fmt.Errorf("%w: unknown tool %q", ErrToolCallInvalid, name)
		if req.ToolObserver != nil {
			_ = req.ToolObserver.OnToolEnd(ctx, name, err)
		}
		return "", err
	}
	if structured := structuredToolFromTool(tool); structured != nil {
		if err := ValidateToolArguments(structured.Schema(), call.Arguments); err != nil {
			if req.ToolObserver != nil {
				_ = req.ToolObserver.OnToolEnd(ctx, name, err)
			}
			return "", err
		}
	}
	if req.ToolObserver != nil {
		if err := req.ToolObserver.OnToolStart(ctx, name); err != nil {
			return "", err
		}
	}
	input, err := json.Marshal(call.Arguments)
	if err != nil {
		return "", fmt.Errorf("%w: marshal arguments for tool %q: %v", ErrToolArgumentInvalid, name, err)
	}
	result, runErr := tool.Run(ctx, string(input))
	if req.ToolObserver != nil {
		if err := req.ToolObserver.OnToolEnd(ctx, name, runErr); err != nil && runErr == nil {
			runErr = err
		}
	}
	if runErr != nil {
		return "", runErr
	}
	return result, nil
}

func structuredToolFromTool(tool Tool) StructuredTool {
	if adapter, ok := tool.(interface{ Structured() StructuredTool }); ok {
		return adapter.Structured()
	}
	if structured, ok := tool.(StructuredTool); ok {
		return structured
	}
	return nil
}

func parseToolCallMessage(content string) (toolCallRequest, string, bool, error) {
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "{") {
		return toolCallRequest{}, "", false, nil
	}
	var envelope toolCallEnvelope
	if err := json.Unmarshal([]byte(trimmed), &envelope); err != nil {
		return toolCallRequest{}, "", false, fmt.Errorf("%w: parse tool call envelope: %v", ErrToolCallInvalid, err)
	}
	if strings.TrimSpace(envelope.Final) != "" {
		return toolCallRequest{}, envelope.Final, true, nil
	}
	if envelope.ToolCall == nil {
		return toolCallRequest{}, "", false, nil
	}
	if envelope.ToolCall.Arguments == nil {
		envelope.ToolCall.Arguments = map[string]any{}
	}
	return *envelope.ToolCall, "", true, nil
}
