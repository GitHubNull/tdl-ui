# 02 后端服务层（AUTH / SVC）

> 审计对象：`src/internal/services/*.go`、`src/internal/config/*.go`、`src/internal/events/events.go`、`src/main.go`。
> 行号仅供参考，以符号名定位为准。

---

## AUTH-01 [High] 登录 cancel 无代际归属，切换登录方式时旧流程会取消新流程

- 类别：并发 / 生命周期
- 涉及文件：`src/internal/services/auth.go`（`begin` / `finish`，约 L313-335；`runCodeLogin` L380-383；`runQRLogin` L441-444）

### 问题描述
`finish()` 取消的是"当前"的 `s.cancel`，而不是"自己这一代流程"的 cancel。场景：验证码登录进行中 → 用户切换到二维码登录，`begin()` 取消旧 ctx 并把 `s.cancel` 替换为新流程的 cancel；旧 goroutine（`runCodeLogin`）因 ctx 取消而退出时，其 `defer s.finish()` 加锁后看到的 `s.cancel` 已是**新流程**的 cancel，直接把刚启动的二维码登录取消掉。`CancelLogin` 先置 nil 能缩小窗口，但旧 goroutine 的网络收尾（`c.Run` 返回）是异步的，与新 `begin()` 之间存在真实竞态窗口。此外，旧流程失败时向前端发出 `stage=error` 事件会干扰新流程 UI（`runCodeLogin` / `runQRLogin` 未判断"自己是否仍是当前流程"）。

### 根因分析
cancel 句柄没有"代际"归属，任何一代流程的收尾都能取消全局唯一的 cancel。

### 修复方案
- `begin()` 返回本代的 `cancel`（或递增的 generation ID），`finish(myCancel)` 中比较 `if s.cancel == myCancel`（指针相等或 generation 计数）才清理。
- `codeCh` / `pwdCh` 也应按代际归属。
- 事件发送前判断当前 generation 是否仍是自己，避免旧流程污染新流程 UI。

### 验收标准
- 快速在验证码/二维码登录间切换，新流程不被旧流程收尾取消。
- 为 begin/finish 的代际逻辑补状态机单测（当前 auth.go 零测试）。

---

## AUTH-02 [High] ImportDesktopSession 校验失败"回滚"实为删除，毁掉原有效会话

- 类别：业务逻辑
- 涉及文件：`src/internal/services/auth.go`（`ImportDesktopSession` L216-264；`clearSession` L305-308）

### 问题描述
"校验失败回滚"实际是**删除**而非回滚。`loader.Save` 在校验前就覆盖了 kv 中的现有会话；若用户此前已有一个有效登录态、导入另一个 Desktop 账号失败，`clearSession` 会把会话和 App 标记全部删掉——用户不但没导入成功，原来的有效会话也被销毁，且 `cfg` 中 `LoggedInUserID` 仍保留旧值，出现"配置显示已登录、kv 中无会话"的假登录态（`Status()` 只读配置）。

### 根因分析
写前未备份，"回滚"语义被实现为"清空"。

### 修复方案
- `Save` 之前先 `kvd.Get` 读出旧的 session 与 app key 备份；校验失败时写回备份而非删除；若原本就无会话再执行删除。
- 失败路径应同步清空 `cfg.LoggedInUserID`，或让 `Status()` 参考 kv 实际状态。

### 验收标准
- 已登录状态下导入一个无效 Desktop 会话失败后，原有效会话仍可用，登录态不变。

---

## SVC-01 [High] chat invoke 调用方超时不传播到 worker 执行，任务不出队

- 类别：并发 / 架构
- 涉及文件：`src/internal/services/chat.go`（`invoke` 约 L205-231；worker 循环约 L359-366）

### 问题描述
两个叠加缺陷：
1. `invoke(ctx, fn)` 的调用方超时 ctx（ListDialogs 120s / ListMedia 90s / thumb 3min）**不会传播到实际执行**：worker 循环 `j.done <- j.fn(ctx, c.API())` 传入的是常驻客户端的 run ctx，调用方超时返回后，fn 仍在后台跑完整个（可能极慢的）API 调用序列。
2. 所有查询共用**单个串行 worker**（见 `01-architecture.md` ARC-04），一个慢任务会阻塞后面排队的所有请求；调用方超时后任务并不出队，浪费配额且持续占用 worker。

### 根因分析
job 未携带调用方 ctx；串行化没有取消与优先级机制。

### 修复方案
- `chatJob` 增加 `ctx` 字段，worker 执行时用 `ctx, cancel := context.WithCancel(runCtx)` 合并调用方 ctx（或直接 `j.fn(j.ctx, ...)` 并在 fn 内的 API 调用感知取消）。
- worker 取 job 后先检查 `j.ctx.Err()` 直接跳过已放弃的任务。
- 中期与 ARC-04 一并把缩略图与列表查询分离队列。

### 验收标准
- 调用方超时/取消后，后台不再继续执行已放弃的 Telegram 请求。

---

## SVC-02 [Medium] 缩略图缓存三重缺陷：无负缓存、失败重试风暴、空切片 panic

- 类别：缓存策略 / panic 风险 / 资源管理
- 涉及文件：`src/internal/services/thumbcache.go`（L41-73）、`src/internal/services/thumb.go`（`pickThumbSize` L80-86；`fetchThumb` L117-157；`downloadSmallFile` L219-241）

### 问题描述
1. **无负缓存**：`len(b)==0`（普通文件无缩略图）时不落盘，media_handler 返回 404，前端每次渲染该列表项都会再次触发完整的 `invoke → GetSingleMessage`，反复消耗 Telegram API 调用与串行队列时间。
2. **失败重试风暴**：fetch 失败后所有等待同一 key 的 goroutine 被唤醒，逐个成为新的拉取方串行重试（`continue` 循环），N 个并发请求 → N 次失败的网络往返。
3. **空切片 panic**：`pickThumbSize` 中 `case *tg.PhotoSizeProgressive: smallestBytes = v.Sizes[len(v.Sizes)-1]`——若服务端返回的 `Sizes` 为空切片，index out of range panic，且该函数在 assetserver HTTP 请求路径上被调用。
4. **无内存上限**：`downloadSmallFile` 下载循环无总大小上限，`out` 全部驻留内存；preview 取像素面积最大的真实尺寸，原图级照片可能数 MB 甚至更大。

### 根因分析
singleflight 只对"成功"路径做了合并；对外部数据切片长度未做防御；下载无上界保护。

### 修复方案
1. 空结果写哨兵文件（如 0 字节 `.empty`）或维护带 TTL 的内存 negative set。
2. fetch 失败时把 error 通过 inflight 通道广播给等待者直接返回，而非各自重试。
3. `pickThumbSize` 加 `if len(v.Sizes) > 0` 保护，并修复该分支不参与"最小尺寸"比较的逻辑不一致。
4. `downloadSmallFile` 加 `maxBytes`（如 10MB）保护，超限报错；preview 优先选 "x"/"y" 等中等规格。

### 验收标准
- 对无缩略图文件的列表滚动不再反复打网络请求。
- 构造空 `Sizes` 的照片消息不触发 panic。

---

## SVC-03 [Medium] Logout 不取消进行中的登录流程与下载任务

- 类别：业务逻辑
- 涉及文件：`src/internal/services/auth.go`（`Logout` L145-174）

### 问题描述
`Logout` 不取消进行中的登录流程（`s.cancel`），也不停止仍在使用该会话的下载任务；只回调了 `chatSvc.Stop`。登出后进行中的下载会以已删除的会话继续跑直到报错，错误信息对用户不可解释。

### 修复方案
- `Logout` 开头调用 `CancelLogin()`。
- 对 engine 中运行中的任务给出"先停止下载再登出"的显式约束或自动暂停。

### 验收标准
- 登出时进行中的登录流程被取消，下载任务被安全暂停或阻止登出。

---

## SVC-04 [Medium] 缩略图/预览走串行队列导致画廊加载线性劣化

- 类别：架构 / 性能
- 涉及文件：`src/internal/services/media_handler.go`、`src/internal/services/chat.go`、`src/internal/services/thumb.go`（L128、L219-241）

### 问题描述
一页媒体列表产生几十个 `/media/thumb` 并发 HTTP 请求，每个排入同一个无缓冲 jobs 通道并带 3 分钟超时挂起；preview 大图通过 `downloadSmallFile` 64KB/次分块串行下载，一张几 MB 的图占用队列几十个往返。大量 HTTP handler goroutine 长时间阻塞在 `jobs <- j` 上。

> 与 ARC-04 / SVC-01 同源，此处从服务层视角登记。

### 修复方案
见 ARC-04；至少将 thumb 请求超时降到 15-30s，前端做加载占位，防止 goroutine 长时间堆积。

---

## SVC-05 [Medium] applyDialogPeers 错误被吞 + 逐条 Apply 放大写入

- 类别：错误处理 / 性能
- 涉及文件：`src/internal/services/chat.go`（约 L454）

### 问题描述
`_ = applyDialogPeers(...)` 在循环内逐 dialog 调用，错误静默丢弃。access hash 落盘失败时用户后续 `ListMedia` 会得到"解析 peer 失败"却无法从日志追溯根因；且每个 dialog 一次 `manager.Apply`（一次 kv 事务），几百个对话就是几百次写。

### 修复方案
- 将整页 `users/chats` 批量 `manager.Apply` 一次（Apply 接受切片）。
- 失败至少 `logChat.Warnf` 记录。

### 验收标准
- 大量对话加载时 kv 写次数显著下降；peer 落盘失败有日志可查。

---

## SVC-06 [Medium] logging.yaml 每次启动静默覆盖并劫持配置

- 类别：配置 / 业务逻辑
- 涉及文件：`src/main.go`（约 L44-52）

### 问题描述
数据目录下存在 `logging.yaml` 时，每次启动都会覆盖 config.yaml 中的日志配置并持久化（`_ = cfg.Update(settings)` 错误还被忽略）。用户在设置页修改日志配置 → 重启 → 被 logging.yaml 静默打回，且解析失败（`perr`）完全静默，用户不知道自己的 yaml 写错了。

### 修复方案
- 解析失败记 Warn 日志；覆盖行为至少打 Info 说明来源。
- 考虑"导入一次后重命名/删除 logging.yaml"的一次性语义，避免永久劫持。

### 验收标准
- logging.yaml 解析失败时有明确日志；设置页修改的日志配置不再被静默打回。

---

## SVC-07 [Medium] auth 服务与相关服务测试覆盖空白

- 类别：代码质量 / 可测试性
- 涉及文件：`src/internal/services/auth.go`（530 行，零测试）、`download.go`、`settings.go`、`logsvc.go`、`media_handler.go`、`config` 包

### 问题描述
现有测试（chat_test / thumb_test / thumbcache_test）质量不错，但 `auth.go`（最复杂的状态机）**零测试**——AUTH-01 的代际竞态正是缺测试的区域；`download.go` / `settings.go` / `logsvc.go` / `media_handler.go` / `config` 包均无测试。`fetchDialogs` / `scanMedia` / `fetchThumb` 依赖 `*tg.Client` 具体类型而非接口，无法注入 mock。

### 修复方案
- auth 的通道/取消逻辑抽出为可注入时钟与假 client 的状态机单测。
- `fetchDialogs` 等改为依赖最小接口（只声明用到的 `MessagesGetDialogs` 等方法）以便打桩。

---

## SVC-08 [Low] 配置持久化失败被静默吞掉

- 类别：错误处理
- 涉及文件：`src/internal/services/auth.go`（`saveUser` 约 L476）；`src/main.go`（约 L50）

`_ = s.cfg.Update(st)` / `_ = cfg.Update(settings)`：配置持久化失败被静默吞掉，登录成功但重启后登录态展示丢失，无日志可查。建议至少 `logAuth.Warnf`。

---

## SVC-09 [Low] begin() 的 error 返回值恒为 nil（死代码）

- 类别：代码质量
- 涉及文件：`src/internal/services/auth.go`（`begin` L313；调用方 `StartCodeLogin` L84-87、`StartQRLogin` L96-99）

`begin()` 声明返回 `(context.Context, error)` 但 error 恒为 nil，是死代码，误导调用方写了永不触发的错误分支。建议去掉 error 返回值。

---

## SVC-10 [Low] keygen.New("session") 魔法字符串耦合 core 内部布局

- 类别：耦合
- 涉及文件：`src/internal/services/auth.go`（约 L155）

魔法字符串 `"session"` 依赖 tdl core 内部 key 布局，core 升级改 key 时这里静默失效（登出删不掉会话）。建议在 core 侧寻找/包一层公开常量，或加集成测试断言 key 一致性。

---

## SVC-11 [Low] openDirectory 子进程未 Wait 产生僵尸进程

- 类别：资源管理
- 涉及文件：`src/internal/services/helpers.go`（`openDirectory` L25-42）

`exec.Command(...).Start()` 后不 `Wait()`：Unix 系（darwin/xdg-open 分支）子进程退出后成为 zombie 直至父进程退出，进程句柄未释放。建议 `Start()` 成功后 `go cmd.Wait()`。

---

## SVC-12 [Low] ListLogFiles 用 Contains(".log") 会误匹配

- 类别：代码质量
- 涉及文件：`src/internal/services/logsvc.go`（`ListLogFiles` 约 L56）

`strings.Contains(e.Name(), ".log")` 会匹配 `foo.log.tmp`、`.logx` 等。建议用 `filepath.Ext(name) == ".log"` 或前缀+后缀白名单。

---

## SVC-13 [Low] 默认下载目录在 home 取失败时变成相对路径

- 类别：健壮性
- 涉及文件：`src/internal/config/config.go`（`defaultSettings` L134-146）

`os.UserHomeDir()` 错误被忽略，home 为空时 `DownloadDir` 变成相对路径 `Downloads\tdl-ui`（相对进程 CWD），下载文件落点不可预期。建议 home 取失败时回落到 `dataDir` 下的 downloads 子目录。

---

## SVC-14 [Low] config.Update 无上限钳制、Proxy 不校验格式

- 类别：校验
- 涉及文件：`src/internal/config/config.go`（`Update` L179-206）

只做了下限兜底，无上限钳制：前端可传 `Threads=100000`、`PoolSize=99999`；`Proxy` 字符串完全不校验，非法值要等到建连时才以晦涩的 gotd 错误暴露。建议 clamp 到合理上限（如 Threads≤16、PoolSize≤64），Proxy 用 `url.Parse` + scheme 白名单预校验。

---

## SVC-15 [Low] media_handler 把内部错误链原样发给前端

- 类别：信息暴露
- 涉及文件：`src/internal/services/media_handler.go`（约 L41）

`http.Error(w, err.Error(), 500)` 把内部错误链（含 kv 路径、gotd 错误）原样发给前端。虽是本地 webview 风险有限，仍建议返回固定文案、详情走日志（可将 Debugf 提升为 Warnf）。

---

## SVC-16 [Low] chat.go 的 Limit>200 静默重置为 50（语义反直觉）

- 类别：代码质量
- 涉及文件：`src/internal/services/chat.go`（约 L171-173）

`Limit > 200` 时静默重置为 50 而非钳到 200，要更多反而得到更少。建议 `if q.Limit > 200 { q.Limit = 200 }`。

---

## SVC-17 [Low] 魔法数字分散各处

- 类别：可维护性
- 涉及文件：`chat.go`（L139 120s、L175 90s、L635 maxScan=2000、L388/L706 batchSize=100）、`thumb.go`（L128 3min、L220 64KB）、`logsvc.go`（L76 5000 行）

分散的超时/批量/上限常量建议集中为具名常量便于调优。

---

## SVC-18 [Low] ClearCache 与 thumbCache 写入存在竞争

- 类别：并发边界
- 涉及文件：`src/internal/services/settings.go`（`ClearCache` L41-55）

`os.RemoveAll` 与 thumbCache 进行中的 `writeFileAtomic` 存在竞争：删除瞬间 in-flight 的 rename 可能失败或把新文件写进刚删的目录。建议 ClearCache 经由 thumbCache 提供的方法在 `mu` 保护下清理，或接受偶发失败并记日志。

---

## SVC-19 [Low] events.go 常量文档注释与实际名不一致、缺 Unbind

- 类别：事件契约
- 涉及文件：`src/internal/events/events.go`（L15-16）

常量 `Task` 的文档注释写的是 "TaskUpdate ... payload 见 TaskUpdate"，与实际常量名不一致；`Emitter` 无 Unbind，shutdown 后 Emit 使用已取消的 ctx（Wails 内部会忽略，但语义上应在 OnShutdown 解绑）。

---

## SVC-20 [Low] writeFileAtomic 在 Windows 上 rename 到已存在目标会失败

- 类别：Windows 兼容
- 涉及文件：`src/internal/services/thumbcache.go`（`writeFileAtomic` L76-85）

Windows 上 `os.Rename` 到已存在的目标会失败。进程内 singleflight 基本排除并发写同一 key，但多开应用时理论上存在窗口。建议 rename 失败且目标已存在时视为成功（重读目标）。

---

## 正面结论（服务层，勿误改）

- 依赖全部走构造函数注入，测试注入点（`userHomeDir` / `connectTimeout` / `runClient`）设计良好。
- `thumbCache` 的 singleflight + 原子写、`ensureStarted` 的防死锁设计（先 close(dead) 再抢锁）体现了对并发细节的认真思考，并有回归测试覆盖。
- chat/thumb/thumbcache 的纯函数测试质量良好。
