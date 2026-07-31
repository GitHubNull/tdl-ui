# tdl_UI 代码审计报告

> 审计时间：2026-07-29
> 审计范围：`src/` 下全部 Go 后端与 Vue 前端源码（不含 `ref/tdl` 上游 submodule、构建产物、node_modules）。
> 审计方法：三路并行通读 —— 后端服务层、下载引擎/存储层、前端；对每个模块做架构、并发、业务逻辑、错误处理、资源管理、代码质量六维审查。

本报告面向后续 AI 编程代理，供其按发现 ID 领任务执行修复。**开始修复前请先阅读 `06-remediation-roadmap.md` 的"AI 代理执行须知"。**

## 文档结构

| 文件 | 内容 |
|---|---|
| `01-architecture.md` | 架构与跨切面问题（ARC / DEP） |
| `02-backend-services.md` | 后端服务层（AUTH / SVC） |
| `03-engine-store.md` | 下载引擎与 SQLite 存储（ENG / STO） |
| `04-scripting-logging.md` | 脚本引擎与日志系统（SCR / LOG） |
| `05-frontend.md` | Vue 前端（FE） |
| `06-remediation-roadmap.md` | 修复升级路线图（P0-P3 分阶段） |

## 严重度定义

| 级别 | 含义 |
|---|---|
| Critical | 可导致进程崩溃 / 数据损坏，必须立即修复 |
| High | 严重业务缺陷、安全越界、错误数据展示，应优先修复 |
| Medium | 资源泄漏、竞态、体验劣化、可维护性隐患 |
| Low | 代码质量、边界健壮性、可读性改进 |

## 发现 ID 命名规则

`ARC`=架构、`DEP`=依赖、`AUTH`=登录、`SVC`=服务层、`ENG`=引擎、`STO`=存储、`SCR`=脚本、`LOG`=日志、`FE`=前端。

## 汇总表（按严重度排序）

### Critical

| ID | 类别 | 一句话描述 | 报告 |
|---|---|---|---|
| ENG-01 | 并发/崩溃 | iter.Finished() 返回内部 map 引用，并发迭代+写致进程 fatal 崩溃 | 03 |

### High

| ID | 类别 | 一句话描述 | 报告 |
|---|---|---|---|
| ARC-01 | 架构/并发 | Telegram 会话无所有权模型，四方并发读写同一 kv 会话 | 01 |
| ARC-02 | 生命周期 | OnShutdown 先关存储再让业务 goroutine 继续跑 | 01 |
| ARC-03 | 架构/并发 | 取消语义系统性断裂：ctx 创建但不贯穿执行层 | 01 |
| ARC-06 | 类型安全 | 前端弃用 wailsjs 生成绑定，用 any 通道手工重建（同 FE-03） | 01 |
| AUTH-01 | 并发 | 登录 cancel 无代际归属，切换登录方式互相取消 | 02 |
| AUTH-02 | 业务逻辑 | ImportDesktopSession 校验失败"回滚"实为删除，毁原有效会话 | 02 |
| SVC-01 | 并发/架构 | chat invoke 调用方超时不传播到 worker 执行、任务不出队 | 02 |
| STO-01 | 迁移健壮性 | migrateV1 的 user_version 写在事务外且建表无 IF NOT EXISTS | 03 |
| SCR-01 | 安全边界 | 脚本沙箱黑名单仅 os/exec，等同任意代码执行 | 04 |
| FE-01 | 竞态/业务 | ChatsPage 媒体分页无请求代际，切换对话乱序污染数据 | 05 |
| FE-02 | 错误处理 | 登录 submitCode/submitPassword 完全 fire-and-forget | 05 |
| FE-03 | 架构/类型 | api.ts/types.ts 与 wailsjs 生成绑定双轨维护 | 05 |

### Medium

| ID | 类别 | 一句话描述 | 报告 |
|---|---|---|---|
| ARC-04 | 架构/性能 | 串行单 worker 队列把并发问题转化为吞吐/延迟问题 | 01 |
| ARC-05 | 依赖 | go.mod replace 使版本号形同虚设、构建不可复现 | 01 |
| SVC-02 | 缓存/panic | 缩略图缓存无负缓存 + 失败重试风暴 + 空切片 panic + 无内存上限 | 02 |
| SVC-03 | 业务逻辑 | Logout 不取消进行中的登录流程与下载任务 | 02 |
| SVC-04 | 架构/性能 | 缩略图/预览走串行队列致画廊加载线性劣化 | 02 |
| SVC-05 | 错误处理 | applyDialogPeers 错误被吞 + 逐条 Apply 放大写入 | 02 |
| SVC-06 | 配置 | logging.yaml 每次启动静默覆盖并劫持配置 | 02 |
| SVC-07 | 可测试性 | auth 服务与相关服务测试覆盖空白 | 02 |
| ENG-02 | 资源泄漏 | 暂停/取消时 elem 通道滞留的 fd 与 .tmp 孤儿文件泄漏 | 03 |
| ENG-03 | 状态机/并发 | Remove/Resume TOCTOU 竞态产生失控僵尸下载 | 03 |
| ENG-04 | 状态机 | Queued 态任务不可暂停/取消 | 03 |
| ENG-05 | 可测试性 | task.go 983 行 5 类职责混杂，状态机/断点续传零测试 | 03 |
| SCR-02 | 并发 | yaegi 闭包被 Filter/Rename 与 OnFileDone 并发调用 | 04 |
| SCR-03 | goroutine 泄漏 | callWithGuard 超时逐文件泄漏 goroutine | 04 |
| LOG-01 | 功能缺陷 | sink 的 With/Write 丢弃结构化字段 | 04 |
| FE-04 | 内存泄漏 | MediaPreview keydown/mousemove 监听未在卸载时清理 | 05 |
| FE-05 | 内存泄漏 | fileStates/thumbFailed 集合只增不减 | 05 |
| FE-06 | 竞态 | LogsPage watch(source) 读取无乱序保护 | 05 |
| FE-07 | 业务逻辑 | 深链 /chats/:id 只在 onMounted 读一次 | 05 |
| FE-08 | 状态一致性 | 主题状态三处存储不同步 | 05 |
| FE-09 | 主题规范 | 主题切换控件不符合 ToggleSwitch 规范 | 05 |
| FE-10 | 错误处理 | 任务操作与脚本页错误处理裸奔 | 05 |
| FE-11 | 架构 | ChatsPage 1369 行上帝组件 | 05 |
| FE-12 | UX/危险操作 | 脚本删除无确认、切换/新建丢弃未保存修改 | 05 |
| FE-13 | 工程质量 | build 无 vue-tsc 类型检查、lint 体系缺失 | 05 |
| FE-14 | 主题/质量 | 硬编码颜色值违反 CSS 变量规范 | 05 |
| FE-15 | 重复 | fmtSize 等格式化函数 4 处重复 + 搜索框模板 3 处重复 | 05 |

### Low

| ID | 一句话描述 | 报告 |
|---|---|---|
| SVC-08 | 配置持久化失败被静默吞掉 | 02 |
| SVC-09 | begin() 的 error 返回值恒为 nil（死代码） | 02 |
| SVC-10 | keygen.New("session") 魔法字符串耦合 core 内部布局 | 02 |
| SVC-11 | openDirectory 子进程未 Wait 产生僵尸进程 | 02 |
| SVC-12 | ListLogFiles 用 Contains(".log") 会误匹配 | 02 |
| SVC-13 | 默认下载目录在 home 取失败时变成相对路径 | 02 |
| SVC-14 | config.Update 无上限钳制、Proxy 不校验格式 | 02 |
| SVC-15 | media_handler 把内部错误链原样发给前端 | 02 |
| SVC-16 | chat.go 的 Limit>200 静默重置为 50（语义反直觉） | 02 |
| SVC-17 | 魔法数字分散各处 | 02 |
| SVC-18 | ClearCache 与 thumbCache 写入存在竞争 | 02 |
| SVC-19 | events.go 常量文档注释与实际名不一致、缺 Unbind | 02 |
| SVC-20 | writeFileAtomic 在 Windows 上 rename 到已存在目标会失败 | 02 |
| ENG-06 | 进度语义误导：finished 永远达不到 total | 03 |
| ENG-07 | iter.err 读写并发规约不一致 | 03 |
| ENG-08 | process 持锁做网络/磁盘 I/O | 03 |
| ENG-09 | elem 通道容量 10 与相册上限的隐性耦合 | 03 |
| ENG-10 | persistState 两条独立 UPDATE 可能乱序覆盖 | 03 |
| ENG-11 | 任务 ID 基于时间戳理论上可碰撞 | 03 |
| ENG-12 | 断点续传每文件完成重写整个 finished 集合（O(n²)） | 03 |
| ENG-13 | loadFromStore 加载失败任务静默消失 | 03 |
| STO-02 | ImportLegacyJSON 未去重会因唯一索引导致重试死循环 | 03 |
| STO-03 | 仓储方法未使用 Context 版 API，无法随取消中断 | 03 |
| STO-04 | WAL 模式未设置 synchronous(NORMAL) | 03 |
| LOG-02 | 锁内执行 lumberjack Write（轮转时全局阻塞） | 04 |
| LOG-03 | 文件写失败被吞掉，Program Files 下日志静默失效 | 04 |
| SCR-04 | 脚本 Resume 按名重载导致语义漂移 | 04 |
| SCR-05 | 脚本名未排除 Windows 保留设备名 | 04 |
| FE-16 | 右面板媒体状态无缓存，切页即丢 | 05 |
| FE-17 | store 事件订阅返回的取消函数被丢弃（HMR 下双触发） | 05 |
| FE-18 | LoginPage detect()/QRCode.toCanvas 缺错误处理 | 05 |
| FE-19 | auth store 的 pending 态无超时兜底 | 05 |
| FE-20 | jumpMonth 归零时 DatePicker 显示值不清 | 05 |
| FE-21 | caseSensitive 声明后无 UI 入口（死代码或功能缺失） | 05 |
| FE-22 | LogsPage matcher computed 内产生副作用 | 05 |
| FE-23 | LogsPage rows computed 全量正则解析无防抖 | 05 |
| FE-24 | 瀑布流魔法数字与 CSS 强耦合 | 05 |
| FE-25 | TasksPage 展开态 record 在任务移除后不清理 | 05 |
| FE-26 | MediaPreview 末项"下一个"与 items 清空的边界 | 05 |

## 统计

- 总计：**约 60 项发现** —— Critical 1、High 12、Medium 27、Low 20。
- 三条必须优先处理的主线：会话所有权（ARC-01 系列）、取消语义完整性（ARC-03 系列）、长生命周期资源纪律（ENG/SCR/FE 泄漏项）。

## 使用说明

1. 先读本 README 与 `06-remediation-roadmap.md`。
2. 按路线图 P0 → P3 顺序领取任务，每项引用发现 ID。
3. 修复前阅读对应报告章节完整描述，修复后按路线图验证方法验证。
4. **注意各报告末尾的"正面结论"段落，列出的是已验证正确的代码，勿误改。**
