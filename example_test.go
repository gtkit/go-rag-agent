package ragagent_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	ragagent "github.com/gtkit/go-rag-agent"
	"github.com/gtkit/pgorm"
)

func ExampleConfig_Validate() {
	cfg := ragagent.Config{
		ChatBaseURL:    "https://api.openai.example/v1",
		ChatAPIKey:     "placeholder-chat-key",
		EmbeddingModel: "text-embedding-3-small",
	}

	err := cfg.Validate()
	fmt.Println(errors.Is(err, ragagent.ErrInvalidConfig))

	// Output: true
}

func ExampleSession_Ask() {
	agent, err := ragagent.New(ragagent.Config{
		ChatModel: "gpt-4o-mini",
		Runtime: ragagent.RuntimeComponents{
			ChatModel: exampleChatModel{},
			Embedder:  exampleEmbedder{},
		},
		Storage: ragagent.StorageComponents{
			VectorStore: exampleVectorStore{},
		},
		TopK:             1,
		ChunkSize:        64,
		ChunkOverlap:     0,
		MaxHistoryRounds: 8,
		RequestTimeout:   time.Second,
	})
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = agent.Close()
	}()

	answer, err := agent.GetSession("basic").Ask(context.Background(), "what changed")
	if err != nil {
		panic(err)
	}
	fmt.Println(answer.Text)
	fmt.Println(len(answer.Citations))

	// Output:
	// ok
	// 1
}

type examplePromptCache struct {
	items map[string][]ragagent.Message
}

func (c *examplePromptCache) Get(_ context.Context, key string) ([]ragagent.Message, bool) {
	msgs, ok := c.items[key]
	return msgs, ok
}

func (c *examplePromptCache) Set(_ context.Context, key string, messages []ragagent.Message) {
	if c.items == nil {
		c.items = make(map[string][]ragagent.Message)
	}
	c.items[key] = messages
}

type exampleChatModel struct{}

func (exampleChatModel) Generate(context.Context, []ragagent.Message) (ragagent.Message, error) {
	return ragagent.Message{Role: ragagent.RoleAssistant, Content: "ok"}, nil
}

func (exampleChatModel) Stream(_ context.Context, _ []ragagent.Message, emit func(string) error) error {
	return emit("ok")
}

type exampleEmbedder struct{}

func (exampleEmbedder) EmbedTexts(_ context.Context, texts []string) ([][]float32, error) {
	rows := make([][]float32, 0, len(texts))
	for range texts {
		rows = append(rows, []float32{1})
	}
	return rows, nil
}

type exampleVectorStore struct{}

func (exampleVectorStore) Upsert(context.Context, []ragagent.ChunkRecord) error { return nil }
func (exampleVectorStore) Search(context.Context, []float32, int, float32) ([]ragagent.SearchHit, error) {
	return []ragagent.SearchHit{
		{
			Chunk: ragagent.ChunkRecord{
				ChunkID:    "doc:0",
				ParentID:   "doc",
				SourcePath: "/tmp/doc.md",
				Title:      "doc",
				Text:       "example evidence",
				StartRune:  0,
				EndRune:    16,
			},
			Score: 0.99,
		},
	}, nil
}
func (exampleVectorStore) SearchWithFilter(context.Context, []float32, int, float32, ragagent.SearchFilter) ([]ragagent.SearchHit, error) {
	return []ragagent.SearchHit{
		{
			Chunk: ragagent.ChunkRecord{
				ChunkID:    "doc:0",
				ParentID:   "doc",
				SourcePath: "/tmp/doc.md",
				Title:      "doc",
				Text:       "example evidence",
				StartRune:  0,
				EndRune:    16,
			},
			Score: 0.99,
		},
	}, nil
}
func (exampleVectorStore) DeleteBySourcePaths(context.Context, []string) error { return nil }
func (exampleVectorStore) Close() error                                        { return nil }

func ExampleNewTwoLevelPromptCache() {
	local := ragagent.NewInMemoryPromptCacheWithConfig(ragagent.PromptCacheConfig{
		MaxEntries: 16,
		TTL:        time.Minute,
	})
	remote := &examplePromptCache{}
	cache := ragagent.NewTwoLevelPromptCache(local, remote)

	cache.Set(context.Background(), "key", []ragagent.Message{{Role: ragagent.RoleUser, Content: "hello"}})
	_, ok := cache.Get(context.Background(), "key")
	fmt.Println(ok)

	// Output: true
}

func ExampleRunEvalSuite() {
	agent, err := ragagent.New(ragagent.Config{
		ChatModel: "gpt-4o-mini",
		Runtime: ragagent.RuntimeComponents{
			ChatModel: exampleChatModel{},
			Embedder:  exampleEmbedder{},
		},
		Storage: ragagent.StorageComponents{
			VectorStore: exampleVectorStore{},
		},
		TopK:             1,
		ChunkSize:        64,
		ChunkOverlap:     0,
		MaxHistoryRounds: 8,
		RequestTimeout:   time.Second,
	})
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = agent.Close()
	}()

	summary, _, err := ragagent.RunEvalSuite(context.Background(), agent, []ragagent.EvalCase{
		{
			Name:                   "basic",
			SessionID:              "eval",
			Query:                  "what changed",
			WantCitationSources:    []string{"/tmp/doc.md"},
			WantAnswerContains:     []string{"ok"},
			WantGroundedSubstrings: []string{"ok"},
		},
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(summary.PassedCases)

	// Output: 1
}

func ExamplePGVectorStoreConfig_pgorm() {
	pgCfg := pgorm.NewConfig(
		pgorm.WithDSN("postgres://user:pass@127.0.0.1:5432/rag?sslmode=disable"),
		pgorm.WithStartupPing(false),
	)

	cfg := ragagent.PGVectorStoreConfig{
		PGORMConfig: &pgCfg,
		TableName:   "knowledge_chunks",
		Dimensions:  1536,
	}

	fmt.Println(cfg.TableName)

	// Output: knowledge_chunks
}

func ExampleWriteEvalReportJSON() {
	path := filepath.Join(os.TempDir(), "ragagent-eval-report.json")
	report := ragagent.EvalReport{
		Summary: ragagent.EvalSummary{
			TotalCases:  1,
			PassedCases: 1,
		},
	}

	if err := ragagent.WriteEvalReportJSON(path, report); err != nil {
		panic(err)
	}
	loaded, err := ragagent.ReadEvalReportJSON(path)
	if err != nil {
		panic(err)
	}
	_ = os.Remove(path)

	fmt.Println(loaded.Summary.PassedCases)

	// Output: 1
}

func ExampleNewCommandDocumentConverter() {
	converter, err := ragagent.NewCommandDocumentConverter(ragagent.CommandDocumentConverterConfig{
		Name:       "office-converter",
		Extensions: []string{".docx"},
		Command:    "office-to-markdown",
		Args:       []string{"{input}", "{output}"},
	})
	if err != nil {
		panic(err)
	}

	fmt.Println(converter.Name())
	fmt.Println(ragagent.ConvertibleFileSource("handbook.docx") != nil)

	// Output:
	// office-converter
	// true
}

func ExampleNewStructuredToolAdapter() {
	tool := ragagent.NewStructuredToolAdapter(exampleStructuredTool{})
	registry := ragagent.NewToolRegistry(tool)

	registered, _ := registry.Lookup("lookup")
	result, err := registered.Run(context.Background(), `{"query":"rag"}`)
	if err != nil {
		panic(err)
	}
	fmt.Println(result)

	// Output:
	// found: rag
}

type exampleStructuredTool struct{}

func (exampleStructuredTool) Name() string { return "lookup" }

func (exampleStructuredTool) Description() string { return "Look up safe internal data." }

func (exampleStructuredTool) Schema() ragagent.ToolSchema {
	return ragagent.ToolSchema{
		Properties: map[string]ragagent.ToolParameterSchema{
			"query": {Type: ragagent.ToolParameterString, MinLength: 1},
		},
		Required: []string{"query"},
	}
}

func (exampleStructuredTool) RunStructured(_ context.Context, args map[string]any) (ragagent.ToolResult, error) {
	return ragagent.ToolResult{Text: "found: " + args["query"].(string)}, nil
}

func ExampleNewJSONLTraceRecorder() {
	var buf bytes.Buffer
	recorder := ragagent.NewJSONLTraceRecorder(&buf)
	recorder.OnExecutionTrace(context.Background(), ragagent.ExecutionTrace{
		SessionID: "session-1",
		Success:   true,
	})

	var line map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &line); err != nil {
		panic(err)
	}
	fmt.Println(line["session_id"])
	fmt.Println(line["success"])

	// Output:
	// session-1
	// true
}
