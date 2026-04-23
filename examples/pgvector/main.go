package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	ragagent "github.com/gtkit/go-rag-agent"
)

func main() {
	dsn := os.Getenv("RAGAGENT_PGVECTOR_DSN")
	if dsn == "" {
		log.Print("set RAGAGENT_PGVECTOR_DSN to run the pgvector deployment example")
		return
	}

	ctx := context.Background()

	store, err := ragagent.NewPGVectorStore(ragagent.PGVectorStoreConfig{
		ConnString:      dsn,
		TableName:       "knowledge_chunks",
		Dimensions:      1536,
		IndexStrategy:   "hnsw",
		UpsertBatchSize: 200,
	})
	if err != nil {
		log.Fatalf("new pgvector store: %v", err)
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil {
			log.Printf("close pgvector store: %v", closeErr)
		}
	}()

	agent, err := ragagent.New(ragagent.Config{
		ChatModel:        "gpt-4o-mini",
		ChatBaseURL:      "https://api.openai.example/v1",
		ChatAPIKey:       "replace-with-your-chat-key",
		EmbeddingModel:   "text-embedding-3-small",
		EmbeddingBaseURL: "https://api.openai.example/v1",
		EmbeddingAPIKey:  "replace-with-your-embedding-key",
		RequestTimeout:   20 * time.Second,
		Storage: ragagent.StorageComponents{
			VectorStore: store,
		},
		ProviderGovernance: ragagent.ProviderGovernanceConfig{
			RetryMaxAttempts: 2,
			RetryBaseDelay:   200 * time.Millisecond,
			RetryMaxDelay:    2 * time.Second,
		},
	})
	if err != nil {
		log.Fatalf("new pgvector-backed agent: %v", err)
	}
	defer func() {
		if closeErr := agent.Close(); closeErr != nil {
			log.Printf("close agent: %v", closeErr)
		}
	}()

	if err := agent.AddKnowledge(ctx, ragagent.DirSource("knowledge")); err != nil {
		log.Fatalf("add knowledge: %v", err)
	}

	answer, err := agent.GetSession("pgvector-demo").Ask(ctx, "请总结当前知识库中的回滚流程")
	if err != nil {
		log.Fatalf("ask: %v", err)
	}

	fmt.Println("Scenario: pgvector-backed deployment")
	fmt.Println(answer.Text)
	fmt.Printf("Citations: %d\n", len(answer.Citations))
}
