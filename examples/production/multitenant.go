package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	ragagent "github.com/gtkit/go-rag-agent"
)

// 多租户接入的两个 footgun（务必在接入侧处理，库不替你兜底）：
//
//  1. SessionID 是全局平坦命名空间。GetSession("order-123") 不带租户维度，
//     两个租户若都用 "order-123" 会撞进同一个会话、共享历史。
//     接入侧必须把 SessionID 按租户命名空间化，例如 tenantID + ":" + bizID。
//
//  2. 库默认做的是基于 metadata / source 过滤的“逻辑隔离”，不是“物理隔离”。
//     所有租户的向量可能共处同一存储，靠 filter 分开即可满足绝大多数业务；
//     若合规要求租户数据绝不共表/共索引，需通过 StorageComponents 为每租户注入独立存储。
//
// 本 demo 演示数据面隔离：per-tenant SourcePrefixes 过滤 + MemoryScope.Tenant。

// namespacedSessionID 演示 footgun #1 的正确做法：把 SessionID 按租户命名空间化。
func namespacedSessionID(tenant, bizID string) string {
	return tenant + ":" + bizID
}

// tenantDoc 是单个租户的一篇知识。
type tenantDoc struct {
	name    string
	content string
}

// writeTenantKnowledge 把某租户的知识写到 root/<tenant> 下，返回该租户的 source 前缀。
func writeTenantKnowledge(root, tenant string, docs []tenantDoc) (string, error) {
	dir := filepath.Join(root, tenant)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir tenant dir %q: %w", dir, err)
	}
	for _, doc := range docs {
		target := filepath.Join(dir, doc.name)
		if err := os.WriteFile(target, []byte(doc.content), 0o600); err != nil {
			return "", fmt.Errorf("write tenant doc %q: %w", target, err)
		}
	}
	return dir, nil
}

// runMultiTenantDemo 在单个 agent 上接入两个租户，交叉验证知识与记忆隔离。
// agent 的向量存储由调用方通过 baseConfig 决定（demo 用真实 store，测试可注入 stub runtime）。
func runMultiTenantDemo(ctx context.Context, out io.Writer, baseConfig ragagent.Config, root string) error {
	prefixA, err := writeTenantKnowledge(root, "tenant-a", []tenantDoc{
		{name: "policy.md", content: "# 甲公司专属政策\n\n甲公司客户的专属折扣是 8 折，仅限甲公司内部使用。"},
	})
	if err != nil {
		return err
	}
	prefixB, err := writeTenantKnowledge(root, "tenant-b", []tenantDoc{
		{name: "policy.md", content: "# 乙公司专属政策\n\n乙公司客户的专属折扣是 9 折，仅限乙公司内部使用。"},
	})
	if err != nil {
		return err
	}

	baseConfig.DataDir = filepath.Join(root, "rag-data")
	agent, err := ragagent.New(baseConfig)
	if err != nil {
		return fmt.Errorf("create multi-tenant agent: %w", err)
	}
	defer func() { _ = agent.Close() }()

	// 一次性导入两个租户的知识到同一 store（逻辑隔离）。
	if err := agent.AddKnowledge(ctx, ragagent.DirSource(root)); err != nil {
		return fmt.Errorf("import tenant knowledge: %w", err)
	}

	askTenant := func(tenant, prefix, bizID, query string) (ragagent.Answer, error) {
		session := agent.GetSession(namespacedSessionID(tenant, bizID))
		return session.AskWithOptions(ctx, query, ragagent.QueryOptions{
			Filter:      ragagent.RetrievalFilter{SourcePrefixes: []string{prefix}},
			MemoryScope: ragagent.MemoryScope{Tenant: tenant},
		})
	}

	answerA, err := askTenant("tenant-a", prefixA, "order-1", "我们的专属折扣是多少？")
	if err != nil {
		return fmt.Errorf("tenant-a ask: %w", err)
	}
	answerB, err := askTenant("tenant-b", prefixB, "order-1", "我们的专属折扣是多少？")
	if err != nil {
		return fmt.Errorf("tenant-b ask: %w", err)
	}

	// 隔离断言：每个租户的 citation 只能来自本租户前缀。
	if err := assertCitationsWithinPrefix(answerA.Citations, prefixA); err != nil {
		return fmt.Errorf("tenant-a 隔离被破坏: %w", err)
	}
	if err := assertCitationsWithinPrefix(answerB.Citations, prefixB); err != nil {
		return fmt.Errorf("tenant-b 隔离被破坏: %w", err)
	}

	if _, err := fmt.Fprintf(out, "多租户隔离验证通过：tenant-a 命中 %d 条引用、tenant-b 命中 %d 条引用，互不串数据。\n",
		len(answerA.Citations), len(answerB.Citations)); err != nil {
		return fmt.Errorf("write multi-tenant output: %w", err)
	}
	return nil
}

// assertCitationsWithinPrefix 校验所有 citation 的 SourcePath 都落在给定前缀内。
func assertCitationsWithinPrefix(citations []ragagent.Citation, prefix string) error {
	for _, c := range citations {
		if !strings.HasPrefix(c.SourcePath, prefix) {
			return fmt.Errorf("citation %q 不在前缀 %q 内", c.SourcePath, prefix)
		}
	}
	return nil
}
