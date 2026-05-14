package ragagent

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	liveProviderSwitchEnv       = "RAGAGENT_INTEGRATION_LIVE"
	liveProviderDefaultTimeout  = 30 * time.Second
	liveProviderValidationQuery = "Return the single word: ok"
)

type liveProviderConfig struct {
	Chat      ChatModelConfig
	Embedding EmbedderConfig
}

type liveProviderSkip struct {
	Reason string
}

func liveProviderConfigFromEnv(getenv func(string) string) (liveProviderConfig, *liveProviderSkip) {
	if getenv == nil {
		getenv = func(string) string { return "" }
	}

	if strings.TrimSpace(getenv(liveProviderSwitchEnv)) != "1" {
		return liveProviderConfig{}, &liveProviderSkip{Reason: liveProviderSwitchEnv + "=1 is required to run live provider validation"}
	}

	timeout, skip := liveProviderTimeoutFromEnv(getenv)
	if skip != nil {
		return liveProviderConfig{}, skip
	}

	chat := ChatModelConfig{
		Model:   strings.TrimSpace(getenv("RAGAGENT_CHAT_MODEL")),
		BaseURL: strings.TrimSpace(getenv("RAGAGENT_CHAT_BASE_URL")),
		APIKey:  strings.TrimSpace(getenv("RAGAGENT_CHAT_API_KEY")),
		Timeout: timeout,
	}
	embedding := EmbedderConfig{
		Model:   strings.TrimSpace(getenv("RAGAGENT_EMBEDDING_MODEL")),
		BaseURL: strings.TrimSpace(getenv("RAGAGENT_EMBEDDING_BASE_URL")),
		APIKey:  strings.TrimSpace(getenv("RAGAGENT_EMBEDDING_API_KEY")),
		Timeout: timeout,
	}
	if embedding.BaseURL == "" {
		embedding.BaseURL = chat.BaseURL
	}
	if embedding.APIKey == "" {
		embedding.APIKey = chat.APIKey
	}

	if reason := missingLiveProviderEnvReason(chat, embedding); reason != "" {
		return liveProviderConfig{}, &liveProviderSkip{Reason: reason}
	}
	if err := chat.Validate(); err != nil {
		return liveProviderConfig{}, &liveProviderSkip{Reason: fmt.Sprintf("invalid chat provider config: %v", err)}
	}
	if err := embedding.Validate(); err != nil {
		return liveProviderConfig{}, &liveProviderSkip{Reason: fmt.Sprintf("invalid embedding provider config: %v", err)}
	}

	return liveProviderConfig{Chat: chat, Embedding: embedding}, nil
}

func liveProviderTimeoutFromEnv(getenv func(string) string) (time.Duration, *liveProviderSkip) {
	raw := strings.TrimSpace(getenv("RAGAGENT_REQUEST_TIMEOUT_SEC"))
	if raw == "" {
		return liveProviderDefaultTimeout, nil
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds <= 0 {
		return 0, &liveProviderSkip{Reason: "RAGAGENT_REQUEST_TIMEOUT_SEC must be a positive integer"}
	}
	return time.Duration(seconds) * time.Second, nil
}

func missingLiveProviderEnvReason(chat ChatModelConfig, embedding EmbedderConfig) string {
	var missing []string
	if chat.Model == "" {
		missing = append(missing, "RAGAGENT_CHAT_MODEL")
	}
	if chat.BaseURL == "" {
		missing = append(missing, "RAGAGENT_CHAT_BASE_URL")
	}
	if chat.APIKey == "" {
		missing = append(missing, "RAGAGENT_CHAT_API_KEY")
	}
	if embedding.Model == "" {
		missing = append(missing, "RAGAGENT_EMBEDDING_MODEL")
	}
	if embedding.BaseURL == "" {
		missing = append(missing, "RAGAGENT_EMBEDDING_BASE_URL or RAGAGENT_CHAT_BASE_URL")
	}
	if embedding.APIKey == "" {
		missing = append(missing, "RAGAGENT_EMBEDDING_API_KEY or RAGAGENT_CHAT_API_KEY")
	}
	if len(missing) == 0 {
		return ""
	}
	return "missing required live provider env: " + strings.Join(missing, ", ")
}

func validateLiveProvider(ctx context.Context, chat ChatModel, embedder Embedder) error {
	if chat == nil {
		return fmt.Errorf("chat provider is required")
	}
	if embedder == nil {
		return fmt.Errorf("embedding provider is required")
	}

	message, err := chat.Generate(ctx, []Message{{Role: RoleUser, Content: liveProviderValidationQuery}})
	if err != nil {
		return fmt.Errorf("chat failed: %w", err)
	}
	if strings.TrimSpace(message.Content) == "" {
		return fmt.Errorf("chat returned blank content")
	}

	vectors, err := embedder.EmbedTexts(ctx, []string{"go-rag-agent live validation"})
	if err != nil {
		return fmt.Errorf("embed failed: %w", err)
	}
	if len(vectors) == 0 || len(vectors[0]) == 0 {
		return fmt.Errorf("embedding returned empty vector")
	}
	return nil
}
