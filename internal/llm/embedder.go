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

// Validate checks whether the embedding config is complete and valid.
func (c EmbeddingConfig) Validate() error {
	if strings.TrimSpace(c.Model) == "" {
		return fmt.Errorf("embedding model is required")
	}
	if strings.TrimSpace(c.BaseURL) == "" {
		return fmt.Errorf("embedding base url is required")
	}
	if strings.TrimSpace(c.APIKey) == "" {
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
