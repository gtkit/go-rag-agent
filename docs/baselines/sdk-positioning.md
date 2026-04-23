# SDK Positioning Baseline

本基线用于回答两个问题：

- 仓库当前对外主叙事是否仍然对应“可嵌入的 Go SDK”
- 现有 `EvalReport` / `ExecutionTraceSummary` API 是否有一条可复现、可提交的最小消费路径

## 生成文件

- `docs/baselines/sdk-positioning/eval-report.json`
- `docs/baselines/sdk-positioning/trace-summary.json`

## 重新生成

在仓库根目录执行：

```bash
go run ./cmd/generate-sdk-positioning-baseline
```

如果你想生成到临时目录：

```bash
go run ./cmd/generate-sdk-positioning-baseline -out /tmp/sdk-positioning-baseline
```

## 对比方法

生成新的 report 后，可以直接和仓库内基线比较：

```go
report, err := ragagent.ReadEvalReportJSON("/tmp/sdk-positioning-baseline/eval-report.json")
if err != nil {
	log.Fatalf("read report: %v", err)
}

baseline, err := ragagent.ReadEvalReportJSON("docs/baselines/sdk-positioning/eval-report.json")
if err != nil {
	log.Fatalf("read baseline: %v", err)
}

if err := ragagent.CompareEvalReportWithBaseline(report, baseline); err != nil {
	log.Fatalf("compare baseline: %v", err)
}
```

## 说明

- baseline 使用固定知识源 `testdata/baseline/overview.md`
- 生成时会把 report 内的 source path 和 chunk ID 归一化为仓库相对路径，避免本机绝对路径污染样例结果
- `trace-summary.json` 中的 `Duration` 会被归一化为 `0`，避免时钟抖动导致提交结果反复变化
- 这份基线是 SDK 定位和接入链路的 smoke artifact，不等同于真实生产 benchmark
