package main

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"time"

	ragagent "github.com/gtkit/go-rag-agent"
)

type SupportService struct {
	agent *ragagent.Agent
}

func NewSupportService() (*SupportService, error) {
	agent, err := ragagent.New(ragagent.Config{
		ChatModel:        "gpt-4o-mini",
		ChatBaseURL:      "https://api.openai.example/v1",
		ChatAPIKey:       "replace-with-your-chat-key",
		EmbeddingModel:   "text-embedding-3-small",
		EmbeddingBaseURL: "https://api.openai.example/v1",
		EmbeddingAPIKey:  "replace-with-your-embedding-key",
		DataDir:          ".rag-data/support-service",
		RequestTimeout:   20 * time.Second,
		AccessBoundary: ragagent.AccessBoundaryConfig{
			Namespace: "support",
		},
		LongTermMemoryTTL: 24 * time.Hour,
	})
	if err != nil {
		return nil, fmt.Errorf("new support service agent: %w", err)
	}
	return &SupportService{agent: agent}, nil
}

func (s *SupportService) Close() error {
	return s.agent.Close()
}

func (s *SupportService) AddTenantKnowledge(ctx context.Context, tenant string) error {
	return s.agent.AddKnowledge(ctx, ragagent.DirSource(filepath.Join("knowledge", tenant)))
}

func (s *SupportService) AnswerTenant(ctx context.Context, tenant string, sessionID string, query string) (ragagent.Answer, error) {
	return s.agent.GetSession(sessionID).AskWithOptions(ctx, query, ragagent.QueryOptions{
		Filter: ragagent.RetrievalFilter{
			SourcePrefixes: []string{filepath.Join("knowledge", tenant)},
		},
		MemoryScope: ragagent.MemoryScope{
			Tenant: tenant,
		},
	})
}

func main() {
	ctx := context.Background()

	service, err := NewSupportService()
	if err != nil {
		log.Fatalf("new support service: %v", err)
	}
	defer func() {
		if closeErr := service.Close(); closeErr != nil {
			log.Printf("close support service: %v", closeErr)
		}
	}()

	if err := service.AddTenantKnowledge(ctx, "acme"); err != nil {
		log.Fatalf("add tenant knowledge: %v", err)
	}

	answer, err := service.AnswerTenant(ctx, "acme", "customer-42", "请总结当前租户的接入步骤")
	if err != nil {
		log.Fatalf("answer tenant query: %v", err)
	}

	fmt.Println("Scenario: embed ragagent inside an existing service")
	fmt.Println(answer.Text)
	fmt.Printf("Citations: %d\n", len(answer.Citations))
}
