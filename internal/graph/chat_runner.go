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
	model   llm.ChatModel
	webTool tools.Tool
}

// NewChatRunner 创建问答 runner。
func NewChatRunner(model llm.ChatModel, toolset ...tools.Tool) (*ChatRunner, error) {
	if model == nil {
		return nil, fmt.Errorf("chat model is required")
	}

	var webTool tools.Tool
	for _, tool := range toolset {
		if tool == nil {
			continue
		}
		if tool.Name() == "search_web" {
			webTool = tool
			break
		}
	}

	return &ChatRunner{
		model:   model,
		webTool: webTool,
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
	msgs := buildPromptMessages(req.History, req.EvidenceText, req.Query)
	if strings.TrimSpace(req.EvidenceText) != "" {
		return msgs, nil
	}
	if r.webTool == nil {
		return nil, fmt.Errorf("web search tool is required when evidence text is empty")
	}

	if req.ToolObserver != nil {
		if err := req.ToolObserver.OnToolStart(ctx, r.webTool.Name()); err != nil {
			return nil, fmt.Errorf("observe web search tool start: %w", err)
		}
	}
	webResult, err := r.webTool.Run(ctx, req.Query)
	if req.ToolObserver != nil {
		if endErr := req.ToolObserver.OnToolEnd(ctx, r.webTool.Name(), err); endErr != nil {
			return nil, fmt.Errorf("observe web search tool end: %w", endErr)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("run web search tool: %w", err)
	}
	msgs = buildPromptMessages(req.History, webResult, req.Query)
	return msgs, nil
}

func buildPromptMessages(history []memory.Turn, evidenceText, query string) []llm.Message {
	msgs := make([]llm.Message, 0, len(history)*2+3)
	msgs = append(msgs, llm.Message{
		Role:    llm.RoleSystem,
		Content: "Answer with retrieved evidence first. If evidence is insufficient, use available tools. Prefer local retrieval before web search. If evidence is still insufficient, say so explicitly.",
	})

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
