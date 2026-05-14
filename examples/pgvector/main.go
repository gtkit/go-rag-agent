package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	ragagent "github.com/gtkit/go-rag-agent"
)

type pgVectorSession interface {
	Ask(ctx context.Context, query string) (ragagent.Answer, error)
}

type pgVectorAgent interface {
	AddKnowledge(ctx context.Context, src ragagent.KnowledgeSource) error
	GetSession(id string) pgVectorSession
	Close() error
}

var newPGVectorStore = func(cfg ragagent.PGVectorStoreConfig) (ragagent.VectorStore, error) {
	return ragagent.NewPGVectorStore(cfg)
}

var newPGVectorAgent = func(cfg ragagent.Config) (pgVectorAgent, error) {
	agent, err := ragagent.New(cfg)
	if err != nil {
		return nil, err
	}
	return realPGVectorAgent{agent: agent}, nil
}

type realPGVectorAgent struct {
	agent *ragagent.Agent
}

func (a realPGVectorAgent) AddKnowledge(ctx context.Context, src ragagent.KnowledgeSource) error {
	return a.agent.AddKnowledge(ctx, src)
}

func (a realPGVectorAgent) GetSession(id string) pgVectorSession {
	return a.agent.GetSession(id)
}

func (a realPGVectorAgent) Close() error {
	return a.agent.Close()
}

func main() {
	if err := run(context.Background(), os.Stdout, os.Getenv); err != nil {
		log.Fatalf("run pgvector example: %v", err)
	}
}

func run(ctx context.Context, stdout io.Writer, getenv func(string) string) error {
	dsn := strings.TrimSpace(getenv("RAGAGENT_PGVECTOR_DSN"))
	if dsn == "" {
		if _, err := fmt.Fprintln(stdout, "set RAGAGENT_PGVECTOR_DSN to run the pgvector deployment example"); err != nil {
			return fmt.Errorf("write output: %w", err)
		}
		return nil
	}

	cfg, err := pgVectorConfigFromEnv(getenv)
	if err != nil {
		return err
	}

	store, err := newPGVectorStore(ragagent.PGVectorStoreConfig{
		ConnString:      dsn,
		TableName:       "knowledge_chunks",
		Dimensions:      1536,
		IndexStrategy:   "hnsw",
		UpsertBatchSize: 200,
	})
	if err != nil {
		return fmt.Errorf("new pgvector store: %w", err)
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil {
			log.Printf("close pgvector store: %v", closeErr)
		}
	}()

	cfg.Storage.VectorStore = store
	agent, err := newPGVectorAgent(cfg)
	if err != nil {
		return fmt.Errorf("new pgvector-backed agent: %w", err)
	}
	defer func() {
		if closeErr := agent.Close(); closeErr != nil {
			log.Printf("close agent: %v", closeErr)
		}
	}()

	if err := agent.AddKnowledge(ctx, ragagent.DirSource("knowledge")); err != nil {
		return fmt.Errorf("add knowledge: %w", err)
	}

	answer, err := agent.GetSession("pgvector-demo").Ask(ctx, "请总结当前知识库中的回滚流程")
	if err != nil {
		return fmt.Errorf("ask: %w", err)
	}

	if _, err := fmt.Fprintln(stdout, "Scenario: pgvector-backed deployment"); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	if _, err := fmt.Fprintln(stdout, answer.Text); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	if _, err := fmt.Fprintf(stdout, "Citations: %d\n", len(answer.Citations)); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	return nil
}

func pgVectorConfigFromEnv(getenv func(string) string) (ragagent.Config, error) {
	cfg := ragagent.Config{
		ChatModel:        strings.TrimSpace(getenv("RAGAGENT_CHAT_MODEL")),
		ChatBaseURL:      strings.TrimSpace(getenv("RAGAGENT_CHAT_BASE_URL")),
		ChatAPIKey:       strings.TrimSpace(getenv("RAGAGENT_CHAT_API_KEY")),
		EmbeddingModel:   strings.TrimSpace(getenv("RAGAGENT_EMBEDDING_MODEL")),
		EmbeddingBaseURL: strings.TrimSpace(getenv("RAGAGENT_EMBEDDING_BASE_URL")),
		EmbeddingAPIKey:  strings.TrimSpace(getenv("RAGAGENT_EMBEDDING_API_KEY")),
		RequestTimeout:   20 * time.Second,
		ProviderGovernance: ragagent.ProviderGovernanceConfig{
			RetryMaxAttempts: 2,
			RetryBaseDelay:   200 * time.Millisecond,
			RetryMaxDelay:    2 * time.Second,
		},
	}
	if cfg.EmbeddingBaseURL == "" {
		cfg.EmbeddingBaseURL = cfg.ChatBaseURL
	}
	if cfg.EmbeddingAPIKey == "" {
		cfg.EmbeddingAPIKey = cfg.ChatAPIKey
	}
	if cfg.ChatModel == "" || cfg.ChatBaseURL == "" || cfg.ChatAPIKey == "" || cfg.EmbeddingModel == "" {
		return ragagent.Config{}, errors.New("set RAGAGENT_CHAT_MODEL, RAGAGENT_CHAT_BASE_URL, RAGAGENT_CHAT_API_KEY and RAGAGENT_EMBEDDING_MODEL")
	}
	return cfg, nil
}
