# 05 修复路线图（P0-P3）

> 读者为 AI 编程代理。本路线图不含任何时间估算；每条以 改动面 / 依赖 / 回归风险 表达轻重缓急，执行顺序按优先级与依赖关系排定。

## AI 代理执行须知（先读后动）

1. **禁止向后兼容层**：修复即直接改到位，不保留旧行为开关、不写 deprecated 包装、不加兼容分支。
2. **`ref/tdl` 子模块只读**：任何修复不得改动 `ref/tdl` 下的文件；需要引擎侧行为变化时在 `src/internal/engine` 包装层实现。
3. **提交方式**：修复完成后由用户发起 **smart-commit** 技能提交，代理不得自行 `git commit`。
4. **前端验证**：涉及前端的修复用 **chrome-devtools mcp** 做交互验证（console 无错误、network 无异常请求），不以"编译通过"替代行为验证。
5. **每项发现的"正面结论（勿误改）"清单是硬约束**：修复前先读所属分报告末尾清单，确认改动不触碰其中条目。
6. **验证命令基线**：Go 侧 `cd src && go vet ./... && go test ./...`（本机无 CGO，**不要**尝试 `-race`）；前端 `cd src/frontend && pnpm build`（含 vue-tsc 门禁）。
7. **文档同步**：修复完成后在本目录 README.md 汇总表中为对应 ID 追加"已修复"标注（不删除原始条目），CHANGELOG.md 按项目现有格式记录。
8. **回归警惕**：ARC-001/ARC-002/LOGIC-002/CODE-001/CODE-003 均为旧轮修复的残留窗口或已知限制，修复时对照旧文档（`doc/audit/2026-07-29-full-audit/`）中对应条目（ARC-02/SVC-18/SCR-03/SVC-20），确保新修复不推翻旧修复已解决的部分。

## P0 —— 无

本轮无 Critical/High 发现，无需立即中断其他工作的修复项。

## P1 —— 并发与生命周期加固（建议一并处理，改动集中在关闭/取消路径）

建议执行顺序如下（存在证据链依赖：先做 ARC-002 的存储防御，再做上游生命周期修复，验证时日志更干净）：

| 顺序 | ID | 摘要 | 改动面 | 依赖 | 回归风险 |
|---|---|---|---|---|---|
| 1 | ARC-002 | store 增加 closed 标志 + ErrStoreClosed，engine 持久化降级日志 | `store/store.go`、`engine/persist.go`、`engine/resume_repo.go` | 无 | 低：仅新增防御分支，不改正常路径 |
| 2 | ARC-001 | ChatService 生命周期句柄提前发布，StopAndWait 覆盖建连阶段 | `services/chat.go`（`start`/`runTelegramClient`） | 无（与 ARC-002 互补） | 中：涉及 dead/cancel 发布时序，须防 double close；勿破坏三队列与 mergeContext（正面结论） |
| 3 | LOGIC-001 | auth 登录流程增加 flowDone join，Logout/prepareLogin 等待退出 | `services/auth.go` | 无 | 中：勿改动 begin/finish 代际模式本身（已验证正确） |
| 4 | LOGIC-002 | thumbCache 引入 epoch 代际，写盘入锁并校验 | `services/thumbcache.go` | 建议先于 CODE-003（同文件，合并改动减少冲突） | 低：缩略图缓存自愈性强，最坏丢缓存 |
| 5 | CODE-001 | 脚本熔断态跳过语义核查 + 泄漏可观测化 + 文档 | `script/engine.go`、脚本说明文档 | 无 | 低：以日志与文档为主，逻辑改动小 |
| 6 | FUNC-001 | ChatsPage resetThumbClick 三调用点接入 | `frontend/src/pages/ChatsPage.vue` | 无 | 低：纯前端状态清理 |

P1 完成门禁：`go vet ./... && go test ./...` 全绿 + `pnpm build` 通过 + ARC-001/ARC-002 的人工退出验证（下载中/建连中关窗，日志无 `database is closed` / bolt 关闭后写入）。

## P2 —— 边界加固与判据收紧

| ID | 摘要 | 改动面 | 依赖 | 回归风险 |
|---|---|---|---|---|
| LOGIC-003 | 登录态启动对账（kv 为权威源），Status 增加 sessionPresent | `services/auth.go`、`main.go`、`frontend/src/stores/auth.ts`、`LoginPage.vue` | 建议在 LOGIC-001 之后（幽灵会话来源先堵住） | 中：涉及启动路径与登录 UI，需 chrome-devtools 验证登录页三种状态 |
| CODE-003 | writeFileAtomic 判据收窄为错误类型 | `services/thumbcache.go` | 与 LOGIC-002 同文件，建议同批 | 低 |
| CODE-002 | LogsPage 防抖定时器卸载清理 | `frontend/src/pages/LogsPage.vue` | 无 | 极低：单行 |
| FUNC-002 | MediaPreview pendingAdvance 代际复位 | `frontend/src/components/MediaPreview.vue` | 无 | 低：勿破坏 FE-26 主路径（正面结论） |

## P3 —— 可配置化与结构优化（无用户可感缺陷，按需执行）

| ID | 摘要 | 改动面 | 依赖 | 回归风险 |
|---|---|---|---|---|
| ARC-003 | worker 数 / 进度间隔配置化（零值回退现常量） | `config/`、`services/chat.go`、`engine/progress.go` | 无 | 低：默认行为不变即无回归 |
| ARC-004 | engine 对 store 依赖接口化（仅接口，不建仓储层） | `engine/manager.go`、`engine/persist.go`、`engine/deps` 定义处 | 建议在 ARC-002 之后（接口需包含 ErrStoreClosed 语义） | 低：编译期收敛，main.go 零改动为验收条件 |
| CODE-004 | 前端路由懒加载 + vendor 拆 chunk | `frontend/src/router/`、`vite.config.ts` | 无 | 中：需逐页路由验证无动态导入 404 |

## 引用完整性

本路线图引用的全部 ID：ARC-001、ARC-002、ARC-003、ARC-004（见 [01-architecture.md](01-architecture.md)）；FUNC-001、FUNC-002（见 [02-business-function.md](02-business-function.md)）；LOGIC-001、LOGIC-002、LOGIC-003（见 [03-business-logic.md](03-business-logic.md)）；CODE-001、CODE-002、CODE-003、CODE-004（见 [04-code-quality.md](04-code-quality.md)）。共 13 项，与 [README.md](README.md) 汇总表一一对应。
