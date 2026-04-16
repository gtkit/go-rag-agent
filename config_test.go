package ragagent

import (
	"errors"
	"testing"
	"time"
)

func TestConfigValidate(t *testing.T) {
	t.Parallel()

	valid := Config{
		ChatModel:      "gpt-4.1-mini",
		ChatBaseURL:    "https://api.example.com/v1",
		ChatAPIKey:     "test-key",
		EmbeddingModel: "text-embedding-3-small",
	}

	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr error
	}{
		{
			name:    "valid minimal config relying on defaults",
			mutate:  nil,
			wantErr: nil,
		},
		{
			name: "missing chat model",
			mutate: func(cfg *Config) {
				cfg.ChatModel = ""
			},
			wantErr: ErrInvalidConfig,
		},
		{
			name: "invalid chunk overlap",
			mutate: func(cfg *Config) {
				cfg.ChunkSize = 100
				cfg.ChunkOverlap = cfg.ChunkSize
			},
			wantErr: ErrInvalidConfig,
		},
		{
			name: "phase two flags rejected",
			mutate: func(cfg *Config) {
				cfg.EnableHybridSearch = true
			},
			wantErr: ErrInvalidConfig,
		},
		{
			name: "chat url invalid",
			mutate: func(cfg *Config) {
				cfg.ChatBaseURL = "://bad-url"
			},
			wantErr: ErrInvalidConfig,
		},
		{
			name: "embedding fallback to chat credentials is valid",
			mutate: func(cfg *Config) {
				cfg.EmbeddingBaseURL = ""
				cfg.EmbeddingAPIKey = ""
			},
			wantErr: nil,
		},
		{
			name: "whitespace inputs are trimmed before validation",
			mutate: func(cfg *Config) {
				cfg.ChatModel = " gpt-4.1-mini "
				cfg.ChatBaseURL = " https://api.example.com/v1 "
				cfg.ChatAPIKey = " test-key "
				cfg.EmbeddingModel = " text-embedding-3-small "
			},
			wantErr: nil,
		},
		{
			name: "whitespace only string is invalid",
			mutate: func(cfg *Config) {
				cfg.ChatAPIKey = "   "
			},
			wantErr: ErrInvalidConfig,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := valid
			if tc.mutate != nil {
				tc.mutate(&cfg)
			}

			err := cfg.Validate()
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("Validate() unexpected error: %v", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Validate() error = %v, want errors.Is(..., %v)", err, tc.wantErr)
			}
		})
	}
}

func TestConfigWithDefaults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		cfg   Config
		check func(t *testing.T, got Config)
	}{
		{
			name: "empty data dir remains empty for in-memory storage",
			cfg:  Config{},
			check: func(t *testing.T, got Config) {
				t.Helper()
				if got.DataDir != "" {
					t.Fatalf("withDefaults() DataDir = %q, want empty", got.DataDir)
				}
			},
		},
		{
			name: "similarity threshold zero is preserved",
			cfg: Config{
				SimilarityThreshold: 0,
			},
			check: func(t *testing.T, got Config) {
				t.Helper()
				if got.SimilarityThreshold != 0 {
					t.Fatalf("withDefaults() SimilarityThreshold = %v, want 0", got.SimilarityThreshold)
				}
			},
		},
		{
			name: "non-zero defaults still apply",
			cfg:  Config{},
			check: func(t *testing.T, got Config) {
				t.Helper()
				if got.TopK != 5 {
					t.Fatalf("withDefaults() TopK = %d, want 5", got.TopK)
				}
				if got.ChunkSize != 1000 {
					t.Fatalf("withDefaults() ChunkSize = %d, want 1000", got.ChunkSize)
				}
				if got.RequestTimeout != 30*time.Second {
					t.Fatalf("withDefaults() RequestTimeout = %v, want %v", got.RequestTimeout, 30*time.Second)
				}
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.cfg.withDefaults()
			tc.check(t, got)
		})
	}
}
