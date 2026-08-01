# 下载引擎装配

> [← 开发者文档目录](../README.md) | [项目主页](../../../README.md)

本文讲解从"消息链接或选集"到"文件落盘"的完整链路。核心代码在 `src/internal/engine/`，改写自 `ref/tdl/app/dl`（CLI 交互替换为事件推送），下载器本体直接复用 `ref/tdl/core` 的公开包。

## 装配链路（engine/task.go 的 execute）

```
TaskOptions{urls, selections, dir, script, ...}
  │
  ├─ 1. kv.Open("default")                     # bolt 命名空间（与 tdl CLI 一致）
  ├─ 2. pkgtclient.New(login=false)            # 构建 Telegram 客户端（代理/重连）
  ├─ 3. tclient.RunWithAuth                    # 校验会话有效性
  ├─ 4. dcpool.NewPool(size, middlewares)      # DC 连接池（限流/重试中间件）
  ├─ 5. 解析输入（二选一）
  │    ├─ URLs: tmessage.FromURL(...).Parse()  # 解析链接 → []Dialog
  │    └─ Selections: selectionsToDialogs()     # 选集 → []Dialog（跳过链接解析）
  ├─ 6. peers.Options{...}.Build(pool)         # peer 解析器（读 bolt 缓存）
  ├─ 7. newIter(...)                           # 迭代器：脚本过滤/命名在这里生效
  ├─ 8. loadResume(fingerprint)                # 读取断点（无询问直接续传）
  └─ 9. downloader.New(Options{
           Pool, Threads, Iter, Progress,      # Progress = engine/progress.go
       }).Download(ctx, limit)                 # limit = 并发文件数
```

## 各文件职责

| 文件 | 改写自 | 职责 |
| --- | --- | --- |
| `task.go` | `app/dl/dl.go` | 任务状态机、装配、断点保存 |
| `iter.go` | `app/dl/iter.go` | 文件迭代器；**脚本 Filter 替代 expr 表达式、脚本 Rename 优先于模板** |
| `progress.go` | `app/dl/progress.go` | 进度回调 → `task:file` 事件（200ms 节流）；完成后重命名 `.tmp` |
| `elem.go` | `app/dl/elem.go` | `downloader.Elem` 实现（单文件描述） |
| `selection.go` | （新增） | 选集下载：按对话+消息 ID 直接指定内容，无需 t.me 链接 |
| `links.go` | （新增） | 消息链接预检与清洗：校验语法、去重，任务创建时即反馈错误 |

## 任务状态机

```
queued ──► running ──► done
              │  ├───► failed
   paused ◄───┘  └───► canceled
      └────► running（恢复 = 重跑 + 断点续传）
```

- **暂停** = `context cancel` + **保留** bolt 中的 resume key
- **恢复** = 重新执行 execute；`loadResume` 按 fingerprint（URL/选集哈希）找回已完成分片位置（logicalPos），跳过已下载部分
- **取消** = cancel + 删除 resume key
- 该 fingerprint/logicalPos 机制与 tdl CLI 完全一致，因此 GUI 与 CLI 的断点可互通

## 选集下载（Selection）

`engine.Selection` 表示「对话 + 消息 ID 集合」：

```go
type Selection struct {
    DialogID   int64  // 对话 ID
    DialogType string // private / group / channel
    MessageIDs []int  // 消息 ID 列表
}
```

前端在 ChatsPage 中勾选媒体卡片后，通过 `Download.createTask({selections: [...]})` 创建任务。后端通过 `ResolveDialogPeer` 解析 peer（依赖对话列表落盘的 access hash），直接定位消息，无需 t.me 链接。

## 脚本注入点（iter.go）

```go
// Filter：返回 false 跳过；脚本出错时默认保留（fail-open）
if keep, err := it.opts.Contracts.SafeFilter(fileInfo); err != nil {
    i.notifySkip(info, fmt.Sprintf("过滤脚本出错，默认保留: %v", err))
} else if !keep {
    i.notifySkip(info, "被过滤脚本跳过")
    return false, true
}

// Rename：非空返回值优先；空串或出错回退 Go template
if it.opts.Contracts.HasRename() {
    if n, err := it.opts.Contracts.SafeRename(fileInfo); err != nil {
        i.notifySkip(info, fmt.Sprintf("命名脚本出错，回退默认模板: %v", err))
    } else {
        name = n
    }
}
```

生命周期钩子在 task.go 中调用：解析完成后 `OnTaskStart`、progress 完成回调里 `OnFileDone`、execute 结束时 `OnTaskDone`。

## SQLite 持久化层（store/）

`src/internal/store/` 提供下载任务的 SQLite 持久化，采用 `modernc.org/sqlite`（纯 Go，无需 CGO），启用 WAL 与外键级联。

### 四表结构

| 表 | 说明 | 级联 |
| --- | --- | --- |
| `tasks` | 任务主表（ID、目录、脚本、状态、计数） | — |
| `task_items` | 消息项（URL 或选集），外键关联 tasks | ON DELETE CASCADE |
| `files` | 文件记录（路径、大小、状态、对话/消息 ID），外键关联 tasks | ON DELETE CASCADE |
| `resume_keys` | 断点逐 key 行（ENG-12 优化），外键关联 tasks | ON DELETE CASCADE |

### Store 接口（TaskRepo）

`store.Store` 实现了 `engine.TaskRepo` 接口，供 `engine.Manager` 调用：

| 方法 | 用途 |
| --- | --- |
| `InsertTask(ctx, Task, []Item)` | 事务内插入任务 + 消息项 |
| `UpdateTaskStatus(ctx, id, status, errMsg)` | 更新任务状态 |
| `UpdateTaskState(ctx, id, status, errMsg, total, finished, failed)` | 单条 UPDATE 同步状态与计数（避免并发撕裂） |
| `DeleteTask(ctx, id)` | 删除任务（级联清理 items/files/resume_keys） |
| `LoadAllTasks(ctx)` | 启动时加载全部任务 |
| `ListItems(ctx, taskID)` | 返回任务的消息项 |
| `UpsertFile(ctx, File)` | 插入或覆盖文件记录 |
| `FinishFile(ctx, taskID, oldPath, File)` | `.tmp` 行替换为最终文件记录 |
| `MarkFileFailed(ctx, taskID, path)` | 标记文件状态为 failed |
| `DropFile(ctx, taskID, path)` | 移除文件记录 |
| `ListFiles(ctx, taskID)` | 返回任务的全部文件记录 |
| `DeleteFilesByPath(ctx, taskID, paths)` | 批量删除指定路径的文件记录 |
| `ListDoneFilesByDialog(ctx, dialogID)` | 对话内已完成的文件记录（"已下载"标记数据源） |
| `GetDoneFile(ctx, dialogID, messageID)` | 指定消息最新的已完成文件记录 |
| `LoadFinished(ctx, taskID)` | 读取断点集合 |
| `AddFinished(ctx, taskID, key)` | 追加单个断点 key（O(1) 行级写入） |
| `SaveFinished(ctx, taskID, map[string]struct{})` | 批量补写断点（任务中断时兜底） |
| `DeleteResume(ctx, taskID)` | 删除任务全部断点 |
| `DeleteResumeKey(ctx, taskID, key)` | 删除单个断点 key（单文件重新下载时使用） |

数据库启用 `PRAGMA journal_mode(WAL)` + `busy_timeout(5000)` + `synchronous(NORMAL)`，单连接池（`SetMaxOpenConns(1)`）避免 WAL 竞争。关闭时先置 `closed` 原子标志，让在途写入拿到 `ErrStoreClosed` 而非驱动错误（ARC-002）。

## 单文件操作接口

`DownloadService` 暴露以下单文件操作，供 TasksPage 右键菜单调用：

| 方法 | 说明 |
| --- | --- |
| `RedownloadFile(taskID, filePath)` | 重新下载：删除断点 key + 删除文件记录 + 触发任务恢复 |
| `DeleteFileRecord(taskID, filePath)` | 仅删除 SQLite 中的文件记录，不碰磁盘文件 |
| `RevealFileInDir(filePath)` | 在系统文件管理器中打开目录并选中该文件 |
| `ResumeTaskWithPending(id)` | 恢复任务时只下载 state ≠ 'done' 的文件（继续未完成部分） |

对应前端操作：任务列表中右键文件行 →「重新下载」「打开所在目录」「删除记录」「删除文件」。

## 并发与存储约束

- **bolt kv 全局单例**：bbolt 有文件锁，重复打开会死锁。所有服务共享 `main.go` 创建的唯一实例
- **SQLite 单连接池**：写入串行化，避免 WAL 竞争
- 下载参数（Threads/Limit/PoolSize）从 Settings 读取，创建任务时快照，运行中修改不影响已有任务

## 下一步

→ [Yaegi 脚本引擎扩展](../advanced/01-yaegi-extend.md)
