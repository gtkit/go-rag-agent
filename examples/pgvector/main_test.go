package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	ragagent "github.com/gtkit/go-rag-agent"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name       string
		env        map[string]string
		store      func(ragagent.PGVectorStoreConfig) (ragagent.VectorStore, error)
		agent      func(ragagent.Config) (pgVectorAgent, error)
		wantErr    bool
		wantOutput string
	}{
		{
			name:       "skips when dsn is missing",
			env:        map[string]string{},
			wantOutput: "set RAGAGENT_PGVECTOR_DSN",
		},
		{
			name: "rejects missing model environment when dsn exists",
			env: map[string]string{
				"RAGAGENT_PGVECTOR_DSN": "postgres://example",
			},
			wantErr: true,
		},
		{
			name: "runs deployment flow with injected dependencies",
			env: map[string]string{
				"RAGAGENT_PGVECTOR_DSN":    "postgres://example",
				"RAGAGENT_CHAT_MODEL":      "chat-model",
				"RAGAGENT_CHAT_BASE_URL":   "https://api.example.test/v1",
				"RAGAGENT_CHAT_API_KEY":    "chat-key",
				"RAGAGENT_EMBEDDING_MODEL": "embedding-model",
			},
			store: func(cfg ragagent.PGVectorStoreConfig) (ragagent.VectorStore, error) {
				if cfg.ConnString != "postgres://example" || cfg.UpsertBatchSize != 200 {
					t.Fatalf("pgvector config = %+v", cfg)
				}
				return &stubVectorStore{}, nil
			},
			agent: func(cfg ragagent.Config) (pgVectorAgent, error) {
				if cfg.Storage.VectorStore == nil {
					t.Fatal("vector store is nil")
				}
				return &stubPGVectorAgent{
					session: &stubPGVectorSession{answer: ragagent.Answer{Text: "pg answer"}},
				}, nil
			},
			wantOutput: "pg answer",
		},
		{
			name: "returns store construction error",
			env: map[string]string{
				"RAGAGENT_PGVECTOR_DSN":    "postgres://example",
				"RAGAGENT_CHAT_MODEL":      "chat-model",
				"RAGAGENT_CHAT_BASE_URL":   "https://api.example.test/v1",
				"RAGAGENT_CHAT_API_KEY":    "chat-key",
				"RAGAGENT_EMBEDDING_MODEL": "embedding-model",
			},
			store: func(ragagent.PGVectorStoreConfig) (ragagent.VectorStore, error) {
				return nil, errors.New("store failed")
			},
			wantErr: true,
		},
		{
			name: "returns agent construction error",
			env: map[string]string{
				"RAGAGENT_PGVECTOR_DSN":    "postgres://example",
				"RAGAGENT_CHAT_MODEL":      "chat-model",
				"RAGAGENT_CHAT_BASE_URL":   "https://api.example.test/v1",
				"RAGAGENT_CHAT_API_KEY":    "chat-key",
				"RAGAGENT_EMBEDDING_MODEL": "embedding-model",
			},
			store: func(ragagent.PGVectorStoreConfig) (ragagent.VectorStore, error) {
				return &stubVectorStore{}, nil
			},
			agent: func(ragagent.Config) (pgVectorAgent, error) {
				return nil, errors.New("agent failed")
			},
			wantErr: true,
		},
		{
			name: "returns add knowledge error",
			env: map[string]string{
				"RAGAGENT_PGVECTOR_DSN":    "postgres://example",
				"RAGAGENT_CHAT_MODEL":      "chat-model",
				"RAGAGENT_CHAT_BASE_URL":   "https://api.example.test/v1",
				"RAGAGENT_CHAT_API_KEY":    "chat-key",
				"RAGAGENT_EMBEDDING_MODEL": "embedding-model",
			},
			store: func(ragagent.PGVectorStoreConfig) (ragagent.VectorStore, error) {
				return &stubVectorStore{}, nil
			},
			agent: func(ragagent.Config) (pgVectorAgent, error) {
				return &stubPGVectorAgent{addErr: errors.New("add failed")}, nil
			},
			wantErr: true,
		},
		{
			name: "returns ask error",
			env: map[string]string{
				"RAGAGENT_PGVECTOR_DSN":    "postgres://example",
				"RAGAGENT_CHAT_MODEL":      "chat-model",
				"RAGAGENT_CHAT_BASE_URL":   "https://api.example.test/v1",
				"RAGAGENT_CHAT_API_KEY":    "chat-key",
				"RAGAGENT_EMBEDDING_MODEL": "embedding-model",
			},
			store: func(ragagent.PGVectorStoreConfig) (ragagent.VectorStore, error) {
				return &stubVectorStore{}, nil
			},
			agent: func(ragagent.Config) (pgVectorAgent, error) {
				return &stubPGVectorAgent{
					session: &stubPGVectorSession{err: errors.New("ask failed")},
				}, nil
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldStore := newPGVectorStore
			oldAgent := newPGVectorAgent
			t.Cleanup(func() {
				newPGVectorStore = oldStore
				newPGVectorAgent = oldAgent
			})
			if tt.store != nil {
				newPGVectorStore = tt.store
			}
			if tt.agent != nil {
				newPGVectorAgent = tt.agent
			}

			var out bytes.Buffer
			err := run(context.Background(), &out, envGetter(tt.env))
			if (err != nil) != tt.wantErr {
				t.Fatalf("run() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantOutput != "" && !strings.Contains(out.String(), tt.wantOutput) {
				t.Fatalf("output = %q, want containing %q", out.String(), tt.wantOutput)
			}
		})
	}
}

func TestRealPGVectorAgent(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "delegates to embedded rag agent"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := t.TempDir()
			agent, err := newPGVectorAgent(ragagent.Config{
				ChatModel: "demo",
				Runtime: ragagent.RuntimeComponents{
					ChatModel: stubPGVectorChatModel{},
					Embedder:  stubPGVectorEmbedder{},
				},
				Storage: ragagent.StorageComponents{
					VectorStore: &stubVectorStore{},
				},
				DataDir: tmp,
			})
			if err != nil {
				t.Fatalf("newPGVectorAgent() error = %v", err)
			}
			if err := agent.AddKnowledge(context.Background(), ragagent.DirSource(tmp)); err != nil {
				t.Fatalf("AddKnowledge() error = %v", err)
			}
			if agent.GetSession("demo") == nil {
				t.Fatal("GetSession() returned nil")
			}
			if err := agent.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}
		})
	}
}

type stubPGVectorAgent struct {
	addErr  error
	session pgVectorSession
}

func (a *stubPGVectorAgent) AddKnowledge(context.Context, ragagent.KnowledgeSource) error {
	return a.addErr
}

func (a *stubPGVectorAgent) GetSession(string) pgVectorSession {
	return a.session
}

func (a *stubPGVectorAgent) Close() error {
	return nil
}

type stubPGVectorSession struct {
	answer ragagent.Answer
	err    error
}

func (s *stubPGVectorSession) Ask(context.Context, string) (ragagent.Answer, error) {
	return s.answer, s.err
}

type stubVectorStore struct{}

func (stubVectorStore) Upsert(context.Context, []ragagent.ChunkRecord) error {
	return nil
}

func (stubVectorStore) SearchWithFilter(context.Context, []float32, int, float32, ragagent.SearchFilter) ([]ragagent.SearchHit, error) {
	return nil, nil
}

func (stubVectorStore) DeleteBySourcePaths(context.Context, []string) error {
	return nil
}

func (stubVectorStore) Search(context.Context, []float32, int, float32) ([]ragagent.SearchHit, error) {
	return nil, nil
}

func (stubVectorStore) Close() error {
	return nil
}

func envGetter(values map[string]string) func(string) string {
	return func(key string) string {
		return values[key]
	}
}

type stubPGVectorChatModel struct{}

func (stubPGVectorChatModel) Generate(context.Context, []ragagent.Message) (ragagent.Message, error) {
	return ragagent.Message{Role: ragagent.RoleAssistant, Content: "ok"}, nil
}

func (stubPGVectorChatModel) Stream(_ context.Context, _ []ragagent.Message, emit func(string) error) error {
	return emit("ok")
}

type stubPGVectorEmbedder struct{}

func (stubPGVectorEmbedder) EmbedTexts(_ context.Context, texts []string) ([][]float32, error) {
	rows := make([][]float32, 0, len(texts))
	for range texts {
		rows = append(rows, []float32{1})
	}
	return rows, nil
}
