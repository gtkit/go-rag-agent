## 1. 存储一致性

- [x] 1.1 在单个 store 实例内串行化 `ChromemStore.Upsert`。
- [x] 1.2 增加同一 store 上重叠 upsert 的回归测试。

## 2. 验证

- [x] 2.1 运行测试、vet、lint 和 strict OpenSpec validate，确认导入一致性变更正确。
