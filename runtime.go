package ragagent

import (
	"context"
	"fmt"

	llmprovider "github.com/gtkit/go-llm-provider/v2/provider"

	"github.com/gtkit/go-rag-agent/internal/graph"
	"github.com/gtkit/go-rag-agent/internal/llm"
)

// Role 表示聊天消息角色。
type Role = llm.Role

const (
	// RoleSystem 表示 system 角色。
	RoleSystem = llm.RoleSystem
	// RoleUser 表示 user 角色。
	RoleUser = llm.RoleUser
	// RoleAssistant 表示 assistant 角色。
	RoleAssistant = llm.RoleAssistant
	// RoleTool 表示工具执行结果角色，消息需携带 ToolCallID。
	RoleTool = llm.RoleTool
)

// Message 表示聊天消息。
type Message = llm.Message

// ToolCall 表示模型请求的一次原生工具调用。
type ToolCall = llm.ToolCall

// ToolDefinition 描述随请求下发给模型的原生工具定义。
type ToolDefinition = llm.ToolDefinition

// ResponseFormatType 标识平台原生结构化输出格式。
type ResponseFormatType = llm.ResponseFormatType

const (
	// ResponseFormatJSONObject 要求模型输出合法 JSON 对象。
	ResponseFormatJSONObject = llm.ResponseFormatJSONObject
	// ResponseFormatJSONSchema 要求模型输出符合 Schema 的 JSON。
	ResponseFormatJSONSchema = llm.ResponseFormatJSONSchema
)

// ResponseFormat 描述一次请求的原生结构化输出格式。
type ResponseFormat = llm.ResponseFormat

// GenerateOptions 是携带工具定义与响应格式的生成参数。
type GenerateOptions = llm.GenerateOptions

// ChatModel 是根包公开的聊天模型抽象。
type ChatModel = llm.ChatModel

// ToolCapableChatModel 是支持平台原生工具调用与结构化输出的聊天模型扩展接口。
// 注入的 ChatModel 实现它时，tool calling 与 AskStructured 自动切换到原生协议。
type ToolCapableChatModel = llm.ToolCapableChatModel

// Embedder 是根包公开的 embedding 抽象。
type Embedder = llm.Embedder

// ChatModelConfig 是默认 OpenAI-compatible 聊天模型构造配置。
type ChatModelConfig = llm.ChatConfig

// EmbedderConfig 是默认 OpenAI-compatible embedding 构造配置。
type EmbedderConfig = llm.EmbeddingConfig

// RuntimeComponents 定义 Agent 可选注入的运行时组件。
type RuntimeComponents struct {
	ChatModel         ChatModel
	Embedder          Embedder
	ToolCallingRunner graph.Runner
}

// NewOpenAIChatModel 创建默认 OpenAI-compatible 聊天模型。
func NewOpenAIChatModel(ctx context.Context, cfg ChatModelConfig) (ToolCapableChatModel, error) {
	model, err := llm.NewOpenAIChatModel(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return model, nil
}

// NewOpenAIEmbedder 创建默认 OpenAI-compatible embedding 实现。
func NewOpenAIEmbedder(ctx context.Context, cfg EmbedderConfig) (Embedder, error) {
	embedder, err := llm.NewOpenAIEmbedder(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return embedder, nil
}

// NewChatModelFromProvider 把 go-llm-provider 构造的任意 Provider（Anthropic、Gemini、Ollama、Azure 等）
// 适配为可注入 RuntimeComponents.ChatModel 的聊天模型；model 为空时使用 Provider 的默认模型。
func NewChatModelFromProvider(client llmprovider.Provider, model string) (ToolCapableChatModel, error) {
	chatModel, err := llm.NewProviderChatModel(client, model)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidConfig, err)
	}
	return chatModel, nil
}

// NewEmbedderFromProvider 把 go-llm-provider 构造的任意 Embedder 适配为可注入 RuntimeComponents.Embedder 的实现。
func NewEmbedderFromProvider(client llmprovider.Embedder) (Embedder, error) {
	embedder, err := llm.NewProviderEmbedder(client)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidConfig, err)
	}
	return embedder, nil
}
