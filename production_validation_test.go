package ragagent

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLiveProviderConfigFromEnv(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		env        map[string]string
		wantSkip   bool
		wantReason string
		wantChat   ChatModelConfig
		wantEmbed  EmbedderConfig
	}{
		{
			name:       "skips when live switch is missing",
			env:        map[string]string{},
			wantSkip:   true,
			wantReason: "RAGAGENT_INTEGRATION_LIVE=1",
		},
		{
			name: "skips when provider config is incomplete",
			env: map[string]string{
				"RAGAGENT_INTEGRATION_LIVE": "1",
				"RAGAGENT_CHAT_MODEL":       "gpt-live",
			},
			wantSkip:   true,
			wantReason: "RAGAGENT_CHAT_BASE_URL",
		},
		{
			name: "uses chat provider as embedding fallback",
			env: map[string]string{
				"RAGAGENT_INTEGRATION_LIVE": "1",
				"RAGAGENT_CHAT_MODEL":       "gpt-live",
				"RAGAGENT_CHAT_BASE_URL":    "https://api.example.test/v1",
				"RAGAGENT_CHAT_API_KEY":     "chat-key",
				"RAGAGENT_EMBEDDING_MODEL":  "embed-live",
			},
			wantChat: ChatModelConfig{
				Model:   "gpt-live",
				BaseURL: "https://api.example.test/v1",
				APIKey:  "chat-key",
				Timeout: 30 * time.Second,
			},
			wantEmbed: EmbedderConfig{
				Model:   "embed-live",
				BaseURL: "https://api.example.test/v1",
				APIKey:  "chat-key",
				Timeout: 30 * time.Second,
			},
		},
		{
			name: "uses explicit embedding provider",
			env: map[string]string{
				"RAGAGENT_INTEGRATION_LIVE":    "1",
				"RAGAGENT_CHAT_MODEL":          "gpt-live",
				"RAGAGENT_CHAT_BASE_URL":       "https://chat.example.test/v1",
				"RAGAGENT_CHAT_API_KEY":        "chat-key",
				"RAGAGENT_EMBEDDING_MODEL":     "embed-live",
				"RAGAGENT_EMBEDDING_BASE_URL":  "https://embed.example.test/v1",
				"RAGAGENT_EMBEDDING_API_KEY":   "embed-key",
				"RAGAGENT_REQUEST_TIMEOUT_SEC": "45",
			},
			wantChat: ChatModelConfig{
				Model:   "gpt-live",
				BaseURL: "https://chat.example.test/v1",
				APIKey:  "chat-key",
				Timeout: 45 * time.Second,
			},
			wantEmbed: EmbedderConfig{
				Model:   "embed-live",
				BaseURL: "https://embed.example.test/v1",
				APIKey:  "embed-key",
				Timeout: 45 * time.Second,
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg, skip := liveProviderConfigFromEnv(envMap(tt.env))
			if (skip != nil) != tt.wantSkip {
				t.Fatalf("skip = %v, wantSkip %v", skip, tt.wantSkip)
			}
			if skip != nil {
				if !strings.Contains(skip.Reason, tt.wantReason) {
					t.Fatalf("skip reason = %q, want containing %q", skip.Reason, tt.wantReason)
				}
				return
			}
			if cfg.Chat != tt.wantChat {
				t.Fatalf("chat config = %#v, want %#v", cfg.Chat, tt.wantChat)
			}
			if cfg.Embedding != tt.wantEmbed {
				t.Fatalf("embedding config = %#v, want %#v", cfg.Embedding, tt.wantEmbed)
			}
		})
	}
}

func TestLiveProviderValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		chat      ChatModel
		embedder  Embedder
		wantErr   bool
		errString string
	}{
		{
			name:     "validates minimal provider calls",
			chat:     stubLiveChatModel{message: "ok"},
			embedder: stubLiveEmbedder{vectors: [][]float32{{1}}},
		},
		{
			name:      "returns chat error",
			chat:      stubLiveChatModel{err: errors.New("chat failed")},
			embedder:  stubLiveEmbedder{vectors: [][]float32{{1}}},
			wantErr:   true,
			errString: "chat failed",
		},
		{
			name:      "rejects blank chat response",
			chat:      stubLiveChatModel{message: " "},
			embedder:  stubLiveEmbedder{vectors: [][]float32{{1}}},
			wantErr:   true,
			errString: "blank",
		},
		{
			name:      "returns embedding error",
			chat:      stubLiveChatModel{message: "ok"},
			embedder:  stubLiveEmbedder{err: errors.New("embed failed")},
			wantErr:   true,
			errString: "embed failed",
		},
		{
			name:      "rejects empty embedding",
			chat:      stubLiveChatModel{message: "ok"},
			embedder:  stubLiveEmbedder{vectors: [][]float32{{}}},
			wantErr:   true,
			errString: "empty",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateLiveProvider(context.Background(), tt.chat, tt.embedder)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateLiveProvider() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), tt.errString) {
				t.Fatalf("error = %q, want containing %q", err.Error(), tt.errString)
			}
		})
	}
}

func TestProductionAssets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		path       string
		contains   []string
		forbidden  []string
		minSecrets int
	}{
		{
			name: "env template lists production variables without real secrets",
			path: ".env.production.example",
			contains: []string{
				"RAGAGENT_CHAT_MODEL=",
				"RAGAGENT_CHAT_BASE_URL=",
				"RAGAGENT_CHAT_API_KEY=",
				"RAGAGENT_EMBEDDING_MODEL=",
				"RAGAGENT_PGVECTOR_DSN=",
				"RAGAGENT_REQUEST_TIMEOUT_SEC=",
				"RAGAGENT_TOP_K=",
				"RAGAGENT_TRACE_JSONL_PATH=",
			},
			forbidden: []string{
				"sk-",
				"Bearer ",
				"postgres://user:pass@",
			},
		},
		{
			name: "pgvector compose declares healthcheck and volume",
			path: "deploy/compose/pgvector.compose.yml",
			contains: []string{
				"pgvector/pgvector",
				"healthcheck:",
				"pg_isready",
				"volumes:",
				"pgvector-data:",
			},
			forbidden: []string{
				"RAGAGENT_CHAT_API_KEY",
				"sk-",
			},
		},
		{
			name: "production runbook documents validation matrix",
			path: "docs/production.md",
			contains: []string{
				"go test -count=1 -cover ./...",
				"RAGAGENT_INTEGRATION_LIVE=1",
				"RAGAGENT_PGVECTOR_TEST_DSN",
				"golangci-lint run ./...",
				"go test -race -count=1 -timeout=5m ./...",
				"Shell",
				"数据库执行",
				"Cron",
			},
			forbidden: []string{
				"replace-with-your",
				"sk-",
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data, err := os.ReadFile(tt.path)
			if err != nil {
				t.Fatalf("ReadFile(%q) error = %v", tt.path, err)
			}
			text := string(data)
			for _, want := range tt.contains {
				if !strings.Contains(text, want) {
					t.Fatalf("%s missing %q", tt.path, want)
				}
			}
			for _, forbidden := range tt.forbidden {
				if strings.Contains(text, forbidden) {
					t.Fatalf("%s contains forbidden %q", tt.path, forbidden)
				}
			}
		})
	}
}

func TestREADMEReferencesProductionRunbook(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		contains []string
	}{
		{
			name: "readme points to production validation assets",
			contains: []string{
				"docs/production.md",
				".env.production.example",
				"deploy/compose/pgvector.compose.yml",
				"RAGAGENT_INTEGRATION_LIVE=1",
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data, err := os.ReadFile("README.md")
			if err != nil {
				t.Fatalf("ReadFile README.md error = %v", err)
			}
			text := string(data)
			for _, want := range tt.contains {
				if !strings.Contains(text, want) {
					t.Fatalf("README missing %q", want)
				}
			}
		})
	}
}

type stubLiveChatModel struct {
	message string
	err     error
}

func (m stubLiveChatModel) Generate(context.Context, []Message) (Message, error) {
	if m.err != nil {
		return Message{}, m.err
	}
	return Message{Role: RoleAssistant, Content: m.message}, nil
}

func (m stubLiveChatModel) Stream(context.Context, []Message, func(string) error) error {
	return nil
}

type stubLiveEmbedder struct {
	vectors [][]float32
	err     error
}

func (e stubLiveEmbedder) EmbedTexts(context.Context, []string) ([][]float32, error) {
	if e.err != nil {
		return nil, e.err
	}
	return e.vectors, nil
}

func envMap(values map[string]string) func(string) string {
	return func(key string) string {
		return values[key]
	}
}
