package main

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	ragagent "github.com/gtkit/go-rag-agent"
)

func TestNewSupportServiceFromEnv(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		newAgent func(ragagent.Config) (supportAgent, error)
		wantErr  bool
	}{
		{
			name: "rejects missing required environment",
			env: map[string]string{
				"RAGAGENT_CHAT_API_KEY": "chat-key",
			},
			newAgent: func(ragagent.Config) (supportAgent, error) {
				t.Fatal("new agent should not be called")
				return nil, nil
			},
			wantErr: true,
		},
		{
			name: "creates service with injected agent",
			env: map[string]string{
				"RAGAGENT_CHAT_MODEL":      "chat-model",
				"RAGAGENT_CHAT_BASE_URL":   "https://api.example.test/v1",
				"RAGAGENT_CHAT_API_KEY":    "chat-key",
				"RAGAGENT_EMBEDDING_MODEL": "embedding-model",
			},
			newAgent: func(cfg ragagent.Config) (supportAgent, error) {
				if cfg.AccessBoundary.Namespace != "support" {
					t.Fatalf("namespace = %q, want support", cfg.AccessBoundary.Namespace)
				}
				return &stubSupportAgent{}, nil
			},
		},
		{
			name: "returns agent construction error",
			env: map[string]string{
				"RAGAGENT_CHAT_MODEL":      "chat-model",
				"RAGAGENT_CHAT_BASE_URL":   "https://api.example.test/v1",
				"RAGAGENT_CHAT_API_KEY":    "chat-key",
				"RAGAGENT_EMBEDDING_MODEL": "embedding-model",
			},
			newAgent: func(ragagent.Config) (supportAgent, error) {
				return nil, errors.New("agent failed")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldNewAgent := newSupportAgent
			t.Cleanup(func() {
				newSupportAgent = oldNewAgent
			})
			newSupportAgent = tt.newAgent

			_, err := NewSupportServiceFromEnv(envGetter(tt.env))
			if (err != nil) != tt.wantErr {
				t.Fatalf("NewSupportServiceFromEnv() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSupportServiceTenantBoundaries(t *testing.T) {
	tests := []struct {
		name       string
		tenant     string
		call       func(*SupportService, context.Context, string) error
		wantErr    bool
		wantPrefix string
	}{
		{
			name:   "uses tenant scoped knowledge path",
			tenant: "acme",
			call: func(service *SupportService, ctx context.Context, tenant string) error {
				_, err := service.AnswerTenant(ctx, tenant, "session-1", "question")
				return err
			},
			wantPrefix: filepath.Join("knowledge", "acme"),
		},
		{
			name:   "adds tenant scoped knowledge",
			tenant: "acme",
			call: func(service *SupportService, ctx context.Context, tenant string) error {
				return service.AddTenantKnowledge(ctx, tenant)
			},
			wantPrefix: filepath.Join("knowledge", "acme"),
		},
		{
			name:   "rejects path traversal",
			tenant: "../secret",
			call: func(service *SupportService, ctx context.Context, tenant string) error {
				_, err := service.AnswerTenant(ctx, tenant, "session-1", "question")
				return err
			},
			wantErr: true,
		},
		{
			name:   "rejects empty tenant",
			tenant: " ",
			call: func(service *SupportService, ctx context.Context, tenant string) error {
				_, err := service.AnswerTenant(ctx, tenant, "session-1", "question")
				return err
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := &stubSupportAgent{session: &stubSupportSession{answer: ragagent.Answer{Text: "ok"}}}
			service := &SupportService{agent: agent}
			err := tt.call(service, context.Background(), tt.tenant)
			if (err != nil) != tt.wantErr {
				t.Fatalf("tenant call error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if len(agent.session.gotOptions.Filter.SourcePrefixes) > 0 && agent.session.gotOptions.Filter.SourcePrefixes[0] != tt.wantPrefix {
				t.Fatalf("source prefix = %q, want %q", agent.session.gotOptions.Filter.SourcePrefixes[0], tt.wantPrefix)
			}
		})
	}
}

func TestRun(t *testing.T) {
	tests := []struct {
		name       string
		env        map[string]string
		newAgent   func(ragagent.Config) (supportAgent, error)
		wantErr    bool
		wantOutput string
	}{
		{
			name: "runs embedded support service",
			env: map[string]string{
				"RAGAGENT_CHAT_MODEL":      "chat-model",
				"RAGAGENT_CHAT_BASE_URL":   "https://api.example.test/v1",
				"RAGAGENT_CHAT_API_KEY":    "chat-key",
				"RAGAGENT_EMBEDDING_MODEL": "embedding-model",
			},
			newAgent: func(ragagent.Config) (supportAgent, error) {
				return &stubSupportAgent{
					session: &stubSupportSession{answer: ragagent.Answer{Text: "tenant answer"}},
				}, nil
			},
			wantOutput: "tenant answer",
		},
		{
			name: "returns add knowledge error",
			env: map[string]string{
				"RAGAGENT_CHAT_MODEL":      "chat-model",
				"RAGAGENT_CHAT_BASE_URL":   "https://api.example.test/v1",
				"RAGAGENT_CHAT_API_KEY":    "chat-key",
				"RAGAGENT_EMBEDDING_MODEL": "embedding-model",
			},
			newAgent: func(ragagent.Config) (supportAgent, error) {
				return &stubSupportAgent{addErr: errors.New("add failed")}, nil
			},
			wantErr: true,
		},
		{
			name: "returns answer error",
			env: map[string]string{
				"RAGAGENT_CHAT_MODEL":      "chat-model",
				"RAGAGENT_CHAT_BASE_URL":   "https://api.example.test/v1",
				"RAGAGENT_CHAT_API_KEY":    "chat-key",
				"RAGAGENT_EMBEDDING_MODEL": "embedding-model",
			},
			newAgent: func(ragagent.Config) (supportAgent, error) {
				return &stubSupportAgent{
					session: &stubSupportSession{err: errors.New("answer failed")},
				}, nil
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldNewAgent := newSupportAgent
			t.Cleanup(func() {
				newSupportAgent = oldNewAgent
			})
			newSupportAgent = tt.newAgent

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

func TestRealSupportAgent(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "delegates to embedded rag agent"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := t.TempDir()
			agent, err := newSupportAgent(ragagent.Config{
				ChatModel: "demo",
				Runtime: ragagent.RuntimeComponents{
					ChatModel: stubSupportChatModel{},
					Embedder:  stubSupportEmbedder{},
				},
				Storage: ragagent.StorageComponents{
					VectorStore: &stubSupportVectorStore{},
				},
				DataDir: tmp,
			})
			if err != nil {
				t.Fatalf("newSupportAgent() error = %v", err)
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

type stubSupportAgent struct {
	addErr  error
	session *stubSupportSession
	closed  bool
}

func (a *stubSupportAgent) AddKnowledge(context.Context, ragagent.KnowledgeSource) error {
	return a.addErr
}

func (a *stubSupportAgent) GetSession(string) supportSession {
	if a.session == nil {
		a.session = &stubSupportSession{}
	}
	return a.session
}

func (a *stubSupportAgent) Close() error {
	a.closed = true
	return nil
}

type stubSupportSession struct {
	gotOptions ragagent.QueryOptions
	answer     ragagent.Answer
	err        error
}

func (s *stubSupportSession) AskWithOptions(_ context.Context, _ string, opts ragagent.QueryOptions) (ragagent.Answer, error) {
	s.gotOptions = opts
	return s.answer, s.err
}

func envGetter(values map[string]string) func(string) string {
	return func(key string) string {
		return values[key]
	}
}

type stubSupportChatModel struct{}

func (stubSupportChatModel) Generate(context.Context, []ragagent.Message) (ragagent.Message, error) {
	return ragagent.Message{Role: ragagent.RoleAssistant, Content: "ok"}, nil
}

func (stubSupportChatModel) Stream(_ context.Context, _ []ragagent.Message, emit func(string) error) error {
	return emit("ok")
}

type stubSupportEmbedder struct{}

func (stubSupportEmbedder) EmbedTexts(_ context.Context, texts []string) ([][]float32, error) {
	rows := make([][]float32, 0, len(texts))
	for range texts {
		rows = append(rows, []float32{1})
	}
	return rows, nil
}

type stubSupportVectorStore struct{}

func (stubSupportVectorStore) Upsert(context.Context, []ragagent.ChunkRecord) error {
	return nil
}

func (stubSupportVectorStore) SearchWithFilter(context.Context, []float32, int, float32, ragagent.SearchFilter) ([]ragagent.SearchHit, error) {
	return nil, nil
}

func (stubSupportVectorStore) DeleteBySourcePaths(context.Context, []string) error {
	return nil
}

func (stubSupportVectorStore) Search(context.Context, []float32, int, float32) ([]ragagent.SearchHit, error) {
	return nil, nil
}

func (stubSupportVectorStore) Close() error {
	return nil
}
