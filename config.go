package ragagent

import (
	"fmt"
	"strings"
	"time"

	"github.com/gtkit/go-rag-agent/internal/llm"
)

// StorageComponents 定义向量存储、文档加载和 rerank 的可选注入组件。
type StorageComponents struct {
	VectorStore    VectorStore
	DocumentLoader DocumentLoader
	Reranker       Reranker
}

// Config 定义 Phase 1 根 Agent 的配置契约。
type Config struct {
	ChatModel   string
	ChatBaseURL string
	ChatAPIKey  string

	EmbeddingModel   string
	EmbeddingBaseURL string
	EmbeddingAPIKey  string

	DataDir                   string
	TopK                      int
	SimilarityThreshold       float64
	ChunkSize                 int
	ChunkOverlap              int
	MaxHistoryRounds          int
	MaxToolCalls              int
	MaxIterations             int
	RequestTimeout            time.Duration
	MaxExecutionDuration      time.Duration
	MaxPromptTokens           int
	MaxHistoryTokens          int
	MaxEvidenceTokens         int
	MaxSummaryTokens          int
	EnablePromptHardening     bool
	EnableHybridSearch        bool
	EnableRerank              bool
	EnableWebSearch           bool
	HybridCandidateMultiplier int
	HybridRRFK                float64
	RerankShortlistMultiplier int
	Runtime                   RuntimeComponents
	Storage                   StorageComponents
	ToolRegistry              *ToolRegistry
	PDFOCRBridge              PDFOCRBridgeConfig
	WebSearch                 WebSearchConfig
	Logger                    Logger
	TraceRecorder             TraceRecorder
	Callbacks                 []Callback
}

// withDefaults 返回一个应用了 Phase 1 默认值的配置副本。
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
	if c.MaxPromptTokens == 0 {
		c.MaxPromptTokens = 4096
	}
	if c.MaxHistoryTokens == 0 {
		c.MaxHistoryTokens = 1024
	}
	if c.MaxEvidenceTokens == 0 {
		c.MaxEvidenceTokens = 2048
	}
	if c.MaxSummaryTokens == 0 {
		c.MaxSummaryTokens = 256
	}
	if !c.EnablePromptHardening {
		c.EnablePromptHardening = true
	}
	if c.HybridCandidateMultiplier == 0 {
		c.HybridCandidateMultiplier = 4
	}
	if c.HybridRRFK == 0 {
		c.HybridRRFK = 60
	}
	if c.RerankShortlistMultiplier == 0 {
		c.RerankShortlistMultiplier = 2
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
	c.PDFOCRBridge = c.PDFOCRBridge.normalized()
	c.WebSearch = c.WebSearch.normalized()
	return c
}

// Validate 检查配置是否满足 Phase 1 的约束。
func (c Config) Validate() error {
	c = c.withDefaults()

	if c.Runtime.ChatModel == nil {
		if err := (llm.ChatConfig{
			Model:   c.ChatModel,
			BaseURL: c.ChatBaseURL,
			APIKey:  c.ChatAPIKey,
			Timeout: c.RequestTimeout,
		}).Validate(); err != nil {
			return fmt.Errorf("chat config is invalid: %w: %w", err, ErrInvalidConfig)
		}
	}
	if c.Runtime.Embedder == nil {
		if err := (llm.EmbeddingConfig{
			Model:   c.EmbeddingModel,
			BaseURL: firstNonEmpty(c.EmbeddingBaseURL, c.ChatBaseURL),
			APIKey:  firstNonEmpty(c.EmbeddingAPIKey, c.ChatAPIKey),
			Timeout: c.RequestTimeout,
		}).Validate(); err != nil {
			return fmt.Errorf("embedding config is invalid: %w: %w", err, ErrInvalidConfig)
		}
	}
	if c.RequestTimeout <= 0 {
		return fmt.Errorf("request timeout must be positive: %w", ErrInvalidConfig)
	}
	if c.MaxExecutionDuration < 0 {
		return fmt.Errorf("max execution duration must be non-negative: %w", ErrInvalidConfig)
	}
	if c.MaxPromptTokens <= 0 {
		return fmt.Errorf("max prompt tokens must be positive: %w", ErrInvalidConfig)
	}
	if c.MaxHistoryTokens <= 0 {
		return fmt.Errorf("max history tokens must be positive: %w", ErrInvalidConfig)
	}
	if c.MaxEvidenceTokens <= 0 {
		return fmt.Errorf("max evidence tokens must be positive: %w", ErrInvalidConfig)
	}
	if c.MaxSummaryTokens <= 0 {
		return fmt.Errorf("max summary tokens must be positive: %w", ErrInvalidConfig)
	}
	if c.ChunkSize <= 0 {
		return fmt.Errorf("chunk size must be positive: %w", ErrInvalidConfig)
	}
	if c.ChunkSize > maxEvidenceChars {
		return fmt.Errorf("chunk size must be <= %d: %w", maxEvidenceChars, ErrInvalidConfig)
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
	if c.EnableRerank && !c.EnableHybridSearch {
		return fmt.Errorf("enable rerank requires hybrid search: %w", ErrInvalidConfig)
	}
	if c.HybridCandidateMultiplier <= 0 {
		return fmt.Errorf("hybrid candidate multiplier must be positive: %w", ErrInvalidConfig)
	}
	if c.HybridRRFK <= 0 {
		return fmt.Errorf("hybrid rrf k must be positive: %w", ErrInvalidConfig)
	}
	if c.RerankShortlistMultiplier <= 0 {
		return fmt.Errorf("rerank shortlist multiplier must be positive: %w", ErrInvalidConfig)
	}
	if err := c.WebSearch.validate(c.EnableWebSearch); err != nil {
		return err
	}
	if err := c.PDFOCRBridge.validate(); err != nil {
		return err
	}
	return nil
}
