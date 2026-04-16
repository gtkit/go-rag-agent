package llm

import (
	"context"
	"fmt"

	openai "github.com/cloudwego/eino-ext/components/model/openai"
)

// NewOpenAIChatModel creates an OpenAI-compatible Eino chat model.
func NewOpenAIChatModel(ctx context.Context, cfg ChatConfig) (ChatModel, error) {
	cfg = cfg.normalized()
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate chat config: %w", err)
	}

	chatModel, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		Model:   cfg.Model,
		BaseURL: cfg.BaseURL,
		APIKey:  cfg.APIKey,
		Timeout: cfg.Timeout,
	})
	if err != nil {
		return nil, fmt.Errorf("new openai chat model: %w", err)
	}

	return chatModel, nil
}
