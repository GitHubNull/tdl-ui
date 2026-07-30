---
kind: external_dependency
name: Yaegi Go 脚本引擎
slug: yaegi-script-engine
category: external_dependency
category_hints:
    - sdk_real_api
scope:
    - '**'
---

### Yaegi 脚本沙箱实现
- **角色**：内置 Go 语言解释器，用于执行用户编写的下载过滤、重命名和自动化脚本
- **安全约束**：沙箱环境禁用危险包（如 `os/exec`），10 秒超时熔断保护
- **API 契约**：暴露 `Filter`、`Rename`、`OnTaskStart`、`OnFileDone`、`OnTaskDone` 等钩子函数
- **生命周期**：每个任务独立执行上下文，脚本错误不影响主程序稳定性
- **集成点**：`src/internal/script/` 目录下的脚本加载、编译和执行逻辑
- **版本锁定**：v0.16.1，由 `go.mod` 固定，升级需验证兼容性