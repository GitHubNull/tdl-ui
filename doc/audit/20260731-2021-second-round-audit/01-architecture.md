# 01 架构设计类发现（ARC）

> 读者为 AI 编程代理。定位以符号名为准，行号仅供参考。禁止时间估算，优先级仅用 P0-P3。
> 本轮 ID 独立于 2026-07-29 轮次；"关联旧 {ID}" 指上一轮文档中的条目。

本类共 4 项：Medium 2、Low 2。

---

## ARC-001 [Medium] ChatService.StopAndWait 存在"建连中"盲区，退出时无法等待未就绪连接

- 类别：架构设计 / 资源管理编排、关闭生命周期（关联旧 ARC-02：关闭编排已建立，但存在残留窗口）
- 涉及文件：`src/internal/services/chat.go` — `ChatService.StopAndWait`、`ChatService.start`、`ChatService.runTelegramClient`；`src/main.go` — `OnShutdown` 回调

### 问题描述（含影响分析）

`StopAndWait` 的等待逻辑依赖 `s.cancel` / `s.dead` 两个字段，但 `start()` **仅在 Telegram 客户端建连成功之后**才发布这两个字段。时序：

1. 前端触发一次对话查询 → `start()` 启动，进入 `telegram.Client.Run`（建连阶段可持续数秒，网络差时更久）。
2. 用户此时关闭窗口 → `OnShutdown` 调用 `chatSvc.StopAndWait(5*time.Second)`。
3. `StopAndWait` 加锁读取：`s.cancel == nil`、`s.dead == nil` → 走 `if dead == nil { return }` 分支**立即返回**。
4. `OnShutdown` 继续执行 `taskStore.Close()`、`kvs.Close()`、`logging.Close()`。
5. 建连 goroutine 随后成功（或失败重试），继续读写已关闭的 bolt kv（会话存储）与日志 sink。

影响：进程退出阶段对已关闭 `kv.Storage` 的读写。bolt 侧表现为 `database not open` 类错误；最坏情形是建连成功后 gotd 回写会话数据被丢弃，下次启动需重新鉴权。窗口仅存在于"查询触发后、建连完成前退出"这一小段时间，故定级 Medium 而非 Critical（子代理初判 Critical，经验证降级：`dead != nil` 时的正常路径等待逻辑是正确的）。

### 根因分析

`start()` 把"可取消/可等待"的句柄发布时机绑定在**建连成功**这一业务事件上，而非 goroutine 启动这一生命周期事件上。生命周期句柄晚于 goroutine 存在，导致关闭编排对早期阶段不可见。

### 修复方案

在 goroutine 启动前就发布生命周期句柄，使 `StopAndWait` 覆盖全生命周期：

1. `start()` 中，创建 `runCtx, cancel := context.WithCancel(...)` 与 `dead := make(chan struct{})` 后，**在启动 goroutine 之前**（仍持 `s.mu`）写入 `s.cancel = cancel; s.dead = dead`。
2. goroutine 内用 `defer close(dead)` 保证无论建连成功、失败还是被取消，`dead` 都会关闭。
3. 建连失败的清理分支中，持锁比较 `if s.dead == dead { s.cancel, s.dead = nil, nil }` 后再退出，避免清掉后继连接的句柄（保持现有代际比较模式）。
4. 删除或保留 `StopAndWait` 中 `dead == nil` 的快速路径均可——修改后该分支仅在"从未启动过连接"时命中，语义正确。

### 实施步骤与优先级（P1）

1. 修改 `ChatService.start`：句柄发布提前至 goroutine 启动前；`defer close(dead)` 移到 goroutine 函数体首行。
2. 核对 `runTelegramClient` 内部所有提前 return 路径不再单独 close dead（统一由 defer 承担），避免 double close panic。
3. 跑 `cd src && go test ./internal/services/`。

### 验收标准（测试验证方法）

- 新增测试：注入一个阻塞在建连阶段的 fake（或用不可达代理配置触发慢建连），调用一次查询后立刻 `StopAndWait(100*time.Millisecond)`，断言其阻塞直到超时而非立即返回（耗时 ≥ 100ms）。
- 既有 `go test ./internal/services/` 全绿。
- 人工验证：设置无效代理 → 打开对话页触发查询 → 1 秒内关闭窗口 → 日志中无 `database not open` / bolt 关闭后写入类错误。

---

## ARC-002 [Medium] engine.Manager.StopAndWait 超时后 run goroutine 逃逸，可能写入已关闭的存储

- 类别：架构设计 / 资源管理编排、关闭生命周期（关联旧 ARC-02）
- 涉及文件：`src/internal/engine/manager.go` — `Manager.StopAndWait`、`Manager.run`；`src/main.go` — `OnShutdown` 回调

### 问题描述（含影响分析）

`Manager.StopAndWait(15*time.Second)` 会 cancel 所有任务并等待其 goroutine 退出，但等待用 `time.After` 超时兜底：**超时后直接 return，仅记录警告**。随后 `OnShutdown` 顺序执行 `taskStore.Close()`（SQLite）与 `kvs.Close()`（bolt）。仍在收尾中的 `run()` goroutine（如正被慢速磁盘 I/O 或网络关闭阻塞）会继续调用 `persistState` / 断点写入，落在已关闭的 `*sql.DB` 上。

影响：退出日志出现 `sql: database is closed` 刷屏（正是旧 ARC-02 想根除的现象），且该任务最后一段进度/断点丢失，重启后从更早的断点续传。不会损坏数据库文件（modernc SQLite 连接关闭是安全的），故定级 Medium。

### 根因分析

超时策略只解决"主流程不被卡死"，没有解决"逃逸 goroutine 与后续资源关闭的先后关系"。关闭编排缺少一个全局"存储即将关闭"的屏障，run goroutine 在超时逃逸后对此无感知。

### 修复方案

让存储层对"已进入关闭阶段"具备防御能力，双向加固：

1. `store.Store` 增加 `closed atomic.Bool`，`Close()` 先置位再关 `*sql.DB`；所有写方法入口检查 `if s.closed.Load() { return ErrStoreClosed }`（定义 `var ErrStoreClosed = errors.New(...)`）。
2. `engine` 侧持久化调用点（`persistState`、`resume_repo` 写入等）对 `ErrStoreClosed` 降级为一条 Debug 日志（不 Warn，不重试），消除刷屏。
3. （可选加固）`StopAndWait` 超时后不立即返回，改为再等一个短周期（如每 500ms 轮询一次剩余 goroutine 计数，总上限不超过调用方传入 timeout 的 2 倍）；若仍未退出才放弃。此步非必需，第 1、2 步已消除实际危害。

### 实施步骤与优先级（P1）

1. `src/internal/store/store.go`：加 `closed` 标志与 `ErrStoreClosed`，写路径统一入口检查。
2. `src/internal/engine/persist.go`、`resume_repo.go`：错误分支用 `errors.Is(err, store.ErrStoreClosed)` 降级日志级别。
3. 跑 `cd src && go test ./internal/store/ ./internal/engine/`。

### 验收标准（测试验证方法）

- 新增 store 测试：`Close()` 后调用 `UpdateTaskState` 等写方法，断言返回 `ErrStoreClosed` 而非 panic 或 `sql: database is closed`。
- 人工验证：启动一个大文件下载任务，立即关闭窗口，退出日志中无 `database is closed` 字样（可 `Select-String` 检查日志文件）。
- 既有 `go test ./internal/engine/` 全绿。

---

## ARC-003 [Low] 并发度与节流参数硬编码，无配置入口

- 类别：架构设计 / 可配置性
- 涉及文件：`src/internal/services/chat.go` — `runTelegramClient` 中 worker 启动段（1 个 jobs + 2 个 thumbJobs + 2 个 videoJobs）；`src/internal/engine/progress.go` — `progressEmitInterval`（200ms）

### 问题描述（含影响分析）

对话服务三队列的 worker 数与下载进度事件节流间隔均为源码常量。高延迟网络下缩略图/视频分段吞吐受限；低配机器上 200ms 进度事件对前端渲染压力固定不可调。不影响正确性，属于调优灵活性缺口。

### 根因分析

设计时选择固定并发度以简化心智模型（有意决策），未预留配置贯通路径。

### 修复方案

1. 在 `config.Settings` 增加可选字段（如 `ThumbWorkers`、`VideoWorkers`、`ProgressIntervalMs`），零值回退到当前常量。
2. `ChatService.runTelegramClient` 与 `engine/progress.go` 启动时从 `cfg.Get()` 读取一次（不要求热更新）。
3. 设置页暂不暴露 UI，仅支持配置文件手改（避免 UI 复杂度）。

### 实施步骤与优先级（P3）

1. 扩展 `config.Settings` 与默认值回退逻辑。
2. 两处调用点改读配置。
3. 跑 `cd src && go test ./...`。

### 验收标准（测试验证方法）

- 配置文件中写入 `thumbWorkers: 4` 后重启，日志或调试断点确认 worker 数为 4；不配置时行为与当前完全一致。
- `go test ./...` 全绿。

---

## ARC-004 [Low] engine.Manager 直接依赖 store.Store 具体类型，业务与持久化耦合

- 类别：架构设计 / 模块划分、可测试性
- 涉及文件：`src/internal/engine/manager.go` — `Deps.Store` 字段及 `Create`/`Remove`/状态更新调用点；`src/internal/engine/persist.go`

### 问题描述（含影响分析）

`Manager` 通过 `Deps.Store *store.Store` 直接持有具体类型，任务生命周期逻辑与 SQL 持久化耦合。单元测试 `Manager` 必须携带真实 SQLite 文件（当前测试确实如此，可行但慢），未来替换存储实现需改动 engine 包。不影响运行时正确性。

### 根因分析

历史演进结果：persist 拆分（旧轮修复）时保留了具体类型依赖，未抽象接口。

### 修复方案

1. 在 `engine` 包内定义窄接口（按 `persist.go` 与 `manager.go` 实际调用的方法集合裁剪，如 `taskRepo` 接口），`Deps.Store` 类型改为该接口。
2. `*store.Store` 天然满足接口，`main.go` 装配零改动。
3. 不建仓储子包、不加转换层——仅接口化，遵循"禁止向后兼容层"约束。

### 实施步骤与优先级（P3）

1. 用 `Grep` 列出 engine 包对 `deps.Store.` 的全部方法调用，据此定义接口。
2. 替换 `Deps` 字段类型，编译驱动收敛。
3. 跑 `cd src && go vet ./... && go test ./internal/engine/`。

### 验收标准（测试验证方法）

- `go build ./...` 通过且 `main.go` 无改动。
- engine 测试可用内存 fake 实现该接口跑通至少一个用例（证明可测试性达成）。

---

## 正面结论（勿误改）

以下架构层实现经本轮验证**正确且健壮**，修复上述发现时不得破坏：

1. **关闭编排主序**（`main.go` `OnShutdown`）：`emitter.Unbind()` → `taskManager.StopAndWait(15s)` → `chatSvc.StopAndWait(5s)` → `taskStore.Close()` → `kvs.Close()` → `logging.Close()` 的顺序正确，ARC-001/ARC-002 只需修复各自组件内部窗口，**不要调整此顺序**。
2. **bolt kv 全局单实例**（`main.go`）：登录与下载共享同一 `kv.Storage`，符合 bbolt 文件锁约束。
3. **会话变更联动**（`main.go`）：`authSvc.OnSessionChanged = chatSvc.Stop` 与 `authSvc.HasActiveDownloads = taskManager.HasActive` 两条装配线正确阻断了会话互踩。
4. **go.mod replace 治理**（`src/go.mod`）：ref/tdl 子模块以 replace 引入且注释完整。
5. **事件发射器 Unbind 防护**（旧 SVC-19 修复）：webview 销毁后不再 Emit，验证仍然有效。
6. **类型单一事实来源**（`src/frontend/src/types.ts`、`api.ts`，旧 ARC-06/FE-03）：前端直接复用 wailsjs 生成绑定，无手写平行类型。
7. **ChatService 三队列拆分**（`chat.go`，旧 ARC-04）：jobs/thumbJobs/videoJobs 队列隔离，重查询不阻塞缩略图，实现正确。
