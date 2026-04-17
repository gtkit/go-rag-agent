package llm

import (
	"context"
	"fmt"

	openaiembed "github.com/cloudwego/eino-ext/components/embedding/openai"
)

// OpenAIEmbedder 将 Eino 的 OpenAI Embedder 适配到当前包的 Embedder 接口。
type OpenAIEmbedder struct {
	client *openaiembed.Embedder
}

// NewOpenAIEmbedder 创建一个 OpenAI-compatible embedding 适配器。
func NewOpenAIEmbedder(ctx context.Context, cfg EmbeddingConfig) (Embedder, error) {
	cfg = cfg.normalized()
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate embedding config: %w", err)
	}

	client, err := openaiembed.NewEmbedder(ctx, &openaiembed.EmbeddingConfig{
		Model:   cfg.Model,
		BaseURL: cfg.BaseURL,
		APIKey:  cfg.APIKey,
		Timeout: cfg.Timeout,
	})
	if err != nil {
		return nil, fmt.Errorf("new openai embedder: %w", err)
	}

	return &OpenAIEmbedder{client: client}, nil
}

// EmbedTexts 对文本做向量化，并把结果转换为 float32 向量。
func (e *OpenAIEmbedder) EmbedTexts(ctx context.Context, texts []string) ([][]float32, error) {
	normalizedTexts, err := normalizeEmbeddingTexts(texts)
	if err != nil {
		return nil, fmt.Errorf("validate embedding input: %w", err)
	}

	if e == nil || e.client == nil {
		return nil, fmt.Errorf("openai embedder is nil")
	}

	rows, err := e.client.EmbedStrings(ctx, normalizedTexts)
	if err != nil {
		return nil, fmt.Errorf("embed texts: %w", err)
	}
	return convertEmbeddingRows(rows), nil
}

func convertEmbeddingRows(rows [][]float64) [][]float32 {
	if rows == nil {
		return nil
	}

	converted := make([][]float32, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			converted = append(converted, nil)
			continue
		}

		vector := make([]float32, len(row))
		for i, value := range row {
			vector[i] = float32(value)
		}
		converted = append(converted, vector)
	}
	return converted
}
