package ragagent

import (
	"fmt"
	"my-gtkit-package/go-rag-agent/internal/llm"
	"strings"
	"time"
)

// Config defines the root agent configuration contract for phase 1.
type Config struct {
	ChatModel   string
	ChatBaseURL string
	ChatAPIKey  string

	EmbeddingModel   string
	EmbeddingBaseURL string
	EmbeddingAPIKey  string

	DataDir             string
	TopK                int
	SimilarityThreshold float64
	ChunkSize           int
	ChunkOverlap        int
	MaxHistoryRounds    int
	MaxToolCalls        int
	MaxIterations       int
	RequestTimeout      time.Duration
	EnableHybridSearch  bool
	EnableRerank        bool
	Logger              Logger
	Callbacks           []Callback
}

// withDefaults returns a copy of c with phase-1 defaults applied.
func (c Config) withDefaults() Config {
	c = c.normalized()
	if c.TopK == 0 {
		c.TopK = 5
	}
	if c.ChunkSize == 0 {
		c.ChunkSize = 1000
	}
	if c.MaxHistoryRounds == 0 {
		c.MaxHistoryRounds = 8
	}
	if c.MaxToolCalls == 0 {
		c.MaxToolCalls = 4
	}
	if c.MaxIterations == 0 {
		c.MaxIterations = 3
	}
	if c.RequestTimeout == 0 {
		c.RequestTimeout = 30 * time.Second
	}
	return c
}

func (c Config) normalized() Config {
	c.ChatModel = strings.TrimSpace(c.ChatModel)
	c.ChatBaseURL = strings.TrimSpace(c.ChatBaseURL)
	c.ChatAPIKey = strings.TrimSpace(c.ChatAPIKey)
	c.EmbeddingModel = strings.TrimSpace(c.EmbeddingModel)
	c.EmbeddingBaseURL = strings.TrimSpace(c.EmbeddingBaseURL)
	c.EmbeddingAPIKey = strings.TrimSpace(c.EmbeddingAPIKey)
	c.DataDir = strings.TrimSpace(c.DataDir)
	return c
}

// Validate checks whether c satisfies the phase-1 configuration contract.
func (c Config) Validate() error {
	c = c.withDefaults()

	if err := (llm.ChatConfig{
		Model:   c.ChatModel,
		BaseURL: c.ChatBaseURL,
		APIKey:  c.ChatAPIKey,
		Timeout: c.RequestTimeout,
	}).Validate(); err != nil {
		return fmt.Errorf("chat config is invalid: %w: %w", err, ErrInvalidConfig)
	}
	if err := (llm.EmbeddingConfig{
		Model:   c.EmbeddingModel,
		BaseURL: firstNonEmpty(c.EmbeddingBaseURL, c.ChatBaseURL),
		APIKey:  firstNonEmpty(c.EmbeddingAPIKey, c.ChatAPIKey),
		Timeout: c.RequestTimeout,
	}).Validate(); err != nil {
		return fmt.Errorf("embedding config is invalid: %w: %w", err, ErrInvalidConfig)
	}
	if c.RequestTimeout <= 0 {
		return fmt.Errorf("request timeout must be positive: %w", ErrInvalidConfig)
	}
	if c.ChunkSize <= 0 {
		return fmt.Errorf("chunk size must be positive: %w", ErrInvalidConfig)
	}
	if c.ChunkOverlap < 0 || c.ChunkOverlap >= c.ChunkSize {
		return fmt.Errorf("chunk overlap must be >=0 and < chunk size: %w", ErrInvalidConfig)
	}
	if c.TopK <= 0 {
		return fmt.Errorf("topk must be positive: %w", ErrInvalidConfig)
	}
	if c.MaxHistoryRounds <= 0 {
		return fmt.Errorf("max history rounds must be positive: %w", ErrInvalidConfig)
	}
	if c.MaxToolCalls <= 0 {
		return fmt.Errorf("max tool calls must be positive: %w", ErrInvalidConfig)
	}
	if c.MaxIterations <= 0 {
		return fmt.Errorf("max iterations must be positive: %w", ErrInvalidConfig)
	}
	if c.EnableHybridSearch {
		return fmt.Errorf("enable hybrid search is phase 2: %w", ErrInvalidConfig)
	}
	if c.EnableRerank {
		return fmt.Errorf("enable rerank is phase 2: %w", ErrInvalidConfig)
	}
	return nil
}
