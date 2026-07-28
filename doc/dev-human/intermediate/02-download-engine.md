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

## 并发与存储约束

- **bolt kv 全局单例**：bbolt 有文件锁，重复打开会死锁。所有服务共享 `main.go` 创建的唯一实例
- 下载参数（Threads/Limit/PoolSize）从 Settings 读取，创建任务时快照，运行中修改不影响已有任务

## 下一步

→ [Yaegi 脚本引擎扩展](../advanced/01-yaegi-extend.md)
