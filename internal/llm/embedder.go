package llm

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// EmbeddingConfig holds construction config for embedding adapters.
type EmbeddingConfig struct {
	Model   string
	BaseURL string
	APIKey  string
	Timeout time.Duration
}

func (c EmbeddingConfig) normalized() EmbeddingConfig {
	c.Model = strings.TrimSpace(c.Model)
	c.BaseURL = strings.TrimSpace(c.BaseURL)
	c.APIKey = strings.TrimSpace(c.APIKey)
	return c
}

// Validate checks whether the embedding config is complete and valid.
func (c EmbeddingConfig) Validate() error {
	c = c.normalized()

	if c.Model == "" {
		return fmt.Errorf("embedding model is required")
	}
	if c.BaseURL == "" {
		return fmt.Errorf("embedding base url is required")
	}
	if _, err := parseAndValidateBaseURL(c.BaseURL); err != nil {
		return fmt.Errorf("embedding base url is invalid: %w", err)
	}
	if c.APIKey == "" {
		return fmt.Errorf("embedding api key is required")
	}
	if c.Timeout <= 0 {
		return fmt.Errorf("embedding timeout must be positive")
	}
	return nil
}

// Embedder is the abstraction used by upper layers.
type Embedder interface {
	EmbedTexts(ctx context.Context, texts []string) ([][]float32, error)
}

func normalizeEmbeddingTexts(texts []string) ([]string, error) {
	if len(texts) == 0 {
		return nil, fmt.Errorf("embedding texts must not be empty")
	}

	normalized := make([]string, 0, len(texts))
	for i, text := range texts {
		trimmed := strings.TrimSpace(text)
		if trimmed == "" {
			return nil, fmt.Errorf("embedding text at index %d is blank", i)
		}
		normalized = append(normalized, trimmed)
	}
	return normalized, nil
}
