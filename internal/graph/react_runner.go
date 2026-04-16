package graph

import (
	"context"
	"fmt"
	"io"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"

	"my-gtkit-package/go-rag-agent/internal/llm"
	"my-gtkit-package/go-rag-agent/internal/tools"
)

const defaultMaxIterations = 12

// ReactRunner executes graph ask calls with Eino ReAct.
type ReactRunner struct {
	agent *react.Agent
}

// NewReactRunner creates a ReAct runner with retrieval tool.
func NewReactRunner(ctx context.Context, model llm.ChatModel, retrievalTool *tools.RetrievalTool, maxIterations int) (*ReactRunner, error) {
	if model == nil {
		return nil, fmt.Errorf("chat model is required")
	}
	if retrievalTool == nil {
		return nil, fmt.Errorf("retrieval tool is required")
	}
	if maxIterations <= 0 {
		maxIterations = defaultMaxIterations
	}

	agent, err := react.NewAgent(ctx, &react.AgentConfig{
		ToolCallingModel: model,
		ToolsConfig: compose.ToolsNodeConfig{
			Tools: []einotool.BaseTool{retrievalTool},
		},
		MaxStep: maxIterations,
	})
	if err != nil {
		return nil, fmt.Errorf("create react agent: %w", err)
	}

	return &ReactRunner{agent: agent}, nil
}

// Ask returns the final text answer.
func (r *ReactRunner) Ask(ctx context.Context, req Request) (string, error) {
	msgs := buildPromptMessages(req.History, req.EvidenceText, req.Query)
	msg, err := r.agent.Generate(ctx, msgs)
	if err != nil {
		return "", fmt.Errorf("generate react answer: %w", err)
	}
	return msg.Content, nil
}

// AskStream emits answer chunks and done event.
func (r *ReactRunner) AskStream(ctx context.Context, req Request, emit StreamEmitter) error {
	msgs := buildPromptMessages(req.History, req.EvidenceText, req.Query)
	stream, err := r.agent.Stream(ctx, msgs)
	if err != nil {
		return fmt.Errorf("start react stream: %w", err)
	}
	defer stream.Close()

	step := 0
	for {
		chunk, recvErr := stream.Recv()
		if recvErr == io.EOF {
			return emit(Event{
				Type: EventDone,
				Step: step,
			})
		}
		if recvErr != nil {
			return fmt.Errorf("receive react stream chunk: %w", recvErr)
		}
		if chunk == nil || chunk.Content == "" {
			continue
		}

		step++
		if err := emit(Event{
			Type:    EventAnswerChunk,
			Content: chunk.Content,
			Step:    step,
		}); err != nil {
			return err
		}
	}
}

func buildPromptMessages(history []string, evidenceText, query string) []*schema.Message {
	msgs := make([]*schema.Message, 0, len(history)+3)
	msgs = append(msgs, schema.SystemMessage("Answer with retrieved evidence first. If evidence is insufficient, say so explicitly."))

	for _, h := range history {
		msgs = append(msgs, schema.UserMessage(h))
	}
	if evidenceText != "" {
		msgs = append(msgs, schema.SystemMessage("Retrieved evidence:\n"+evidenceText))
	}
	msgs = append(msgs, schema.UserMessage(query))
	return msgs
}
