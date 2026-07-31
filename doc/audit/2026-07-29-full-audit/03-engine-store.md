# 03 下载引擎与 SQLite 存储（ENG / STO）

> 审计对象：`src/internal/engine/*.go`、`src/internal/store/*.go`。
> 行号仅供参考，以符号名定位为准。

---

## ENG-01 [Critical] iter.Finished() 返回内部 map 引用，导致并发迭代/写 map 触发进程崩溃

- 类别：并发安全 / 崩溃风险
- 涉及文件：`src/internal/engine/iter.go`（`Finished` L301-311）、`src/internal/engine/progress.go`（`OnDone` L102-103）、`src/internal/store/resume_repo.go`（`SaveFinished` L35-43）

### 问题描述
`iter.Finished()` 在锁内返回的是**内部 map 的引用而非副本**。`progress.OnDone` 中 `p.task.saveResume(p.it.Finished())` 把这个活 map 传给 `store.SaveFinished`，后者在**不持有 iter.mu** 的情况下 `for k := range finished` 迭代；与此同时其他并发下载 worker 的 `OnDone` 会调用 `it.Finish(key)` 持锁写同一个 map。core/downloader 在 `Download(ctx, settings.Limit)` 下多文件并发，多个 `OnDone` 并发是常态。

Go 的 map 并发"迭代 + 写"会直接触发 `fatal error: concurrent map iteration and map write`，**无法 recover，整个进程崩溃**。锁的保护范围没有覆盖 map 的消费方。任何多文件并发任务都在赌运气。

### 根因分析
锁只保护了 map 的写入端（`Finish`），但把内部 map 的引用泄漏给了不持锁的消费端（`SaveFinished`）。

### 修复方案
`Finished()` 返回拷贝：
```go
func (i *iter) Finished() map[string]struct{} {
    i.mu.Lock()
    defer i.mu.Unlock()
    out := make(map[string]struct{}, len(i.finished))
    for k := range i.finished {
        out[k] = struct{}{}
    }
    return out
}
```
`execute` 末尾 defer 中的 `it.Finished()` 也一并受益。

### 验收标准
- 在多文件并发下载任务下运行 `go test ./... -race`，无 map 并发读写告警。
- 构造高并发 OnDone 的压力测试，进程不再崩溃。

---

## STO-01 [High] migrateV1 的 user_version 写在建表事务外且建表无 IF NOT EXISTS

- 类别：迁移健壮性
- 涉及文件：`src/internal/store/store.go`（`Open` / `migrateV1` L109-129、L131-195）

### 问题描述
`migrateV1` 的建表事务提交后，`PRAGMA user_version = 1` 是**独立的第二次 Exec**。若在两者之间进程崩溃/断电，重启时 user_version 仍为 0，会重跑 `migrateV1`，而建表语句全部没有 `IF NOT EXISTS` → `CREATE TABLE tasks` 报错 → `Open` 永久失败，**应用再也打不开任务库**，且无自愈路径。

### 根因分析
版本标记与结构变更不在同一原子单元。SQLite 的 `PRAGMA user_version` 可以在事务内设置并随之回滚/提交。

### 修复方案
- 把 `tx.Exec("PRAGMA user_version = 1")` 移入 `migrateV1` 事务内。
- 全部建表语句加 `IF NOT EXISTS`（两者都做最稳）。

### 验收标准
- 在建表提交后、版本写入前模拟崩溃（或手工把 user_version 置 0）重启，应用仍能正常打开任务库。

---

## ENG-02 [Medium] 暂停/取消时 elem 通道内滞留的 fd 与 .tmp 孤儿文件泄漏

- 类别：资源泄漏
- 涉及文件：`src/internal/engine/iter.go`（`processSingle` L238-255）

### 问题描述
`processSingle` 中 `os.Create(path)` 打开的 `*os.File` 被塞进 `i.elem` 缓冲通道（容量 10）。任务被暂停/取消时，通道里尚未被 downloader 消费的 elem 的文件句柄**永远不会 Close**（只有 `OnDone` 才 Close），对应 `.tmp` 文件也因未触发 `OnAdd`/`addFile` 而**未登记到 t.files**。CLI 版进程跑完即退句柄随进程回收；本项目是长生命周期 GUI，反复暂停/恢复会累积 fd 泄漏（Windows 下句柄占用还会导致该 `.tmp` 无法删除），且 `DeleteAllFiles` 只删登记过的文件，这些孤儿 `.tmp` 永远清不掉。

### 修复方案
`execute` 返回前排空通道并逐个 `elem.to.Close()` + `os.Remove()`；或在 iter 上加 `Close()` 方法，`defer it.Close()`。

### 验收标准
- 反复暂停/恢复同一任务多次，进程 fd 数量不持续增长，无孤儿 `.tmp` 残留。

---

## ENG-03 [Medium] Remove/Resume TOCTOU 竞态产生失控僵尸下载

- 类别：状态机 / 并发
- 涉及文件：`src/internal/engine/task.go`（`Remove` L404-432；`run` L350-383）

### 问题描述
`Remove` 的检查（status 非 Running/Queued）与随后删除 map 条目之间没有原子性。并发场景：Remove 读到 status=Failed 通过检查 → 此刻 `Resume` 把状态置 Queued 并 `go m.run(t)` → run 的 `m.exists(t.ID)` 若在 Remove 删除之前执行则通过 → Remove 删除 map 条目和 store 行 → 任务在后台继续下载，**UI 无任何句柄可暂停/取消它**（僵尸 goroutine），且后续 `UpsertFile`/`SaveFinished` 会因外键约束（task 行已删）持续报错。

### 根因分析
TOCTOU——状态检查、map 删除、run 启动三者跨越多把锁没有统一协调。

### 修复方案
- `Remove` 在持有 `t.mu` 期间同时完成状态判定与一个"已删除"标记（如 `t.removed = true`）；`run()` 开头及 `Resume` 中检查该标记。
- 或让 `run()` 的 `exists` 检查与 `status=Running` 的赋值放进 manager 级临界区。

### 验收标准
- 并发 Remove + Resume 压力测试下不产生无句柄的后台下载 goroutine。

---

## ENG-04 [Medium] Queued 态任务不可暂停/取消

- 类别：状态机不完整
- 涉及文件：`src/internal/engine/task.go`（`Pause` L332-347、`Cancel` L386-401）

### 问题描述
`Pause`/`Cancel` 要求 `status == StatusRunning && cancel != nil`，因此 **Queued 状态的任务既不能暂停也不能取消**。窗口包括：`Create`/`Resume` 到 `run()` 设置 Running 之间；更实际的是 `run()` 中建连（`pkgtclient.New`、`RunWithAuth`，含 5 分钟 `reconnectTimeout`）阶段一旦网络不通，任务长时间卡在 Running 前/中，用户若恰好点在窗口内会得到"任务未在运行中"的报错。

### 根因分析
状态机只对 Running 态定义了 pause/cancel 转换，缺 Queued → Canceled/Paused 边。

### 修复方案
- Queued 态允许打"预取消/预暂停"标记，`run()` 启动时检查标记直接落终态。
- cancel 函数在 `Create`/`Resume` 时即创建并保存。

### 验收标准
- 对处于 Queued/建连中的任务点击取消/暂停能生效，不再报"任务未在运行中"。

---

## ENG-05 [Medium] task.go 983 行 5 类职责混杂，状态机与断点续传零测试

- 类别：代码质量 / 可测试性
- 涉及文件：`src/internal/engine/task.go`（全文 983 行）

### 问题描述
单文件混杂至少 5 类职责：Manager 生命周期 CRUD（Create/List/Pause/Resume/Cancel/Remove）、store 双向映射（itemsToOptions/optionsToItems/toStoreTask/storeFilesToTaskFiles）、文件删除业务（DeleteFiles/DeleteAllFiles）、任务执行编排（run/execute）、文件登记回调（addFile/finishFile/markFileFailed/dropFile）。Manager 与 Task 双向引用（`t.mgr`），持久化调用散落在每个内存变更点旁，形成"改内存 + 写库"成对样板代码。`execute` 对 tclient/dcpool 的直接依赖使其完全不可单测——当前测试仅覆盖 links/selection/DeleteFiles，**状态机转换、断点续传、progress 回调均无测试**。

### 修复方案
- 拆分为 `manager.go`（CRUD）、`executor.go`（run/execute）、`persist.go`（store 映射与写回）。
- 把"内存 + 库"双写收敛为一个 `taskFiles` 小结构统一处理。
- 抽 `runner interface` 注入以便对 execute 打桩，补状态机转换测试。

### 验收标准
- task.go 拆分后各文件职责单一；新增状态机转换与断点续传单测。

---

## ENG-06 [Low] 进度语义误导：finished 永远达不到 total

- 涉及文件：`src/internal/engine/iter.go`（`Total` L313-322）

`Total()` 统计所有消息数，但无媒体消息、已删除消息、被脚本 Filter 跳过、SkipSame 跳过的都不计入 finished，导致任务已 Done 而 UI 显示 12/20 之类误导性进度。建议跳过时递减 total（加 `Decr()` 回调），或前端按状态 Done 强制显示完成。

---

## ENG-07 [Low] iter.err 读写并发规约不一致

- 涉及文件：`src/internal/engine/iter.go`（`Next` L103-107、`Err` L291-293）

`Next()` 顶部 `i.err = ctx.Err()` 无锁写、`Err()` 无锁读，而 `process` 内是持 `i.mu` 写 `i.err`。当前单 goroutine 驱动不出事，但 `-race` 下若调用方模式变化即报警。建议统一入锁或改用 `atomic.Pointer[error]`。

---

## ENG-08 [Low] process 持锁做网络/磁盘 I/O

- 涉及文件：`src/internal/engine/iter.go`（`process` L124-163、L260-283）

`process` 全程持有 `i.mu`，其间执行 `FromInputPeer`、`GetSingleMessage`、`GetGroupedMessages`（网络）和 `os.MkdirAll`/`os.Create`（磁盘）。并发 worker 的 `it.Finish()` 会被这些 I/O 阻塞，连带阻塞断点持久化。建议缩小锁范围至索引推进与 finished 集合访问。

---

## ENG-09 [Low] elem 通道容量 10 与相册上限的隐性耦合

- 涉及文件：`src/internal/engine/iter.go`（L99、L111）

`elem` 通道容量 10 与"Telegram 相册最多 10 项"是隐性耦合；不死锁的前提是 `Next()` 先检查 `len(i.elem) > 0`。三者关系无注释，未来改动任一处都可能造成持锁阻塞在 `i.elem <-` 上的死锁。建议加注释固化不变量，或将容量定义为具名常量 `maxAlbumSize = 10`。

---

## ENG-10 [Low] persistState 两条独立 UPDATE 可能乱序覆盖

- 涉及文件：`src/internal/engine/task.go`（`persistState` L850-864）

`persistState` 在锁内取快照、锁外写库，且状态与计数是两条独立 UPDATE。多 worker 并发时旧快照可能后写覆盖新快照（DB 短暂回退）。任务结束的最终 emitUpdate 会纠正，属瞬态；但若恰在窗口崩溃重启后计数偏小。建议两条 UPDATE 合并为一条，或改为 `finished = MAX(finished, ?)` 语义。

---

## ENG-11 [Low] 任务 ID 基于时间戳理论上可碰撞

- 涉及文件：`src/internal/engine/task.go`（约 L293）

任务 ID 用 `t%d + UnixNano`。Windows 时钟粒度较粗，脚本化快速连建任务理论上可碰撞，碰撞时 `InsertTask` 主键冲突报"保存任务失败"。建议改用 `crypto/rand` 短 ID 或 uuid。

---

## ENG-12 [Low] 断点续传每文件完成重写整个 finished 集合（O(n²)）

- 涉及文件：`src/internal/engine/task.go`（L801-809）、`src/internal/store/resume_repo.go`（L35-52）

每完成一个文件就把整个 finished 集合 JSON 序列化重写一行，万级文件任务下每次写数百 KB。建议断点改为独立行表 `(task_id, resume_key)` 逐条 INSERT，删除时按 task_id 清。

---

## ENG-13 [Low] loadFromStore 加载失败任务静默消失

- 涉及文件：`src/internal/engine/task.go`（`loadFromStore` L151-155）

`ListItems` 失败时 `continue`，该任务静默从 UI 消失但库中行仍在，用户无从感知也无法删除。建议降级加载（items 置空、状态标 failed 并附错误信息），保留可见性。

---

## STO-02 [Low] ImportLegacyJSON 未去重会因唯一索引导致重试死循环

- 涉及文件：`src/internal/store/migrate.go`（L102-106）、`src/internal/store/store.go`（L160）

`ImportLegacyJSON` 直接逐条插入旧 URLs，未去重；而 v1 schema 有 `uq_items_url` 唯一索引。旧 tasks.json 若同一任务含重复链接，整个导入事务回滚、tasks.json 不改名，**每次启动都重复失败**。建议 url 插入改用 `INSERT OR IGNORE` 或导入前去重；导入失败时把原文件改名为 `.failed` 打破重试循环并保留现场。

---

## STO-03 [Low] 仓储方法未使用 Context 版 API，无法随取消中断

- 涉及文件：`src/internal/store/` 全部 repo 文件

所有查询/写入使用 `db.Exec`/`db.Query` 而非 `ExecContext`/`QueryContext`，无法随任务取消而中断，也无法设置操作超时（busy_timeout 只覆盖锁等待）。建议仓储方法签名统一加 `ctx context.Context`。

---

## STO-04 [Low] WAL 模式未设置 synchronous(NORMAL)

- 涉及文件：`src/internal/store/store.go`（约 L84）

WAL 模式下未设置 `synchronous`，SQLite 默认 FULL；WAL + NORMAL 是公认安全且明显更快的组合。本项目每文件完成要写 3~4 条语句，fsync 开销可感知。建议 DSN 追加 `&_pragma=synchronous(NORMAL)`。

---

## 设计取舍确认（非缺陷，但应知晓）

- `progress.go` L93-99：暂停（context.Canceled）时会 `os.Remove(tmpPath)` 删除半成品并 `dropFile`，断点粒度是**文件级**而非字节级——大文件暂停即整文件重下。若为有意设计建议在 UI 文案说明。

## 正面结论（引擎/存储，勿误改）

- **SQL 注入：无风险**。全部 SQL 均为参数化占位符，无任何字符串拼接进 SQL 文本（含 `MarkFileFailed` 的 `'failed'` 为硬编码字面量）。
- 事务使用得当：`InsertTask`、`FinishFile`、`DeleteFilesByPath`、`ImportLegacyJSON`、`migrateV1` 均有事务 + `defer Rollback` 兜底。
- `SetMaxOpenConns(1)` + WAL + busy_timeout(5000) 的单连接策略对 modernc（纯 Go 驱动）是正确的规避写竞争的做法；PRAGMA 走 DSN 保证每连接生效。
- 外键级联删除有测试覆盖（TestCascadeDelete），`sql.NullString`/`NullInt64` 处理 NULL 列规范。
