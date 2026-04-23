package ragagent

import (
	"context"
	"errors"
	"testing"
	"time"
)

type stubRuntimeChatModel struct{}

func (stubRuntimeChatModel) Generate(context.Context, []Message) (Message, error) {
	return Message{Role: RoleAssistant, Content: "ok"}, nil
}

func (stubRuntimeChatModel) Stream(_ context.Context, _ []Message, emit func(string) error) error {
	if emit == nil {
		return nil
	}
	return emit("ok")
}

type stubRuntimeEmbedder struct{}

func (stubRuntimeEmbedder) EmbedTexts(_ context.Context, texts []string) ([][]float32, error) {
	rows := make([][]float32, 0, len(texts))
	for range texts {
		rows = append(rows, []float32{1})
	}
	return rows, nil
}

type stubStorageVectorStore struct{}

func (stubStorageVectorStore) Upsert(context.Context, []ChunkRecord) error { return nil }

func (stubStorageVectorStore) Search(context.Context, []float32, int, float32) ([]SearchHit, error) {
	return nil, nil
}

func (stubStorageVectorStore) SearchWithFilter(context.Context, []float32, int, float32, SearchFilter) ([]SearchHit, error) {
	return nil, nil
}

func (stubStorageVectorStore) DeleteBySourcePaths(context.Context, []string) error { return nil }

func (stubStorageVectorStore) Close() error { return nil }

type stubStorageDocumentLoader struct{}

func (stubStorageDocumentLoader) Load(context.Context, string, string, map[string]string, DocumentLoadOptions) (Document, error) {
	return Document{}, nil
}

type stubStorageReranker struct{}

func (stubStorageReranker) Rerank(context.Context, string, []SearchHit, RerankOptions) ([]SearchHit, error) {
	return nil, nil
}

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
			name: "chunk size above context budget rejected",
			mutate: func(cfg *Config) {
				cfg.ChunkSize = maxEvidenceChars + 1
			},
			wantErr: ErrInvalidConfig,
		},
		{
			name: "hybrid search is now valid",
			mutate: func(cfg *Config) {
				cfg.EnableHybridSearch = true
			},
			wantErr: nil,
		},
		{
			name: "rerank requires hybrid search",
			mutate: func(cfg *Config) {
				cfg.EnableRerank = true
			},
			wantErr: ErrInvalidConfig,
		},
		{
			name: "rerank with hybrid search is valid",
			mutate: func(cfg *Config) {
				cfg.EnableHybridSearch = true
				cfg.EnableRerank = true
			},
			wantErr: nil,
		},
		{
			name: "hybrid candidate multiplier must be positive",
			mutate: func(cfg *Config) {
				cfg.HybridCandidateMultiplier = -1
			},
			wantErr: ErrInvalidConfig,
		},
		{
			name: "hybrid rrf k must be positive",
			mutate: func(cfg *Config) {
				cfg.HybridRRFK = -1
			},
			wantErr: ErrInvalidConfig,
		},
		{
			name: "rerank shortlist multiplier must be positive",
			mutate: func(cfg *Config) {
				cfg.RerankShortlistMultiplier = -1
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
		{
			name: "zero similarity threshold is valid",
			mutate: func(cfg *Config) {
				cfg.SimilarityThreshold = 0
			},
			wantErr: nil,
		},
		{
			name: "empty data dir is valid for in-memory mode",
			mutate: func(cfg *Config) {
				cfg.DataDir = ""
			},
			wantErr: nil,
		},
		{
			name: "pdf ocr bridge requires command when configured",
			mutate: func(cfg *Config) {
				cfg.PDFOCRBridge = PDFOCRBridgeConfig{
					Args: []string{"{input}", "{output}"},
				}
			},
			wantErr: ErrInvalidConfig,
		},
		{
			name: "pdf ocr bridge requires input and output placeholders",
			mutate: func(cfg *Config) {
				cfg.PDFOCRBridge = PDFOCRBridgeConfig{
					Command: "ocr-tool",
					Args:    []string{"{input}"},
				}
			},
			wantErr: ErrInvalidConfig,
		},
		{
			name: "pdf ocr bridge rejects negative min direct text runes",
			mutate: func(cfg *Config) {
				cfg.PDFOCRBridge = PDFOCRBridgeConfig{
					Command:            "ocr-tool",
					Args:               []string{"{input}", "{output}"},
					MinDirectTextRunes: -1,
				}
			},
			wantErr: ErrInvalidConfig,
		},
		{
			name: "pdf ocr bridge accepts complete configuration",
			mutate: func(cfg *Config) {
				cfg.PDFOCRBridge = PDFOCRBridgeConfig{
					Command:            "ocr-tool",
					Args:               []string{"{input}", "{output}"},
					MinDirectTextRunes: 24,
				}
			},
			wantErr: nil,
		},
		{
			name: "image text bridge requires command when configured",
			mutate: func(cfg *Config) {
				cfg.ImageTextBridge = ImageTextBridgeConfig{
					Args: []string{"{input}", "{output}"},
				}
			},
			wantErr: ErrInvalidConfig,
		},
		{
			name: "image text bridge requires input and output placeholders",
			mutate: func(cfg *Config) {
				cfg.ImageTextBridge = ImageTextBridgeConfig{
					Command: "image-tool",
					Args:    []string{"{input}"},
				}
			},
			wantErr: ErrInvalidConfig,
		},
		{
			name: "image text bridge accepts complete configuration",
			mutate: func(cfg *Config) {
				cfg.ImageTextBridge = ImageTextBridgeConfig{
					Command: "image-tool",
					Args:    []string{"{input}", "{output}"},
				}
			},
			wantErr: nil,
		},
		{
			name: "web search requires api key when enabled",
			mutate: func(cfg *Config) {
				cfg.EnableWebSearch = true
			},
			wantErr: ErrInvalidConfig,
		},
		{
			name: "web search accepts complete configuration",
			mutate: func(cfg *Config) {
				cfg.EnableWebSearch = true
				cfg.WebSearch = WebSearchConfig{
					APIKey:      "tvly-test",
					MaxResults:  5,
					SearchDepth: "basic",
					Topic:       "general",
				}
			},
			wantErr: nil,
		},
		{
			name: "web search rejects invalid depth",
			mutate: func(cfg *Config) {
				cfg.EnableWebSearch = true
				cfg.WebSearch = WebSearchConfig{
					APIKey:      "tvly-test",
					SearchDepth: "bad",
				}
			},
			wantErr: ErrInvalidConfig,
		},
		{
			name: "max memory tokens must be positive",
			mutate: func(cfg *Config) {
				cfg.MaxMemoryTokens = -1
			},
			wantErr: ErrInvalidConfig,
		},
		{
			name: "long-term memory topk must be non-negative",
			mutate: func(cfg *Config) {
				cfg.LongTermMemoryTopK = -1
			},
			wantErr: ErrInvalidConfig,
		},
		{
			name: "long-term memory threshold must be non-negative",
			mutate: func(cfg *Config) {
				cfg.LongTermMemoryThreshold = -1
			},
			wantErr: ErrInvalidConfig,
		},
		{
			name: "long-term memory ttl must be non-negative",
			mutate: func(cfg *Config) {
				cfg.LongTermMemoryTTL = -time.Second
			},
			wantErr: ErrInvalidConfig,
		},
		{
			name: "long-term memory max stored runes must be positive",
			mutate: func(cfg *Config) {
				cfg.LongTermMemoryMaxStoredRunes = 0
			},
			wantErr: nil,
		},
		{
			name: "long-term memory max stored runes rejects negative",
			mutate: func(cfg *Config) {
				cfg.LongTermMemoryMaxStoredRunes = -1
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

func TestConfigValidateRuntimeComponents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     Config
		wantErr error
	}{
		{
			name: "fully injected runtime skips default provider validation",
			cfg: Config{
				Runtime: RuntimeComponents{
					ChatModel: stubRuntimeChatModel{},
					Embedder:  stubRuntimeEmbedder{},
				},
			},
			wantErr: nil,
		},
		{
			name: "mixed runtime keeps default chat validation path",
			cfg: Config{
				ChatModel:   "gpt-4.1-mini",
				ChatBaseURL: "https://api.example.com/v1",
				ChatAPIKey:  "test-key",
				Runtime: RuntimeComponents{
					Embedder: stubRuntimeEmbedder{},
				},
			},
			wantErr: nil,
		},
		{
			name: "missing default chat config still fails when chat runtime not injected",
			cfg: Config{
				Runtime: RuntimeComponents{
					Embedder: stubRuntimeEmbedder{},
				},
			},
			wantErr: ErrInvalidConfig,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.cfg.Validate()
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("Validate() unexpected error = %v", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Validate() error = %v, want errors.Is(..., %v)", err, tc.wantErr)
			}
		})
	}
}

func TestConfigValidateStorageComponents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     Config
		wantErr error
	}{
		{
			name: "all storage components injected",
			cfg: Config{
				ChatModel:      "gpt-4.1-mini",
				ChatBaseURL:    "https://api.example.com/v1",
				ChatAPIKey:     "test-key",
				EmbeddingModel: "text-embedding-3-small",
				Storage: StorageComponents{
					VectorStore:    stubStorageVectorStore{},
					DocumentLoader: stubStorageDocumentLoader{},
					Reranker:       stubStorageReranker{},
				},
			},
			wantErr: nil,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.cfg.Validate()
			if tc.wantErr == nil && err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
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
				if got.HybridCandidateMultiplier != 4 {
					t.Fatalf("withDefaults() HybridCandidateMultiplier = %d, want 4", got.HybridCandidateMultiplier)
				}
				if got.HybridRRFK != 60 {
					t.Fatalf("withDefaults() HybridRRFK = %v, want 60", got.HybridRRFK)
				}
				if got.RerankShortlistMultiplier != 2 {
					t.Fatalf("withDefaults() RerankShortlistMultiplier = %d, want 2", got.RerankShortlistMultiplier)
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
