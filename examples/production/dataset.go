package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	ragagent "github.com/gtkit/go-rag-agent"
)

// 通过 go:embed 把业务知识库与评测集随二进制打包，保证 demo 自包含、可离线运行。
//
//go:embed knowledge/*.md
var knowledgeFS embed.FS

//go:embed cases.json
var casesJSON []byte

// datasetCase 是评测集 JSON 的单条用例 DTO。
// category 仅用于覆盖标注（answerable/unanswerable/low_similarity/citation_conflict/tool_failure），
// 不参与判定；expect_refusal 决定通过条件；want_citations 存知识文件 basename。
type datasetCase struct {
	Name          string   `json:"name"`
	Category      string   `json:"category"`
	Query         string   `json:"query"`
	WantCitations []string `json:"want_citations,omitempty"`
	WantContains  []string `json:"want_contains,omitempty"`
	WantGrounded  []string `json:"want_grounded,omitempty"`
	ExpectRefusal bool     `json:"expect_refusal,omitempty"`
}

type datasetFile struct {
	Description string        `json:"description"`
	Cases       []datasetCase `json:"cases"`
}

// MaterializeKnowledge 把内嵌知识库写入 dir，返回该目录，供 DirSource 导入。
// 之所以落地到磁盘，是因为根包的 DirSource 走真实文件加载路径，与生产接入一致。
func MaterializeKnowledge(dir string) (string, error) {
	entries, err := fs.ReadDir(knowledgeFS, "knowledge")
	if err != nil {
		return "", fmt.Errorf("read embedded knowledge: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir knowledge dir %q: %w", dir, err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		content, err := knowledgeFS.ReadFile(filepath.Join("knowledge", entry.Name()))
		if err != nil {
			return "", fmt.Errorf("read embedded knowledge %q: %w", entry.Name(), err)
		}
		target := filepath.Join(dir, entry.Name())
		if err := os.WriteFile(target, content, 0o600); err != nil {
			return "", fmt.Errorf("write knowledge file %q: %w", target, err)
		}
	}
	return dir, nil
}

// LoadEvalCases 读取内嵌评测集，并把 want_citations 的 basename 与 knowledgeDir 拼接成
// 与实际导入一致的 SourcePath，返回可直接传入 ragagent.RunEvalSuite 的用例。
func LoadEvalCases(knowledgeDir string) ([]ragagent.EvalCase, error) {
	var file datasetFile
	if err := json.Unmarshal(casesJSON, &file); err != nil {
		return nil, fmt.Errorf("unmarshal eval cases: %w", err)
	}
	cases := make([]ragagent.EvalCase, 0, len(file.Cases))
	for _, c := range file.Cases {
		want := make([]string, 0, len(c.WantCitations))
		for _, name := range c.WantCitations {
			want = append(want, filepath.Join(knowledgeDir, name))
		}
		cases = append(cases, ragagent.EvalCase{
			Name:                   c.Name,
			SessionID:              c.Name,
			Query:                  c.Query,
			WantCitationSources:    want,
			WantAnswerContains:     c.WantContains,
			WantGroundedSubstrings: c.WantGrounded,
			ExpectRefusal:          c.ExpectRefusal,
		})
	}
	return cases, nil
}

// datasetCategories 返回评测集中出现的全部 category，用于自检覆盖面。
func datasetCategories() ([]string, error) {
	var file datasetFile
	if err := json.Unmarshal(casesJSON, &file); err != nil {
		return nil, fmt.Errorf("unmarshal eval cases: %w", err)
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, 5)
	for _, c := range file.Cases {
		if _, ok := seen[c.Category]; ok {
			continue
		}
		seen[c.Category] = struct{}{}
		out = append(out, c.Category)
	}
	return out, nil
}
