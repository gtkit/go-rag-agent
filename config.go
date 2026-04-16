package ragagent

import (
	"fmt"
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
	if c.DataDir == "" {
		c.DataDir = "."
	}
	if c.TopK == 0 {
		c.TopK = 5
	}
	if c.SimilarityThreshold == 0 {
		c.SimilarityThreshold = 0.6
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

// Validate checks whether c satisfies the phase-1 configuration contract.
func (c Config) Validate() error {
	c = c.withDefaults()

	if c.ChatModel == "" {
		return fmt.Errorf("chat model is required: %w", ErrInvalidConfig)
	}
	if c.ChatBaseURL == "" {
		return fmt.Errorf("chat base url is required: %w", ErrInvalidConfig)
	}
	if c.ChatAPIKey == "" {
		return fmt.Errorf("chat api key is required: %w", ErrInvalidConfig)
	}
	if c.EmbeddingModel == "" {
		return fmt.Errorf("embedding model is required: %w", ErrInvalidConfig)
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
