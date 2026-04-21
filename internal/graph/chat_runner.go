package graph

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
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
	if req.PromptCache != nil {
		key := promptCacheKey(req)
		if cached, ok := req.PromptCache.Get(ctx, key); ok {
			if req.PromptCacheObserver != nil {
				req.PromptCacheObserver(true)
			}
			return cached, nil
		}
		msgs, err := r.messagesForRequestUncached(ctx, req)
		if err != nil {
			return nil, err
		}
		req.PromptCache.Set(ctx, key, msgs)
		if req.PromptCacheObserver != nil {
			req.PromptCacheObserver(false)
		}
		return msgs, nil
	}
	return r.messagesForRequestUncached(ctx, req)
}

func (r *ChatRunner) messagesForRequestUncached(ctx context.Context, req Request) ([]llm.Message, error) {
	msgs := buildPromptMessages(
		req.History,
		req.EvidenceText,
		req.LongTermMemoryText,
		req.Query,
		req.ResponseFormatInstruction,
		req.ConversationSummary,
		req.MaxPromptTokens,
		req.MaxHistoryTokens,
		req.MaxEvidenceTokens,
		req.MaxMemoryTokens,
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
			req.LongTermMemoryText,
			req.Query,
			req.ResponseFormatInstruction,
			req.ConversationSummary,
			req.MaxPromptTokens,
			req.MaxHistoryTokens,
			req.MaxEvidenceTokens,
			req.MaxMemoryTokens,
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

func promptCacheKey(req Request) string {
	var builder strings.Builder
	builder.WriteString(req.Query)
	builder.WriteString("\n--evidence--\n")
	builder.WriteString(req.EvidenceText)
	builder.WriteString("\n--memory--\n")
	builder.WriteString(req.LongTermMemoryText)
	builder.WriteString("\n--summary--\n")
	builder.WriteString(req.ConversationSummary)
	builder.WriteString("\n--format--\n")
	builder.WriteString(req.ResponseFormatInstruction)
	builder.WriteString("\n--limits--\n")
	builder.WriteString(strconv.Itoa(req.MaxPromptTokens))
	builder.WriteByte('|')
	builder.WriteString(strconv.Itoa(req.MaxHistoryTokens))
	builder.WriteByte('|')
	builder.WriteString(strconv.Itoa(req.MaxEvidenceTokens))
	builder.WriteByte('|')
	builder.WriteString(strconv.Itoa(req.MaxMemoryTokens))
	builder.WriteByte('|')
	builder.WriteString(strconv.Itoa(req.MaxSummaryTokens))
	builder.WriteByte('|')
	builder.WriteString(strconv.FormatBool(req.EnablePromptHardening))
	for _, turn := range req.History {
		builder.WriteString("\n--turn--\n")
		builder.WriteString(turn.User)
		builder.WriteString("\n--assistant--\n")
		builder.WriteString(turn.Assistant)
	}
	sum := sha256.Sum256([]byte(builder.String()))
	return hex.EncodeToString(sum[:])
}
