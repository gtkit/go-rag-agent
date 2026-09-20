package llm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	provider "github.com/gtkit/go-llm-provider/v2/provider"
)

// ProviderChatModel 把 go-llm-provider 的任意 Provider 适配为 ToolCapableChatModel。
type ProviderChatModel struct {
	client provider.Provider
	model  string
}

// NewProviderChatModel 用已构造的 Provider 创建聊天模型；model 为空时使用 Provider 自身的默认模型。
func NewProviderChatModel(client provider.Provider, model string) (*ProviderChatModel, error) {
	if client == nil {
		return nil, fmt.Errorf("chat provider is required")
	}
	return &ProviderChatModel{client: client, model: strings.TrimSpace(model)}, nil
}

func (m *ProviderChatModel) Generate(ctx context.Context, input []Message) (Message, error) {
	return m.GenerateWithOptions(ctx, input, GenerateOptions{})
}

func (m *ProviderChatModel) Stream(ctx context.Context, input []Message, emit func(string) error) error {
	_, err := m.StreamWithOptions(ctx, input, GenerateOptions{}, emit)
	return err
}

func (m *ProviderChatModel) GenerateWithOptions(ctx context.Context, input []Message, opts GenerateOptions) (Message, error) {
	req, err := m.chatRequest(input, opts)
	if err != nil {
		return Message{}, err
	}
	resp, err := m.client.Chat(ctx, req)
	if err != nil {
		return Message{}, fmt.Errorf("generate content: %w", err)
	}
	if resp == nil {
		return Message{}, fmt.Errorf("generate content returned no response")
	}
	return Message{
		Role:           RoleAssistant,
		Content:        resp.Content,
		ToolCalls:      fromProviderToolCalls(resp.ToolCalls),
		GenerationInfo: usageGenerationInfo(resp.Usage),
	}, nil
}

func (m *ProviderChatModel) StreamWithOptions(ctx context.Context, input []Message, opts GenerateOptions, emit func(string) error) (Message, error) {
	if emit == nil {
		return Message{}, fmt.Errorf("stream emitter is required")
	}
	req, err := m.chatRequest(input, opts)
	if err != nil {
		return Message{}, err
	}
	stream, err := m.client.ChatStream(ctx, req)
	if err != nil {
		return Message{}, fmt.Errorf("stream content: %w", err)
	}
	defer func() { _ = stream.Close() }()

	var (
		content strings.Builder
		usage   provider.Usage
		calls   = map[int]*ToolCall{}
	)
	for {
		chunk, recvErr := stream.Recv()
		if errors.Is(recvErr, io.EOF) {
			break
		}
		if recvErr != nil {
			return Message{}, fmt.Errorf("stream content: %w", recvErr)
		}
		if chunk == nil {
			continue
		}
		if chunk.Usage != (provider.Usage{}) {
			usage = chunk.Usage
		}
		for _, delta := range chunk.ToolCalls {
			call, ok := calls[delta.Index]
			if !ok {
				call = &ToolCall{}
				calls[delta.Index] = call
			}
			if delta.ID != "" {
				call.ID = delta.ID
			}
			if delta.Function.Name != "" {
				call.Name = delta.Function.Name
			}
			call.Arguments += delta.Function.Arguments
		}
		if chunk.Delta == "" {
			continue
		}
		content.WriteString(chunk.Delta)
		if emitErr := emit(chunk.Delta); emitErr != nil {
			return Message{}, emitErr
		}
	}
	return Message{
		Role:           RoleAssistant,
		Content:        content.String(),
		ToolCalls:      orderedToolCalls(calls),
		GenerationInfo: usageGenerationInfo(usage),
	}, nil
}

func (m *ProviderChatModel) chatRequest(input []Message, opts GenerateOptions) (*provider.ChatRequest, error) {
	if m == nil || m.client == nil {
		return nil, fmt.Errorf("chat model provider is nil")
	}
	req := &provider.ChatRequest{
		Model:    m.model,
		Messages: toProviderMessages(input),
		Tools:    toProviderTools(opts.Tools),
	}
	if opts.ResponseFormat != nil {
		format, err := toProviderResponseFormat(*opts.ResponseFormat)
		if err != nil {
			return nil, err
		}
		req.ResponseFormat = format
	}
	if effort := strings.TrimSpace(opts.ReasoningEffort); effort != "" {
		req.Thinking = &provider.Thinking{Effort: effort}
	}
	return req, nil
}

func orderedToolCalls(calls map[int]*ToolCall) []ToolCall {
	if len(calls) == 0 {
		return nil
	}
	indexes := make([]int, 0, len(calls))
	for index := range calls {
		indexes = append(indexes, index)
	}
	slices.Sort(indexes)
	out := make([]ToolCall, 0, len(indexes))
	for _, index := range indexes {
		out = append(out, *calls[index])
	}
	return out
}

// usageGenerationInfo 把 provider 的结构化 usage 映射进 GenerationInfo map，
// 键名与 usage 提取逻辑（PromptTokens/CompletionTokens/ReasoningTokens/TotalTokens）保持一致。
func usageGenerationInfo(usage provider.Usage) map[string]any {
	if usage == (provider.Usage{}) {
		return nil
	}
	return map[string]any{
		"PromptTokens":     usage.PromptTokens,
		"CompletionTokens": usage.CompletionTokens,
		"ReasoningTokens":  usage.ReasoningTokens,
		"TotalTokens":      usage.TotalTokens,
	}
}

func toProviderMessages(input []Message) []provider.Message {
	msgs := make([]provider.Message, 0, len(input))
	for _, msg := range input {
		role := provider.RoleUser
		switch msg.Role {
		case RoleSystem:
			role = provider.RoleSystem
		case RoleAssistant:
			role = provider.RoleAssistant
		case RoleTool:
			role = provider.RoleTool
		case RoleUser:
			role = provider.RoleUser
		}
		msgs = append(msgs, provider.Message{
			Role:       role,
			Content:    []provider.ContentPart{provider.TextPart(msg.Content)},
			ToolCalls:  toProviderToolCalls(msg.ToolCalls),
			ToolCallID: msg.ToolCallID,
		})
	}
	return msgs
}

func toProviderToolCalls(calls []ToolCall) []provider.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]provider.ToolCall, 0, len(calls))
	for _, call := range calls {
		out = append(out, provider.ToolCall{
			ID:       call.ID,
			Function: provider.FunctionCall{Name: call.Name, Arguments: call.Arguments},
		})
	}
	return out
}

func fromProviderToolCalls(calls []provider.ToolCall) []ToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]ToolCall, 0, len(calls))
	for _, call := range calls {
		out = append(out, ToolCall{ID: call.ID, Name: call.Function.Name, Arguments: call.Function.Arguments})
	}
	return out
}

// emptyObjectSchema 是无参数工具的占位 schema：OpenAI 兼容平台要求 parameters 必须是 object。
var emptyObjectSchema = map[string]any{"type": "object", "properties": map[string]any{}}

func toProviderTools(defs []ToolDefinition) []provider.Tool {
	if len(defs) == 0 {
		return nil
	}
	out := make([]provider.Tool, 0, len(defs))
	for _, def := range defs {
		params := def.Parameters
		if params == nil {
			params = emptyObjectSchema
		}
		out = append(out, provider.Tool{Function: provider.FunctionDef{
			Name:        def.Name,
			Description: def.Description,
			Parameters:  params,
		}})
	}
	return out
}

func toProviderResponseFormat(format ResponseFormat) (*provider.ResponseFormat, error) {
	switch format.Type {
	case ResponseFormatJSONObject:
		return provider.JSONObjectFormat(), nil
	case ResponseFormatJSONSchema:
		if format.Schema == nil {
			return nil, fmt.Errorf("json_schema response format requires a schema")
		}
		name := strings.TrimSpace(format.Name)
		if name == "" {
			name = "response"
		}
		return provider.JSONSchemaFormat(name, format.Schema), nil
	default:
		return nil, fmt.Errorf("unsupported response format %q", format.Type)
	}
}
