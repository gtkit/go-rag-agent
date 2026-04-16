package llm

import (
	"strings"
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
			name: "valid config with whitespace padding",
			mutate: func(cfg *ChatConfig) {
				cfg.Model = "  gpt-4.1-mini  "
				cfg.BaseURL = "  https://api.example.com/v1  "
				cfg.APIKey = "  test-key  "
			},
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
			name: "invalid base url",
			mutate: func(cfg *ChatConfig) {
				cfg.BaseURL = "://bad-url"
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

func TestChatConfigNormalized(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  ChatConfig
		want ChatConfig
	}{
		{
			name: "trims model base url and api key",
			cfg: ChatConfig{
				Model:   "  gpt-4.1-mini  ",
				BaseURL: "  https://api.example.com/v1  ",
				APIKey:  "  key  ",
				Timeout: 30 * time.Second,
			},
			want: ChatConfig{
				Model:   "gpt-4.1-mini",
				BaseURL: "https://api.example.com/v1",
				APIKey:  "key",
				Timeout: 30 * time.Second,
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.cfg.normalized()
			if got != tc.want {
				t.Fatalf("normalized() = %#v, want %#v", got, tc.want)
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
			name: "valid config with whitespace padding",
			mutate: func(cfg *EmbeddingConfig) {
				cfg.Model = "  text-embedding-3-small  "
				cfg.BaseURL = "  https://api.example.com/v1  "
				cfg.APIKey = "  test-key  "
			},
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
			name: "invalid base url",
			mutate: func(cfg *EmbeddingConfig) {
				cfg.BaseURL = "http://"
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

func TestEmbeddingConfigNormalized(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  EmbeddingConfig
		want EmbeddingConfig
	}{
		{
			name: "trims model base url and api key",
			cfg: EmbeddingConfig{
				Model:   "  text-embedding-3-small  ",
				BaseURL: "  https://api.example.com/v1  ",
				APIKey:  "  key  ",
				Timeout: 15 * time.Second,
			},
			want: EmbeddingConfig{
				Model:   "text-embedding-3-small",
				BaseURL: "https://api.example.com/v1",
				APIKey:  "key",
				Timeout: 15 * time.Second,
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.cfg.normalized()
			if got != tc.want {
				t.Fatalf("normalized() = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestNormalizeEmbeddingTexts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     []string
		want      []string
		wantErr   bool
		errPrefix string
	}{
		{
			name:      "nil input rejected",
			input:     nil,
			wantErr:   true,
			errPrefix: "embedding texts must not be empty",
		},
		{
			name:      "empty input rejected",
			input:     []string{},
			wantErr:   true,
			errPrefix: "embedding texts must not be empty",
		},
		{
			name:      "blank text rejected",
			input:     []string{"valid", "  "},
			wantErr:   true,
			errPrefix: "embedding text at index 1 is blank",
		},
		{
			name:  "texts are trimmed",
			input: []string{"  a  ", "\tb\t"},
			want:  []string{"a", "b"},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := normalizeEmbeddingTexts(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("normalizeEmbeddingTexts() error = nil, want non-nil")
				}
				if tc.errPrefix != "" && !strings.Contains(err.Error(), tc.errPrefix) {
					t.Fatalf("normalizeEmbeddingTexts() error = %q, want contains %q", err.Error(), tc.errPrefix)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeEmbeddingTexts() unexpected error = %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("len(normalizeEmbeddingTexts()) = %d, want %d", len(got), len(tc.want))
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("value[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestOpenAIEmbedderEmbedTextsFailFastValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		texts     []string
		wantErr   bool
		errPrefix string
	}{
		{
			name:      "rejects nil input",
			texts:     nil,
			wantErr:   true,
			errPrefix: "validate embedding input: embedding texts must not be empty",
		},
		{
			name:      "rejects empty input",
			texts:     []string{},
			wantErr:   true,
			errPrefix: "validate embedding input: embedding texts must not be empty",
		},
		{
			name:      "rejects blank input",
			texts:     []string{"   "},
			wantErr:   true,
			errPrefix: "validate embedding input: embedding text at index 0 is blank",
		},
		{
			name:      "non-empty reaches nil-client guard",
			texts:     []string{"  hello  "},
			wantErr:   true,
			errPrefix: "openai embedder is nil",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			embedder := &OpenAIEmbedder{}
			_, err := embedder.EmbedTexts(t.Context(), tc.texts)
			if tc.wantErr && err == nil {
				t.Fatalf("EmbedTexts() error = nil, want non-nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("EmbedTexts() unexpected error = %v", err)
			}
			if tc.errPrefix != "" && (err == nil || !strings.Contains(err.Error(), tc.errPrefix)) {
				t.Fatalf("EmbedTexts() error = %v, want contains %q", err, tc.errPrefix)
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
