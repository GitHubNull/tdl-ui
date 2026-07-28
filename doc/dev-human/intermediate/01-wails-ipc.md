# Wails 前后端通信机制

> [← 开发者文档目录](../README.md) | [项目主页](../../../README.md)

tdl UI 的前后端通过 Wails v2 的两条通道通信：**方法绑定**（前端 → 后端，请求/响应）与**事件**（后端 → 前端，单向推送）。

## 1. 方法绑定（Bind）

`src/main.go` 中注册五个服务：

```go
Bind: []interface{}{ authSvc, chatSvc, downloadSvc, scriptSvc, settingsSvc },
```

Wails 将服务的**导出方法**暴露为 `window.go.services.<服务名>.<方法名>`，返回 Promise。前端统一通过 `src/frontend/src/api.ts` 封装调用，**页面组件不允许直接触碰 `window.go`**：

```ts
// api.ts
export const Chat = {
  listDialogs: (): Promise<DialogView[]> => svc('ChatService').ListDialogs(),
  listMedia: (q: MediaQuery): Promise<MediaPage> => svc('ChatService').ListMedia(q),
  getThumbnail: (dialogId: number, dialogType: string, messageId: number): Promise<string> =>
    svc('ChatService').GetThumbnail(dialogId, dialogType, messageId),
}
```

### 类型契约同步（重要）

Go 结构体经 JSON 序列化传给前端，`src/frontend/src/types.ts` 手工维护对应的 TS 接口。**修改 Go 结构体字段或 JSON tag 后必须同步更新 types.ts**，否则前端将静默读到 `undefined`。

对应关系一览：

| Go | TS（types.ts） |
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

## 2. 事件系统（Events）

后端通过 `internal/events.Emitter`（封装 `runtime.EventsEmit`）推送，前端用 `api.ts` 的 `on()`（封装 `window.runtime.EventsOn`）订阅。事件名与负载契约集中定义在 `internal/events/events.go`：

| 事件 | 负载 | 用途 |
| --- | --- | --- |
| `login:update` | `LoginUpdate{stage, qrUrl?, user?, error?}` | 登录流程状态机 |
| `task:update` | `TaskView` | 任务级状态/计数变化 |
| `task:file` | `FileEvent`（200ms 节流） | 单文件下载进度 |
| `script:log` | string | 脚本 `api.Log` 输出 |

前端约定：**事件订阅统一放在 Pinia store 的 `init()` 中**，由 `App.vue` 的 `onMounted` 一次性调用，避免组件卸载导致漏订阅。

## 3. 登录流程的双向交互

登录需要"后端等待用户输入"（验证码/密码），实现为 **事件 + channel**：

```
前端                          后端 (AuthService)
StartCodeLogin(phone) ──────► 启动 goroutine，走 gotd auth flow
        ◄── login:update{stage:need_code} ── guiAuth.Code() 阻塞在 codeCh
SubmitCode(code) ───────────► codeCh <- code，flow 继续
        ◄── login:update{stage:success}  ── 登录完成
```

二维码登录同理：`qr` 阶段推送 `qrUrl`，前端用 `qrcode` 库渲染 canvas；token 过期时后端会推送新 URL。

## 4. ChatService 的常驻连接

`ChatService` 维护一个常驻 Telegram 连接（懒启动、断线自动重建），所有查询任务经 `jobs` 通道串行化执行：

- `ListDialogs()`：拉取全部对话列表，缓存 access hash 到 bolt
- `ListMedia(q)`：游标分页查询对话内媒体，支持服务端过滤（photo/video/document）
- `GetThumbnail(...)`：按需下载清晰缩略图，内存缓存（LRU 上限 500 条）

登出时调用 `chatSvc.Stop()` 关闭连接，下次查询自动重建。

## 5. 运行时上下文

`Emitter.Bind(ctx)` 在 `OnStartup` 中保存 Wails 运行时 ctx；需要调用运行时 API（如 `runtime.OpenDirectoryDialog`）的服务通过 `emitter.Ctx()` 获取。启动前调用 `Emit` 是安全的（静默丢弃）。

## 下一步

→ [下载引擎装配](02-download-engine.md)
