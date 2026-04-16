package tools

import (
	"context"
	"fmt"
	"strings"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	gtjson "github.com/gtkit/json"

	"my-gtkit-package/go-rag-agent/internal/storage"
)

const retrievalToolName = "retrieve_context"

// Retriever defines retrieval search by plain-text query.
type Retriever interface {
	Search(ctx context.Context, query string) ([]storage.SearchHit, error)
}

// RetrievalTool adapts retriever search into an Eino invokable tool.
type RetrievalTool struct {
	retriever Retriever
}

type retrieveArgs struct {
	Query string `json:"query"`
}

// NewRetrievalTool creates a retrieval tool.
func NewRetrievalTool(retriever Retriever) *RetrievalTool {
	return &RetrievalTool{
		retriever: retriever,
	}
}

// Info returns tool metadata for model tool-calling.
func (t *RetrievalTool) Info(context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: retrievalToolName,
		Desc: "Retrieve relevant evidence chunks from local knowledge.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {
				Type:     schema.String,
				Desc:     "standalone query for evidence retrieval",
				Required: true,
			},
		}),
	}, nil
}

// InvokableRun executes retrieval and returns newline-joined evidence lines.
func (t *RetrievalTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...einotool.Option) (string, error) {
	if t.retriever == nil {
		return "", fmt.Errorf("retriever is required")
	}

	var args retrieveArgs
	if err := gtjson.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", fmt.Errorf("decode retrieval args: %w", err)
	}
	args.Query = strings.TrimSpace(args.Query)
	if args.Query == "" {
		return "", fmt.Errorf("query is required")
	}

	hits, err := t.retriever.Search(ctx, args.Query)
	if err != nil {
		return "", fmt.Errorf("search retrieval hits: %w", err)
	}

	lines := make([]string, 0, len(hits))
	for _, hit := range hits {
		lines = append(lines, fmt.Sprintf("[%s] %s", hit.Chunk.SourcePath, strings.TrimSpace(hit.Chunk.Text)))
	}
	return strings.Join(lines, "\n"), nil
}
