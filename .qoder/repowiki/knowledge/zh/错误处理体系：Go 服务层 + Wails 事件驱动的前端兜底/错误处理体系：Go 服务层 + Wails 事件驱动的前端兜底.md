---
kind: error_handling
name: 错误处理体系：Go 服务层 + Wails 事件驱动的前端兜底
category: error_handling
scope:
    - '**'
source_files:
    - src/internal/services/auth.go
    - src/internal/services/download.go
    - src/internal/services/helpers.go
    - src/internal/logging/logger.go
    - src/main.go
    - src/frontend/src/api.ts
    - src/frontend/src/stores/tasks.ts
---

本仓库的错误处理采用 Go 后端 github.com/go-faster/errors 统一包装、Wails 服务方法返回 error 的传统模式，配合前端 try/catch 与事件通道进行用户可见错误的呈现。整体分为三层：后端业务错误定义与传播、日志与结构化错误输出、前端 UI 层的兜底展示。

## 1. 使用的系统与框架
- Go 错误库：github.com/go-faster/errors 提供 errors.New、errors.Wrap、errors.Errorf、errors.Is 等 API，用于构造可携带上下文的错误并做类型判断（如 context.Canceled、storage.ErrNotFound）。
- Wails v2 绑定：所有暴露给前端的 Go 方法均通过 Wails 自动绑定生成 TypeScript 调用，错误以 JS Promise reject 形式返回，由前端 try/catch 捕获。
- 日志系统：internal/logging 基于 go.uber.org/zap，支持动态级别调整、文件滚动、环形缓冲快照，并通过事件将日志批量推送到前端 LogsPage。

## 2. 核心文件与位置
- src/internal/services/*.go：所有 Wails 服务（auth、download、chat、script、settings、log）集中实现错误返回与包装。
- src/internal/logging/logger.go：全局 zap logger 初始化、热更新与关闭。
- src/main.go：应用启动/退出时的错误处理入口（配置、存储、Wails 运行失败均 log.Fatalf）。
- src/frontend/src/api.ts：封装 wailsjs 生成的绑定，注释明确非 Wails 环境调用会抛错并由页面层 try/catch 兜底。
- src/frontend/src/stores/*.ts：Pinia store 中对异步调用统一 try/catch，忽略非 Wails 环境的异常。

## 3. 架构与约定
- 服务层错误传播：每个 Service 方法直接返回 error，内部使用 errors.Wrap(err, "上下文说明") 保留堆栈信息，关键路径记录 logAuth.Errorf(...) 便于排查。
- 登录流程特殊处理：AuthService 中验证码/二维码登录通过 goroutine 异步执行，错误仅在 isCurrent(gen) 且非 context.Canceled 时通过 events.Login 事件推送 stage=error 及 Error 字段到前端。
- 会话导入回滚：ImportDesktopSession 在写入新会话前先备份旧数据，校验失败后调用 restoreSession 恢复原状态，避免假登录态。
- 资源清理顺序：main.go 的 OnShutdown 严格按“解绑事件 → 停止任务 → 关闭存储 → 关闭日志”顺序，避免“database is closed”刷屏。
- 前端兜底策略：api.ts 注释声明“纯浏览器环境下调用会抛错，由页面层 try/catch 兜底提示”，各 store 对 Download.listTasks() 等调用包裹 try/catch 忽略异常。

## 4. 约定与约束
- 错误包装规范：所有业务错误必须用 errors.Wrap 或 errors.Errorf 包装，禁止裸 fmt.Errorf（services 包内未见直接使用）。
- 上下文错误识别：通过 errors.Is(err, context.Canceled) 区分用户取消与真实错误，取消错误不向用户展示。
- Telegram 协议错误：使用 github.com/gotd/td/tgerr 的 tgerr.Is 判断特定 Telegram 错误码（如 SESSION_PASSWORD_NEEDED），走差异化分支。
- 不可恢复错误：main.go 中配置初始化、存储打开、Wails 运行失败直接 log.Fatalf 终止进程，不尝试恢复。
- 并发安全：登录流程通过代际计数 gen 与 sync.Mutex 保护，确保旧流程不会污染新流程的 UI 状态。
- 子进程回收：openDirectory/openFile 中使用 go func() { _ = cmd.Wait() }() 避免 Unix 系 zombie 进程泄漏。

## 5. 前端错误呈现
- 登录错误通过 EVENT_LOGIN 事件的 LoginUpdate.Error 字段显示。
- 下载任务状态变更通过 EVENT_TASK 事件推送，store 根据 status 字段更新 UI。
- 日志错误通过 EVENT_LOG 批量事件渲染到 LogsPage，支持按级别过滤。
- 非 Wails 环境（Vite 独立预览）下的 API 调用被显式忽略，避免开发时报错干扰。

该体系在 Go 侧保持强类型错误传播，在前端侧通过事件与 try/catch 双重保障，既保证调试时可追溯，又确保用户可见错误友好。