package graph

import (
	"context"
	"fmt"
	"io"
	"strings"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"

	"github.com/gtkit/go-rag-agent/internal/llm"
	"github.com/gtkit/go-rag-agent/internal/memory"
)

const defaultMaxIterations = 12

// ReactRunner 使用 Eino ReAct 执行 graph 层问答。
type ReactRunner struct {
	model llm.ChatModel
	agent *react.Agent
}

// NewReactRunner 创建带检索工具的 ReAct runner。
func NewReactRunner(ctx context.Context, model llm.ChatModel, maxIterations int, toolset ...einotool.BaseTool) (*ReactRunner, error) {
	if model == nil {
		return nil, fmt.Errorf("chat model is required")
	}
	if maxIterations <= 0 {
		maxIterations = defaultMaxIterations
	}

	var agentRunner *react.Agent
	if len(toolset) > 0 {
		// 这里假设当前 OpenAI-compatible 适配器会在流式输出早期暴露 tool call。
		agent, err := react.NewAgent(ctx, &react.AgentConfig{
			ToolCallingModel: model,
			ToolsConfig: compose.ToolsNodeConfig{
				Tools: toolset,
			},
			MaxStep: maxIterations,
		})
		if err != nil {
			return nil, fmt.Errorf("create react agent: %w", err)
		}
		agentRunner = agent
	}

	return &ReactRunner{
		model: model,
		agent: agentRunner,
	}, nil
}

// Ask 返回最终答案文本。
func (r *ReactRunner) Ask(ctx context.Context, req Request) (string, error) {
	msgs := buildPromptMessages(req.History, req.EvidenceText, req.Query)
	if strings.TrimSpace(req.EvidenceText) != "" {
		msg, err := r.model.Generate(ctx, msgs)
		if err != nil {
			return "", fmt.Errorf("generate model answer: %w", err)
		}
		return msg.Content, nil
	}
	if r.agent == nil {
		return "", fmt.Errorf("react agent is required when evidence text is empty")
	}

	msg, err := r.agent.Generate(ctx, msgs)
	if err != nil {
		return "", fmt.Errorf("generate react answer: %w", err)
	}
	return msg.Content, nil
}

// AskStream 发出答案分块和完成事件。
func (r *ReactRunner) AskStream(ctx context.Context, req Request, emit StreamEmitter) error {
	msgs := buildPromptMessages(req.History, req.EvidenceText, req.Query)
	if strings.TrimSpace(req.EvidenceText) != "" {
		stream, err := r.model.Stream(ctx, msgs)
		if err != nil {
			return fmt.Errorf("start model stream: %w", err)
		}
		defer stream.Close()
		return emitStream(stream, emit)
	}
	if r.agent == nil {
		return fmt.Errorf("react agent is required when evidence text is empty")
	}

	stream, err := r.agent.Stream(ctx, msgs)
	if err != nil {
		return fmt.Errorf("start react stream: %w", err)
	}
	defer stream.Close()
	return emitStream(stream, emit)
}

func emitStream(stream *schema.StreamReader[*schema.Message], emit StreamEmitter) error {
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

func buildPromptMessages(history []memory.Turn, evidenceText, query string) []*schema.Message {
	msgs := make([]*schema.Message, 0, len(history)*2+3)
	msgs = append(msgs, schema.SystemMessage("Answer with retrieved evidence first. If evidence is insufficient, use available tools. Prefer local retrieval before web search. If evidence is still insufficient, say so explicitly."))

	for _, turn := range history {
		if strings.TrimSpace(turn.User) != "" {
			msgs = append(msgs, schema.UserMessage(turn.User))
		}
		if strings.TrimSpace(turn.Assistant) != "" {
			msgs = append(msgs, &schema.Message{
				Role:    schema.Assistant,
				Content: turn.Assistant,
			})
		}
	}
	if strings.TrimSpace(evidenceText) != "" {
		msgs = append(msgs, schema.UserMessage("Relevant context:\n"+evidenceText))
	}
	msgs = append(msgs, schema.UserMessage(query))
	return msgs
}
