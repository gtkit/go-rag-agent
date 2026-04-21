package graph

import (
	"context"
	"fmt"
	"strings"

	"github.com/gtkit/go-rag-agent/internal/llm"
	"github.com/gtkit/go-rag-agent/internal/memory"
	"github.com/gtkit/go-rag-agent/internal/tools"
)

// ChatRunner 使用项目内工具编排和 LangChainGo 聊天模型执行问答。
type ChatRunner struct {
	model         llm.ChatModel
	fallbackTools []tools.Tool
}

// NewChatRunner 创建问答 runner。
func NewChatRunner(model llm.ChatModel, toolset ...tools.Tool) (*ChatRunner, error) {
	if model == nil {
		return nil, fmt.Errorf("chat model is required")
	}

	fallbackTools := make([]tools.Tool, 0, len(toolset))
	for _, tool := range toolset {
		if tool == nil {
			continue
		}
		if tool.Name() == "retrieve_context" {
			continue
		}
		fallbackTools = append(fallbackTools, tool)
	}

	return &ChatRunner{
		model:         model,
		fallbackTools: fallbackTools,
	}, nil
}

// Ask 返回最终答案文本。
func (r *ChatRunner) Ask(ctx context.Context, req Request) (string, error) {
	msgs, err := r.messagesForRequest(ctx, req)
	if err != nil {
		return "", err
	}

	msg, err := r.model.Generate(ctx, msgs)
	if err != nil {
		return "", fmt.Errorf("generate model answer: %w", err)
	}
	return msg.Content, nil
}

// AskStream 发出答案分块和完成事件。
func (r *ChatRunner) AskStream(ctx context.Context, req Request, emit StreamEmitter) error {
	msgs, err := r.messagesForRequest(ctx, req)
	if err != nil {
		return err
	}

	step := 0
	if err := r.model.Stream(ctx, msgs, func(chunk string) error {
		if strings.TrimSpace(chunk) == "" {
			return nil
		}
		step++
		return emit(Event{
			Type:    EventAnswerChunk,
			Content: chunk,
			Step:    step,
		})
	}); err != nil {
		return fmt.Errorf("stream model answer: %w", err)
	}

	return emit(Event{
		Type: EventDone,
		Step: step,
	})
}

func (r *ChatRunner) messagesForRequest(ctx context.Context, req Request) ([]llm.Message, error) {
	msgs := buildPromptMessages(req.History, req.EvidenceText, req.Query, req.ResponseFormatInstruction)
	if strings.TrimSpace(req.EvidenceText) != "" {
		return msgs, nil
	}
	if len(r.fallbackTools) == 0 {
		return nil, fmt.Errorf("web search tool is required when evidence text is empty")
	}

	var lastErr error
	for _, tool := range r.fallbackTools {
		if req.ToolCallLimiter != nil {
			if err := req.ToolCallLimiter.Acquire(tool.Name()); err != nil {
				return nil, err
			}
		}
		if req.ToolObserver != nil {
			if err := req.ToolObserver.OnToolStart(ctx, tool.Name()); err != nil {
				return nil, fmt.Errorf("observe tool start: %w", err)
			}
		}
		toolResult, err := tool.Run(ctx, req.Query)
		if req.ToolObserver != nil {
			if endErr := req.ToolObserver.OnToolEnd(ctx, tool.Name(), err); endErr != nil {
				return nil, fmt.Errorf("observe tool end: %w", endErr)
			}
		}
		if err != nil {
			lastErr = err
			continue
		}
		if strings.TrimSpace(toolResult) == "" {
			continue
		}
		msgs = buildPromptMessages(req.History, toolResult, req.Query, req.ResponseFormatInstruction)
		return msgs, nil
	}
	if lastErr != nil {
		return nil, fmt.Errorf("run fallback tools: %w", lastErr)
	}
	return nil, fmt.Errorf("no fallback tool produced evidence")
}

func buildPromptMessages(history []memory.Turn, evidenceText, query string, responseFormatInstruction string) []llm.Message {
	msgs := make([]llm.Message, 0, len(history)*2+4)
	msgs = append(msgs, llm.Message{
		Role:    llm.RoleSystem,
		Content: "Answer with retrieved evidence first. If evidence is insufficient, use available tools. Prefer local retrieval before web search. If evidence is still insufficient, say so explicitly.",
	})
	if strings.TrimSpace(responseFormatInstruction) != "" {
		msgs = append(msgs, llm.Message{
			Role:    llm.RoleSystem,
			Content: responseFormatInstruction,
		})
	}

	for _, turn := range history {
		if strings.TrimSpace(turn.User) != "" {
			msgs = append(msgs, llm.Message{Role: llm.RoleUser, Content: turn.User})
		}
		if strings.TrimSpace(turn.Assistant) != "" {
			msgs = append(msgs, llm.Message{Role: llm.RoleAssistant, Content: turn.Assistant})
		}
	}
	if strings.TrimSpace(evidenceText) != "" {
		msgs = append(msgs, llm.Message{Role: llm.RoleUser, Content: "Relevant context:\n" + evidenceText})
	}
	msgs = append(msgs, llm.Message{Role: llm.RoleUser, Content: query})
	return msgs
}
