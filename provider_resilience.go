package ragagent

import (
	"context"
	"errors"
	"math/rand"
	"strings"
	"time"

	"github.com/pkoukk/tiktoken-go"
	langllms "github.com/tmc/langchaingo/llms"
	openailllm "github.com/tmc/langchaingo/llms/openai"

	"github.com/gtkit/go-rag-agent/internal/llm"
	"github.com/gtkit/go-rag-agent/internal/websearch"
)

const (
	providerErrorClassUnknown   = "unknown"
	providerErrorClassRateLimit = "rate_limit"
	providerErrorClassAuth      = "auth"
	providerErrorClassTransient = "transient"
	providerErrorClassPermanent = "permanent"
)

type resilientChatModel struct {
	inner      llm.ChatModel
	governance ProviderGovernanceConfig
	provider   string
	model      string
}

func newResilientChatModel(inner llm.ChatModel, governance ProviderGovernanceConfig, providerName string, model string) llm.ChatModel {
	if inner == nil {
		return nil
	}
	return &resilientChatModel{
		inner:      inner,
		governance: governance.normalized(),
		provider:   providerName,
		model:      model,
	}
}

func (m *resilientChatModel) Generate(ctx context.Context, input []llm.Message) (llm.Message, error) {
	var (
		lastErr  error
		attempts int
		started  = time.Now()
		message  llm.Message
	)
	for attempts = 1; attempts <= m.governance.RetryMaxAttempts; attempts++ {
		message, lastErr = m.inner.Generate(ctx, input)
		class := classifyProviderError(lastErr, m.provider)
		if lastErr == nil || !shouldRetryProviderError(class) || attempts == m.governance.RetryMaxAttempts {
			usage := generationInfoUsage(message.GenerationInfo)
			emitProviderTrace(ctx, ProviderCallTrace{
				Provider:         m.provider,
				Operation:        "chat_generate",
				Model:            m.model,
				Attempts:         attempts,
				Duration:         time.Since(started),
				ErrorClass:       class,
				InputTokens:      usage.PromptTokens,
				OutputTokens:     usage.CompletionTokens,
				TotalTokens:      usage.TotalTokens,
				EstimatedCostUSD: estimateChatCost(m.governance, m.model, usage.PromptTokens, usage.CompletionTokens),
				Err:              lastErr,
			})
			return message, lastErr
		}
		if err := sleepWithBackoff(ctx, m.governance, attempts); err != nil {
			return llm.Message{}, err
		}
	}
	return message, lastErr
}

func (m *resilientChatModel) Stream(ctx context.Context, input []llm.Message, emit func(string) error) error {
	var (
		lastErr   error
		attempts  int
		started   = time.Now()
		chunkSeen bool
	)
	for attempts = 1; attempts <= m.governance.RetryMaxAttempts; attempts++ {
		chunkSeen = false
		lastErr = m.inner.Stream(ctx, input, func(chunk string) error {
			if strings.TrimSpace(chunk) != "" {
				chunkSeen = true
			}
			return emit(chunk)
		})
		class := classifyProviderError(lastErr, m.provider)
		if lastErr == nil || chunkSeen || !shouldRetryProviderError(class) || attempts == m.governance.RetryMaxAttempts {
			emitProviderTrace(ctx, ProviderCallTrace{
				Provider:   m.provider,
				Operation:  "chat_stream",
				Model:      m.model,
				Attempts:   attempts,
				Duration:   time.Since(started),
				ErrorClass: class,
				Err:        lastErr,
			})
			return lastErr
		}
		if err := sleepWithBackoff(ctx, m.governance, attempts); err != nil {
			return err
		}
	}
	return lastErr
}

type resilientEmbedder struct {
	inner      llm.Embedder
	governance ProviderGovernanceConfig
	provider   string
	model      string
}

func newResilientEmbedder(inner llm.Embedder, governance ProviderGovernanceConfig, providerName string, model string) llm.Embedder {
	if inner == nil {
		return nil
	}
	return &resilientEmbedder{
		inner:      inner,
		governance: governance.normalized(),
		provider:   providerName,
		model:      model,
	}
}

func (e *resilientEmbedder) EmbedTexts(ctx context.Context, texts []string) ([][]float32, error) {
	var (
		lastErr  error
		attempts int
		rows     [][]float32
		started  = time.Now()
	)
	for attempts = 1; attempts <= e.governance.RetryMaxAttempts; attempts++ {
		rows, lastErr = e.inner.EmbedTexts(ctx, texts)
		class := classifyProviderError(lastErr, e.provider)
		if lastErr == nil || !shouldRetryProviderError(class) || attempts == e.governance.RetryMaxAttempts {
			inputTokens := 0
			for _, text := range texts {
				inputTokens += estimateProviderTokens(text)
			}
			emitProviderTrace(ctx, ProviderCallTrace{
				Provider:         e.provider,
				Operation:        "embed_texts",
				Model:            e.model,
				Attempts:         attempts,
				Duration:         time.Since(started),
				ErrorClass:       class,
				InputTokens:      inputTokens,
				TotalTokens:      inputTokens,
				EstimatedCostUSD: estimateEmbeddingCost(e.governance, e.model, inputTokens),
				Err:              lastErr,
			})
			return rows, lastErr
		}
		if err := sleepWithBackoff(ctx, e.governance, attempts); err != nil {
			return nil, err
		}
	}
	return rows, lastErr
}

type resilientSearcher struct {
	inner      websearch.Searcher
	governance ProviderGovernanceConfig
	provider   string
}

func newResilientSearcher(inner websearch.Searcher, governance ProviderGovernanceConfig, providerName string) websearch.Searcher {
	if inner == nil {
		return nil
	}
	return &resilientSearcher{
		inner:      inner,
		governance: governance.normalized(),
		provider:   providerName,
	}
}

func (s *resilientSearcher) Search(ctx context.Context, query string) ([]websearch.Result, error) {
	var (
		lastErr  error
		attempts int
		results  []websearch.Result
		started  = time.Now()
	)
	for attempts = 1; attempts <= s.governance.RetryMaxAttempts; attempts++ {
		results, lastErr = s.inner.Search(ctx, query)
		class := classifyProviderError(lastErr, s.provider)
		if lastErr == nil || !shouldRetryProviderError(class) || attempts == s.governance.RetryMaxAttempts {
			emitProviderTrace(ctx, ProviderCallTrace{
				Provider:         s.provider,
				Operation:        "web_search",
				Attempts:         attempts,
				Duration:         time.Since(started),
				ErrorClass:       class,
				EstimatedCostUSD: s.governance.Pricing.WebSearchPerCallUSD,
				Err:              lastErr,
			})
			return results, lastErr
		}
		if err := sleepWithBackoff(ctx, s.governance, attempts); err != nil {
			return nil, err
		}
	}
	return results, lastErr
}

type generationUsage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

func generationInfoUsage(info map[string]any) generationUsage {
	if len(info) == 0 {
		return generationUsage{}
	}
	return generationUsage{
		PromptTokens:     intFromAny(info["PromptTokens"]),
		CompletionTokens: intFromAny(info["CompletionTokens"]),
		TotalTokens:      intFromAny(info["TotalTokens"]),
	}
}

func intFromAny(v any) int {
	switch val := v.(type) {
	case int:
		return val
	case int32:
		return int(val)
	case int64:
		return int(val)
	case float64:
		return int(val)
	case float32:
		return int(val)
	default:
		return 0
	}
}

func estimateChatCost(cfg ProviderGovernanceConfig, model string, inputTokens int, outputTokens int) float64 {
	price := cfg.chatPrice(model)
	return (float64(inputTokens)/1000.0)*price.InputUSDPer1K + (float64(outputTokens)/1000.0)*price.OutputUSDPer1K
}

func estimateEmbeddingCost(cfg ProviderGovernanceConfig, model string, inputTokens int) float64 {
	price := cfg.embeddingPrice(model)
	return (float64(inputTokens) / 1000.0) * price.InputUSDPer1K
}

func classifyProviderError(err error, providerName string) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return providerErrorClassTransient
	}
	switch providerName {
	case "openai":
		mapped := openailllm.MapError(err)
		switch {
		case errors.Is(mapped, langllms.ErrRateLimit):
			return providerErrorClassRateLimit
		case errors.Is(mapped, langllms.ErrAuthentication):
			return providerErrorClassAuth
		case errors.Is(mapped, langllms.ErrTimeout), errors.Is(mapped, langllms.ErrProviderUnavailable):
			return providerErrorClassTransient
		case errors.Is(mapped, langllms.ErrInvalidRequest), errors.Is(mapped, langllms.ErrTokenLimit):
			return providerErrorClassPermanent
		}
	}

	lower := strings.ToLower(err.Error())
	switch {
	case strings.Contains(lower, "429"), strings.Contains(lower, "rate limit"), strings.Contains(lower, "too many requests"):
		return providerErrorClassRateLimit
	case strings.Contains(lower, "401"), strings.Contains(lower, "403"), strings.Contains(lower, "unauthorized"), strings.Contains(lower, "forbidden"), strings.Contains(lower, "api key"):
		return providerErrorClassAuth
	case strings.Contains(lower, "timeout"), strings.Contains(lower, "temporarily unavailable"), strings.Contains(lower, "connection reset"), strings.Contains(lower, "503"), strings.Contains(lower, "502"), strings.Contains(lower, "504"):
		return providerErrorClassTransient
	case strings.Contains(lower, "400"), strings.Contains(lower, "404"), strings.Contains(lower, "invalid request"), strings.Contains(lower, "not found"):
		return providerErrorClassPermanent
	default:
		return providerErrorClassUnknown
	}
}

func shouldRetryProviderError(class string) bool {
	switch class {
	case providerErrorClassTransient, providerErrorClassRateLimit:
		return true
	default:
		return false
	}
}

func sleepWithBackoff(ctx context.Context, cfg ProviderGovernanceConfig, attempt int) error {
	delay := cfg.RetryBaseDelay
	for i := 1; i < attempt; i++ {
		delay *= 2
		if delay > cfg.RetryMaxDelay {
			delay = cfg.RetryMaxDelay
			break
		}
	}
	if delay <= 0 {
		return nil
	}
	jitter := time.Duration(rand.Int63n(int64(max(delay/4, time.Millisecond))))
	timer := time.NewTimer(minDuration(delay+jitter, cfg.RetryMaxDelay))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func estimateProviderTokens(text string) int {
	if strings.TrimSpace(text) == "" {
		return 0
	}
	enc, err := tiktoken.EncodingForModel("gpt-4o-mini")
	if err != nil {
		enc, err = tiktoken.GetEncoding(tiktoken.MODEL_CL100K_BASE)
		if err != nil {
			return max(1, len([]rune(text))/4)
		}
	}
	return len(enc.Encode(text, nil, nil))
}
