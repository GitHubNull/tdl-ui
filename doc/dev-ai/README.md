# AI 编程代理维护文档

> [← 项目主页](../../README.md) | [人类开发者文档](../dev-human/README.md) | [根目录 agent.md](../../agent.md)

本系列面向维护 tdl UI 的 **AI 编程代理**，以**约束清单 + 任务配方**形式编写：先读约束避免破坏性修改，再按配方逐步执行常见任务。

## 阅读顺序

| 级别 | 文档 | 内容 |
| --- | --- | --- |
| 基础 | [项目约束与目录规范](basic/01-constraints.md) | 硬性约束清单、目录职责边界、验证命令 |
| 进阶 | [服务绑定与事件契约](intermediate/01-binding-events.md) | Wails 绑定面、事件负载契约、前后端类型同步规则 |
| 进阶 | [修改任务模式](intermediate/02-modification-patterns.md) | 改 UI / 改服务方法 / 改设置项 / 改脚本 API 的标准配方 |
| 高级 | [跨层功能新增流程](advanced/01-cross-layer-feature.md) | 从后端服务到前端页面的完整新增功能剧本 |
| 高级 | [升级迁移剧本](advanced/02-upgrade-playbook.md) | 上游子模块同步、依赖升级的逐步操作剧本 |

## 最小上下文加载建议

执行任务前按需读取，避免全仓库扫描：

- **任何任务**：`agent.md`（根目录，硬约束速览）
- **改后端**：`src/internal/` 对应包 + `src/main.go`（绑定注册）
- **改前端**：`src/frontend/src/api.ts` + `types.ts`（契约层）+ 目标页面/store
- **改脚本引擎**：`src/internal/script/engine.go` + `scriptapi/scriptapi.go` + `engine_test.go`
- **升级依赖**：本系列 [升级迁移剧本](advanced/02-upgrade-playbook.md)

## 完成任务的统一验收门槛

任何代码改动提交前必须全部通过：

```bash
cd src
go build ./...          # 后端编译
go test ./...           # 后端测试
cd frontend && pnpm build   # 前端类型检查 + 生产构建
```

涉及 UI 的改动还需 `wails dev` 启动后用浏览器工具（chrome-devtools MCP）目视验证。提交使用 smart-commit 技能，禁止手动 `git commit`。
