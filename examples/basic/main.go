package main

import (
	"context"
	"fmt"
	"log"
	"time"

	ragagent "my-gtkit-package/go-rag-agent"
)

func main() {
	ctx := context.Background()

	cfg := ragagent.Config{
		ChatModel:        "gpt-4o-mini",
		ChatBaseURL:      "https://api.openai.example/v1",
		ChatAPIKey:       "replace-with-your-chat-key",
		EmbeddingModel:   "text-embedding-3-small",
		EmbeddingBaseURL: "https://api.openai.example/v1",
		EmbeddingAPIKey:  "replace-with-your-embedding-key",
		DataDir:          ".rag-data",
		RequestTimeout:   20 * time.Second,
		ChunkSize:        800,
		ChunkOverlap:     120,
		TopK:             4,
	}

	agent, err := ragagent.New(cfg)
	if err != nil {
		log.Fatalf("new agent: %v", err)
	}
	defer func() {
		if closeErr := agent.Close(); closeErr != nil {
			log.Printf("close agent: %v", closeErr)
		}
	}()

	if err := agent.AddKnowledge(ctx, ragagent.DirSource("./knowledge")); err != nil {
		log.Fatalf("add knowledge: %v", err)
	}

	session := agent.GetSession("team-a")
	answer, err := session.Ask(ctx, "请基于知识库总结接入步骤")
	if err != nil {
		log.Fatalf("ask: %v", err)
	}

	fmt.Println("Answer:")
	fmt.Println(answer.Text)
	fmt.Printf("Citations: %d\n", len(answer.Citations))
}
