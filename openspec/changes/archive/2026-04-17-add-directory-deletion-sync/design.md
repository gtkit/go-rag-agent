## Context

当前 RAG 导入链路已经支持：
- `FileSource(path)` 单文件导入
- `DirSource(path)` 递归导入本地目录中的 `.txt`、`.md`、`.pdf`
- 对同一个文档父 ID 的 chunk 执行 stale cleanup

但 `DirSource(path)` 只会处理本次解析到的文件。某个文件从磁盘被删除后，只要没有新的 chunk 批次直接覆盖它的 `ParentID`，该文件旧 chunk 就会一直留在存储中。这个问题在“把一个本地目录长期作为个人知识库”的使用方式里会持续累积。

约束：
- 不能要求用户手工清理索引
- 不能误伤 `FileSource(...)` 或桥接导入的临时目录
- `DataDir` 模式下需要跨重启继续生效

## Goals / Non-Goals

**Goals：**

- 让 `AddKnowledge(ctx, DirSource(path))` 在成功导入后与当前目录文件集保持一致
- 支持空目录重导入时清空该目录之前导入过的历史索引
- 在 `DataDir` 持久化模式下跨重启保留目录删除同步状态
- 保持现有单文件导入、桥接导入、查询行为不变

**Non-Goals：**

- 不实现任意来源的“全量双向同步”
- 不为有道桥接导入增加删除同步；其导出目录是临时路径，不代表稳定知识库根目录
- 不新增复杂的目录版本控制、文件级 diff 或后台 watcher

## Decisions

### 1. 只对直接 `DirSource(path)` 启用目录删除同步

选择：新增一个内部目录同步作用域判断，只在 `src` 为直接 `dirSource` 时启用删除感知。

原因：
- `DirSource(path)` 的根路径稳定，适合作为同步主键。
- `YoudaoNoteSource(...)` 运行时会导出到临时目录，若按导出目录做删除同步会把临时路径当成长期知识库根目录，语义错误。

备选方案：
- 对所有能解析出多文件的来源统一做删除同步：会误伤临时桥接目录。
- 在 `KnowledgeSource` 公共接口上新增同步语义：会扩大公开 API 面。

### 2. 用独立目录清单文件记录“目录根路径 -> 最近一次成功导入的 source path 集合”

选择：在 Agent 侧维护目录导入清单；`DataDir` 非空时持久化到 `DataDir` 下的 JSON 文件，内存模式则只保存在进程内。

原因：
- 只需要按 source path 删除，不需要侵入现有 chunk 元数据结构。
- 持久化模式可跨重启保留删除同步语义。
- 避免依赖 chromem 未公开的内部遍历能力来枚举目录前缀。

备选方案：
- 直接从向量存储反查某个目录下已有 source path：当前底层只支持精确 metadata 过滤，不支持目录前缀过滤。
- 把目录映射嵌入 chunk metadata 再全文扫描：实现更重，也需要处理空目录场景。

### 3. 在成功写入当前批次后，再删除 stale source path，并在最后提交目录清单

选择：`DirSource` 导入流程顺序为：
1. 解析文件、加载文档、切块、生成 embedding
2. upsert 当前文件 chunk
3. 删除旧清单里存在、但当前目录已不存在的 source path
4. 提交并持久化新清单

原因：
- 先 upsert 再删 stale，可以保证更新后的文件已经可检索，再清掉旧文件。
- 只有当写入和删除都成功后才更新清单，避免清单领先于实际存储状态。

备选方案：
- 先更新清单再删：删除失败时清单会撒谎。
- 先删再写：中间失败会造成目录暂时性数据缺失。

### 4. 扩展 VectorStore 契约，增加按 source path 删除能力

选择：在 `VectorStore` 上新增 `DeleteBySourcePaths(ctx, sourcePaths []string) error`。

原因：
- 目录删除同步是存储层行为，不应把 chromem 细节泄漏到 Agent。
- 未来其他存储实现只需补同一个最小契约。

备选方案：
- 在 Agent 中直接依赖具体 `ChromemStore` 类型：会破坏抽象边界。

## Risks / Trade-offs

- [目录清单与存储状态不一致] → 只在 upsert 和 stale delete 都成功后提交清单；失败时保持旧清单不变
- [并发目录导入同一路径交错] → 用目录同步状态锁把“对比旧清单 + 存储删除 + 清单提交”串成一个原子流程
- [空目录没有当前 chunk 可写] → 目录同步逻辑允许在无新 chunk 的情况下仅执行 stale 删除
- [桥接导入被误判成目录同步] → 明确只识别直接 `dirSource`

## Migration Plan

1. 新增目录清单状态组件，并在 `New(...)` 时按 `DataDir` 初始化
2. 扩展 `VectorStore` 与 `ChromemStore`
3. 修改 `AddKnowledge(...)` 对 `DirSource(path)` 的执行顺序
4. 增加回归测试并更新 README

回滚策略：
- 回滚代码即可恢复旧行为
- 已落盘的目录清单文件即使保留，也不会被旧版本使用

## Open Questions

无
