package graph

import (
	"context"

	"my-gtkit-package/go-rag-agent/internal/memory"
)

// Request 表示内部 graph 层的请求输入。
type Request struct {
	Query        string
	History      []memory.Turn
	EvidenceText string
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
