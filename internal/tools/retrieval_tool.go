package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/gtkit/go-rag-agent/internal/storage"
)

const retrievalToolName = "retrieve_context"

// Tool 定义项目内统一工具契约。
type Tool interface {
	Name() string
	Description() string
	Run(ctx context.Context, input string) (string, error)
}

// Retriever 定义基于纯文本查询的检索接口。
type Retriever interface {
	Search(ctx context.Context, query string) ([]storage.SearchHit, error)
}

// RetrievalTool 把检索能力适配为 Eino 可调用工具。
type RetrievalTool struct {
	retriever Retriever
}

// NewRetrievalTool 创建检索工具。
func NewRetrievalTool(retriever Retriever) *RetrievalTool {
	return &RetrievalTool{
		retriever: retriever,
	}
}

func (t *RetrievalTool) Name() string {
	return retrievalToolName
}

func (t *RetrievalTool) Description() string {
	return "Retrieve relevant evidence chunks from local knowledge."
}

// Run 执行检索，并返回按行拼接的证据文本。
func (t *RetrievalTool) Run(ctx context.Context, input string) (string, error) {
	if t.retriever == nil {
		return "", fmt.Errorf("retriever is required")
	}

	query := strings.TrimSpace(input)
	if query == "" {
		return "", fmt.Errorf("query is required")
	}

	hits, err := t.retriever.Search(ctx, query)
	if err != nil {
		return "", fmt.Errorf("search retrieval hits: %w", err)
	}

	lines := make([]string, 0, len(hits))
	for _, hit := range hits {
		lines = append(lines, fmt.Sprintf("[%s] %s", hit.Chunk.SourcePath, strings.TrimSpace(hit.Chunk.Text)))
	}
	return strings.Join(lines, "\n"), nil
}
