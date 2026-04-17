package llm

import (
	"context"
	"fmt"
	"net/http"

	lcllms "github.com/tmc/langchaingo/llms"
	lcopenai "github.com/tmc/langchaingo/llms/openai"
)

// OpenAIChatModel 将 LangChainGo OpenAI 模型适配到当前包的 ChatModel 接口。
type OpenAIChatModel struct {
	client *lcopenai.LLM
}

// NewOpenAIChatModel 创建一个 OpenAI-compatible 的 LangChainGo 聊天模型。
func NewOpenAIChatModel(_ context.Context, cfg ChatConfig) (ChatModel, error) {
	cfg = cfg.normalized()
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate chat config: %w", err)
	}

	client, err := lcopenai.New(
		lcopenai.WithModel(cfg.Model),
		lcopenai.WithBaseURL(cfg.BaseURL),
		lcopenai.WithToken(cfg.APIKey),
		lcopenai.WithHTTPClient(&http.Client{Timeout: cfg.Timeout}),
	)
	if err != nil {
		return nil, fmt.Errorf("new openai chat model: %w", err)
	}

	return &OpenAIChatModel{client: client}, nil
}

func (m *OpenAIChatModel) Generate(ctx context.Context, input []Message) (Message, error) {
	if m == nil || m.client == nil {
		return Message{}, fmt.Errorf("openai chat model is nil")
	}

	resp, err := m.client.GenerateContent(ctx, toLangChainMessages(input))
	if err != nil {
		return Message{}, fmt.Errorf("generate content: %w", err)
	}
	if len(resp.Choices) == 0 || resp.Choices[0] == nil {
		return Message{}, fmt.Errorf("generate content returned no choices")
	}

	return Message{
		Role:    RoleAssistant,
		Content: resp.Choices[0].Content,
	}, nil
}

func (m *OpenAIChatModel) Stream(ctx context.Context, input []Message, emit func(string) error) error {
	if m == nil || m.client == nil {
		return fmt.Errorf("openai chat model is nil")
	}
	if emit == nil {
		return fmt.Errorf("stream emitter is required")
	}

	_, err := m.client.GenerateContent(ctx, toLangChainMessages(input), lcllms.WithStreamingFunc(func(ctx context.Context, chunk []byte) error {
		if len(chunk) == 0 {
			return nil
		}
		return emit(string(chunk))
	}))
	if err != nil {
		return fmt.Errorf("stream content: %w", err)
	}

	return nil
}

func toLangChainMessages(input []Message) []lcllms.MessageContent {
	msgs := make([]lcllms.MessageContent, 0, len(input))
	for _, msg := range input {
		role := lcllms.ChatMessageTypeHuman
		switch msg.Role {
		case RoleSystem:
			role = lcllms.ChatMessageTypeSystem
		case RoleAssistant:
			role = lcllms.ChatMessageTypeAI
		case RoleUser:
			role = lcllms.ChatMessageTypeHuman
		}
		msgs = append(msgs, lcllms.MessageContent{
			Role:  role,
			Parts: []lcllms.ContentPart{lcllms.TextPart(msg.Content)},
		})
	}
	return msgs
}
