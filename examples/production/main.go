// Command production 是一个端到端实战 demo：
// 导入业务知识 -> 运行业务级评测（含拒答正确率）-> 输出评测报告 JSON ->
// 通过可查询 trace store 回看每次执行 -> 演示只读订单工具 -> 演示多租户数据隔离。
//
// 设计要点：
//   - run() 接收一个 baseConfig，因此既能用环境变量接真实模型（main），
//     也能在测试里注入 stub runtime 离线跑通全流程。
//   - 评测 agent 不注册 fallback 工具，使"不可回答类"问题能确定性地走到拒答路径。
//   - 只读订单工具单独演示，展示工具契约与"何时不该调用"，不污染评测的拒答判定。
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	ragagent "github.com/gtkit/go-rag-agent"
)

func main() {
	cfg, err := configFromEnv(os.Getenv)
	if err != nil {
		log.Fatalf("production demo 需要配置: %v", err)
	}
	workdir, err := os.MkdirTemp("", "ragagent-production-demo-")
	if err != nil {
		log.Fatalf("create workdir: %v", err)
	}
	defer func() { _ = os.RemoveAll(workdir) }()

	if err := run(context.Background(), os.Stdout, cfg, workdir); err != nil {
		log.Fatalf("run production demo: %v", err)
	}
}

// configFromEnv 从环境变量构造接真实模型的基础配置。
func configFromEnv(getenv func(string) string) (ragagent.Config, error) {
	cfg := ragagent.Config{
		ChatModel:        strings.TrimSpace(getenv("RAGAGENT_CHAT_MODEL")),
		ChatBaseURL:      strings.TrimSpace(getenv("RAGAGENT_CHAT_BASE_URL")),
		ChatAPIKey:       strings.TrimSpace(getenv("RAGAGENT_CHAT_API_KEY")),
		EmbeddingModel:   strings.TrimSpace(getenv("RAGAGENT_EMBEDDING_MODEL")),
		EmbeddingBaseURL: strings.TrimSpace(getenv("RAGAGENT_EMBEDDING_BASE_URL")),
		EmbeddingAPIKey:  strings.TrimSpace(getenv("RAGAGENT_EMBEDDING_API_KEY")),
		RequestTimeout:   20 * time.Second,
	}
	if cfg.EmbeddingBaseURL == "" {
		cfg.EmbeddingBaseURL = cfg.ChatBaseURL
	}
	if cfg.EmbeddingAPIKey == "" {
		cfg.EmbeddingAPIKey = cfg.ChatAPIKey
	}
	if cfg.ChatModel == "" || cfg.ChatBaseURL == "" || cfg.ChatAPIKey == "" || cfg.EmbeddingModel == "" {
		return ragagent.Config{}, errors.New("请设置 RAGAGENT_CHAT_MODEL、RAGAGENT_CHAT_BASE_URL、RAGAGENT_CHAT_API_KEY、RAGAGENT_EMBEDDING_MODEL")
	}
	return cfg, nil
}

// errWriter 包装 io.Writer，记录首个写错误，避免在每行输出处重复检查。
type errWriter struct {
	w   io.Writer
	err error
}

func (e *errWriter) printf(format string, a ...any) {
	if e.err != nil {
		return
	}
	_, e.err = fmt.Fprintf(e.w, format, a...)
}

// run 串起端到端流程，所有产物落在 workdir 下。
func run(ctx context.Context, out io.Writer, baseConfig ragagent.Config, workdir string) error {
	if err := runEvalPipeline(ctx, out, baseConfig, workdir); err != nil {
		return err
	}
	if err := demoOrderTool(ctx, out); err != nil {
		return err
	}
	mtRoot := filepath.Join(workdir, "multitenant")
	if err := runMultiTenantDemo(ctx, out, baseConfig, mtRoot); err != nil {
		return err
	}
	return nil
}

// runEvalPipeline 完成"导入 -> 评测 -> 报告 -> trace 查询"。
func runEvalPipeline(ctx context.Context, out io.Writer, baseConfig ragagent.Config, workdir string) error {
	knowledgeDir, err := MaterializeKnowledge(filepath.Join(workdir, "knowledge"))
	if err != nil {
		return err
	}

	// 1) 注册可查询 trace store 作为 recorder，使每次评测执行都被持久化。
	store := ragagent.NewInMemoryTraceStore(0)
	evalCfg := baseConfig
	evalCfg.DataDir = filepath.Join(workdir, "rag-eval")
	evalCfg.TraceRecorder = store

	agent, err := ragagent.New(evalCfg)
	if err != nil {
		return fmt.Errorf("create eval agent: %w", err)
	}
	defer func() { _ = agent.Close() }()

	// 2) 导入业务知识。
	if err := agent.AddKnowledge(ctx, ragagent.DirSource(knowledgeDir)); err != nil {
		return fmt.Errorf("import knowledge: %w", err)
	}

	// 3) 加载业务级评测集并运行评测。
	cases, err := LoadEvalCases(knowledgeDir)
	if err != nil {
		return err
	}
	summary, results, err := ragagent.RunEvalSuite(ctx, agent, cases)
	if err != nil {
		return fmt.Errorf("run eval suite: %w", err)
	}

	// 4) 输出评测报告 JSON。
	reportPath := filepath.Join(workdir, "eval-report.json")
	if err := ragagent.WriteEvalReportJSON(reportPath, ragagent.EvalReport{Summary: summary, Results: results}); err != nil {
		return fmt.Errorf("write eval report: %w", err)
	}

	ew := &errWriter{w: out}
	ew.printf("==== 评测结果 ====\n")
	ew.printf("用例总数=%d 通过=%d 拒答正确率=%.2f 召回=%.2f 引用准确=%.2f\n",
		summary.TotalCases, summary.PassedCases, summary.RefusalAccuracy, summary.RetrievalRecall, summary.CitationPrecision)
	ew.printf("报告已写入：%s\n", reportPath)

	// 5) 通过 trace store 回看：拒答原因 + 可回答用例的 citation 来源。
	refused, err := store.Query(ctx, ragagent.TraceQuery{Status: ragagent.TraceStatusRefused})
	if err != nil {
		return fmt.Errorf("query refused traces: %w", err)
	}
	ew.printf("==== 拒答样本（共 %d 条）====\n", len(refused))
	for _, tr := range refused {
		ew.printf("- [%s] 原因=%s\n", tr.QuerySummary, tr.RefusalReason)
	}

	succeeded, err := store.Query(ctx, ragagent.TraceQuery{Status: ragagent.TraceStatusSuccess, Limit: 3})
	if err != nil {
		return fmt.Errorf("query success traces: %w", err)
	}
	ew.printf("==== 可回答样本 citation 来源（取前 %d 条）====\n", len(succeeded))
	for _, tr := range succeeded {
		ew.printf("- [%s] 引用=%v 延迟=%s\n", tr.QuerySummary, tr.CitationSources, tr.Latency)
	}
	return ew.err
}

// demoOrderTool 演示只读订单工具：注册到 ToolRegistry + 三类调用结果 + 何时不该调用。
func demoOrderTool(ctx context.Context, out io.Writer) error {
	tool := newOrderTool(2 * time.Second)
	registry := ragagent.NewToolRegistry()
	if err := registerOrderTool(registry, tool); err != nil {
		return fmt.Errorf("register order tool: %w", err)
	}
	if _, ok := registry.Lookup(tool.Name()); !ok {
		return fmt.Errorf("order tool 未注册成功")
	}

	ew := &errWriter{w: out}
	ew.printf("==== 只读订单工具 ====\n")

	// 成功
	if res, err := tool.RunStructured(ctx, map[string]any{"order_id": "NO-1001"}); err == nil {
		ew.printf("查询成功：%s\n", res.Text)
	} else {
		return fmt.Errorf("expected success for NO-1001: %w", err)
	}
	// 参数错误
	if _, err := tool.RunStructured(ctx, map[string]any{}); errors.Is(err, ErrOrderArgInvalid) {
		ew.printf("参数错误已正确分类：%v\n", err)
	} else {
		return fmt.Errorf("expected ErrOrderArgInvalid, got %v", err)
	}
	// 未命中
	if _, err := tool.RunStructured(ctx, map[string]any{"order_id": "NO-9999"}); errors.Is(err, ErrOrderNotFound) {
		ew.printf("未命中已正确分类：%v\n", err)
	} else {
		return fmt.Errorf("expected ErrOrderNotFound, got %v", err)
	}

	ew.printf("使用边界：配送/退货/保修等政策问题应走知识库，不调用本工具；缺订单号应追问用户；本工具不提供任何写操作。\n")
	return ew.err
}
