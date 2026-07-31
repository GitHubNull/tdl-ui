# 03 业务逻辑类发现（LOGIC）

> 读者为 AI 编程代理。定位以符号名为准，行号仅供参考。禁止时间估算，优先级仅用 P0-P3。
> 本轮 ID 独立于 2026-07-29 轮次；"关联旧 {ID}" 指上一轮文档中的条目。

本类共 3 项：Medium 2、Low 1。

---

## LOGIC-001 [Medium] AuthService.Logout 不等待登录流程 goroutine 退出即删除会话

- 类别：业务逻辑 / 状态管理、异常处理流程
- 涉及文件：`src/internal/services/auth.go` — `AuthService.Logout`、`AuthService.CancelLogin`、`AuthService.begin`、`AuthService.finish`，以及 `runCodeLogin`/`runQRLogin` 流程 goroutine

### 问题描述（含影响分析）

`Logout()` 的取消手段是调用 `CancelLogin()`（仅执行 `s.cancel()` 并置 nil），随后**立即**继续删除 kv 中的会话与 App 标记、清空 config 登录态字段。但 `s.cancel()` 只是向登录流程 goroutine（`runCodeLogin`/`runQRLogin`）发出取消信号，**不等待其真正退出**。竞争窗口：

1. 登录流程正处于 gotd 客户端已连接、即将写入会话数据的阶段。
2. `Logout()` 发出 cancel 后立刻 `kvd.Delete(session)`。
3. 登录 goroutine 的 gotd 层尚未观察到 ctx 取消（网络调用中），完成后回写会话数据。
4. 结果：登出"成功"返回，但 kv 中又出现一份会话 → 重启后应用认为已登录，处于**幽灵登录态**（config 中 `LoggedInUserID=0` 而 kv 有会话，两源不一致，另见 LOGIC-003）。

反向顺序（先删后写被 cancel 中断）则表现为登出正常，属幸运时序。窗口取决于网络时延，用户在"登录转圈中途点登出"这一常见操作即可触发，定级 Medium。

### 根因分析

取消是异步信号而非同步屏障：`CancelLogin` 缺少"等待流程 goroutine 退出"的 join 语义，`Logout` 把"已发信号"当作"已停止"。

### 修复方案

为登录流程补一个可等待句柄：

1. `AuthService` 增加 `flowDone chan struct{}` 字段（与 `cancel`/`gen` 同受 `s.mu` 保护）。`begin()` 中创建新的 `flowDone`；流程 goroutine 退出路径（`finish` 或 defer）`close(flowDone)`。
2. 新增内部方法 `cancelLoginAndWait(timeout time.Duration)`：先 `CancelLogin()`，再对快照到的 `flowDone` 做 `select { case <-done: case <-time.After(timeout) }`（done 为 nil 时直接返回）。
3. `Logout()` 与 `prepareLogin()` 改调 `cancelLoginAndWait(3*time.Second)`；超时仍未退出则返回错误"登录流程未能及时终止，请稍后重试"，**不继续删除会话**（宁可让用户重试，不留幽灵态）。
4. 保持 `begin`/`finish` 现有代际模式不变（该模式已验证正确，见下方正面结论 2）。

### 实施步骤与优先级（P1）

1. 修改 `auth.go`：加 `flowDone`、`cancelLoginAndWait`，改 `Logout`/`prepareLogin` 调用点。
2. 核对 `runCodeLogin`/`runQRLogin` 的所有退出路径（成功/失败/取消）都会关闭 `flowDone`（建议在 goroutine 顶层 defer）。
3. 跑 `cd src && go test ./internal/services/`。

### 验收标准（测试验证方法）

- 新增测试：启动一个阻塞在验证码等待的登录流程，调用 `Logout()`，断言其阻塞至流程 goroutine 退出后才删除会话；流程 goroutine 故意不退出时断言 `Logout` 超时返回错误且 kv 会话未被删除。
- 人工验证：开始扫码登录 → 二维码显示期间点登出 → 重启应用为未登录态（无幽灵登录）。
- `go test ./internal/services/` 全绿。

---

## LOGIC-002 [Medium] thumbCache.Clear 与锁外 fetch/写盘竞争：清空后幽灵文件重现、Windows 下 Clear 可能失败

- 类别：业务逻辑 / 数据一致性、并发（关联旧 SVC-18：锁保护已加，但覆盖面不含锁外写盘段）
- 涉及文件：`src/internal/services/thumbcache.go` — `thumbCache.Get`、`thumbCache.Clear`、`writeFileAtomic`；`src/internal/services/settings.go` — 清空缓存入口

### 问题描述（含影响分析）

旧 SVC-18 修复让 `Clear()` 持锁执行 `RemoveAll + MkdirAll`，但 `Get()` 的 singleflight 拉取与 `writeFileAtomic` 落盘**在锁外**执行。竞争序列：

1. goroutine A：`Get()` 未命中 → 进入 singleflight → 锁外 `fetch()` 拉取 Telegram 缩略图。
2. goroutine B：用户点设置页"清空缓存" → `Clear()` 持锁删目录、重建空目录、返回"已清空"。
3. goroutine A：fetch 完成，`writeFileAtomic` 把缩略图写进刚清空的目录。

影响两点：(a) 用户视角"清空缓存"后目录立即又出现文件（幽灵文件），清理语义打折但数据本身无损（rename 原子，内容完整可用）；(b) Windows 特有：若 `RemoveAll` 执行时 A 正持有目录内 tmp 文件句柄或正 rename，`RemoveAll` 可能返回 `Access is denied`，`Clear` 整体失败报错给前端。无数据损坏路径，定级 Medium。

### 根因分析

singleflight 的设计目标是去重拉取、避免锁内长 I/O（这是对的），但由此产生的"锁外写盘段"未纳入 Clear 的互斥域；缺一个"清空代际"概念让在途写盘知道自己已过期。

### 修复方案

引入代际号使在途写盘自我作废，不把网络 I/O 拉回锁内：

1. `thumbCache` 增加 `epoch uint64` 字段（受现有 mu 保护）。`Clear()` 持锁时 `epoch++`。
2. `Get()` 进入 singleflight 前持锁快照 `e := c.epoch`；fetch 完成后、写盘前再持锁校验 `if c.epoch != e`——不等则**跳过 writeFileAtomic**，仅把字节返回给本次请求方（内存返回不落盘）。
3. 写盘调用挪入该持锁校验块内（写单个小文件为本地磁盘 I/O，毫秒级，持锁可接受；如不接受，可在校验通过后锁外写+写后二次校验删除，复杂度更高，不推荐）。
4. Windows `Access is denied` 场景随之消解：写盘与 Clear 互斥后，`RemoveAll` 不再与 rename 并发。

### 实施步骤与优先级（P1）

1. 修改 `thumbcache.go`：加 epoch、快照、校验，写盘入锁。
2. 跑 `cd src && go test ./internal/services/`（如有 thumbcache 测试）并新增并发测试。

### 验收标准（测试验证方法）

- 新增测试：并发 N 个 `Get`（fake fetch 睡 50ms）+ 中途 `Clear`，结束后断言缓存目录为空或仅含 Clear 之后代际写入的文件；无 panic、无 `Access is denied`（Windows runner 上验证）。
- 人工验证：对话页快速滚动加载缩略图的同时点"清空缓存"，设置页无报错，缓存目录不出现清空前发起的旧文件。

---

## LOGIC-003 [Low] 登录态双源（kv 会话 + config 字段）无一致性兜底

- 类别：业务逻辑 / 状态管理、数据一致性
- 涉及文件：`src/internal/services/auth.go` — `saveUser`（config 落盘失败仅 Warn）、`Status`/启动时登录态判断逻辑、`Logout`

### 问题描述（含影响分析）

登录态存在两个事实源：bolt kv 中的 gotd 会话（真实凭据）与 `config.Settings` 的 `LoggedInUserID`/`LoggedInUsername`（展示用）。两者在三条路径上可能失步：

1. `saveUser` 中 `cfg.Update(st)` 失败只记 Warn → kv 有会话、config 无登录态。
2. LOGIC-001 的幽灵会话 → kv 有会话、config 已清零。
3. 用户手动删除数据目录下 config 文件（保留 kv）→ 同上。

失步后 UI 显示未登录，但对话/下载功能实际可用（引擎直接用 kv 会话），或反之 UI 显示已登录但操作报未授权，用户无从自愈（除非重新登录覆盖）。频率低、有绕行手段（重新登录），定级 Low。

### 根因分析

双源缓存（config 是 kv 的展示投影）没有以单一权威源做启动校准：启动时既不校验 kv 会话有效性，也不据 kv 反推 config。

### 修复方案

以 kv 为权威源，在启动路径做一次轻量对账：

1. 在 `AuthService` 增加 `ReconcileOnStartup()`（或并入现有 `Status()` 首次调用）：检查 kv 中会话 key 是否存在。
2. kv 无会话而 config 有登录态 → 清零 config 字段并落盘（显示未登录，与功能一致）。
3. kv 有会话而 config 无登录态 → 保持 config 不动，但 `Status()` 返回值增加 `sessionPresent bool` 字段，前端 LoginPage 据此提示"检测到本地会话，可直接重连或重新登录"。不自动标已登录（无法从会话反查用户名，避免伪造展示信息）。
4. `main.go` 装配后调用一次（在 `wails.Run` 前或 `OnStartup` 内）。

### 实施步骤与优先级（P2）

1. `auth.go` 实现对账函数；`Status` 返回结构扩展。
2. 前端 `stores/auth.ts` / `LoginPage.vue` 消费 `sessionPresent`。
3. 跑 `cd src && go test ./internal/services/ && cd frontend && pnpm build`。

### 验收标准（测试验证方法）

- 测试：构造"kv 无会话 + config 有登录态"，启动对账后断言 config 字段被清零；构造反向组合，断言 `Status().sessionPresent == true` 且 config 未被改写。
- 人工验证：手删 config 文件保留 kv 后启动，登录页出现"检测到本地会话"提示。

---

## 正面结论（勿误改）

以下业务逻辑实现经本轮验证**正确**，修复上述发现时不得破坏：

1. **导入会话备份回滚**（`auth.go` 导入流程，旧 AUTH-02）：写入新会话前备份旧数据、校验失败恢复备份，逻辑严密。
2. **begin/finish 代际自取消模式**（`auth.go` `begin`/`finish`）：经完整推演正确——新流程 `begin` 先 cancel 旧流程再 `gen++`；旧流程 `finish(旧gen)` 因代际不等提前返回，不影响新流程。子代理"新流程被误取消"的初判**不成立**。建议仅在 `finish` 上补一条注释说明"gen 相等时 cancel 的是本流程自己的 cancel"，防止后续误读。
3. **登出/重登的活跃下载守卫**（`auth.go` `Logout`/`prepareLogin` + `HasActiveDownloads`）：存在进行中下载时明确拒绝，防会话互踩。
4. **ImportLegacyJSON 全链路**（`store/migrate.go`，旧 STO-02）：tasks 表非空跳过、URL `seen` map 去重、解析/导入失败改名 `.failed` 防重复失败循环、成功改名 `.bak`、running/queued 降级 paused——整条链路验证正确。子代理"依赖 INSERT OR IGNORE / 去重缺失"的说法与代码不符。
5. **Remove/Resume TOCTOU 防护**（`engine/manager.go`，旧 ENG-03）：持锁检查 removed 标记，阻断任务复活。
6. **Queued 态可暂停/取消**（`engine/manager.go`，旧 ENG-04）：Pause/Cancel 对 StatusQueued 特殊处理正确。
7. **persistState 单语句同步**（`engine/persist.go`，旧 ENG-10）：状态与计数单条 UPDATE，无乱序窗口。
8. **loadFromStore 降级**（`engine/persist.go`，旧 ENG-13）：消息项加载失败降级 failed 而非静默丢任务。
9. **FE-19 pending 超时兜底**（`src/frontend/src/stores/auth.ts`）：后端既不 reject 也不发事件时避免永久转圈。
10. **FE-18 出错复位收敛**（`stores/auth.ts` + `LoginPage.vue`）：stage 复位在 action 内统一处理。
