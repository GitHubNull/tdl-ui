# 01 架构与跨切面问题（ARC / DEP）

> 审计对象：`src/main.go`、`src/go.mod` 及跨模块的架构性问题。
> 行号以审计时（2026-07-29）代码为准，仅供参考，实际修改时请以符号名定位。

本章收录**跨越多个模块、无法在单个文件内修完**的系统性问题。修复它们通常需要先设计、再分步实施，建议由路线图（`06-remediation-roadmap.md`）中 P1/P2 阶段统一编排。

---

## ARC-01 [High] Telegram 会话缺少所有权模型，四方并发读写同一 kv 会话

- 类别：架构 / 并发 / 业务逻辑
- 涉及文件：`src/internal/services/auth.go`、`src/internal/services/chat.go`、`src/main.go`、下载 engine（`src/internal/engine/task.go` 的 `execute`）

### 问题描述
同一份 kv 会话（gotd 的 `engine.Namespace` session storage）会被至少四个 gotd client 使用，且彼此之间只有 `main.go` 里一根 `OnLogout = chatSvc.Stop` 的协调线：

1. 登录流程 client（`runCodeLogin` / `runQRLogin`）——会**重写**会话（`kvd.Set(key.App())` + auth flow 写 session）。
2. `chat` 服务的常驻连接（`chat.go` 的 `ensureStarted` 建立的长生命周期 client）。
3. 下载 engine 的 client（每个下载任务 `execute` 时建连）。
4. `verifySession` 的临时校验 client。

gotd 的 session storage 同时保存 auth key 与更新状态（update state），多个 client 并发读写同一 session 记录会互相覆盖。最严重的场景是：用户在**不登出**的情况下重新登录或导入另一个 Desktop 账号成功，`ChatService` 的常驻连接仍持有旧账号的客户端，`ListDialogs` 返回旧账号数据，直到重启或手动登出。

### 根因分析
"会话"是一个全局单例资源，但代码中没有任何"所有权 / 使用中"的协调机制。各方各自 `New` 一个 client 直接用，登录流程甚至会在其他 client 运行期间重写底层会话。

### 修复方案
1. 引入会话生命周期钩子：在 `main.go` 装配处，除已有的 `OnLogout` 外，新增 `OnLoginSuccess` 回调，同样接 `chatSvc.Stop()`（下次查询会用新会话自动重建常驻连接）。在 `loginSuccess()` 与 `ImportDesktopSession` 成功路径调用它。
2. 登录/导入入口（`StartCodeLogin` / `StartQRLogin` / `ImportDesktopSession`）执行前先停止 chat 常驻连接，并在存在活跃下载任务时拒绝重登（返回明确错误提示用户先停止下载）。
3. 中期方案：把会话包装为单一持有者（holder），chat + thumb 已复用同一 client 是正确方向，再把登录/校验也纳入同一把互斥锁下，保证任一时刻只有一类操作在写会话。

### 验收标准
- 已登录状态下导入另一账号成功后，`ListDialogs` 立即返回新账号数据，无需重启。
- 有下载任务运行时触发重新登录被明确拒绝，或下载被安全暂停后再登录。

---

## ARC-02 [High] 关闭编排缺失：OnShutdown 先关存储再让业务 goroutine 继续跑

- 类别：架构 / 生命周期 / 资源管理
- 涉及文件：`src/main.go`（`OnShutdown`，约 L120-125）

### 问题描述
`OnShutdown` 直接 `taskStore.Close()`、`kvs.Close()`，但没有先停止：

1. `taskManager` 中进行中的下载任务（持有 TG 连接、正在写 taskStore 与 kv）；
2. `chatSvc` 的常驻 Telegram 连接（`ChatService.Stop()` 从未在退出路径被调用）。

bolt/SQLite 被关闭后，仍在运行的 goroutine 会持续报错，下载进度可能丢失最后一批写入；常驻连接 goroutine 直接泄漏到进程退出。

### 根因分析
缺少统一的关闭编排：正确顺序应为"取消业务 goroutine → 等待其退出 → 再关存储"，当前实现把顺序反了。

### 修复方案
在 `OnShutdown` 中依次执行：
1. 调用 `taskManager` 的停止/等待方法（若 engine 尚无此能力，需补充一个带超时的 `StopAndWait`，取消所有运行任务的 ctx 并等待 goroutine 退出）。
2. 调用 `chatSvc.Stop()` 并等待其 `dead` 通道关闭（建议给 `ChatService` 加带超时的 `StopAndWait`）。
3. 最后再关闭 `taskStore` / `kvs` / logging。

### 验收标准
- 下载进行中关闭窗口，任务进度被完整持久化到最后一刻，无"database is closed"类错误刷屏。
- 关闭后无残留 goroutine（可用退出前打印 `runtime.NumGoroutine()` 或 pprof 验证）。

---

## ARC-03 [High] 取消语义系统性断裂：ctx 被创建但不贯穿执行层

- 类别：架构 / 并发
- 涉及文件：`src/internal/services/auth.go`（begin/finish）、`src/internal/services/chat.go`（invoke/worker）、`src/main.go`（shutdown）

### 问题描述
这是一个横跨三处的同源问题——ctx 都被正确创建了，但没有传播到"真正做事"的那一层：

1. **auth 的 cancel 无代际归属**（详见 `02-backend-services.md` AUTH-01）：任何一代登录流程的收尾都能取消全局唯一的 `s.cancel`。
2. **chat 的 invoke 超时不传播到 worker 执行**（详见 `02-backend-services.md` SVC-01）：调用方超时 ctx 不进入实际执行的 `fn`。
3. **OnShutdown 不编排业务 goroutine 退出**（见 ARC-02）。

### 根因分析
三者本质相同：`context.Context` 作为取消信号的载体被创建出来，但在关键链路上被"截断"——要么归属错乱，要么根本没往下传。

### 修复方案
按 AUTH-01、SVC-01、ARC-02 分别修复。修复时确立统一原则：**任何异步操作都必须接受并尊重调用方传入的 ctx**，禁止在 worker 内替换为常驻 ctx 而丢弃调用方 ctx。

### 验收标准
见 AUTH-01 / SVC-01 / ARC-02 各自的验收标准。

---

## ARC-04 [Medium] 串行单 worker 队列把并发正确性问题转化为吞吐/延迟问题

- 类别：架构 / 性能
- 涉及文件：`src/internal/services/chat.go`（jobs 通道 + 单 worker 循环）、`src/internal/services/thumb.go`、`src/internal/services/media_handler.go`

### 问题描述
所有 Telegram 查询（`ListDialogs` 全量翻页、`ListMedia`、缩略图拉取、preview 大图 64KB 分块下载）共用**单个串行 worker**，经由一个无缓冲 `jobs` 通道排队。一页媒体列表会产生几十个 `/media/thumb` 并发 HTTP 请求，每个都要排入同一队列，每个带 3 分钟超时挂起；preview 大图一次占用队列几十个往返。结果：画廊加载体验随队列长度线性劣化，大量 HTTP handler goroutine 长时间阻塞在 `jobs <- j` 上。`thumb.go` 注释"超时需覆盖排队时间"正是这个设计缺陷的补丁式规避。

### 根因分析
串行化是为规避 Telegram 并发限速与会话并发访问，但代价是没有取消、没有优先级、慢任务阻塞快任务。

### 修复方案
1. 短期：`chatJob` 增加 `ctx` 字段（配合 SVC-01），worker 取 job 后先检查 `j.ctx.Err()` 直接跳过已放弃的任务；把 thumb 请求超时降到 15-30s。
2. 中期：将缩略图拉取与列表查询分离为两个队列，或改用带小并发度（2-3）的 worker 池，缩略图队列可与列表查询队列独立限速。

### 验收标准
- 打开大频道媒体页时，单张大图预览不再阻塞其余缩略图加载。
- 快速切换对话时，被放弃的旧请求不再持续占用 worker 配额。

---

## ARC-05 [Medium] go.mod replace => ../ref/tdl 使版本号形同虚设、构建不可复现

- 类别：依赖 / 构建可复现性
- 涉及文件：`src/go.mod`（约 L96-100）

### 问题描述
`require github.com/iyear/tdl v0.20.3`（及 core）被 `replace => ../ref/tdl` 整体覆盖，实际编译的是 git submodule 当前 checkout 的代码，**版本号形同虚设**：submodule 若被本地改动或指向别的提交，go.mod 完全无感知，`go.sum` 对 replace 目标也不校验。CI/协作者未执行 `git submodule update --init` 时会直接构建失败。此外 `replace github.com/iyear/tdl/extension => ../ref/tdl/extension`（约 L99）在 require 列表中并无对应模块，属死指令。`go 1.25.8`（L3）写到 patch 版本会触发 Go 的 toolchain 自动切换，强制所有构建环境 ≥1.25.8。

### 根因分析
为了对 tdl 上游打本地补丁（或直接引用其内部包）而使用了本地 replace，但缺少对 submodule commit 的固化与 CI 校验。

### 修复方案
1. 若 `ref/tdl` 无本地补丁：删除 replace，直接用上游 tag（可复现、可 `govulncheck`）。
2. 若有补丁：在 `README` / `agent.md` 中固化 submodule commit 要求，并在 CI 加 submodule 状态校验步骤。
3. 删除冗余的 `extension` replace 指令。
4. 将 `go 1.25.8` 改为 `go 1.25`，按需用单独的 `toolchain` 行声明。
5. replace 治理后将 `govulncheck` 纳入 CI。

### 验收标准
- 干净 clone（含 submodule 初始化步骤）后 `go build ./...` 成功。
- `go vet` / `govulncheck` 可正常运行。

---

## ARC-06 [High] 前端弃用 Wails 生成的强类型绑定，用 any 通道手工重建 API 层

- 类别：架构 / 类型安全（前端）
- 涉及文件：`src/frontend/src/api.ts`、`src/frontend/src/types.ts`、`src/frontend/wailsjs/`

### 问题描述
项目已有 Wails 生成的强类型绑定（`wailsjs/go/services/*.d.ts` + `wailsjs/go/models.ts`、`wailsjs/runtime/runtime.d.ts`），但 `api.ts` 弃之不用，通过 `window.go?.services?.[name]` 的 `any` 通道手工重建了一整层 API，`types.ts` 又手工维护了一份与 `models.ts` 平行的 DTO。后果：方法名/参数拼写错误编译期无法发现；后端结构体变更后 `wailsjs` 会重新生成，而 `types.ts` 需人肉同步，存在漂移风险。

> 本条与前端章节 FE-03 是同一问题，此处从架构视角登记，详细修复步骤见 `05-frontend.md` FE-03。

### 修复方案
`api.ts` 改为 re-export `wailsjs/go/services/*` 的生成函数；事件用 `wailsjs/runtime` 的 `EventsOn`；`types.ts` 仅保留 `models.ts` 没有的事件负载类型（`LoginUpdate` / `FileEvent`），其余 `import type` 自 `models.ts`。

### 验收标准
- 后端结构体字段变更后，前端引用处能在 `vue-tsc` 检查时报错（需配合 FE build 门禁）。

---

## 正面结论（架构层面，勿误改）

- 分层清晰：`main` 负责装配、`services` 是薄绑定层、`engine` / `config` / `logging` 各司其职。
- 依赖注入规范：`userHomeDir`、`connectTimeout`、`runClient` 等测试注入点都是有意识的设计，未使用全局单例。
- `Emitter` 对 Wails 运行时 ctx 的封装、"未绑定时静默丢弃"的语义处理得当。
- 注释密度高且诚实，多处标注了设计权衡与已知限制。

整体是一个"骨架正确、边界场景欠打磨"的代码库，上述问题都可在不推翻现有架构的前提下渐进修复。
