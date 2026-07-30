# Wails 服务层 API 契约

## 概述

Wails 服务层位于 `src/internal/services/`，是 Go 后端暴露给前端 Vue 调用的唯一入口。每个服务对应一个业务领域，方法经 Wails 绑定生成后通过 `window.go.services.*` 在前端调用。服务层本身不包含复杂业务逻辑，而是将调用委托给下层引擎/存储模块，并负责参数校验、日志记录与错误包装。

五大服务：

| 服务 | 文件 | 职责 |
|------|------|------|
| AuthService | `auth.go` | 账号登录（验证码/二维码/Desktop 导入）与登出 |
| ChatService | `chat.go` | 对话列表与媒体文件查询 |
| DownloadService | `download.go` | 下载任务管理（CRUD + 状态控制） |
| ScriptService | `script.go` | 用户脚本 CRUD、校验与试运行 |
| SettingsService | `settings.go` | 应用设置读写与缓存清理 |

此外还有辅助服务：LogService（`logsvc.go`）、缩略图服务（`thumb.go` / `thumbcache.go`）。

## AuthService

**位置**：`src/internal/services/auth.go`（611 行）

### 公开方法

| 方法 | 签名 | 说明 |
|------|------|------|
| Status | `() LoginStatus` | 返回本地记录的登录状态（不发起网络请求） |
| StartCodeLogin | `(phone string) error` | 启动验证码登录流程 |
| StartQRLogin | `() error` | 启动二维码登录流程 |
| SubmitCode | `(code string)` | 提交收到的验证码（通道写入） |
| SubmitPassword | `(pwd string)` | 提交二步验证密码（通道写入） |
| CancelLogin | `()` | 取消进行中的登录流程 |
| Logout | `() error` | 清除本地会话数据 |
| DetectDesktopPath | `() string` | 自动探测 Telegram Desktop 数据目录 |
| ListDesktopAccounts | `(path, passcode string) ([]DesktopAccount, error)` | 读取 Desktop 会话中的账号列表 |
| ImportDesktopSession | `(path, passcode, userID string) error` | 导入指定 Desktop 会话 |

### 登录流程架构

AuthService 采用**代际（generation）模型**管理并发登录流程：

- 每次 `begin()` 递增 `gen` 计数，创建新的 `loginFlow`（含独立 ctx、codeCh、pwdCh）
- `finish(gen)` 仅当 `gen` 匹配当前代际时才清理 cancel，避免旧流程异步收尾取消新流程
- `isCurrent(gen)` 在事件发送前判定，防止旧流程事件污染新流程 UI

验证码登录与二维码登录分别对应 `runCodeLogin()` 和 `runQRLogin()`，两者均异步在 goroutine 中执行，通过 `login:update` 事件向前端推送阶段变化。

### 安全设计

- **活跃下载检查**：`prepareLogin()` / `Logout()` 通过注入的 `HasActiveDownloads` 回调检查是否存在进行中的下载任务，避免会话重写与下载连接互踩
- **Desktop 导入回滚**：导入前先备份现有会话与 App 标记，校验失败时恢复备份，避免假登录态
- **会话校验**：`verifySession()` 建连后查询授权状态，带 30 秒超时

## ChatService

**位置**：`src/internal/services/chat.go`（986 行）

### 公开方法

| 方法 | 签名 | 说明 |
|------|------|------|
| ListDialogs | `() ([]Dialog, error)` | 返回当前账号全部对话 |
| ListMedia | `(q MediaQuery) (*MediaPage, error)` | 游标分页查询对话内媒体文件 |
| ClearThumbCache | `() error` | 清空缩略图/预览图磁盘缓存 |
| Stop / StopAndWait | `()` / `(timeout)` | 关闭常驻连接 |

### 常驻连接架构

ChatService 维护一个**常驻 Telegram 客户端连接**，采用三重策略优化性能与稳定性：

1. **懒启动**：首次查询时 `ensureStarted()` 建连，后续查询复用同一连接
2. **断线重建**：连接意外断开后，下次查询自动重新建连
3. **双队列串行化**：
   - `jobs` 通道：列表/媒体查询单 worker 串行执行
   - `thumbJobs` 通道：缩略图/预览图 2 个 worker 并发，避免单张大图阻塞列表查询（ARC-04）

### 媒体查询

`MediaQuery` 支持多维过滤：

- **类型过滤**：`kinds`（video/photo/audio/file），能映射到服务端过滤器的走 `messages.search`，否则回退 `GetHistory`
- **扩展名过滤**：`exts`（小写不含点）
- **大小范围**：`minSize` / `maxSize`（字节）
- **关键词**：`query`（文件名 + caption 包含匹配，不区分大小写）
- **月份跳转**：`offsetDate`（unix 秒）+ `offsetID=0` 时从该日期附近开始

`scanMedia()` 单次调用最多扫描 2000 条消息（`maxScan`），防止苛刻过滤条件下调用过久。

## DownloadService

**位置**：`src/internal/services/download.go`（169 行）

### 公开方法

| 方法 | 签名 | 说明 |
|------|------|------|
| CreateTask | `(opts TaskOptions) (string, error)` | 创建并启动下载任务 |
| ListTasks | `() []TaskView` | 返回全部任务（倒序） |
| PauseTask | `(id string) error` | 暂停任务 |
| ResumeTask | `(id string) error` | 恢复任务（断点续传） |
| CancelTask | `(id string) error` | 取消任务 |
| RemoveTask | `(id string) error` | 移除终态任务 |
| ListTaskFiles | `(id string) ([]TaskFile, error)` | 返回任务内文件列表 |
| ClearFinishedTasks | `() error` | 批量移除已完成任务 |
| DeleteTaskFiles | `(id string, paths []string) error` | 删除任务内指定文件 |
| DeleteAllFiles | `() error` | 删除全部文件与任务记录 |
| OpenTaskDir | `(id string) error` | 在文件管理器中打开目录 |
| SelectDirectory | `() (string, error)` | 弹出系统目录选择框 |
| ListDownloadedMessages | `(dialogID int64) ([]DownloadedFile, error)` | 对话内已下载消息列表 |
| OpenDownloadedFile | `(dialogID int64, messageID int) error` | 用系统默认程序打开文件 |

DownloadService 是 `engine.Manager` 的薄封装层，所有方法直接委托给 Manager，仅增加日志记录与错误包装。

## ScriptService

**位置**：`src/internal/services/script.go`（157 行）

### 公开方法

| 方法 | 签名 | 说明 |
|------|------|------|
| List | `() ([]script.Meta, error)` | 列出全部脚本 |
| Read | `(name string) (string, error)` | 读取脚本源码 |
| Save | `(name, src string) error` | 保存脚本源码 |
| Delete | `(name string) error` | 删除脚本 |
| Validate | `(src string) ValidateResult` | 编译校验并列出检测到的契约函数 |
| TestRun | `(src string) TestRunResult` | 用内置示例文件试运行 Filter/Rename |
| StarterTemplate | `() string` | 返回新建脚本的起始模板 |

ScriptService 委托 `script.Store` 进行文件 CRUD，委托 `script.Load()` 进行编译与契约提取。

## SettingsService

**位置**：`src/internal/services/settings.go`（49 行）

### 公开方法

| 方法 | 签名 | 说明 |
|------|------|------|
| Get | `() config.Settings` | 获取当前设置 |
| Save | `(settings config.Settings) error` | 保存设置，日志配置热生效 |
| DataDir | `() string` | 返回应用数据目录 |
| ClearCache | `() error` | 清空缩略图缓存 |

设置持久化由 `config.Manager` 负责（`%AppData%\tdl-ui\settings.json`）。

## 错误处理统一模式

服务层错误处理遵循以下约定：

1. **参数校验错误**：直接返回 `errors.New("...")`，前端显示为 Toast 提示
2. **业务状态错误**：如「存在进行中的下载任务，请先暂停或取消」，返回明确的用户可读错误
3. **底层错误**：使用 `errors.Wrap(err, "...")` 添加上下文，日志记录完整错误链
4. **超时错误**：统一转换为「连接 Telegram 超时，请检查网络或在「设置」中配置代理」
5. **静默忽略**：非 Wails 环境（vite dev 独立预览）的调用失败由前端 try/catch 兜底

## 前端 api.ts 映射表

`src/frontend/src/api.ts` 是前后端调用的唯一通道：

```typescript
// 服务分组
export const Auth = { status, startCodeLogin, startQRLogin, ... }
export const Download = { createTask, listTasks, pauseTask, ... }
export const Script = { list, read, save, remove, validate, testRun, starterTemplate }
export const SettingsApi = { get, save, dataDir, clearCache }
export const Chat = { listDialogs, listMedia }
export const LogApi = { getRecent, listLogFiles, readLogFile, ... }

// 事件订阅
export function on<T>(event: string, cb: (payload: T) => void): () => void

// 媒体图 URL 生成
export function thumbURL(dialogId, dialogType, messageId): string
export function previewURL(dialogId, dialogType, messageId): string
```

Wails 绑定生成后，Go 方法自动映射为 `window.go.services.*` 调用，api.ts 做了一层命名空间封装以保持一致性。
