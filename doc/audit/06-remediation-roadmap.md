# 06 修复升级路线图（供 AI 编程代理执行）

> 本文档把前 5 章的发现按优先级编排为四个阶段。每个条目引用发现 ID，标注依赖、改动面、回归风险与验证方法。
> AI 代理执行任一条目前，必须先阅读该 ID 对应报告章节的完整描述。

---

## AI 代理执行须知（先读这段）

1. **单次修复引用一个或一组相关的发现 ID**（如"修 ENG-01"或"修 FE-08 + FE-09"）。
2. **修改前先读对应报告节**：ARC/DEP 见 `01-architecture.md`，AUTH/SVC 见 `02-backend-services.md`，ENG/STO 见 `03-engine-store.md`，SCR/LOG 见 `04-scripting-logging.md`，FE 见 `05-frontend.md`。
3. **行号仅供参考，以符号名定位**——报告写作后代码可能已变动。
4. **禁止向后兼容层**：本项目要求直接改造，不保留兼容旧行为的分支。
5. **勿误改"正面结论"列出的代码**（如 SQL 参数化、日志 sink 并发设计、v-html 转义），这些是已验证正确的部分。
6. **修复后必须运行的验证命令**：
   - 后端：`cd src && go build ./... && go test ./... -race`
   - 前端：`cd src/frontend && pnpm build`（引入 vue-tsc 门禁后会含类型检查）
   - 提交：使用 `smart-commit` 技能，禁止手动 git commit。
7. **前端 UI 验证强制使用 chrome-devtools mcp**（竞态、主题、错误提示类改动）。

---

## P0 — 稳定性止血（最高优先，多为小改动、高故障成本）

这些问题会导致进程崩溃、数据损坏、安全越界或错误数据展示，应优先且可独立修复。

| ID | 严重度 | 一句话 | 改动面 | 回归风险 |
|---|---|---|---|---|
| ENG-01 | Critical | Finished() 返回 map 引用致并发崩溃 | 单函数改为返回拷贝 | 极低 |
| STO-01 | High | 迁移 user_version 在事务外 + 无 IF NOT EXISTS | migrateV1 一处 | 低 |
| SCR-01 | High | 脚本沙箱黑名单改白名单 | sandboxSymbols 一处 | 中（需确认脚本契约所需包） |
| AUTH-01 | High | 登录 cancel 无代际归属 | begin/finish + 事件判定 | 中 |
| AUTH-02 | High | ImportDesktopSession 假回滚毁会话 | 导入路径加备份/写回 | 中 |
| FE-01 | High | 媒体分页无 epoch 乱序污染 | loadMore/resetMedia | 中 |
| FE-02 | High | 登录提交 fire-and-forget | LoginPage 两个函数 | 低 |

### 验证方法
- ENG-01：多文件并发下载任务 + `go test ./... -race`，确认无 `concurrent map` 崩溃。
- STO-01：手工将 tasks.db 的 `user_version` 置 0 后重启，应用正常打开库。
- SCR-01：编写调用 `os.StartProcess`/`net.Dial`/`syscall.*` 的脚本，校验被拒。
- AUTH-01：快速在验证码/二维码登录间切换，新流程不被旧流程取消。
- AUTH-02：已登录下导入无效 Desktop 会话失败，原会话仍可用。
- FE-01：chrome-devtools mcp 下快速切换对话，右面板只显示当前对话数据、无重复 key 警告。
- FE-02：输错验证码提交，出现错误提示且输入不被清空。

---

## P1 — 生命周期与资源纪律

长生命周期 GUI 下会累积的资源泄漏、竞态与关闭编排问题。

| ID | 严重度 | 一句话 | 依赖 |
|---|---|---|---|
| ARC-02 | High | OnShutdown 关闭编排缺失 | 需 ChatService/taskManager 提供 StopAndWait |
| ARC-01 | High | 会话所有权模型 + OnLoginSuccess 钩子 | 与 ARC-02 协同 |
| SVC-03 | Medium | Logout 不取消登录/下载 | 依赖 AUTH-01 |
| ENG-02 | Medium | elem 通道 fd/.tmp 泄漏 | 独立 |
| ENG-03 | Medium | Remove/Resume TOCTOU 僵尸下载 | 独立 |
| ENG-04 | Medium | Queued 态不可暂停/取消 | 独立 |
| SCR-02 | Medium | yaegi 闭包并发调用 | 独立 |
| SCR-03 | Medium | 脚本超时逐文件泄漏 goroutine | 独立 |
| FE-04 | Medium | MediaPreview 监听未清理 | 独立 |
| FE-05 | Medium | fileStates/thumbFailed 只增不减 | 独立 |
| FE-06 | Medium | LogsPage watch 乱序 | 独立 |
| FE-07 | Medium | 深链 route.params 不响应 | 独立 |
| FE-10 | Medium | 任务/脚本操作错误处理裸奔 | 独立 |
| LOG-01 | Medium | sink 丢弃结构化字段 | 独立 |

### 验证方法
- ARC-02：下载进行中关闭窗口，进度完整持久化、无"database is closed"刷屏；退出前 `runtime.NumGoroutine()` 无残留。
- ENG-02：反复暂停/恢复任务，fd 数量不增长、无孤儿 `.tmp`。
- ENG-03：并发 Remove+Resume 压测无失控后台下载。
- SCR-02/SCR-03：`go test ./... -race` 并发脚本调用无竞争；死循环脚本超时后被熔断。
- FE-04/FE-05：chrome-devtools mcp 观察监听器与集合，切换/卸载后正确释放。

---

## P2 — 架构重构（需先设计，改动面大）

| ID | 严重度 | 一句话 | 收益 |
|---|---|---|---|
| ARC-03/ARC-04/SVC-01 | High/Medium | invoke ctx 传播 + 队列拆分 | 取消语义完整、画廊加载不再线性劣化 |
| ENG-05 | Medium | task.go 拆分 + 状态机测试 | 可测试性、可维护性 |
| ARC-06/FE-03 | High | api.ts 切换 wailsjs 生成绑定 | 类型安全从"声明"变"强制" |
| FE-11 | Medium | ChatsPage 拆分 + useMediaPager | 一次消解 FE-01/FE-11/FE-16 |

### 验证方法
- SVC-01：调用方超时后后台不再继续执行已放弃请求（加日志或 pprof 验证）。
- ENG-05：拆分后 `go test ./...` 通过，新增状态机转换与断点续传单测。
- FE-03：后端结构体字段改名后，前端引用处 `vue-tsc` 报错。
- FE-11：拆分后 FE-01 竞态消失、右面板可缓存。

---

## P3 — 质量加固

| ID | 一句话 |
|---|---|
| FE-13 | 引入 vue-tsc 构建门禁 + eslint 体系 |
| FE-08/FE-09 | 主题状态收敛单一源 + ToggleSwitch 控件 |
| FE-14 | 硬编码颜色改 CSS 变量 |
| FE-15 | fmtSize 等抽 utils/format.ts + SearchBox 组件 |
| FE-17~FE-26 | HMR 订阅清理、错误处理补齐、防抖、魔法数字常量化等 |
| ARC-05 | go.mod replace 治理 + go 版本降 patch + govulncheck |
| ENG-06~ENG-13 | 进度语义、锁范围、断点表化、ID 生成、加载可见性等 |
| STO-02~STO-04 | ImportLegacyJSON 去重、Context 化仓储、synchronous(NORMAL) |
| SVC-04~SVC-20 | 队列超时、批量 Apply、logging.yaml、config 校验、魔法数字等 |
| LOG-02/LOG-03 | 锁内轮转 I/O、Program Files 不可写探测 |
| SCR-04/SCR-05 | 脚本快照、Windows 保留名 |

### 验证方法
- FE-13：`pnpm build` 会先跑 `vue-tsc --noEmit`，故意引入类型错误应阻断构建。
- FE-08/FE-09：chrome-devtools mcp 验证侧栏 ToggleSwitch 与设置页状态一致、系统深浅色切换时图标同步。
- ARC-05：干净 clone（含 submodule 初始化）后 `go build ./...` 成功，`govulncheck` 可运行。

---

## 阶段执行顺序建议

1. P0 全部（可并行，互不依赖除 AUTH-01→SVC-03）。
2. P1 中 ARC-02 + ARC-01 需一起设计（会话生命周期）；其余 P1 可并行穿插。
3. P2 按 FE-11、ENG-05、FE-03、队列重构 逐项进行，每项独立成 PR。
4. P3 作为收尾与持续改进，其中 FE-13（构建门禁）建议尽早做，为后续所有前端改动提供保护网。
