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

// RetrievalMetrics 描述一次检索阶段的聚合指标。
type RetrievalMetrics struct {
	Duration              time.Duration
	HybridEnabled         bool
	RerankEnabled         bool
	VectorCandidateCount  int
	LexicalCandidateCount int
	FusedCandidateCount   int
	RerankShortlistCount  int
	FinalHitCount         int
}

// ModelMetrics 描述一次模型调用的聚合指标。
type ModelMetrics struct {
	Model       string
	Duration    time.Duration
	Stream      bool
	OutputChars int
}

const (
	// FallbackStageHybrid 表示 hybrid 阶段降级。
	FallbackStageHybrid = "hybrid"
	// FallbackStageRerank 表示 rerank 阶段降级。
	FallbackStageRerank = "rerank"
	// FallbackTargetVectorOnly 表示退回 vector-only。
	FallbackTargetVectorOnly = "vector_only"
	// FallbackTargetHybrid 表示退回未 rerank 的 hybrid 结果。
	FallbackTargetHybrid = "hybrid"
)

// FallbackEvent 描述一次运行时降级事件。
type FallbackEvent struct {
	Stage      string
	FallbackTo string
	Err        error
}

// RetrievalMetricsCallback 是可选的检索指标回调接口。
type RetrievalMetricsCallback interface {
	OnRetrieveMetrics(ctx context.Context, metrics RetrievalMetrics)
}

// ModelMetricsCallback 是可选的模型指标回调接口。
type ModelMetricsCallback interface {
	OnModelMetrics(ctx context.Context, metrics ModelMetrics)
}

// FallbackCallback 是可选的降级事件回调接口。
type FallbackCallback interface {
	OnFallback(ctx context.Context, event FallbackEvent)
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
// SourcePath 对本地知识表示 source path，对联网搜索结果表示 canonical URL。
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
	Trace     *ExecutionTrace
}

// RetrievalFilter 定义单次问答检索阶段的过滤条件。
type RetrievalFilter struct {
	SourcePaths    []string
	SourcePrefixes []string
	Metadata       map[string]string
}

// MemoryScope 定义长期记忆的 user/tenant 分层范围。
type MemoryScope struct {
	UserID string
	Tenant string
}

// QueryOptions 定义单次问答的可选执行参数。
type QueryOptions struct {
	Filter      RetrievalFilter
	MemoryScope MemoryScope
}

// StreamEvent 是流式执行回调的事件载荷。
type StreamEvent struct {
	Type      EventType
	Content   string
	ToolName  string
	Step      int
	Citation  *Citation
	Trace     *ExecutionTrace
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
