# 04 脚本引擎与日志系统（SCR / LOG）

> 审计对象：`src/internal/script/*.go`、`src/internal/scriptapi/scriptapi.go`、`src/internal/logging/*.go`。
> 行号仅供参考，以符号名定位为准。

---

## SCR-01 [High] 脚本"沙箱"黑名单名不副实，等同任意代码执行

- 类别：安全边界
- 涉及文件：`src/internal/script/engine.go`（`sandboxSymbols` L170-188）

### 问题描述
`sandboxSymbols` 宣称"移除 os/exec 等危险包"，实际 banned 列表**只有 `"os/exec"` 一项**。实测符号表中仍完整暴露：

- `os` 包 → `os.StartProcess` 可直接启动任意进程，**exec 禁令被一行代码绕过**；`os.RemoveAll`/`os.WriteFile` 可任意读写删文件；
- `syscall`（yaegi stdlib 含其符号）→ 直接系统调用；
- `net`/`net/http` → 任意外联，脚本可把下载目录内容（乃至 kv 目录中的 Telegram session）外传。

脚本若仅由用户本人编写威胁有限，但项目文档形态（脚本可分享/复制粘贴网上示例）意味着恶意脚本即任意代码执行。

### 根因分析
黑名单思路 + 名单严重不全。

### 修复方案
改为**白名单**：只导出脚本契约实际需要的包（`strings`、`strconv`、`regexp`、`path/filepath`、`time`、`unicode`、`fmt` 等纯计算类），移除 `os`、`os/*`、`net`、`net/*`、`syscall`、`runtime`、`unsafe`、`reflect`、`plugin`。并在 UI 明示脚本能力边界。

### 验收标准
- 编写调用 `os.StartProcess` / `net.Dial` / `syscall.*` 的脚本在校验或执行时被拒绝。
- 现有测试补充白名单拦截用例（当前仅覆盖 os/exec 拦截）。

---

## SCR-02 [Medium] yaegi 闭包被 Filter/Rename 与 OnFileDone 并发调用

- 类别：并发安全
- 涉及文件：`src/internal/script/engine.go`（L101-142、`callWithGuard` L144-168）、`src/internal/engine/iter.go`（L187/197）、`src/internal/engine/task.go`（L965-975）

### 问题描述
同一个 yaegi 解释器实例提取出的闭包会被**并发调用**：`SafeFilter`/`SafeRename` 在迭代器（downloader 驱动 goroutine）中执行，而 `SafeOnFileDone` 在下载 worker 的 `OnDone` 回调链中执行，二者时间上必然重叠。yaegi 解释器不保证并发执行安全，脚本内的全局变量更是裸奔。另外 `callWithGuard` 超时后原 goroutine 仍在解释器内运行，与后续新调用天然并发。

### 修复方案
在 `Contracts` 内加一把 `sync.Mutex`，所有 Safe* 调用串行化（脚本调用本就应轻量，串行成本可接受）。

### 验收标准
- `go test ./... -race` 下并发调用 Filter/Rename/OnFileDone 无数据竞争告警。

---

## SCR-03 [Medium] callWithGuard 超时逐文件泄漏 goroutine

- 类别：goroutine 泄漏
- 涉及文件：`src/internal/script/engine.go`（`callWithGuard` L144-168）

### 问题描述
`callWithGuard` 超时即弃（注释已自认）。Filter/Rename 是**每文件**调用——一个写了死循环的脚本 × 一个 5000 文件的任务 = 每文件卡 10 秒 + 泄漏 5000 个永不退出的 goroutine（每个都困在解释器里烧 CPU）。任务整体被拖成约 14 小时且进程 CPU 打满。

### 修复方案
首次超时后将该契约函数熔断（置 nil 并向 ScriptLog 发一条"脚本已禁用"），避免逐文件重复超时；长期方案是 yaegi 侧用带 ctx 的 `EvalWithContext` 编排（对提取闭包场景仍有限制，熔断是务实解）。

### 验收标准
- 死循环脚本触发一次超时后被熔断，不再逐文件重复超时、不再持续泄漏 goroutine。

---

## LOG-01 [Medium] sink 的 With/Write 丢弃结构化字段

- 类别：功能缺陷
- 涉及文件：`src/internal/logging/sink.go`（`With` L101、`Write` L110）

### 问题描述
`With([]zapcore.Field) zapcore.Core { return s }` 忽略字段，`Write(ent, _ []zapcore.Field)` 丢弃字段。任何 `logger.With(...)` 或 Sugared 的 `Infow/Errorw("msg", "key", val)` 的**结构化字段会静默消失**，日志里只剩 message 文本。当前代码库虽只用 `Infof/Errorf`，但这是暗坑——任何后来者按 zap 惯例写 `Errorw` 都会丢数据。

### 修复方案
`Write` 中把 fields 用 `zapcore.NewMapObjectEncoder` 编码后附加到 `e.Msg`（如 `msg {k=v ...}`）；`With` 至少返回一个记住 fields 的浅拷贝 core。

### 验收标准
- 用 `Errorw("msg", "key", "val")` 打日志，输出中包含 key=val。

---

## LOG-02 [Low] 锁内执行 lumberjack Write（轮转时全局阻塞）

- 涉及文件：`src/internal/logging/sink.go`（L110-141）

`s.mu` 覆盖了 lumberjack 的 `Write`（可能触发同步轮转：rename + 新建文件）。所有 goroutine 的日志调用在此串行化，轮转瞬间所有打日志的下载 worker 都会被卡住。建议文件写移出临界区（快照 `s.file` 后在锁外写；lumberjack 自身并发安全），或接受短暂阻塞并注释说明。

---

## LOG-03 [Low] 文件写失败被完全吞掉，Program Files 下日志静默失效

- 涉及文件：`src/internal/logging/sink.go`（L127-130）、`src/internal/logging/settings.go`（L41-47）

文件写失败被完全吞掉（`_, _ =`），而默认目录是**可执行文件所在目录**下的 logs——Windows 上安装到 `Program Files` 时普通权限不可写，结果是文件日志整体静默失效，用户与开发者都无感知。建议 `reconfigure` 时对目标目录做一次可写探测，失败则回退到 `%LOCALAPPDATA%` 并向 UI ring 缓冲写一条 WARN。

---

## SCR-04 [Low] 脚本 Resume 按名重载导致语义漂移

- 涉及文件：`src/internal/script/store.go`（`LoadByName` L101-107）、`src/internal/engine/task.go`（L363-371）

`Resume` 时按名字重新 `LoadByName`，脚本文件若在任务创建后被编辑，恢复后的过滤/命名规则与前半程不一致（同一任务两截文件命名规则不同）。建议任务创建时把脚本源码快照存入 tasks 表（schema 加一列），恢复时优先用快照。

---

## SCR-05 [Low] 脚本名未排除 Windows 保留设备名

- 涉及文件：`src/internal/script/store.go`（L33-41）

脚本名正则 `^[\w\p{Han}-]+$` 防路径穿越正确，但未排除 Windows 保留设备名（`con`、`nul`、`aux`、`com1` 等），`con.go` 在部分 Windows 环境上创建/删除行为异常。建议加保留名黑名单校验。

---

## 正面结论（脚本/日志，勿误改）

- 脚本引擎：panic 恢复、签名类型断言校验、空脚本拒绝、`Contracts` 全方法 nil 安全均正确，测试覆盖了 panic/语法错误/空脚本/os-exec 拦截四种失败路径。
- `scriptapi.logSink` 的 RWMutex 保护正确。
- 日志 sink 并发设计整体正确：共享状态均在 `s.mu` 内访问；`flush` 先摘 pending 再锁外 emit，避免回调持锁；`stopOnce` 防重复 close；`AtomicLevel` 支持热更级别且有测试（TestLevelGating）；ring 裁剪、批量推送节流（250ms）设计合理且测试覆盖良好。
