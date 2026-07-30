# 下载引擎状态机

## 概述

下载引擎位于 `src/internal/engine/`，是 tdl UI 的核心模块，负责管理下载任务的完整生命周期。它复用了 tdl CLI 的 `core/downloader` + `core/dcpool` 下载能力，在其之上封装了任务状态机、断点续传、选集下载、脚本过滤/命名等 GUI 特有的功能。

引擎对外暴露的入口是 `Manager`（`manager.go`），内部由 `Task`（`task.go`）表示单次下载任务，`iter`（`iter.go`）驱动消息到下载元素的迭代，`progress`（`progress.go`）将下载进度转化为前端事件。

## 状态机设计

任务具有六种互斥状态：

```
queued → running ↔ paused → done / failed / canceled
         ↑___________|
```

| 状态 | 常量 | 语义 |
|------|------|------|
| 排队中 | `StatusQueued` | 已创建，等待 `run()` 调度 |
| 运行中 | `StatusRunning` | `execute()` 正在执行下载 |
| 已暂停 | `StatusPaused` | 用户暂停或应用关闭，进度保留，可恢复 |
| 已完成 | `StatusDone` | 全部文件下载成功 |
| 已失败 | `StatusFailed` | 执行过程中发生错误 |
| 已取消 | `StatusCanceled` | 用户取消 |

**状态迁移规则：**

- `Create()` → `queued` → 异步 `run()` → `running`
- `Pause()`：`running` → 取消 ctx → `paused`；`queued` → 直接 `paused`
- `Resume()`：`paused/failed/canceled` → `queued` → 重新 `run()`
- `Cancel()`：`running` → 取消 ctx → `canceled`；`queued` → 直接 `canceled`
- `run()` 结束：成功 → `done`；ctx 取消且 `paused=true` → `paused`；否则 → `failed`

**并发安全：** `Manager` 持有 `sync.Mutex` 保护任务映射表与顺序切片；每个 `Task` 独立持有 `sync.Mutex` 保护自身状态字段。`removed` 标志（ENG-03）与状态判定在同一临界区，阻断并发 `Resume` 复活已移除任务的僵尸下载。

## 断点续传机制

断点以**内容坐标**（`dialogID:messageID`）为 key，存储于 SQLite `resume_keys` 表：

1. **创建时**：`execute()` 从 `store.LoadFinished()` 加载已完成集合，调用 `iter.SetFinished()` 标记
2. **运行时**：每完成一个文件，`progress.OnDone()` 调用 `task.saveResumeKey()` 行级追加（ENG-12：O(1) 写入，替代旧版重写整个集合）
3. **暂停/失败时**：`defer` 中调用 `store.SaveFinished()` 持久化当前断点集合
4. **恢复时**：`Resume()` 设置 `opts.Restart = false`，重新加载断点；用户勾选「重新开始」则 `Restart = true`，清空断点
5. **完成时**：`store.DeleteResume()` 清理断点记录

**关键设计**：断点写入发生在任务 ctx 可能已取消的阶段，因此所有 store 写操作统一使用 `context.Background()`（STO-03），避免 ctx 取消导致断点丢失。

## 选集下载逻辑

选集下载（`TaskOptions.Selections`）允许用户从对话列表直接勾选消息创建下载任务，无需粘贴链接。

- `Selection` 结构：`DialogID` + `DialogType` + `MessageIDs` 数组
- `execute()` 中，`selectionsToDialogs()` 将选集解析为 `tmessage.Dialog`，与 URL 解析结果合并
- 持久化时，`optionsToItems()` 将选集拆分为 `store.ItemTypeSelection` 行存入 `task_items` 表
- 恢复时，`itemsToOptions()` 从 `task_items` 重组 URLs/Selections，保证与原始输入一致

选集与链接可混用：同一任务中同时提供 `URLs` 和 `Selections` 时，引擎会合并两者的对话结果。

## 与 tdl 引擎的复用关系

| 组件 | tdl CLI 来源 | 本引擎改动 |
|------|-------------|-----------|
| 下载执行 | `ref/tdl/core/downloader` | 无改动，直接复用 |
| 连接池 | `ref/tdl/core/dcpool` | 无改动，直接复用 |
| 客户端 | `ref/tdl/pkg/tclient` | 无改动，直接复用 |
| 消息解析 | `ref/tdl/pkg/tmessage` | 无改动，直接复用 |
| 迭代器 | `ref/tdl/app/dl/iter.go` | 重写为 `engine/iter.go`，去除 include/exclude，由脚本 Filter 接管 |
| 进度回调 | `ref/tdl/app/dl/progress.go` | 重写为 `engine/progress.go`，进度条替换为事件发射 |
| 任务管理 | `ref/tdl/app/dl/dl.go` | 重写为 `engine/manager.go` + `task.go`，增加状态机与持久化 |

**复用原则**：网络通信、媒体解析、文件下载等底层能力完全复用 tdl；状态管理、用户交互、持久化、脚本集成在 `src/internal/engine/` 中重新实现。

## 关键数据结构

```go
// TaskOptions —— 创建任务的参数
type TaskOptions struct {
    URLs       []string    // 消息链接
    Selections []Selection // 对话选集
    Label      string      // 任务显示名
    Dir        string      // 保存目录
    ScriptName string      // 关联脚本名
    Template   string      // 文件名模板
    RewriteExt bool        // 按 MIME 重写扩展名
    SkipSame   bool        // 跳过同名同大小文件
    Group      bool        // 展开相册分组
    Restart    bool        // 是否重新开始（清空断点）
}

// TaskView —— 前端展示/事件负载
type TaskView struct {
    ID         string   // 任务 ID（时间戳+随机后缀，ENG-11）
    URLs       []string
    Label      string
    Dir        string
    ScriptName string
    Status     string   // 六态之一
    Error      string
    Total      int      // 文件总数
    Finished   int      // 已完成数
    Failed     int      // 失败数
    FileCount  int      // 登记文件数
    CreatedAt  string
}

// TaskFile —— 任务内单个文件记录
type TaskFile struct {
    Name      string // 文件名
    Path      string // .tmp 或最终路径
    Size      int64
    State     string // downloading / done / failed
    DialogID  int64  // 来源对话（已下载标记用）
    MessageID int    // 来源消息（已下载标记用）
}
```

## 时序图

### 任务创建与执行

```
前端 ──CreateTask()──> DownloadService ──Create()──> Manager
                                                      │
                                                      ▼
                                              [SQLite] InsertTask
                                                      │
                                                      ▼
                                                   启动 goroutine
                                                      │
                                                      ▼
                                                    run(t)
                                                      │
                                    ┌─────────────────┼─────────────────┐
                                    ▼                 ▼                 ▼
                              [StatusRunning]    [StatusPaused]    [StatusDone]
                                    │                 │                 │
                                    ▼                 ▼                 ▼
                              execute(ctx,t)     ctx.Cancel()      清理断点
                                    │                 │            emitUpdate
                                    ▼                 ▼
                              建连→解析→迭代→下载   emitUpdate      SafeOnTaskDone
                                    │
                                    ▼
                              task:file 事件流
```

### 断点续传

```
Resume(id)
  │
  ▼
加载脚本契约（优先源码快照 SCR-04）
  │
  ▼
status = queued → 启动 run()
  │
  ▼
execute() ──LoadFinished()──> SQLite resume_keys
  │                              │
  ▼                              ▼
iter.SetFinished(keys)      断点集合
  │
  ▼
iter.Next() → isFinished(key)? → true: 跳过（不减 total）
                                  false: 正常下载
  │
  ▼
OnDone() → saveResumeKey(key) → SQLite 行级追加
```
