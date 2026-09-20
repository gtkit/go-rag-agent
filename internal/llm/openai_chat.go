package llm

import (
	"context"
	"fmt"
	"net/http"

	provider "github.com/gtkit/go-llm-provider/v2/provider"
)

// NewOpenAIChatModel 创建一个 OpenAI-compatible 的聊天模型，底层由 go-llm-provider/v2 驱动。
func NewOpenAIChatModel(_ context.Context, cfg ChatConfig) (*ProviderChatModel, error) {
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
	return NewProviderChatModel(client, cfg.Model)
}
