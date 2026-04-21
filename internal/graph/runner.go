package graph

import (
	"context"

	"github.com/gtkit/go-rag-agent/internal/llm"
	"github.com/gtkit/go-rag-agent/internal/memory"
)

// ToolObserver 接收 graph 内部工具执行事件。
type ToolObserver interface {
	OnToolStart(ctx context.Context, tool string) error
	OnToolEnd(ctx context.Context, tool string, err error) error
}

// ToolCallLimiter limits tool call attempts during one request.
type ToolCallLimiter interface {
	Acquire(tool string) error
}

// PromptCache caches built prompt messages for deterministic requests.
type PromptCache interface {
	Get(ctx context.Context, key string) ([]llm.Message, bool)
	Set(ctx context.Context, key string, messages []llm.Message)
}

// Request 表示内部 graph 层的请求输入。
type Request struct {
	Query                     string
	History                   []memory.Turn
	EvidenceText              string
	LongTermMemoryText        string
	ConversationSummary       string
	ResponseFormatInstruction string
	MaxPromptTokens           int
	MaxHistoryTokens          int
	MaxEvidenceTokens         int
	MaxMemoryTokens           int
	MaxSummaryTokens          int
	EnablePromptHardening     bool
	PromptCache               PromptCache
	PromptCacheObserver       func(bool)
	ToolObserver              ToolObserver
	ToolCallLimiter           ToolCallLimiter
}

// EventType 标识 graph 层流式事件类型。
type EventType string

const (
	EventAnswerChunk EventType = "answer_chunk"
	EventDone        EventType = "done"
)

// Event 表示一次 graph 层流式事件。
type Event struct {
	Type    EventType
	Content string
	Step    int
}

// StreamEmitter 负责发送 graph 层流式事件。
type StreamEmitter func(Event) error

// Runner 执行内部问答请求。
type Runner interface {
	Ask(ctx context.Context, req Request) (string, error)
	AskStream(ctx context.Context, req Request, emit StreamEmitter) error
}
