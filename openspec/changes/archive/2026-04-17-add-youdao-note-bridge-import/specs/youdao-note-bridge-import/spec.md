## ADDED Requirements

### Requirement: 有道笔记桥接导入
系统 SHALL 提供一个知识源，通过本地安装的有道笔记 CLI 桥接命令把笔记导出到临时目录，再按常规本地文件导入流程完成导入。

#### Scenario: 通过本地桥接成功导出并导入笔记
- **WHEN** 调用方配置了有道笔记桥接知识源，且本地 `youdaonote` 命令成功把笔记导出到指定临时目录
- **THEN** 库 MUST 通过与 `DirSource` 相同的本地文件导入流程导入这些导出文件

#### Scenario: 本地桥接命令缺失
- **WHEN** 调用方使用有道笔记桥接知识源，但配置的 `youdaonote` 命令不存在
- **THEN** 库 MUST 返回一个清晰的 unsupported-source 风格错误，并指出缺失的命令名

#### Scenario: 桥接导出命令执行失败
- **WHEN** 本地 `youdaonote` 导出命令执行失败并返回错误
- **THEN** 库 MUST 把该失败作为导入错误返回，且 MUST NOT 静默继续执行空导入
