# 修改任务模式

> [← AI 代理文档目录](../README.md) | [项目主页](../../../README.md)

常见修改任务的标准配方。每个配方 = 触碰文件清单 + 步骤 + 验证方式。

## 配方 A：改 UI（样式/布局/文案）

**触碰**：`src/frontend/src/pages/*.vue`、`App.vue`、`style.css`

1. 定位目标页面（导航项与路由见 `router.ts` 的 `meta`）
2. 保持三段式结构与既有设计规范：8px 间距栅格、圆角卡片、单一品牌蓝主色、亮/暗双主题均可读
3. 颜色一律使用 PrimeVue 语义 token（`var(--p-primary-color)`、`var(--p-surface-*)`、`var(--p-text-color)` 等），**禁止硬编码 hex**，否则暗黑模式会破相
4. `ChatsPage.vue` 为双面板复杂布局（左对话列表 + 右媒体网格），改动时注意保持 `split-body` 结构
5. 验证：`pnpm build` 通过 → `wails dev` + 浏览器工具截图核对亮/暗两种主题

## 配方 B：给服务加方法（前端可调用的新能力）

**触碰**：`src/internal/services/<对应>.go` → `api.ts` →（可选）`types.ts` → store/页面

按 [服务绑定与事件契约](01-binding-events.md) 的"新增绑定方法配方"执行。注意：

- 长耗时操作放 goroutine + 事件推送结果，绑定方法立即返回，避免阻塞前端 Promise
- 需要 Wails 运行时 API（如目录选择对话框）时通过 `emitter.Ctx()` 取 ctx
- `ChatService` 的方法需通过 `invoke()` 投递到常驻客户端执行，避免并发问题

## 配方 C：加设置项

**触碰**：`config/config.go` → `services/settings.go`（如需校验） → `types.ts` → `stores/settings.ts` → `SettingsPage.vue`

1. `config.Settings` 加字段（含 JSON tag 与 `defaultSettings()` 默认值）
2. `types.ts` 的 `Settings` 接口同步加字段
3. `stores/settings.ts` 若有局部编辑副本则同步
4. `SettingsPage.vue` 添加控件（复用现有 PrimeVue 组件：InputText/InputNumber/Select/SelectButton）
5. 若设置影响下载引擎（线程数、代理等），检查 `engine/task.go` 装配处是否读取该字段
6. 验证：`go build` + `pnpm build` + `wails dev` 中修改并保存设置，重启确认持久化（设置存于用户数据目录 `config.yaml`）

## 配方 D：扩展脚本 API / 契约函数

**触碰**：`scriptapi/scriptapi.go` → `script/engine.go` → `services/script.go`（模板与校验清单） → `script/engine_test.go` → 教程文档

详细步骤见 [dev-human：Yaegi 脚本引擎扩展](../../dev-human/advanced/01-yaegi-extend.md)。AI 代理额外注意：

- 新契约函数必须经 `callWithGuard` 包裹（panic 恢复 + 10 秒超时）
- 空脚本（无任何契约函数）`Load` 报错是**设计行为**，不要"修复"它
- `engine_test.go` 必须新增覆盖用例并全量通过

## 配方 E：改任务状态机 / 进度推送

**触碰**：`engine/task.go`（状态机）、`engine/progress.go`（节流推送）、`types.ts`、`stores/tasks.ts`、`TasksPage.vue`

约束：

- 状态取值：`queued/running/paused/done/failed/canceled`；新增状态需同步前端徽标颜色映射与终态清理逻辑（`stores/tasks.ts` 在终态删除 files 明细）
- 暂停实现 = cancel + 保留断点（fingerprint 与 tdl CLI 互通）；恢复 = 重建任务 + `skipSame`/resume key 续传。改动此逻辑前先读 `engine/` 四个文件全文
- `task:file` 事件保持 200ms 节流，禁止逐 chunk 推送

## 配方 F：改文档

1. 行为变更后定位受影响篇目：用户可见变化 → `doc/tutorials/`；架构/接口变化 → `doc/dev-human/` + 本系列；约束变化 → `AGENTS.md` + `basic/01-constraints.md`
2. 保持每篇文档头部的返回链接格式与相对路径正确
3. 中英 README 同步修改（`README.md` / `README_EN.md`）

## 下一步

→ [跨层功能新增流程](../advanced/01-cross-layer-feature.md)
