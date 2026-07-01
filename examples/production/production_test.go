package main

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	ragagent "github.com/gtkit/go-rag-agent"
)

// 业务词表：词项重叠 embedder 用它把文本映射为确定性向量，
// 使无关问题与知识库正交（低于阈值->拒答），相关问题命中。
var demoVocab = []string{
	"配送", "运费", "包邮", "偏远地区", "物流",
	"退货", "退款", "无理由", "个人护理",
	"会员", "金卡", "钻石", "积分",
	"支付", "支付宝", "微信", "发票", "货到付款",
	"保修", "电子产品", "订单",
}

// termOverlapEmbedder 是离线确定性 embedder：命中词表的维度置 1，并加一个极小 bias 维避免零向量。
type termOverlapEmbedder struct{}

func (termOverlapEmbedder) EmbedTexts(_ context.Context, texts []string) ([][]float32, error) {
	rows := make([][]float32, 0, len(texts))
	for _, text := range texts {
		vec := make([]float32, len(demoVocab)+1)
		for i, term := range demoVocab {
			if strings.Contains(text, term) {
				vec[i] = 1
			}
		}
		vec[len(demoVocab)] = 0.001
		rows = append(rows, vec)
	}
	return rows, nil
}

// fixedChatModel 离线返回固定答案，足以让可回答用例产生 success trace。
type fixedChatModel struct{}

func (fixedChatModel) Generate(context.Context, []ragagent.Message) (ragagent.Message, error) {
	return ragagent.Message{Role: ragagent.RoleAssistant, Content: "依据知识库信息回答。"}, nil
}

func (fixedChatModel) Stream(_ context.Context, _ []ragagent.Message, emit func(string) error) error {
	if emit == nil {
		return nil
	}
	return emit("依据知识库信息回答。")
}

func offlineBaseConfig() ragagent.Config {
	return ragagent.Config{
		ChatModel: "offline-stub",
		Runtime: ragagent.RuntimeComponents{
			ChatModel: fixedChatModel{},
			Embedder:  termOverlapEmbedder{},
		},
		TopK:                5,
		SimilarityThreshold: 0.2,
		ChunkSize:           512,
		ChunkOverlap:        0,
		MaxHistoryRounds:    8,
		RequestTimeout:      5 * time.Second,
	}
}

func TestLoadEvalCasesAndCategories(t *testing.T) {
	t.Parallel()

	cases, err := LoadEvalCases("/kb")
	if err != nil {
		t.Fatalf("LoadEvalCases() error = %v", err)
	}
	if len(cases) < 30 {
		t.Fatalf("eval cases = %d, want >= 30", len(cases))
	}
	// want_citations 应被拼接为以 knowledgeDir 开头的 SourcePath。
	for _, c := range cases {
		for _, src := range c.WantCitationSources {
			if !strings.HasPrefix(src, "/kb/") {
				t.Fatalf("citation %q should be under /kb", src)
			}
		}
	}

	categories, err := datasetCategories()
	if err != nil {
		t.Fatalf("datasetCategories() error = %v", err)
	}
	for _, want := range []string{"answerable", "unanswerable", "low_similarity", "citation_conflict", "tool_failure"} {
		if !slices.Contains(categories, want) {
			t.Fatalf("dataset missing category %q (got %v)", want, categories)
		}
	}
}

func TestOrderToolOutcomes(t *testing.T) {
	t.Parallel()

	tool := newOrderTool(time.Second)
	ctx := context.Background()

	tests := []struct {
		name    string
		args    map[string]any
		wantErr error
		wantSub string
	}{
		{name: "成功", args: map[string]any{"order_id": "NO-1001"}, wantSub: "运输中"},
		{name: "缺参数", args: map[string]any{}, wantErr: ErrOrderArgInvalid},
		{name: "空参数", args: map[string]any{"order_id": "  "}, wantErr: ErrOrderArgInvalid},
		{name: "未命中", args: map[string]any{"order_id": "NO-0000"}, wantErr: ErrOrderNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			res, err := tool.RunStructured(ctx, tt.args)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want errors.Is(..., %v)", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error = %v", err)
			}
			if !strings.Contains(res.Text, tt.wantSub) {
				t.Fatalf("result %q does not contain %q", res.Text, tt.wantSub)
			}
			if res.Metadata["access"] != "read_only" {
				t.Fatalf("expected read_only access metadata, got %v", res.Metadata)
			}
		})
	}
}

func TestOrderToolRegistersToRegistry(t *testing.T) {
	t.Parallel()

	registry := ragagent.NewToolRegistry()
	tool := newOrderTool(time.Second)
	if err := registerOrderTool(registry, tool); err != nil {
		t.Fatalf("registerOrderTool() error = %v", err)
	}
	if _, ok := registry.Lookup(tool.Name()); !ok {
		t.Fatalf("tool %q not found in registry", tool.Name())
	}
}

func TestRunMultiTenantDemoIsolation(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	if err := runMultiTenantDemo(context.Background(), &out, offlineBaseConfig(), t.TempDir()); err != nil {
		t.Fatalf("runMultiTenantDemo() error = %v", err)
	}
	if !strings.Contains(out.String(), "多租户隔离验证通过") {
		t.Fatalf("multi-tenant demo output missing success line: %q", out.String())
	}
}

func TestRunEndToEndPipeline(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	if err := run(context.Background(), &out, offlineBaseConfig(), t.TempDir()); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	output := out.String()
	for _, want := range []string{"评测结果", "拒答样本", "只读订单工具", "多租户隔离验证通过", "参数错误已正确分类", "未命中已正确分类"} {
		if !strings.Contains(output, want) {
			t.Fatalf("end-to-end output missing %q\n%s", want, output)
		}
	}
}
