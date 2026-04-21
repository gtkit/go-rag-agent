package graph

import (
	"context"
	"fmt"
	"strings"

	"github.com/gtkit/go-rag-agent/internal/llm"
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
	msgs := buildPromptMessages(
		req.History,
		req.EvidenceText,
		req.Query,
		req.ResponseFormatInstruction,
		req.ConversationSummary,
		req.MaxPromptTokens,
		req.MaxHistoryTokens,
		req.MaxEvidenceTokens,
		req.MaxSummaryTokens,
		req.EnablePromptHardening,
	)
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
		msgs = buildPromptMessages(
			req.History,
			toolResult,
			req.Query,
			req.ResponseFormatInstruction,
			req.ConversationSummary,
			req.MaxPromptTokens,
			req.MaxHistoryTokens,
			req.MaxEvidenceTokens,
			req.MaxSummaryTokens,
			req.EnablePromptHardening,
		)
		return msgs, nil
	}
	if lastErr != nil {
		return nil, fmt.Errorf("run fallback tools: %w", lastErr)
	}
	return nil, fmt.Errorf("no fallback tool produced evidence")
}
