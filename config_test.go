package ragagent

import (
	"errors"
	"testing"
	"time"
)

func TestConfigValidate(t *testing.T) {
	t.Parallel()

	valid := Config{
		ChatModel:           "gpt-4.1-mini",
		ChatBaseURL:         "https://api.example.com/v1",
		ChatAPIKey:          "test-key",
		EmbeddingModel:      "text-embedding-3-small",
		RequestTimeout:      time.Second,
		ChunkSize:           1000,
		ChunkOverlap:        100,
		TopK:                5,
		MaxHistoryRounds:    8,
		MaxToolCalls:        4,
		MaxIterations:       3,
		SimilarityThreshold: 0.6,
	}

	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr error
	}{
		{
			name:    "valid minimal config",
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
