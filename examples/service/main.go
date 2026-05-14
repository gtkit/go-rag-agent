package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	ragagent "github.com/gtkit/go-rag-agent"
)

type supportSession interface {
	AskWithOptions(ctx context.Context, query string, opts ragagent.QueryOptions) (ragagent.Answer, error)
}

type supportAgent interface {
	AddKnowledge(ctx context.Context, src ragagent.KnowledgeSource) error
	GetSession(id string) supportSession
	Close() error
}

type SupportService struct {
	agent supportAgent
}

var newSupportAgent = func(cfg ragagent.Config) (supportAgent, error) {
	agent, err := ragagent.New(cfg)
	if err != nil {
		return nil, err
	}
	return realSupportAgent{agent: agent}, nil
}

type realSupportAgent struct {
	agent *ragagent.Agent
}

func (a realSupportAgent) AddKnowledge(ctx context.Context, src ragagent.KnowledgeSource) error {
	return a.agent.AddKnowledge(ctx, src)
}

func (a realSupportAgent) GetSession(id string) supportSession {
	return a.agent.GetSession(id)
}

func (a realSupportAgent) Close() error {
	return a.agent.Close()
}

func NewSupportServiceFromEnv(getenv func(string) string) (*SupportService, error) {
	cfg, err := supportConfigFromEnv(getenv)
	if err != nil {
		return nil, err
	}
	agent, err := newSupportAgent(cfg)
	if err != nil {
		return nil, fmt.Errorf("new support service agent: %w", err)
	}
	return &SupportService{agent: agent}, nil
}

func supportConfigFromEnv(getenv func(string) string) (ragagent.Config, error) {
	cfg := ragagent.Config{
		ChatModel:        strings.TrimSpace(getenv("RAGAGENT_CHAT_MODEL")),
		ChatBaseURL:      strings.TrimSpace(getenv("RAGAGENT_CHAT_BASE_URL")),
		ChatAPIKey:       strings.TrimSpace(getenv("RAGAGENT_CHAT_API_KEY")),
		EmbeddingModel:   strings.TrimSpace(getenv("RAGAGENT_EMBEDDING_MODEL")),
		EmbeddingBaseURL: strings.TrimSpace(getenv("RAGAGENT_EMBEDDING_BASE_URL")),
		EmbeddingAPIKey:  strings.TrimSpace(getenv("RAGAGENT_EMBEDDING_API_KEY")),
		DataDir:          ".rag-data/support-service",
		RequestTimeout:   20 * time.Second,
		AccessBoundary: ragagent.AccessBoundaryConfig{
			Namespace: "support",
		},
		LongTermMemoryTTL: 24 * time.Hour,
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

func (s *SupportService) Close() error {
	return s.agent.Close()
}

func (s *SupportService) AddTenantKnowledge(ctx context.Context, tenant string) error {
	tenantPath, err := tenantKnowledgePath(tenant)
	if err != nil {
		return err
	}
	return s.agent.AddKnowledge(ctx, ragagent.DirSource(tenantPath))
}

func (s *SupportService) AnswerTenant(ctx context.Context, tenant string, sessionID string, query string) (ragagent.Answer, error) {
	tenantPath, err := tenantKnowledgePath(tenant)
	if err != nil {
		return ragagent.Answer{}, err
	}
	return s.agent.GetSession(sessionID).AskWithOptions(ctx, query, ragagent.QueryOptions{
		Filter: ragagent.RetrievalFilter{
			SourcePrefixes: []string{tenantPath},
		},
		MemoryScope: ragagent.MemoryScope{
			Tenant: tenant,
		},
	})
}

func tenantKnowledgePath(tenant string) (string, error) {
	tenant = strings.TrimSpace(tenant)
	if tenant == "" {
		return "", errors.New("tenant is required")
	}
	if tenant != filepath.Base(tenant) || strings.Contains(tenant, "..") {
		return "", fmt.Errorf("tenant %q must be a single path segment", tenant)
	}
	return filepath.Join("knowledge", tenant), nil
}

func main() {
	if err := run(context.Background(), os.Stdout, os.Getenv); err != nil {
		log.Fatalf("run support service example: %v", err)
	}
}

func run(ctx context.Context, stdout io.Writer, getenv func(string) string) error {
	service, err := NewSupportServiceFromEnv(getenv)
	if err != nil {
		return fmt.Errorf("new support service: %w", err)
	}
	defer func() {
		if closeErr := service.Close(); closeErr != nil {
			log.Printf("close support service: %v", closeErr)
		}
	}()

	if err := service.AddTenantKnowledge(ctx, "acme"); err != nil {
		return fmt.Errorf("add tenant knowledge: %w", err)
	}

	answer, err := service.AnswerTenant(ctx, "acme", "customer-42", "请总结当前租户的接入步骤")
	if err != nil {
		return fmt.Errorf("answer tenant query: %w", err)
	}

	if _, err := fmt.Fprintln(stdout, "Scenario: embed ragagent inside an existing service"); err != nil {
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
