package ragagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"

	"github.com/gtkit/go-rag-agent/internal/graph"
	"github.com/gtkit/go-rag-agent/internal/llm"
)

// ToolInvocation 描述模型请求的一次工具调用，供 ToolCallPolicy 授权。
type ToolInvocation struct {
	Name      string
	Arguments map[string]any
}

// ToolCallPolicy 在执行模型请求的工具调用前授权；返回错误时本次问答以 ErrToolCallDenied 中断。
// 会话、租户等上下文由调用方通过 ctx 传入。
type ToolCallPolicy interface {
	AuthorizeToolCall(ctx context.Context, call ToolInvocation) error
}

// ToolCallingRunnerConfig configures the optional tool-calling runner.
type ToolCallingRunnerConfig struct {
	MaxToolCalls  int
	MaxIterations int
	Policy        ToolCallPolicy
}

// ToolCallingRunner 是有界的单 agent 工具循环 runner。
// 模型实现 ToolCapableChatModel 时使用平台原生工具协议，否则使用 JSON 信封文本协议。
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

// recoverableToolError 标记可回传给模型自纠的工具失败（未知工具、参数无效、工具执行失败）。
type recoverableToolError struct{ err error }

func (e recoverableToolError) Error() string { return e.err.Error() }
func (e recoverableToolError) Unwrap() error { return e.err }

// plainToolSchema 是纯文本 Tool 在原生协议下的参数约定：单字段 input。
var plainToolSchema = ToolSchema{
	Properties: map[string]ToolParameterSchema{
		"input": {Type: ToolParameterString, Description: "Plain-text input for the tool."},
	},
	Required: []string{"input"},
}

const (
	envelopeSystemPrompt = "You may either answer directly or request one tool call. " +
		"To call a tool, output JSON exactly as {\"tool_call\":{\"name\":\"tool_name\",\"arguments\":{...}}}. " +
		"To finish with JSON, output {\"final\":\"answer\"}. Otherwise output the final answer text."
	nativeSystemPrompt = "Use the available tools whenever you need information beyond the provided context. " +
		"After tool results arrive, answer the user directly."
)

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
	if capable, ok := r.model.(ToolCapableChatModel); ok {
		return r.askNative(ctx, req, capable, nil)
	}
	return r.askEnvelope(ctx, req)
}

// AskStream 运行工具循环并流式发出答案。原生协议下每轮模型输出的文本增量都会作为 answer_chunk 发出；
// JSON 信封协议下最终答案以单个 chunk 发出。
func (r *ToolCallingRunner) AskStream(ctx context.Context, req graph.Request, emit graph.StreamEmitter) error {
	if emit == nil {
		return fmt.Errorf("stream emitter is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	step := 0
	emitChunk := func(chunk string) error {
		step++
		return emit(graph.Event{Type: graph.EventAnswerChunk, Content: chunk, Step: step})
	}
	if capable, ok := r.model.(ToolCapableChatModel); ok {
		if _, err := r.askNative(ctx, req, capable, emitChunk); err != nil {
			return err
		}
		return emit(graph.Event{Type: graph.EventDone, Step: step})
	}
	answer, err := r.askEnvelope(ctx, req)
	if err != nil {
		return err
	}
	if strings.TrimSpace(answer) != "" {
		if err := emitChunk(answer); err != nil {
			return err
		}
	}
	return emit(graph.Event{Type: graph.EventDone, Step: step})
}

// askNative 用平台原生工具协议循环；emit 非 nil 时逐 token 流式输出。
// 多轮都产出文本时，各轮文本以换行拼接为最终答案，流式路径同样发出该换行，保证同步与流式答案一致。
func (r *ToolCallingRunner) askNative(ctx context.Context, req graph.Request, model ToolCapableChatModel, emit func(string) error) (string, error) {
	messages := append([]llm.Message{{Role: llm.RoleSystem, Content: nativeSystemPrompt}}, graph.PromptMessages(req)...)
	opts := GenerateOptions{Tools: r.toolDefinitions(), ResponseFormat: req.ResponseFormat, ReasoningEffort: req.ReasoningEffort}
	var answer strings.Builder
	// startRound 在多轮都产出文本时插入换行分隔，同步与流式两条路径共用，保证最终答案一致。
	startRound := func() error {
		if answer.Len() == 0 || strings.HasSuffix(answer.String(), "\n") {
			return nil
		}
		answer.WriteString("\n")
		if emit == nil {
			return nil
		}
		return emit("\n")
	}
	toolCalls := 0
	for range r.cfg.MaxIterations {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		var (
			msg llm.Message
			err error
		)
		if emit != nil {
			roundStarted := false
			msg, err = model.StreamWithOptions(ctx, messages, opts, func(chunk string) error {
				if !roundStarted {
					roundStarted = true
					if err := startRound(); err != nil {
						return err
					}
				}
				answer.WriteString(chunk)
				return emit(chunk)
			})
		} else {
			msg, err = model.GenerateWithOptions(ctx, messages, opts)
			if err == nil && msg.Content != "" {
				_ = startRound()
				answer.WriteString(msg.Content)
			}
		}
		if err != nil {
			return "", fmt.Errorf("generate tool-calling response: %w", err)
		}
		if len(msg.ToolCalls) == 0 {
			return answer.String(), nil
		}
		messages = append(messages, msg)
		for _, call := range msg.ToolCalls {
			if toolCalls >= r.cfg.MaxToolCalls {
				return "", fmt.Errorf("%w: tool call limit=%d", ErrToolCallLimitExceeded, r.cfg.MaxToolCalls)
			}
			toolCalls++
			result, err := r.runNativeToolCall(ctx, req, call)
			if err != nil {
				return "", err
			}
			messages = append(messages, llm.Message{Role: llm.RoleTool, ToolCallID: call.ID, Content: result})
		}
	}
	return "", fmt.Errorf("%w: max iterations=%d", ErrToolCallLimitExceeded, r.cfg.MaxIterations)
}

// runNativeToolCall 执行一次原生工具调用；可恢复的失败编码为 tool 结果回传模型，其余错误中断循环。
func (r *ToolCallingRunner) runNativeToolCall(ctx context.Context, req graph.Request, call llm.ToolCall) (string, error) {
	args, err := decodeToolArguments(call.Arguments)
	if err != nil {
		err = recoverableToolError{err: fmt.Errorf("%w: decode arguments for tool %q: %w", ErrToolArgumentInvalid, call.Name, err)}
		if req.ToolObserver != nil {
			if endErr := req.ToolObserver.OnToolEnd(ctx, strings.TrimSpace(call.Name), err); endErr != nil {
				return "", endErr
			}
		}
		return encodeToolError(err), nil
	}
	result, err := r.executeToolCall(ctx, req, call.Name, args)
	if err == nil {
		return result, nil
	}
	if _, ok := errors.AsType[recoverableToolError](err); !ok {
		return "", err
	}
	return encodeToolError(err), nil
}

func decodeToolArguments(raw string) (map[string]any, error) {
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}, nil
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return nil, err
	}
	if args == nil {
		args = map[string]any{}
	}
	return args, nil
}

// encodeToolError 把工具失败编码为回传模型的 tool 结果。SDK 自身产生的协议错误（未知工具、参数无效）
// 原文回传以便模型自纠；工具执行错误只回传泛化描述，避免把内部细节泄漏进对话。
func encodeToolError(err error) string {
	message := "tool execution failed"
	if errors.Is(err, ErrToolCallInvalid) || errors.Is(err, ErrToolArgumentInvalid) {
		message = err.Error()
	}
	data, _ := json.Marshal(map[string]string{"error": message})
	return string(data)
}

func (r *ToolCallingRunner) askEnvelope(ctx context.Context, req graph.Request) (string, error) {
	messages := r.envelopeMessages(req)
	toolCalls := 0
	for range r.cfg.MaxIterations {
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
		result, err := r.executeToolCall(ctx, req, call.Name, call.Arguments)
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

func (r *ToolCallingRunner) envelopeMessages(req graph.Request) []llm.Message {
	messages := []llm.Message{{Role: llm.RoleSystem, Content: envelopeSystemPrompt}}
	prompt := graph.PromptMessages(req)
	// 工具清单放在用户 query 之前，保持 query 为最后一条消息。
	messages = append(messages, prompt[:len(prompt)-1]...)
	messages = append(messages, llm.Message{Role: llm.RoleUser, Content: "Available tools:\n" + r.toolDescriptions()})
	return append(messages, prompt[len(prompt)-1])
}

func (r *ToolCallingRunner) toolDefinitions() []ToolDefinition {
	tools := r.tools.Tools()
	defs := make([]ToolDefinition, 0, len(tools))
	for _, tool := range tools {
		schema := plainToolSchema
		if structured := structuredToolFromTool(tool); structured != nil {
			schema = structured.Schema()
		}
		defs = append(defs, ToolDefinition{Name: tool.Name(), Description: tool.Description(), Parameters: schema})
	}
	return defs
}

func (r *ToolCallingRunner) toolDescriptions() string {
	defs := r.toolDefinitions()
	descriptions := make([]string, 0, len(defs))
	for _, def := range defs {
		schema, _ := json.Marshal(def.Parameters)
		descriptions = append(descriptions, fmt.Sprintf("- %s: %s schema=%s", def.Name, def.Description, string(schema)))
	}
	return strings.Join(descriptions, "\n")
}

// executeToolCall 校验、授权并执行一次工具调用。可回传模型自纠的失败以 recoverableToolError 返回；
// 授权拒绝、观察者错误与 context 取消原样返回。
func (r *ToolCallingRunner) executeToolCall(ctx context.Context, req graph.Request, name string, args map[string]any) (string, error) {
	name = strings.TrimSpace(name)
	if args == nil {
		args = map[string]any{}
	}
	observeEnd := func(err error) error {
		if req.ToolObserver == nil {
			return nil
		}
		return req.ToolObserver.OnToolEnd(ctx, name, err)
	}
	if name == "" {
		return "", recoverableToolError{err: fmt.Errorf("%w: tool name is required", ErrToolCallInvalid)}
	}
	tool, ok := r.tools.Lookup(name)
	if !ok {
		err := recoverableToolError{err: fmt.Errorf("%w: unknown tool %q", ErrToolCallInvalid, name)}
		if endErr := observeEnd(err); endErr != nil {
			return "", endErr
		}
		return "", err
	}
	if r.cfg.Policy != nil {
		if err := r.cfg.Policy.AuthorizeToolCall(ctx, ToolInvocation{Name: name, Arguments: maps.Clone(args)}); err != nil {
			err = fmt.Errorf("%w: tool %q: %w", ErrToolCallDenied, name, err)
			if endErr := observeEnd(err); endErr != nil {
				return "", endErr
			}
			return "", err
		}
	}
	structured := structuredToolFromTool(tool)
	if structured != nil {
		if err := ValidateToolArguments(structured.Schema(), args); err != nil {
			err = recoverableToolError{err: err}
			if endErr := observeEnd(err); endErr != nil {
				return "", endErr
			}
			return "", err
		}
	}
	if req.ToolObserver != nil {
		if err := req.ToolObserver.OnToolStart(ctx, name); err != nil {
			return "", err
		}
	}
	input, err := toolInput(args, structured == nil)
	if err != nil {
		err = recoverableToolError{err: fmt.Errorf("%w: marshal arguments for tool %q: %w", ErrToolArgumentInvalid, name, err)}
		if endErr := observeEnd(err); endErr != nil {
			return "", endErr
		}
		return "", err
	}
	result, runErr := tool.Run(ctx, input)
	if runErr != nil && ctx.Err() == nil {
		runErr = recoverableToolError{err: runErr}
	}
	if endErr := observeEnd(runErr); endErr != nil && runErr == nil {
		runErr = endErr
	}
	if runErr != nil {
		return "", runErr
	}
	return result, nil
}

// toolInput 把模型参数转成 Tool 的输入：纯文本工具且参数恰为 {"input": <string>} 时直传该字符串，
// 其余情况（含结构化工具自带 input 字段）传 JSON 文本。
func toolInput(args map[string]any, plain bool) (string, error) {
	if plain && len(args) == 1 {
		if text, ok := args["input"].(string); ok {
			return text, nil
		}
	}
	data, err := json.Marshal(args)
	if err != nil {
		return "", err
	}
	return string(data), nil
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
