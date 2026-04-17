## 为什么

当前 `ChromemStore.Upsert` 会执行多步写入：先写入新 chunk，再按父文档删除旧 chunk。若同一个 store 上有并发 `Upsert`，这些步骤可能交错，最终留下一个既不属于前者也不属于后者的混合状态。

## 变更内容

- 让单个 `ChromemStore` 实例内的 `Upsert` 串行执行。
- 增加并发 upsert 回归测试，证明同一 store 上的写入不会互相穿插。
- 同步 README 里 `SimilarityThreshold` 的真实行为描述。

## 能力

### 新能力

- 无。

### 修改的能力

- `embedded-rag-library`：嵌入式存储在并发写入时，必须保证单实例内 upsert 顺序一致，避免 stale cleanup 交错。

## 影响范围

- 影响 `internal/storage/chromem_store.go` 及其测试。
- 更新 README 中关于阈值的说明。
- 不改变对外 API 形状。
