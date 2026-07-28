# 服务绑定与事件契约

> [← AI 代理文档目录](../README.md) | [项目主页](../../../README.md)

本文是修改前后端交互面时的**契约对照表**。改动任何一侧都必须按表同步另一侧。

## 绑定面：五个服务

`src/main.go` 中 `Bind` 注册，前端经 `window.go.services.<服务名>.<方法>` 访问（统一封装在 `src/frontend/src/api.ts`）：

| Go 服务（src/internal/services/） | api.ts 导出 | 职责 |
| --- | --- | --- |
| `AuthService`（auth.go） | `Auth` | 验证码/二维码/Desktop 导入登录、登出、状态查询 |
| `ChatService`（chat.go/thumb.go） | `Chat` | 对话列表、媒体分页查询、缩略图下载 |
| `DownloadService`（download.go） | `Download` | 任务创建/暂停/恢复/取消/移除、目录选择 |
| `ScriptService`（script.go） | `Script` | 脚本 CRUD、语法校验、试运行、起始模板 |
| `SettingsService`（settings.go） | `SettingsApi` | 设置读写、数据目录 |

## 事件契约（后端 → 前端）

事件名常量：后端 `src/internal/events/events.go`，前端 `api.ts`。**两处必须逐字一致**：

| 事件 | 负载（Go → TS） | 前端订阅位置 |
| --- | --- | --- |
| `login:update` | `LoginUpdate{stage, qrUrl?, user?, error?}` | `stores/auth.ts` 的 `init()` |
| `task:update` | `TaskView` | `stores/tasks.ts` 的 `init()` |
| `task:file` | `FileEvent`（后端 200ms 节流） | `stores/tasks.ts` 的 `init()` |
| `script:log` | `string` | `stores/scripts.ts` 的 `init()` |

`login:update.stage` 状态机取值：`qr` / `need_code` / `need_password` / `success` / `error` / `logout`。新增 stage 时需同步 `types.ts` 联合类型与 `LoginPage.vue` 分支。

## 类型同步规则（强制）

Go 结构体 ↔ `src/frontend/src/types.ts` 手工同步，对应关系：

| Go | TS |
| --- | --- |
| `config.Settings` | `Settings` |
| `services.LoginStatus` | `LoginStatus` |
| `services.Dialog` | `DialogView` |
| `services.MediaItem` | `MediaItem` |
| `services.MediaQuery` | `MediaQuery` |
| `engine.TaskOptions` / `TaskView` | `TaskOptions` / `TaskView` |
| `engine.FileEvent` | `FileEvent` |
| `engine.Selection` | `Selection` |
| `script.Meta` | `ScriptMeta` |
| `services.ValidateResult` / `TestRunResult` | 同名 |
| `services.DesktopAccount` | `DesktopAccount` |

同步检查配方：

1. 改 Go 字段后，Grep 该结构体所有 JSON tag
2. 对照更新 `types.ts` 中同名接口（字段名 = JSON tag，可选性 = `omitempty`）
3. `pnpm build` —— TS 编译会捕获前端引用处的类型错误（但**捕获不了**后端多发/少发字段，需人工核对）

## 新增绑定方法配方

1. 在对应 service 文件添加导出方法（首字母大写；参数/返回值必须可 JSON 序列化；错误用 `error` 返回，Wails 自动转 Promise reject）
2. `api.ts` 对应对象添加封装方法，签名用 `types.ts` 类型
3. 调用方（store 或页面）通过 api.ts 调用，**禁止直接触碰 `window.go`**
4. 验证：`go build ./...` + `pnpm build` + `wails dev` 实测

## 新增事件配方

1. `events/events.go` 定义常量与负载结构
2. 后端在业务处 `emitter.Emit(events.XXX, payload)`
3. `api.ts` 添加事件名常量；`types.ts` 添加负载接口
4. 对应 store 的 `init()` 中 `on<T>(EVT_XXX, handler)` 订阅
5. 高频事件（进度类）必须在后端节流（参考 `engine/progress.go` 的 200ms 模式）

## 下一步

→ [修改任务模式](02-modification-patterns.md)
