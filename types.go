package ragagent

import (
	"context"
	"time"
)

// Logger 是根包使用的日志接口。
type Logger interface {
	Debug(msg string, kv ...any)
	Info(msg string, kv ...any)
	Warn(msg string, kv ...any)
	Error(msg string, kv ...any)
}

// Callback 接收检索、工具和模型执行的生命周期回调。
type Callback interface {
	OnRetrieveStart(ctx context.Context, query string)
	OnRetrieveEnd(ctx context.Context, resultCount int, err error)
	OnToolStart(ctx context.Context, tool string)
	OnToolEnd(ctx context.Context, tool string, err error)
	OnModelStart(ctx context.Context, model string)
	OnModelEnd(ctx context.Context, model string, err error)
}

// EventType 标识流式事件的类型。
type EventType string

const (
	// EventRetrieveStart 表示开始检索。
	EventRetrieveStart EventType = "retrieve_start"
	// EventRetrieveEnd 表示检索完成。
	EventRetrieveEnd EventType = "retrieve_end"
	// EventToolStart 表示工具调用开始。
	EventToolStart EventType = "tool_start"
	// EventToolEnd 表示工具调用结束。
	EventToolEnd EventType = "tool_end"
	// EventAnswerChunk 表示增量答案文本输出。
	EventAnswerChunk EventType = "answer_chunk"
	// EventCitation 表示引用信息已可用。
	EventCitation EventType = "citation"
	// EventError 表示执行期间发生错误。
	EventError EventType = "error"
	// EventDone 表示流式输出完成。
	EventDone EventType = "done"
)

// Citation 描述支撑答案的一段来源信息。
type Citation struct {
	SourcePath string
	Title      string
	ChunkID    string
	StartRune  int
	EndRune    int
}

// Answer 表示最终答案及其引用。
type Answer struct {
	Text      string
	Citations []Citation
}

// StreamEvent 是流式执行回调的事件载荷。
type StreamEvent struct {
	Type      EventType
	Content   string
	ToolName  string
	Step      int
	Citation  *Citation
	Err       error
	Timestamp time.Time
}

// KnowledgeFile 表示解析后的可导入文件。
type KnowledgeFile struct {
	Path     string
	Title    string
	Metadata map[string]string
}

// KnowledgeSource 从一种来源定义中解析出可导入文件。
type KnowledgeSource interface {
	Resolve(ctx context.Context) ([]KnowledgeFile, error)
}
