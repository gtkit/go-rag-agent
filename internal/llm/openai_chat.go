package llm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	provider "github.com/gtkit/go-llm-provider/v2/provider"
)

// OpenAIChatModel 将 go-llm-provider 的 OpenAI 兼容 Provider 适配到当前包的 ChatModel 接口。
type OpenAIChatModel struct {
	client provider.Provider
	model  string
}

// NewOpenAIChatModel 创建一个 OpenAI-compatible 的聊天模型，底层由 go-llm-provider/v2 驱动。
func NewOpenAIChatModel(_ context.Context, cfg ChatConfig) (ChatModel, error) {
	cfg = cfg.normalized()
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate chat config: %w", err)
	}

	client, err := provider.NewProvider(provider.ProviderConfig{
		Name:       provider.ProviderOpenAI,
		BaseURL:    cfg.BaseURL,
		APIKey:     cfg.APIKey,
		Model:      cfg.Model,
		HTTPClient: &http.Client{Timeout: cfg.Timeout},
	})
	if err != nil {
		return nil, fmt.Errorf("new openai chat model: %w", err)
	}

	return &OpenAIChatModel{client: client, model: cfg.Model}, nil
}

func (m *OpenAIChatModel) Generate(ctx context.Context, input []Message) (Message, error) {
	if m == nil || m.client == nil {
		return Message{}, fmt.Errorf("openai chat model is nil")
	}

	resp, err := m.client.Chat(ctx, &provider.ChatRequest{
		Model:    m.model,
		Messages: toProviderMessages(input),
	})
	if err != nil {
		return Message{}, fmt.Errorf("generate content: %w", err)
	}
	if resp == nil {
		return Message{}, fmt.Errorf("generate content returned no response")
	}

	return Message{
		Role:           RoleAssistant,
		Content:        resp.Content,
		GenerationInfo: usageGenerationInfo(resp.Usage),
	}, nil
}

func (m *OpenAIChatModel) Stream(ctx context.Context, input []Message, emit func(string) error) error {
	if m == nil || m.client == nil {
		return fmt.Errorf("openai chat model is nil")
	}
	if emit == nil {
		return fmt.Errorf("stream emitter is required")
	}

	stream, err := m.client.ChatStream(ctx, &provider.ChatRequest{
		Model:    m.model,
		Messages: toProviderMessages(input),
	})
	if err != nil {
		return fmt.Errorf("stream content: %w", err)
	}
	defer func() { _ = stream.Close() }()

	for {
		chunk, recvErr := stream.Recv()
		if errors.Is(recvErr, io.EOF) {
			return nil
		}
		if recvErr != nil {
			return fmt.Errorf("stream content: %w", recvErr)
		}
		if chunk == nil || chunk.Delta == "" {
			continue
		}
		if emitErr := emit(chunk.Delta); emitErr != nil {
			return emitErr
		}
	}
}

// usageGenerationInfo 把 provider 的结构化 usage 映射进 GenerationInfo map，
// 保持与既有 usage 提取逻辑（PromptTokens/CompletionTokens/TotalTokens 键）一致。
func usageGenerationInfo(usage provider.Usage) map[string]any {
	if usage.PromptTokens == 0 && usage.CompletionTokens == 0 && usage.TotalTokens == 0 {
		return nil
	}
	return map[string]any{
		"PromptTokens":     usage.PromptTokens,
		"CompletionTokens": usage.CompletionTokens,
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
		case RoleUser:
			role = provider.RoleUser
		}
		msgs = append(msgs, provider.Message{
			Role:    role,
			Content: []provider.ContentPart{provider.TextPart(msg.Content)},
		})
	}
	return msgs
}
