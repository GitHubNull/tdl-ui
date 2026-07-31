# 04 代码功能类发现（CODE）

> 读者为 AI 编程代理。定位以符号名为准，行号仅供参考。禁止时间估算，优先级仅用 P0-P3。
> 本轮 ID 独立于 2026-07-29 轮次；"关联旧 {ID}" 指上一轮文档中的条目。

本类共 4 项：Medium 1、Low 3。

---

## CODE-001 [Medium] 脚本超时熔断后 yaegi goroutine 无法中断，恶意/死循环脚本持续占用 CPU

- 类别：代码功能 / 资源泄漏、安全（关联旧 SCR-03：超时保护已实现，goroutine 不可中断是遗留已知限制；吸收子代理 SCR-07 关于 math/rand 的初判——经验证该点本身不构成独立漏洞，纯循环同样耗 CPU，实质缺口在此项）
- 涉及文件：`src/internal/script/engine.go` — `callWithGuard`、熔断逻辑、白名单 `Use` 段

### 问题描述（含影响分析）

`callWithGuard` 对每次脚本函数调用施加超时（超时后熔断该函数不再调用），但 yaegi 解释器**不支持中断执行中的 goroutine**：超时触发后，承载脚本调用的 goroutine 继续跑到自然结束。若脚本函数体是死循环（`for {}`、密集 rand 计算等），该 goroutine **永不结束**，对应一颗 CPU 核被永久占满，直到进程退出。多个含死循环函数的脚本（filter/rename 各一）可占满多核。

熔断保证了后续文件不再进入死循环（不会无限累积 goroutine），因此影响上限是"每个熔断函数最多泄漏一个吃满单核的 goroutine + 内存驻留"，不构成崩溃或数据风险，定级 Medium。脚本来源是用户本机脚本目录（非远程投递），威胁模型温和。

### 根因分析

yaegi 是纯 Go 解释器，`interp.Interpreter` 无抢占式取消 API；`context` 取消只能在脚本主动调用宿主 API 时被观察到，纯计算循环无观察点。

### 修复方案

无法根治（解释器限制），做三层缓解并显式告知：

1. **限制单飞并发**：`Contracts` 已用 mu 串行化调用（旧 SCR-02），确认死循环发生后同脚本的其余函数调用会因熔断跳过而非排队阻塞下载主流程——若现状是阻塞等锁，改为 `TryLock` + 熔断计数直接跳过。
2. **泄漏可观测**：熔断触发时日志从当前级别提升为 Error，文案明确"函数 {name} 已熔断，其执行体可能仍在后台占用 CPU，建议修正脚本后重启应用"；同时经 `scriptapi.SetLogSink` 通道推给前端脚本日志面板。
3. **文档固化**：`doc/`（或脚本编写说明）中写明脚本函数禁止无界循环、超时上限（10s）与熔断语义。
4. **不采纳**移除 math/rand 白名单的子代理建议：与 DoS 无因果（任何纯计算都耗 CPU），且会破坏合法的随机命名用例。

### 实施步骤与优先级（P1）

1. 核查 `Contracts` 各 `Safe*` 方法在熔断态下的行为，确保跳过而非等锁。
2. 调整熔断日志级别与文案；确认前端脚本日志面板可见。
3. 补脚本编写约束文档。
4. 跑 `cd src && go test ./internal/script/`。

### 验收标准（测试验证方法）

- 既有超时/熔断测试全绿；新增测试：注册死循环 filter，触发熔断后连续再调用 100 次 `SafeFilter`，断言全部立即返回（跳过语义）且总耗时 < 1s。
- 人工验证：加载含 `for {}` 的脚本运行下载任务，任务正常完成（熔断放行），前端脚本日志出现熔断 Error 提示；任务管理器可见单核占用（已知限制，提示到位即验收通过）。

---

## CODE-002 [Low] LogsPage 搜索防抖定时器无卸载清理

- 类别：代码功能 / 资源清理（关联旧 FE-23：防抖本身是该轮修复，遗漏卸载清理）
- 涉及文件：`src/frontend/src/pages/LogsPage.vue` — `queryTimer`、`watch(query)` 防抖块、组件卸载钩子

### 问题描述（含影响分析）

`watch(query)` 内 `queryTimer = setTimeout(..., 200)` 无对应 `onBeforeUnmount` 清理。用户在输入后 200ms 内切换路由离开日志页时，定时器回调仍触发一次，写入已卸载组件作用域的 `debouncedQuery` ref。Vue 3 下写孤儿 ref 无副作用、无告警，仅一次无效闭包执行，不泄漏（定时器一次性）。子代理初判 High，验证后降为 Low——纯卫生问题。

### 根因分析

防抖实现时只考虑了输入序列内的清理（每次 watch 先 `clearTimeout`），未挂组件生命周期。

### 修复方案

1. `LogsPage.vue` 增加 `onBeforeUnmount(() => clearTimeout(queryTimer))`（`onBeforeUnmount` 需确认已在 import 列表中）。

### 实施步骤与优先级（P2）

1. 单行修改 + import 核对。
2. `cd src/frontend && pnpm build`。

### 验收标准（测试验证方法）

- `pnpm build` 通过；代码审查确认卸载钩子存在。
- 交互验证：日志页输入搜索词后立即切走再切回，无异常、搜索框状态正常。

---

## CODE-003 [Low] writeFileAtomic 在 Windows rename 失败时以"目标存在"误判成功

- 类别：代码功能 / 错误处理实现（关联旧 SVC-20：Windows 兼容处理已加，判据过宽）
- 涉及文件：`src/internal/services/thumbcache.go` — `writeFileAtomic`

### 问题描述（含影响分析）

`os.Rename(tmp, dst)` 失败后，当前代码以 `os.Stat(dst)` 成功（目标存在）作为"另一并发写入者已完成，等价成功"的判据。判据过宽：若失败原因是权限/磁盘错误而 dst 恰好是**历史遗留的旧文件**，会误判成功并删除 tmp，调用方拿到"写入成功"但磁盘上是旧内容。缩略图场景内容按 key 寻址（同 key 内容相同），旧文件即正确内容，实际危害趋近于零——故 Low，仅收紧判据。

### 根因分析

用"结果状态"（目标存在）代替"失败原因"（是否 EEXIST/共享冲突）做判定，把两类不同失败折叠了。

### 修复方案

1. 判据收窄为错误类型：`errors.Is(err, fs.ErrExist)` 或 Windows 下 `errors.Is(err, syscall.ERROR_SHARING_VIOLATION)` / `ERROR_ACCESS_DENIED`（并发 rename 到同名目标的典型错误码）时才走"并发写入者已完成"分支；其余错误原样返回并清理 tmp。
2. LOGIC-002 的 epoch 修复落地后，同 key 并发写盘会被 singleflight + 锁互斥消除，此分支命中率进一步降低，但保留作防御。

### 实施步骤与优先级（P2）

1. 修改 `writeFileAtomic` 错误分支。
2. 跑 `cd src && go vet ./... && go test ./internal/services/`。

### 验收标准（测试验证方法）

- 新增测试（Windows）：dst 预先放置旧内容文件并以只读句柄占用，调用 `writeFileAtomic` 断言返回共享冲突分支的成功语义；将 dst 目录设为不可写，断言返回错误而非成功。
- `go test ./internal/services/` 全绿。

---

## CODE-004 [Low] 前端构建产物单 chunk 1.1MB，无代码分割

- 类别：代码功能 / 性能（证据：`pnpm build` 输出警告 "Some chunks are larger than 500 kB after minification"，日志 `tmp/audit2-febuild.log`）
- 涉及文件：`src/frontend/vite.config.ts`；`src/frontend/src/router/`（路由组件均为静态 import）

### 问题描述（含影响分析）

全部页面（Login/Chats/Tasks/Scripts/Logs/Settings）与 PrimeVue 组件打进单个 JS chunk（~1.1MB min）。Wails 场景资源走本地 assetserver，无网络下载成本，**首屏影响远小于 Web 部署**——仅 webview 解析/编译 1.1MB JS 的一次性开销（现代机器数十毫秒级）。定级 Low，属优化项而非缺陷。

### 根因分析

路由组件静态 import + 未配置 `manualChunks`，Vite 默认单 bundle。

### 修复方案

1. 路由懒加载：`router` 中各页面改为 `component: () => import('../pages/XxxPage.vue')`（LoginPage 可保留静态以保首屏）。
2. 可选：`vite.config.ts` 增加 `build.rollupOptions.output.manualChunks` 把 `primevue`、`@primeuix` 拆为 vendor chunk。
3. 完成后确认警告消失或剩余 chunk 均 < 500kB；若 Wails embed 对多 chunk 有路径问题（不应有，`all:frontend/dist` 全量嵌入），回退第 2 步仅保留第 1 步。

### 实施步骤与优先级（P3）

1. 修改 router 与 vite 配置。
2. `cd src/frontend && pnpm build` 检查产物尺寸与警告。
3. `cd src && wails build`（或 dev 模式）验证各页面路由切换正常加载。

### 验收标准（测试验证方法）

- `pnpm build` 无 500kB 警告，dist 中出现多个页面级 chunk。
- 应用内逐页切换（六个页面）均正常渲染，无动态导入 404（用 chrome-devtools mcp 检查 console/network）。

---

## 正面结论（勿误改）

以下代码级实现经本轮验证**正确**，修复上述发现时不得破坏：

1. **脚本沙箱白名单 + 超时熔断主体**（`script/engine.go`，旧 SCR-01/SCR-03）：白名单严格、10s 超时、熔断跳过后续调用，主体机制正确；math/rand 在白名单中**不是**漏洞（见 CODE-001 说明）。
2. **yaegi 调用串行化**（`script/engine.go` `Contracts`，旧 SCR-02）：mu 保护所有 `Safe*` 调用，闭包并发问题已消除。
3. **Windows 保留名检查**（`script/store.go`，旧 SCR-05）：con/prn/aux/nul/com/lpt 检查正确。
4. **日志 sink 锁纪律**（`logging/sink.go`，旧 LOG-02）：文件写与 emit 回调均在临界区外（`Write` 先解锁再写文件；`flush` 先取批次解锁再 emit），子代理的嵌套死锁担忧不成立。
5. **日志写失败探测**（`logging/sink.go`，旧 LOG-03）：目录可写探测 + 首次失败告警防刷屏，正确。
6. **结构化字段合并**（`logging/sink.go`，旧 LOG-01）：With 绑定与调用点字段合并、键名排序稳定，正确。
7. **resume_keys O(1) 断点**（`store.go` + `engine/resume_repo.go`，旧 ENG-12）：AddFinished 逐行插入，无 O(n²)。
8. **建表迁移事务**（`store/store.go`，旧 STO-01）：建表与版本号更新同事务 + IF NOT EXISTS，正确。
9. **WAL + synchronous(NORMAL)**（`store/store.go`，旧 STO-04）：组合安全且性能合理。
10. **Finished() map 拷贝**（`engine/iter.go`，旧 ENG-01）与 **iter.err 持锁规约**（旧 ENG-07）：并发访问纪律正确。
11. **暂停/取消清理 .tmp**（`engine/iter.go` + `executor.go`，旧 ENG-02）：Close 排空通道并删孤儿 tmp，正确。
12. **newTaskID 时间戳+随机后缀**（`engine/manager.go`，旧 ENG-11）：碰撞防护正确。
13. **子进程回收**（`services/helpers.go`，旧 SVC-11）：`go cmd.Wait()` 防僵尸进程，正确。
14. **日志文件精确匹配**（`services/logsvc.go`，旧 SVC-12）：`filepath.Ext` 精确判 `.log`，正确。
15. **FE-17 HMR 事件注销**（`stores/*.ts`）：模块重建时注销旧回调，事件不双触发。
16. **FE-22/FE-23 计算纪律**（`LogsPage.vue`）：编译结果纯 computed、搜索 200ms 防抖主体正确（仅缺卸载清理，见 CODE-002）。
17. **FE-24 网格几何单一来源**（`composables/useWaterfall.ts` + `ChatsPage.vue`）：JS 与 CSS 变量同源，正确。
