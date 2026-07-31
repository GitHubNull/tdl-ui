# 第二轮全库代码审计总览（2026-07-31）

> 本文档面向 **AI 编程代理**：代码定位以符号名为准（行号仅供参考，可能随编辑漂移）；修复方案为可机械执行的步骤；验收标准均可通过命令、断言或可观察行为验证。**全部文档不含任何时间估算字段**，轻重缓急仅以 P0-P3 优先级表达。

## 与上一轮的关系

- 上一轮审计：[`doc/audit/2026-07-29-full-audit/`](../2026-07-29-full-audit/README.md)，约 60 项发现，**已全部修复**。
- 本轮是对**修复后代码**的全新审计：验证修复质量（回归/修复不彻底记为新发现并标注"关联旧 {ID}"），并覆盖修复后新增模块（视频预览、useMediaPager、拆分后的 engine manager/executor/persist、schema v3 等）。
- **本轮 ID 命名空间独立于 2026-07-29 轮次**，编号从 001 起；正文中出现的 `ARC-02`、`SVC-18`、`SCR-03` 等旧格式 ID 均指上一轮文档中的条目。

## 审计范围与方法

- 范围：`src/` 全部 Go 后端（`main.go` + `internal` 9 个包）与 `src/frontend/src/` Vue 前端源码。
- 排除：`ref/tdl` 子模块（只读约束）、`frontend/wailsjs` 生成物、`node_modules`、构建产物。
- 方法：三路并行只读通读（后端服务与装配 / 引擎·存储·脚本·日志 / 前端），每路按架构、并发、业务逻辑、错误处理、资源管理、代码质量六维审查；子代理产出的每项发现均经主流程**逐项对照源码交叉验证**后才收录，严重度以验证结论为准（多项子代理初判被否决或降级，见下文"否决与降级项"）。

## 证据基线（审计时实测）

| 命令 | 结果 |
|---|---|
| `cd src && go vet ./...` | 通过（exit 0，无输出） |
| `cd src && go test ./...` | 全部包通过（config/engine/logging/script/services/store 均 ok，日志：`tmp/audit2-gotest.log`） |
| `cd src && go test ./... -race` | **无法执行**：本机 `CGO_ENABLED=0` 且 `-race requires cgo`；数据竞争类发现均来自人工代码走查，未经 race detector 佐证 |
| `cd src/frontend && pnpm build` | 成功（含 `vue-tsc` 类型门禁）；警告：主 chunk 1.1MB 超过 500kB 阈值（→ CODE-004，日志：`tmp/audit2-febuild.log`） |

## 严重度定义（沿用上一轮）

| 级别 | 含义 |
|---|---|
| Critical | 数据丢失/损坏、崩溃、安全漏洞，正常使用路径可触发 |
| High | 功能错误或资源泄漏，常见场景可触发 |
| Medium | 边界/并发场景下可触发的缺陷，或明确的健壮性缺口 |
| Low | 代码质量、可维护性、性能优化点，不影响正确性 |

## 编号规则

| 前缀 | 类别 | 覆盖 | 报告文件 |
|---|---|---|---|
| ARC-XXX | 架构设计 | 耦合、依赖、模块划分、数据流、并发安全模型、资源管理编排 | [01-architecture.md](01-architecture.md) |
| FUNC-XXX | 业务功能 | 核心功能实现、交互流程、功能完整性、边界条件 | [02-business-function.md](02-business-function.md) |
| LOGIC-XXX | 业务逻辑 | 业务规则、状态管理、数据一致性、事务、异常处理流程 | [03-business-logic.md](03-business-logic.md) |
| CODE-XXX | 代码功能 | 代码质量、性能、内存泄漏、错误处理实现、安全漏洞 | [04-code-quality.md](04-code-quality.md) |

修复路线图见 [05-remediation-roadmap.md](05-remediation-roadmap.md)。

## 发现汇总（按严重度排序）

本轮共 **13 项**发现：Critical 0、High 0、**Medium 6、Low 7**。

| ID | 严重度 | 标题 | 涉及文件 | 优先级 |
|---|---|---|---|---|
| ARC-001 | Medium | ChatService.StopAndWait 存在"建连中"盲区，退出时无法等待未就绪连接（✅ 已修复 2026-07-31） | `src/internal/services/chat.go` | P1 |
| ARC-002 | Medium | engine.Manager.StopAndWait 超时后 run goroutine 逃逸，可能写入已关闭的存储（✅ 已修复 2026-07-31） | `src/internal/engine/manager.go` | P1 |
| FUNC-001 | Medium | ChatsPage 单/双击延时定时器跨对话未清理，切换对话后可误开旧对话预览（✅ 已修复 2026-07-31） | `src/frontend/src/pages/ChatsPage.vue` | P1 |
| LOGIC-001 | Medium | AuthService.Logout 不等待登录流程 goroutine 退出即删除会话（✅ 已修复 2026-07-31） | `src/internal/services/auth.go` | P1 |
| LOGIC-002 | Medium | thumbCache.Clear 与锁外 fetch/写盘竞争：清空后幽灵文件重现、Windows 下 Clear 可能失败（✅ 已修复 2026-07-31） | `src/internal/services/thumbcache.go` | P1 |
| CODE-001 | Medium | 脚本超时熔断后 yaegi goroutine 无法中断，恶意/死循环脚本持续占用 CPU（✅ 已修复 2026-07-31：熔断可观测化+文档固化，执行体不可中断为 yaegi 已知限制） | `src/internal/script/engine.go` | P1 |
| ARC-003 | Low | 并发度与节流参数硬编码（chat worker 数、进度事件间隔）（✅ 已修复 2026-07-31） | `src/internal/services/chat.go`、`src/internal/engine/progress.go` | P3 |
| ARC-004 | Low | engine.Manager 直接依赖 store.Store 具体类型，业务与持久化耦合（✅ 已修复 2026-07-31） | `src/internal/engine/manager.go`、`persist.go` | P3 |
| FUNC-002 | Low | MediaPreview pendingAdvance 在 items 整体重置时未复位，理论上可误前进（✅ 已修复 2026-07-31） | `src/frontend/src/components/MediaPreview.vue` | P2 |
| LOGIC-003 | Low | 登录态双源（kv 会话 + config 字段）无一致性兜底（✅ 已修复 2026-07-31） | `src/internal/services/auth.go` | P2 |
| CODE-002 | Low | LogsPage 搜索防抖定时器无卸载清理（✅ 已修复 2026-07-31） | `src/frontend/src/pages/LogsPage.vue` | P2 |
| CODE-003 | Low | writeFileAtomic 在 Windows rename 失败时以"目标存在"误判成功（✅ 已修复 2026-07-31） | `src/internal/services/thumbcache.go` | P2 |
| CODE-004 | Low | 前端构建产物单 chunk 1.1MB，无代码分割（✅ 已修复 2026-07-31） | `src/frontend/vite.config.ts`、`src/frontend/src/router/` | P3 |

## 统计

| 类别 | Critical | High | Medium | Low | 小计 |
|---|---|---|---|---|---|
| ARC | 0 | 0 | 2 | 2 | 4 |
| FUNC | 0 | 0 | 1 | 1 | 2 |
| LOGIC | 0 | 0 | 2 | 1 | 3 |
| CODE | 0 | 0 | 1 | 3 | 4 |
| **合计** | **0** | **0** | **6** | **7** | **13** |

上一轮关联：ARC-001/ARC-002 关联旧 ARC-02（关闭编排修复不彻底的残留窗口）、LOGIC-002 关联旧 SVC-18、CODE-001 关联旧 SCR-03（已知限制仍在）、CODE-003 关联旧 SVC-20。

## 否决与降级项（子代理初判 → 验证结论）

以下子代理报告的发现经逐项对照源码验证后**否决或大幅降级**，记录在此防止后续轮次重复误报：

| 子代理初判 | 验证结论 |
|---|---|
| "SVC-01 ctx 传播不完全 / done channel 错误丢失"（High） | **否决**。`chatJob.done` 为每 job 独享的 buffered(1) channel（`chat.go` `submit`），发送永不阻塞不丢失；`mergeContext` 用 `context.AfterFunc` 实现且 stop 函数在 defer 中调用，无泄漏 |
| "AUTH-01 finish 代际窗口导致新流程被误取消"（Medium） | **否决**。`begin`/`finish` 的代际自取消模式经推演正确：旧流程 `finish(旧gen)` 因 `s.gen != gen` 提前返回，不会碰新流程的 cancel。仅建议补注释说明该模式（不单列发现） |
| "SCR-07 沙箱白名单含 math/rand 可被 DoS"（Critical） | **降级并合并**。math/rand 与纯 for 循环耗 CPU 无本质区别，callWithGuard 超时熔断已限制影响面；真实缺口是超时后 goroutine 不可中断 → 并入 CODE-001（Medium） |
| "STO-05 ImportLegacyJSON 依赖 INSERT OR IGNORE、去重不在事务外"（Medium） | **基本否决**。实际代码（`migrate.go` `importLegacyRecords`）用 `seen` map 在插入前显式去重，不依赖唯一索引冲突；整体事务回滚 + `.failed` 改名（`markLegacyFailed`）正是 STO-02 设计的防重试现场保留，行为正确 |
| "LOG-04 emit 回调可能嵌套死锁"（Low） | **否决**。`sink.go` 中文件写与 emit 均在临界区外：`Write` 先解锁再写文件，`flush` 先取批次解锁再调用 emit，无嵌套锁路径 |
| "CONFIG-01 CacheDir/TempDir RWMutex 读不一致"（Low） | **否决**。RLock 下对 settings 快照单字段读取 + 空值回退是自洽的原子读，不同 goroutine 读到不同代的配置属预期语义 |
| "ENG-08 iter 网络 I/O 阻塞进度"（Low） | **不立项**。单消费者迭代器语义即如此，子代理自身结论亦为"接受设计特性" |
| "TasksPage watch 依赖问题"（前端 M2） | **否决**。经查 watch 依赖与清理逻辑（FE-25）实现正常 |

## 阅读约定

- 每份分报告末尾的"正面结论（勿误改）"段落列出**经验证正确**的关键实现，修复任何发现时不得破坏这些点。
- 修复执行顺序与约束见 [05-remediation-roadmap.md](05-remediation-roadmap.md) 的"AI 代理执行须知"。
