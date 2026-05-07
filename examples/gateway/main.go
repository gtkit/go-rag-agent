package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	ragagent "github.com/gtkit/go-rag-agent"
)

type gatewaySession interface {
	Ask(ctx context.Context, query string) (ragagent.Answer, error)
}

type gatewayAgent interface {
	GetSession(id string) gatewaySession
}

type chatCompletionRequest struct {
	Model    string                `json:"model"`
	User     string                `json:"user"`
	Stream   bool                  `json:"stream"`
	Messages []chatCompletionInput `json:"messages"`
}

type chatCompletionInput struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionResponse struct {
	ID      string                 `json:"id"`
	Object  string                 `json:"object"`
	Created int64                  `json:"created"`
	Model   string                 `json:"model"`
	Choices []chatCompletionChoice `json:"choices"`
}

type chatCompletionChoice struct {
	Index        int                 `json:"index"`
	Message      chatCompletionInput `json:"message"`
	FinishReason string              `json:"finish_reason"`
}

// NewChatCompletionsHandler returns a minimal OpenAI-compatible non-streaming chat completions handler.
func NewChatCompletionsHandler(agent gatewayAgent) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req chatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.Stream {
			http.Error(w, "streaming is not implemented in this minimal example", http.StatusBadRequest)
			return
		}
		query := lastUserMessage(req.Messages)
		if query == "" {
			http.Error(w, "last user message is required", http.StatusBadRequest)
			return
		}
		sessionID := strings.TrimSpace(req.User)
		if sessionID == "" {
			sessionID = "default"
		}
		answer, err := agent.GetSession(sessionID).Ask(r.Context(), query)
		if err != nil {
			http.Error(w, fmt.Sprintf("ask rag agent: %v", err), http.StatusInternalServerError)
			return
		}
		model := strings.TrimSpace(req.Model)
		if model == "" {
			model = "ragagent"
		}
		writeJSON(w, http.StatusOK, chatCompletionResponse{
			ID:      "chatcmpl-ragagent-example",
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   model,
			Choices: []chatCompletionChoice{
				{
					Index: 0,
					Message: chatCompletionInput{
						Role:    "assistant",
						Content: answer.Text,
					},
					FinishReason: "stop",
				},
			},
		})
	})
}

func lastUserMessage(messages []chatCompletionInput) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != "user" {
			continue
		}
		if content := strings.TrimSpace(messages[i].Content); content != "" {
			return content
		}
	}
	return ""
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func main() {}
