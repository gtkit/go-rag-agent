## 背景

`ChromemStore.Upsert` 当前是一个多阶段过程：校验批次、写入/覆盖文档、再按父文档查询并删除 stale chunk。即使 stale cleanup 已经按父文档收缩，若同一个 store 上存在并发 upsert，多个过程仍可能交错。

## 目标 / 非目标

**目标：**

- 保证同一个 store 实例内的 upsert 在并发下是确定性的。
- 保持现有 upsert 语义与测试思路。
- 增加回归测试，证明第二个 upsert 不会在第一个中途插入。

**非目标：**

- 跨进程协调。
- 更细粒度的 per-parent 锁。
- 修改公开 API。

## 决策

### 为 store 增加局部 upsert 互斥

`ChromemStore` 使用一个内部互斥锁串行化 `Upsert`。

原因：
- 这是最小且安全的改动。
- 可以直接消除单进程内的混合写入。
- 不影响上层调用方式。

备选方案：
- 按 `ParentID` 做锁分片。当前 Phase 1 不需要这么复杂，因此暂不采用。

## 风险 / 取舍

- [降低并发导入吞吐] → 对 Phase 1 可接受，正确性优先于并发写吞吐。
- [测试 seam 复杂度增加] → 复用已有 `afterAddHook` 测试钩子，不再额外引入更多测试专用字段。

## 迁移计划

1. 增加 change artifact。
2. 在 store 层串行化 `Upsert`。
3. 增加并发回归测试。
4. 跑常规校验。

## 未决问题

- 无。
