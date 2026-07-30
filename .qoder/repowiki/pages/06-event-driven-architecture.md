# 事件驱动架构

## 概述

tdl UI 采用**事件驱动架构**实现前后端实时通信。后端通过 Wails `runtime.EventsEmit` 推送事件，前端通过 `EventsOn` 订阅。事件系统位于 `src/internal/events/`，统一封装了事件契约定义、发射器管理与前后端事件流。

核心文件：

| 文件 | 职责 |
|------|------|
| `src/internal/events/events.go` | 事件契约定义、Emitter 实现 |
| `src/frontend/src/api.ts` | 前端事件订阅封装 |
| `src/frontend/src/stores/*.ts` | Pinia Store 中的事件处理 |

## 事件契约定义

### 事件名称常量

```go
const (
    Login     = "login:update"  // 登录流程事件
    Task      = "task:update"   // 任务级状态事件
    TaskFile  = "task:file"     // 文件级进度事件
    ScriptLog = "script:log"    // 脚本日志事件
    Log       = "log:batch"     // 应用日志批量事件
)
```

### 事件负载类型

```go
// LoginUpdate —— 登录事件负载
type LoginUpdate struct {
    Stage string  // qr / need_code / need_password / success / error / logout
    QRURL string  // 二维码 URL（stage=qr 时）
    User  *User   // 登录成功后的账号信息
    Error string  // 错误信息（stage=error 时）
}

type User struct {
    ID       int64
    Username string
    Name     string
}
```

前端对应类型定义在 `src/frontend/src/types.ts`：

```typescript
export interface LoginUpdate {
  stage: 'qr' | 'need_code' | 'need_password' | 'success' | 'error' | 'logout'
  qrUrl?: string
  user?: LoginUser
  error?: string
}

export interface FileEvent {
  taskId: string
  fileId: number
  dialogId: number
  messageId: number
  name: string
  total: number
  downloaded: number
  state: 'downloading' | 'done' | 'failed'
  error?: string
  path?: string  // 仅 done 时携带
}
```

## 后端事件发送

### Emitter 实现

```go
type Emitter struct {
    mu  sync.RWMutex
    ctx context.Context  // Wails 运行时上下文
}

func (e *Emitter) Bind(ctx context.Context)    // OnStartup 中绑定
func (e *Emitter) Unbind()                     // OnShutdown 中解绑
func (e *Emitter) Emit(event string, payload any)  // 发射事件
func (e *Emitter) Ctx() context.Context        // 获取运行时上下文
```

**关键设计**：

- **启动前安全**：`OnStartup` 前调用 `Emit` 是安全的（静默丢弃），避免初始化阶段事件丢失导致 panic
- **关闭后安全**：`OnShutdown` 中调用 `Unbind()`，后续 `Emit` 恢复静默丢弃语义（SVC-19）
- **并发安全**：使用 `RWMutex` 保护 ctx 读写，Emit 时只读锁，Bind/Unbind 时写锁

### 各服务事件发射点

| 服务 | 事件 | 发射时机 |
|------|------|----------|
| AuthService | `login:update` | 登录阶段变化（qr/need_code/need_password/success/error/logout） |
| Task (engine) | `task:update` | 任务状态变化、计数更新 |
| progress (engine) | `task:file` | 文件下载进度（200ms 节流）、完成、失败 |
| script (engine) | `script:log` | 脚本 Filter 跳过、Rename 错误、钩子输出 |
| logging | `log:batch` | 日志批量推送（日志页实时更新） |

### 事件发射示例

```go
// AuthService —— 验证码登录需要用户输入
func (a *guiAuth) Code(ctx context.Context, _ *tg.AuthSentCode) (string, error) {
    a.svc.emitter.Emit(events.Login, events.LoginUpdate{Stage: "need_code"})
    select {
    case code := <-a.codeCh:
        return code, nil
    case <-ctx.Done():
        return "", ctx.Err()
    }
}

// Task —— 状态更新
func (t *Task) emitUpdate() {
    t.persistState()  // 先持久化
    t.mgr.deps.Emitter.Emit(events.Task, t.view())
}

// progress —— 文件进度（200ms 节流）
func (p *progress) OnDownload(elem downloader.Elem, state downloader.ProgressState) {
    // 节流逻辑...
    p.task.emitFile(p.buildEvent(e, state.Total, state.Downloaded, "downloading", nil))
}
```

## 前端事件订阅

### api.ts 封装

```typescript
export const EVENT_LOGIN = 'login:update'
export const EVENT_TASK = 'task:update'
export const EVENT_TASK_FILE = 'task:file'
export const EVENT_SCRIPT_LOG = 'script:log'
export const EVENT_LOG = 'log:batch'

export function on<T = unknown>(event: string, cb: (payload: T) => void): () => void {
  if (!window.runtime) return () => {}  // 非 Wails 环境守卫
  return EventsOn(event, (payload: T) => cb(payload))
}
```

- 返回取消函数，用于注销订阅
- 非 Wails 环境（vite dev）返回 no-op，避免报错

## Pinia Store 事件管理

### 订阅初始化模式

每个需要事件订阅的 store 在 `init()` 中完成订阅：

```typescript
// stores/auth.ts
async init() {
  if (this.inited) return
  this.inited = true
  unsubs.push(on<LoginUpdate>(EVENT_LOGIN, (u) => this.handleUpdate(u)))
  // ...
}
```

```typescript
// stores/tasks.ts
async init() {
  if (this.inited) return
  this.inited = true
  unsubs.push(on<TaskView>(EVENT_TASK, (t) => this.upsert(t)))
  unsubs.push(on<FileEvent>(EVENT_TASK_FILE, (f) => this.onFile(f)))
  await this.refresh()
}
```

### App.vue 统一调用

```typescript
onMounted(async () => {
  await Promise.all([
    useAuthStore().init(),
    useTasksStore().init(),
    useScriptsStore().init(),
    useSettingsStore().init(),
    useLogsStore().init(),
  ])
})
```

### 事件处理逻辑

**AuthStore —— 登录流程状态机**：

```typescript
handleUpdate(u: LoginUpdate) {
  clearTimeout(pendingTimer)  // 解除 pending 超时兜底
  switch (u.stage) {
    case 'qr':
      this.stage = 'qr'
      this.qrUrl = u.qrUrl ?? ''
      break
    case 'need_code':
    case 'need_password':
      this.stage = u.stage
      break
    case 'success':
      this.stage = 'idle'
      this.applyUser(u.user)
      break
    case 'error':
      this.stage = 'error'
      this.error = u.error ?? '未知错误'
      break
    case 'logout':
      this.loggedIn = false
      this.userId = 0
      this.username = ''
      break
  }
}
```

**TasksStore —— 任务与文件进度**：

```typescript
upsert(t: TaskView) {
  const i = this.tasks.findIndex((x) => x.id === t.id)
  if (i >= 0) {
    this.tasks[i] = t
  } else {
    this.tasks.unshift(t)
  }
  // 终态任务清理文件进度
  if (['done', 'failed', 'canceled', 'paused'].includes(t.status)) {
    delete this.files[t.id]
  }
}

onFile(f: FileEvent) {
  if (f.state === 'done') {
    delete this.files[f.taskId]?.[f.fileId]
    return
  }
  if (!this.files[f.taskId]) this.files[f.taskId] = {}
  this.files[f.taskId][f.fileId] = f
}
```

## 事件流时序图

### 验证码登录流程

```
用户 ──点击「验证码登录」──> LoginPage
                              │
                              ▼
                         startCodeLogin(phone)
                              │
                              ▼
前端 ──StartCodeLogin()──> AuthService
                              │
                              ▼
                         begin() → gen=1, 启动 goroutine
                              │
                              ▼
                         runCodeLogin(flow, phone)
                              │
                              ▼
                         Emit(login:update, {stage:"need_code"})
                              │
                              ▼
前端 <──EVENT_LOGIN─────── 接收事件
  │                           │
  ▼                           │
显示验证码输入框               │
  │                           │
  ▼                           │
用户输入验证码 ──SubmitCode()──> AuthService
  │                           │
  ▼                           │
写入 codeCh ────────────────> guiAuth.Code() 返回
  │                           │
  ▼                           │
登录成功                      │
  │                           │
  ▼                           ▼
Emit(login:update, {stage:"success", user})
  │
  ▼
前端更新登录状态 → 跳转对话页
```

### 下载任务进度流

```
引擎 ──execute()──> downloader.Download()
                         │
                         ▼
                    progress.OnAdd() → addFile() → emitFile()
                         │
                         ▼
                    progress.OnDownload() → [200ms 节流] → emitFile()
                         │
                         ▼
                    progress.OnDone() → finishFile() → onFileDone() → emitFile()
                         │                                              │
                         ▼                                              ▼
                    task:file 事件流 ──────────────────────────────> 前端 TasksStore
                         │                                              │
                         ▼                                              ▼
                    emitUpdate() → task:update 事件 ──────────────> 更新任务状态
```

## 内存泄漏防护

### HMR 安全注销

```typescript
const unsubs: Array<() => void> = []
import.meta.hot?.dispose(() => {
  unsubs.splice(0).forEach((off) => off())
})
```

开发模式下模块热替换时，旧模块的事件回调被注销，避免双触发。

### Store 单例初始化

```typescript
async init() {
  if (this.inited) return  // 防止重复订阅
  this.inited = true
  // ...
}
```

每个 store 的 `init()` 通过 `inited` 标志确保只订阅一次。

### 后端事件丢弃

```go
func (e *Emitter) Emit(event string, payload any) {
    e.mu.RLock()
    ctx := e.ctx
    e.mu.RUnlock()
    if ctx == nil {
        return  // 未绑定或已解绑，静默丢弃
    }
    runtime.EventsEmit(ctx, event, payload)
}
```

应用启动前或关闭后的事件发射不会 panic，也不会累积内存。
