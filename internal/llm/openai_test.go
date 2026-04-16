package llm

import (
	"testing"
	"time"
)

func TestChatConfigValidate(t *testing.T) {
	t.Parallel()

	valid := ChatConfig{
		Model:   "gpt-4.1-mini",
		BaseURL: "https://api.example.com/v1",
		APIKey:  "test-key",
		Timeout: 30 * time.Second,
	}

	tests := []struct {
		name    string
		mutate  func(*ChatConfig)
		wantErr bool
	}{
		{
			name:    "valid config",
			mutate:  nil,
			wantErr: false,
		},
		{
			name: "missing model",
			mutate: func(cfg *ChatConfig) {
				cfg.Model = ""
			},
			wantErr: true,
		},
		{
			name: "missing base url",
			mutate: func(cfg *ChatConfig) {
				cfg.BaseURL = ""
			},
			wantErr: true,
		},
		{
			name: "missing api key",
			mutate: func(cfg *ChatConfig) {
				cfg.APIKey = ""
			},
			wantErr: true,
		},
		{
			name: "non-positive timeout",
			mutate: func(cfg *ChatConfig) {
				cfg.Timeout = 0
			},
			wantErr: true,
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
			if tc.wantErr && err == nil {
				t.Fatalf("Validate() error = nil, want non-nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("Validate() unexpected error = %v", err)
			}
		})
	}
}

func TestEmbeddingConfigValidate(t *testing.T) {
	t.Parallel()

	valid := EmbeddingConfig{
		Model:   "text-embedding-3-small",
		BaseURL: "https://api.example.com/v1",
		APIKey:  "test-key",
		Timeout: 15 * time.Second,
	}

	tests := []struct {
		name    string
		mutate  func(*EmbeddingConfig)
		wantErr bool
	}{
		{
			name:    "valid config",
			mutate:  nil,
			wantErr: false,
		},
		{
			name: "missing model",
			mutate: func(cfg *EmbeddingConfig) {
				cfg.Model = ""
			},
			wantErr: true,
		},
		{
			name: "missing base url",
			mutate: func(cfg *EmbeddingConfig) {
				cfg.BaseURL = ""
			},
			wantErr: true,
		},
		{
			name: "missing api key",
			mutate: func(cfg *EmbeddingConfig) {
				cfg.APIKey = ""
			},
			wantErr: true,
		},
		{
			name: "negative timeout",
			mutate: func(cfg *EmbeddingConfig) {
				cfg.Timeout = -time.Second
			},
			wantErr: true,
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
			if tc.wantErr && err == nil {
				t.Fatalf("Validate() error = nil, want non-nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("Validate() unexpected error = %v", err)
			}
		})
	}
}

func TestConvertEmbeddingRows(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input [][]float64
		want  [][]float32
	}{
		{
			name: "converts each value",
			input: [][]float64{
				{1.5, -2.25, 0},
				{3.125},
			},
			want: [][]float32{
				{1.5, -2.25, 0},
				{3.125},
			},
		},
		{
			name:  "empty rows",
			input: [][]float64{},
			want:  [][]float32{},
		},
		{
			name:  "nil rows",
			input: nil,
			want:  nil,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := convertEmbeddingRows(tc.input)
			if len(got) != len(tc.want) {
				t.Fatalf("len(convertEmbeddingRows()) = %d, want %d", len(got), len(tc.want))
			}

			for i := range tc.want {
				if len(got[i]) != len(tc.want[i]) {
					t.Fatalf("len(row %d) = %d, want %d", i, len(got[i]), len(tc.want[i]))
				}
				for j := range tc.want[i] {
					if got[i][j] != tc.want[i][j] {
						t.Fatalf("value[%d][%d] = %v, want %v", i, j, got[i][j], tc.want[i][j])
					}
				}
			}
		})
	}
}
