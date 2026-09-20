package llm

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Role 表示聊天消息角色。
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	// RoleTool 表示工具执行结果消息，必须携带 ToolCallID 关联到模型请求的调用。
	RoleTool Role = "tool"
)

// ToolCall 表示模型请求的一次原生工具调用。
type ToolCall struct {
	ID   string
	Name string
	// Arguments 是模型生成的 JSON 参数文本。
	Arguments string
}

// Message 表示聊天模型输入输出消息。
type Message struct {
	Role    Role
	Content string
	// ToolCalls 仅在 Role == RoleAssistant 时有意义，表示模型请求的工具调用。
	ToolCalls []ToolCall
	// ToolCallID 仅在 Role == RoleTool 时有意义。
	ToolCallID     string
	GenerationInfo map[string]any
}

// ToolDefinition 描述随请求下发给模型的原生工具。
type ToolDefinition struct {
	Name        string
	Description string
	// Parameters 是 JSON Schema 对象，需可被 json.Marshal；nil 表示无参数。
	Parameters any
}

// ResponseFormatType 标识平台原生结构化输出格式。
type ResponseFormatType string

const (
	// ResponseFormatJSONObject 要求模型输出合法 JSON 对象。
	ResponseFormatJSONObject ResponseFormatType = "json_object"
	// ResponseFormatJSONSchema 要求模型输出符合 Schema 的 JSON。
	ResponseFormatJSONSchema ResponseFormatType = "json_schema"
)

// ResponseFormat 描述一次请求的原生结构化输出格式。
type ResponseFormat struct {
	Type ResponseFormatType
	// Name 与 Schema 仅在 Type == ResponseFormatJSONSchema 时使用。
	Name   string
	Schema any
}

// GenerateOptions 是携带工具定义与响应格式的生成参数。
type GenerateOptions struct {
	Tools          []ToolDefinition
	ResponseFormat *ResponseFormat
	// ReasoningEffort 非空时映射为平台推理强度参数（如 reasoning_effort），合法取值由平台决定。
	ReasoningEffort string
}

// ChatModel 表示当前项目使用的聊天模型抽象。
type ChatModel interface {
	Generate(ctx context.Context, input []Message) (Message, error)
	Stream(ctx context.Context, input []Message, emit func(string) error) error
}

// ToolCapableChatModel 是可选扩展接口：支持平台原生工具调用与结构化输出。
// StreamWithOptions 对每个文本增量调用 emit，并在流结束后返回包含完整文本与累积 ToolCalls 的消息。
type ToolCapableChatModel interface {
	ChatModel
	GenerateWithOptions(ctx context.Context, input []Message, opts GenerateOptions) (Message, error)
	StreamWithOptions(ctx context.Context, input []Message, opts GenerateOptions, emit func(string) error) (Message, error)
}

// ChatConfig 保存聊天模型适配器的构造配置。
type ChatConfig struct {
	Model   string
	BaseURL string
	APIKey  string
	Timeout time.Duration
}

func (c ChatConfig) normalized() ChatConfig {
	c.Model = strings.TrimSpace(c.Model)
	c.BaseURL = strings.TrimSpace(c.BaseURL)
	c.APIKey = strings.TrimSpace(c.APIKey)
	return c
}

// Validate 检查聊天模型配置是否完整且有效。
func (c ChatConfig) Validate() error {
	c = c.normalized()

	if c.Model == "" {
		return fmt.Errorf("chat model is required")
	}
	if c.BaseURL == "" {
		return fmt.Errorf("chat base url is required")
	}
	if _, err := parseAndValidateBaseURL(c.BaseURL); err != nil {
		return fmt.Errorf("chat base url is invalid: %w", err)
	}
	if c.APIKey == "" {
		return fmt.Errorf("chat api key is required")
	}
	if c.Timeout <= 0 {
		return fmt.Errorf("chat timeout must be positive")
	}
	return nil
}

func parseAndValidateBaseURL(raw string) (*url.URL, error) {
	parsed, err := url.ParseRequestURI(raw)
	if err != nil {
		return nil, fmt.Errorf("parse base url: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("base url must include scheme and host")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("base url scheme must be http or https")
	}
	return parsed, nil
}
