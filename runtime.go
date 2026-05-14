package ragagent

import (
	"context"

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
)

// Message 表示聊天消息。
type Message = llm.Message

// ChatModel 是根包公开的聊天模型抽象。
type ChatModel = llm.ChatModel

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
func NewOpenAIChatModel(ctx context.Context, cfg ChatModelConfig) (ChatModel, error) {
	return llm.NewOpenAIChatModel(ctx, cfg)
}

// NewOpenAIEmbedder 创建默认 OpenAI-compatible embedding 实现。
func NewOpenAIEmbedder(ctx context.Context, cfg EmbedderConfig) (Embedder, error) {
	return llm.NewOpenAIEmbedder(ctx, cfg)
}
