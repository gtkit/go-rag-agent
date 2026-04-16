package ragagent

import (
	"context"
	"time"
)

type Logger interface {
	Debug(msg string, kv ...any)
	Info(msg string, kv ...any)
	Warn(msg string, kv ...any)
	Error(msg string, kv ...any)
}

type Callback interface {
	OnRetrieveStart(ctx context.Context, query string)
	OnRetrieveEnd(ctx context.Context, resultCount int, err error)
	OnToolStart(ctx context.Context, tool string)
	OnToolEnd(ctx context.Context, tool string, err error)
	OnModelStart(ctx context.Context, model string)
	OnModelEnd(ctx context.Context, model string, err error)
}

type EventType string

const (
	EventRetrieveStart EventType = "retrieve_start"
	EventRetrieveEnd   EventType = "retrieve_end"
	EventToolStart     EventType = "tool_start"
	EventToolEnd       EventType = "tool_end"
	EventAnswerChunk   EventType = "answer_chunk"
	EventCitation      EventType = "citation"
	EventError         EventType = "error"
	EventDone          EventType = "done"
)

type Citation struct {
	SourcePath string
	Title      string
	ChunkID    string
	StartRune  int
	EndRune    int
}

type Answer struct {
	Text      string
	Citations []Citation
}

type StreamEvent struct {
	Type      EventType
	Content   string
	ToolName  string
	Step      int
	Citation  *Citation
	Err       error
	Timestamp time.Time
}

type KnowledgeFile struct {
	Path     string
	Title    string
	Metadata map[string]string
}

type KnowledgeSource interface {
	Resolve(ctx context.Context) ([]KnowledgeFile, error)
}
