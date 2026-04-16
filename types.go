package ragagent

import (
	"context"
	"time"
)

// Logger is the logging contract used by the root package.
type Logger interface {
	Debug(msg string, kv ...any)
	Info(msg string, kv ...any)
	Warn(msg string, kv ...any)
	Error(msg string, kv ...any)
}

// Callback receives lifecycle hooks for retrieval, tool, and model operations.
type Callback interface {
	OnRetrieveStart(ctx context.Context, query string)
	OnRetrieveEnd(ctx context.Context, resultCount int, err error)
	OnToolStart(ctx context.Context, tool string)
	OnToolEnd(ctx context.Context, tool string, err error)
	OnModelStart(ctx context.Context, model string)
	OnModelEnd(ctx context.Context, model string, err error)
}

// EventType identifies the type of a streaming event.
type EventType string

const (
	// EventRetrieveStart is emitted when retrieval begins.
	EventRetrieveStart EventType = "retrieve_start"
	// EventRetrieveEnd is emitted when retrieval completes.
	EventRetrieveEnd EventType = "retrieve_end"
	// EventToolStart is emitted when a tool call begins.
	EventToolStart EventType = "tool_start"
	// EventToolEnd is emitted when a tool call completes.
	EventToolEnd EventType = "tool_end"
	// EventAnswerChunk is emitted for incremental answer text output.
	EventAnswerChunk EventType = "answer_chunk"
	// EventCitation is emitted when a citation becomes available.
	EventCitation EventType = "citation"
	// EventError is emitted when an execution error occurs.
	EventError EventType = "error"
	// EventDone is emitted when streaming completes.
	EventDone EventType = "done"
)

// Citation describes the source span backing part of an answer.
type Citation struct {
	SourcePath string
	Title      string
	ChunkID    string
	StartRune  int
	EndRune    int
}

// Answer is a final answer plus its supporting citations.
type Answer struct {
	Text      string
	Citations []Citation
}

// StreamEvent is a callback payload for incremental execution updates.
type StreamEvent struct {
	Type      EventType
	Content   string
	ToolName  string
	Step      int
	Citation  *Citation
	Err       error
	Timestamp time.Time
}

// KnowledgeFile represents a resolved file candidate for ingestion.
type KnowledgeFile struct {
	Path     string
	Title    string
	Metadata map[string]string
}

// KnowledgeSource resolves knowledge files from one source definition.
type KnowledgeSource interface {
	Resolve(ctx context.Context) ([]KnowledgeFile, error)
}
