package llm

import (
	"context"
	"fmt"
	"net/http"

	provider "github.com/gtkit/go-llm-provider/v2/provider"
)

// OpenAIEmbedder 将 go-llm-provider 的 OpenAI 兼容 Embedder 适配到当前包的 Embedder 接口。
type OpenAIEmbedder struct {
	client provider.Embedder
}

// NewOpenAIEmbedder 创建一个 OpenAI-compatible embedding 适配器，底层由 go-llm-provider/v2 驱动。
func NewOpenAIEmbedder(_ context.Context, cfg EmbeddingConfig) (Embedder, error) {
	cfg = cfg.normalized()
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate embedding config: %w", err)
	}

	client, err := provider.NewEmbedder(provider.EmbedderConfig{
		Name:       provider.ProviderOpenAI,
		BaseURL:    cfg.BaseURL,
		APIKey:     cfg.APIKey,
		Model:      cfg.Model,
		HTTPClient: &http.Client{Timeout: cfg.Timeout},
	})
	if err != nil {
		return nil, fmt.Errorf("new openai embedder: %w", err)
	}

	return &OpenAIEmbedder{client: client}, nil
}

// EmbedTexts 对文本做向量化，并返回 float32 向量。
func (e *OpenAIEmbedder) EmbedTexts(ctx context.Context, texts []string) ([][]float32, error) {
	normalizedTexts, err := normalizeEmbeddingTexts(texts)
	if err != nil {
		return nil, fmt.Errorf("validate embedding input: %w", err)
	}

	if e == nil || e.client == nil {
		return nil, fmt.Errorf("openai embedder is nil")
	}

	rows, err := provider.EmbedBatch(ctx, e.client, normalizedTexts)
	if err != nil {
		return nil, fmt.Errorf("embed texts: %w", err)
	}
	return rows, nil
}
