package graph

import (
	"context"

	"my-gtkit-package/go-rag-agent/internal/memory"
)

// Request is the internal graph ask input.
type Request struct {
	Query        string
	History      []memory.Turn
	EvidenceText string
}

// EventType identifies graph-layer stream events.
type EventType string

const (
	EventAnswerChunk EventType = "answer_chunk"
	EventDone        EventType = "done"
)

// Event is one graph stream event.
type Event struct {
	Type    EventType
	Content string
	Step    int
}

// StreamEmitter emits graph stream events.
type StreamEmitter func(Event) error

// Runner executes ask requests.
type Runner interface {
	Ask(ctx context.Context, req Request) (string, error)
	AskStream(ctx context.Context, req Request, emit StreamEmitter) error
}
