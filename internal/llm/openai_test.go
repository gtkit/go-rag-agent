package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	provider "github.com/gtkit/go-llm-provider/v2/provider"
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
			name: "non-http scheme base url",
			mutate: func(cfg *EmbeddingConfig) {
				cfg.BaseURL = "ftp://api.example.com/v1"
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
		{
			name:      "nil receiver reaches nil-client guard",
			texts:     []string{"hello"},
			wantErr:   true,
			errPrefix: "openai embedder is nil",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var embedder *OpenAIEmbedder
			if tc.name != "nil receiver reaches nil-client guard" {
				embedder = &OpenAIEmbedder{}
			}
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

func TestOpenAIChatModelFailFastAndMessageConversion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		run     func() error
		wantErr string
	}{
		{
			name: "constructor rejects invalid config",
			run: func() error {
				_, err := NewOpenAIChatModel(context.Background(), ChatConfig{})
				return err
			},
			wantErr: "validate chat config",
		},
		{
			name: "generate rejects nil client",
			run: func() error {
				_, err := (&OpenAIChatModel{}).Generate(context.Background(), []Message{{Role: RoleUser, Content: "hello"}})
				return err
			},
			wantErr: "openai chat model is nil",
		},
		{
			name: "stream rejects nil client",
			run: func() error {
				return (&OpenAIChatModel{}).Stream(context.Background(), []Message{{Role: RoleUser, Content: "hello"}}, func(string) error { return nil })
			},
			wantErr: "openai chat model is nil",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.run()
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("run() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}

	msgs := toProviderMessages([]Message{
		{Role: RoleSystem, Content: "system"},
		{Role: RoleAssistant, Content: "assistant"},
		{Role: RoleUser, Content: "user"},
		{Role: Role("custom"), Content: "custom"},
	})
	if len(msgs) != 4 {
		t.Fatalf("len(toProviderMessages()) = %d, want 4", len(msgs))
	}
	wantRoles := []provider.Role{provider.RoleSystem, provider.RoleAssistant, provider.RoleUser, provider.RoleUser}
	for i, want := range []string{"system", "assistant", "user", "custom"} {
		if len(msgs[i].Content) != 1 || msgs[i].Content[0].Text != want {
			t.Fatalf("message[%d] text = %+v, want %q", i, msgs[i].Content, want)
		}
		if msgs[i].Role != wantRoles[i] {
			t.Fatalf("message[%d] role = %q, want %q", i, msgs[i].Role, wantRoles[i])
		}
	}
}

func TestOpenAIEmbedderConstructorValidation(t *testing.T) {
	t.Parallel()

	_, err := NewOpenAIEmbedder(context.Background(), EmbeddingConfig{})
	if err == nil || !strings.Contains(err.Error(), "validate embedding config") {
		t.Fatalf("NewOpenAIEmbedder() error = %v, want validation error", err)
	}
}

func TestOpenAIConstructorsCreateClients(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"not used"}`))
	}))
	t.Cleanup(server.Close)

	chat, err := NewOpenAIChatModel(context.Background(), ChatConfig{
		Model:   "test-chat",
		BaseURL: server.URL,
		APIKey:  "test-key",
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewOpenAIChatModel() error = %v", err)
	}
	if chat == nil {
		t.Fatal("NewOpenAIChatModel() = nil, want model")
	}

	embedder, err := NewOpenAIEmbedder(context.Background(), EmbeddingConfig{
		Model:   "test-embedding",
		BaseURL: server.URL,
		APIKey:  "test-key",
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewOpenAIEmbedder() error = %v", err)
	}
	if embedder == nil {
		t.Fatal("NewOpenAIEmbedder() = nil, want embedder")
	}
}

// fakeOpenAIServer 返回一个最小的 OpenAI 兼容服务，覆盖 chat（流式/非流式）与 embedding。
func fakeOpenAIServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		if strings.Contains(string(buf), "\"stream\":true") {
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, _ := w.(http.Flusher)
			for _, delta := range []string{"你好", "，世界"} {
				_, _ = w.Write([]byte("data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"" + delta + "\"}}]}\n\n"))
				if flusher != nil {
					flusher.Flush()
				}
			}
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"index":0,"message":{"role":"assistant","content":"你好，世界"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`))
	})
	mux.HandleFunc("/embeddings", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","model":"test-embedding","data":[{"object":"embedding","index":0,"embedding":[0.1,0.2,0.3]},{"object":"embedding","index":1,"embedding":[0.4,0.5,0.6]}],"usage":{"prompt_tokens":4,"total_tokens":4}}`))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func TestOpenAIChatModelGenerateMapsContentAndUsage(t *testing.T) {
	t.Parallel()

	server := fakeOpenAIServer(t)
	model, err := NewOpenAIChatModel(context.Background(), ChatConfig{
		Model: "test-chat", BaseURL: server.URL, APIKey: "k", Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewOpenAIChatModel() error = %v", err)
	}

	msg, err := model.Generate(context.Background(), []Message{{Role: RoleUser, Content: "hi"}})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if msg.Role != RoleAssistant || msg.Content != "你好，世界" {
		t.Fatalf("Generate() msg = %+v, want assistant 你好，世界", msg)
	}
	if got := msg.GenerationInfo["TotalTokens"]; got != 15 {
		t.Fatalf("GenerationInfo[TotalTokens] = %v, want 15", got)
	}
	if got := msg.GenerationInfo["PromptTokens"]; got != 10 {
		t.Fatalf("GenerationInfo[PromptTokens] = %v, want 10", got)
	}
}

func TestOpenAIChatModelStreamEmitsDeltas(t *testing.T) {
	t.Parallel()

	server := fakeOpenAIServer(t)
	model, err := NewOpenAIChatModel(context.Background(), ChatConfig{
		Model: "test-chat", BaseURL: server.URL, APIKey: "k", Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewOpenAIChatModel() error = %v", err)
	}

	var sb strings.Builder
	if err := model.Stream(context.Background(), []Message{{Role: RoleUser, Content: "hi"}}, func(chunk string) error {
		sb.WriteString(chunk)
		return nil
	}); err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if sb.String() != "你好，世界" {
		t.Fatalf("Stream() concatenated = %q, want 你好，世界", sb.String())
	}
}

func TestOpenAIEmbedderEmbedTextsReturnsVectors(t *testing.T) {
	t.Parallel()

	server := fakeOpenAIServer(t)
	embedder, err := NewOpenAIEmbedder(context.Background(), EmbeddingConfig{
		Model: "test-embedding", BaseURL: server.URL, APIKey: "k", Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewOpenAIEmbedder() error = %v", err)
	}

	rows, err := embedder.EmbedTexts(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("EmbedTexts() error = %v", err)
	}
	if len(rows) != 2 || len(rows[0]) != 3 {
		t.Fatalf("EmbedTexts() shape = %dx?, want 2x3", len(rows))
	}
	if rows[1][0] != 0.4 {
		t.Fatalf("rows[1][0] = %v, want 0.4", rows[1][0])
	}
}
