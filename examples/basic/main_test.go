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
		newAgent   func(ragagent.Config) (basicAgent, error)
		wantErr    bool
		wantOutput string
	}{
		{
			name: "rejects missing required model environment",
			env: map[string]string{
				"RAGAGENT_CHAT_BASE_URL": "https://api.example.test/v1",
				"RAGAGENT_CHAT_API_KEY":  "chat-key",
			},
			newAgent: func(ragagent.Config) (basicAgent, error) {
				t.Fatal("new agent should not be called")
				return nil, nil
			},
			wantErr: true,
		},
		{
			name: "runs embedded setup with injected agent",
			env: map[string]string{
				"RAGAGENT_CHAT_MODEL":      "chat-model",
				"RAGAGENT_CHAT_BASE_URL":   "https://api.example.test/v1",
				"RAGAGENT_CHAT_API_KEY":    "chat-key",
				"RAGAGENT_EMBEDDING_MODEL": "embedding-model",
			},
			newAgent: func(cfg ragagent.Config) (basicAgent, error) {
				if cfg.ChatAPIKey != "chat-key" || cfg.EmbeddingAPIKey != "chat-key" {
					t.Fatalf("config API keys = %q/%q, want chat key fallback", cfg.ChatAPIKey, cfg.EmbeddingAPIKey)
				}
				return &stubBasicAgent{
					session: &stubBasicSession{answer: ragagent.Answer{Text: "basic answer"}},
				}, nil
			},
			wantOutput: "basic answer",
		},
		{
			name: "returns add knowledge error",
			env: map[string]string{
				"RAGAGENT_CHAT_MODEL":      "chat-model",
				"RAGAGENT_CHAT_BASE_URL":   "https://api.example.test/v1",
				"RAGAGENT_CHAT_API_KEY":    "chat-key",
				"RAGAGENT_EMBEDDING_MODEL": "embedding-model",
			},
			newAgent: func(ragagent.Config) (basicAgent, error) {
				return &stubBasicAgent{addErr: errors.New("index failed")}, nil
			},
			wantErr: true,
		},
		{
			name: "returns agent construction error",
			env: map[string]string{
				"RAGAGENT_CHAT_MODEL":      "chat-model",
				"RAGAGENT_CHAT_BASE_URL":   "https://api.example.test/v1",
				"RAGAGENT_CHAT_API_KEY":    "chat-key",
				"RAGAGENT_EMBEDDING_MODEL": "embedding-model",
			},
			newAgent: func(ragagent.Config) (basicAgent, error) {
				return nil, errors.New("new failed")
			},
			wantErr: true,
		},
		{
			name: "returns ask error",
			env: map[string]string{
				"RAGAGENT_CHAT_MODEL":      "chat-model",
				"RAGAGENT_CHAT_BASE_URL":   "https://api.example.test/v1",
				"RAGAGENT_CHAT_API_KEY":    "chat-key",
				"RAGAGENT_EMBEDDING_MODEL": "embedding-model",
			},
			newAgent: func(ragagent.Config) (basicAgent, error) {
				return &stubBasicAgent{
					session: &stubBasicSession{err: errors.New("ask failed")},
				}, nil
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldNewAgent := newBasicAgent
			t.Cleanup(func() {
				newBasicAgent = oldNewAgent
			})
			newBasicAgent = tt.newAgent

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

func TestRealBasicAgent(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "delegates to embedded rag agent"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := t.TempDir()
			agent, err := newBasicAgent(ragagent.Config{
				ChatModel: "demo",
				Runtime: ragagent.RuntimeComponents{
					ChatModel: stubBasicChatModel{},
					Embedder:  stubBasicEmbedder{},
				},
				Storage: ragagent.StorageComponents{
					VectorStore: &stubBasicVectorStore{},
				},
				DataDir: tmp,
			})
			if err != nil {
				t.Fatalf("newBasicAgent() error = %v", err)
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

type stubBasicAgent struct {
	addErr  error
	closed  bool
	session basicSession
}

func (a *stubBasicAgent) AddKnowledge(context.Context, ragagent.KnowledgeSource) error {
	return a.addErr
}

func (a *stubBasicAgent) GetSession(string) basicSession {
	return a.session
}

func (a *stubBasicAgent) Close() error {
	a.closed = true
	return nil
}

type stubBasicSession struct {
	answer ragagent.Answer
	err    error
}

func (s *stubBasicSession) Ask(context.Context, string) (ragagent.Answer, error) {
	return s.answer, s.err
}

func envGetter(values map[string]string) func(string) string {
	return func(key string) string {
		return values[key]
	}
}

type stubBasicChatModel struct{}

func (stubBasicChatModel) Generate(context.Context, []ragagent.Message) (ragagent.Message, error) {
	return ragagent.Message{Role: ragagent.RoleAssistant, Content: "ok"}, nil
}

func (stubBasicChatModel) Stream(_ context.Context, _ []ragagent.Message, emit func(string) error) error {
	return emit("ok")
}

type stubBasicEmbedder struct{}

func (stubBasicEmbedder) EmbedTexts(_ context.Context, texts []string) ([][]float32, error) {
	rows := make([][]float32, 0, len(texts))
	for range texts {
		rows = append(rows, []float32{1})
	}
	return rows, nil
}

type stubBasicVectorStore struct {
	closed bool
}

func (s *stubBasicVectorStore) Upsert(context.Context, []ragagent.ChunkRecord) error {
	return nil
}

func (s *stubBasicVectorStore) SearchWithFilter(context.Context, []float32, int, float32, ragagent.SearchFilter) ([]ragagent.SearchHit, error) {
	return nil, nil
}

func (s *stubBasicVectorStore) DeleteBySourcePaths(context.Context, []string) error {
	return nil
}

func (s *stubBasicVectorStore) Search(context.Context, []float32, int, float32) ([]ragagent.SearchHit, error) {
	return nil, nil
}

func (s *stubBasicVectorStore) Close() error {
	s.closed = true
	return nil
}
