package ragagent

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestLiveOpenAICompatibleProviders(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "chat and embedding providers respond"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			cfg, skip := liveProviderConfigFromEnv(os.Getenv)
			if skip != nil {
				t.Skip(skip.Reason)
			}

			ctx, cancel := context.WithTimeout(context.Background(), cfg.Chat.Timeout+30*time.Second)
			defer cancel()

			chat, err := NewOpenAIChatModel(ctx, cfg.Chat)
			if err != nil {
				t.Fatalf("NewOpenAIChatModel() error = %v", err)
			}
			embedder, err := NewOpenAIEmbedder(ctx, cfg.Embedding)
			if err != nil {
				t.Fatalf("NewOpenAIEmbedder() error = %v", err)
			}
			if err := validateLiveProvider(ctx, chat, embedder); err != nil {
				t.Fatalf("validateLiveProvider() error = %v", err)
			}
		})
	}
}
