# Changelog

## [0.14.0] - 2026-08-01 01:26:14

### Added
- **日志导出功能**：日志页新增「导出」按钮与导出对话框（`ExportLogsDialog.vue`），支持选择导出范围（当前筛选结果 / 全部记录）与文件格式（`.log` 原始文本 / `.csv` 结构化表格含 time/level/module/source/message 列），后端 `LogService.ExportLogs` 执行文件写出；新增 `logsvc_export_test.go` 单元测试。
- **脚本内置模板系统**：新增 `src/internal/script/templates/` 嵌入 4 个内置脚本模板（自动归档 `auto-archive`、媒体过滤 `filter-media`、按日期重命名 `rename-by-date`、跳过重复 `skip-duplicates`），通过 `go:embed` 嵌入二进制，`TemplateService` 提供清单 API；前端脚本编辑器新增「模板」按钮与 `ScriptTemplateDialog.vue` 弹窗，选择模板后灌入编辑器可修改保存；新增 `templates_test.go` 单元测试。
- **DirSelect 可复用目录选择器**：新增 `DirSelect.vue` 组件（PrimeVue Select + 文件夹浏览按钮），支持历史下拉与自动填充最近目录，按 `kind` 分组持久化历史，统一各处手动目录选择实现。
- **RepoWiki 知识库文档**：新增下载引擎状态机、Wails 服务层 API 契约、前端页面架构、脚本引擎安全约束、主题切换系统、事件驱动架构等架构文档及对应知识卡。

## [0.13.1] - 2026-07-31 22:54:03

### Fixed
- **第二轮全库审计 13 项修复**（`doc/audit/20260731-2021-second-round-audit/`，Medium 6 / Low 7 全部闭环）：
  - ARC-001：ChatService 生命周期句柄（cancel/dead）在建连 goroutine 启动前发布，StopAndWait 覆盖"建连中"盲区。
  - ARC-002：store 增加 closed 标志与 `ErrStoreClosed`，engine 超时逃逸 goroutine 写入已关闭存储时降级为日志告警，不再 panic。
  - LOGIC-001：auth 登录流程增加 `flowDone` join——Logout/重新登录先 `cancelLoginAndWait(3s)` 等待旧流程 goroutine 真正退出，杜绝幽灵会话回写。
  - LOGIC-002：thumbCache 引入 epoch 代际，Clear 时代际自增，锁外 fetch 完成后入锁校验代际再落盘，消除清空后幽灵文件与 Windows 下 Clear 失败。
  - CODE-001：脚本超时熔断统一出口 `fuseContract`（Error 级宿主日志 + 前端脚本日志推送）；yaegi 执行体不可中断为已知限制，已固化至脚本调试教程。
  - FUNC-001：ChatsPage 单/双击延时定时器抽取 `resetThumbClick()`，切换对话/登出/卸载三处复位，不再误开旧对话预览。
  - LOGIC-003：登录态双源对账——启动时 kv 会话为权威源校准 config 展示态（`ReconcileOnStartup`）；`LoginStatus` 新增 `sessionPresent`，登录页对失步状态给出自愈提示。
  - CODE-003：`writeFileAtomic` rename 失败判据从"目标已存在"收窄为错误类型（`fs.ErrExist` / Windows 共享冲突 32 / 拒绝访问 5）。
  - CODE-002：LogsPage 搜索防抖定时器补 `onBeforeUnmount` 清理。
  - FUNC-002：MediaPreview `pendingAdvance` 在 items 收缩或首元素身份变化（列表整体重置）时复位，保留末项续拉自动前进主路径。

### Changed
- ARC-003：缩略图/视频队列 worker 数与下载进度事件节流间隔配置化（config.yaml `tuning` 段：`thumbWorkers`/`videoWorkers`/`progressIntervalMs`，零值回退内置默认 2/2/200ms，无 UI 入口）。
- ARC-004：engine 对持久化依赖接口化——新增 `TaskRepo` 窄接口（18 方法），`Deps.Store` 由 `*store.Store` 改为接口类型，装配零改动；新增内存 fake 单测证明可测试性。
- CODE-004：前端路由页面懒加载（LoginPage 保留静态导入保首屏）+ `@primeuix` 主题引擎拆独立 vendor chunk，构建产物由单 chunk 1.1MB 降为多 chunk（最大 332kB），500kB 警告消除。

## [0.13.0] - 2026-07-31 00:36:44

### Added
- **使用教程页**：新增「教程」标签页（`/tutorial`），按 基础 → 中级 → 高级 三级组织 8 个章节（安装启动、账号登录、创建下载任务、代理与设置、脚本语法基础、脚本实战示例、内置函数参考、脚本调试）；左侧章节导航含分级分组与当前章节 h2/h3 小节目录（锚点平滑跳转），底部提供上一章/下一章切换。
- **Markdown 渲染组件**：新增 `MarkdownView.vue` + `utils/markdown.ts`（markdown-it `html:false` 防注入 + highlight.js 按需注册 go/bash/yaml/json），样式全部使用 `--p-*` 变量适配明暗主题；新增依赖 `markdown-it`、`highlight.js`、`@types/markdown-it`。
- **代码块一键复制与行号**：所有围栏代码块带工具栏（语言标签 + 复制按钮，复制成功显示"已复制 ✓"，剪贴板 API 失败回退 execCommand）；Go 脚本示例代码块附带行号列（sticky 定位，选中/复制不含行号）。
- **教程内嵌真实图标**：`{{icon:xxx}}` 白名单占位符机制，界面总览表格等处直接渲染与左侧导航同套 `nav-*.svg` 图标（currentColor 随主题变色），替换原先与实际 UI 不符的 emoji。
- **关于页**：新增「关于」标签页（`/about`），居中英雄区（logo/名称/版本徽章/简介/操作按钮）+ 技术栈网格 + 作者与许可证元信息卡片；版本号经 Vite `define` 注入 `__APP_VERSION__`（读取 package.json，消除硬编码）；外链经 `BrowserOpenURL` 打开系统浏览器。
- **导航图标**：新增 `nav-tutorial.svg`（打开的书本）与 `nav-about.svg`（信息圆圈），注册至 `AppIcon.vue` 并同步到 `img/icons/navigation/`。

### Fixed
- 修复暗色主题下行内代码背景色因选择器特异性渗透到代码块内部导致缩进处出现浅色条的问题。

## [0.12.0] - 2026-07-30 23:39:21

### Added
- **自定义视频播放器**：新建 `VideoPlayer.vue` 组件，移除原生 `<video controls>`，基于 PrimeVue Button/Slider 自建控制栏（播放/暂停、可拖拽进度条含 buffered 区段显示、时间显示、音量滑条与静音、全屏切换），播放中鼠标静止 3s 自动隐藏控制栏，暂停时常显。
- **播放器键盘快捷键**：视频条目预览时 ←/→ 快退/快进 10 秒、↑/↓ 音量 ±10%、Space 播放暂停、M 静音、F 全屏；非视频条目保持 ←/→ 切换媒体（`utils/format.ts` 新增 `fmtDuration`）。
- **视频磁盘分段缓存（边下边播）**：新增 `videodisk.go`，在线视频分段落盘 `<TempDir>/video/<dialogID>_<msgID>/<partIdx>.part`，2GB 上限按 mtime LRU 淘汰；缓存查找链升级为 内存 LRU → 磁盘 → Telegram（`video.go`），重看/回拖命中磁盘不重复拉取。
- **后台顺序预取**：`videoPrefetcher` 单活跃视频从当前播放位置顺序预取全部分段；新增 `StopVideoPrefetch` 绑定，关闭预览/切换条目时停止预取省流量。
- **视频临时目录设置**：`Settings.TempDir` 新配置项（config.yaml + 设置页，含浏览按钮与可写校验），留空回落 `<数据目录>/tmp`；「清空缓存」联动清理视频临时分段。

### Fixed
- 修复暂停后 seek 被 `canplay` 事件强制恢复播放的问题（自动播放仅首次 canplay 触发）。
- 修复点击视频预览四周空白区域无法关闭预览的问题（`.mp-video-wrap` 铺满舞台吞掉了 mousedown，改为 wrap 自身被点击时关闭）。

## [0.11.0] - 2026-07-30 21:30:05

### Added
- **重复下载确认弹窗**：当用户尝试下载一个已处于下载中状态的多媒体文件时，弹出确认对话框，告知"该文件当前正在下载中，是否确认重复下载？"，提供"取消"/"重复下载"两个选项。
  - 前端 `ChatsPage` 新增 `isDownloading(messageId)` 检测函数：以 `fileStates`（`task:file` 事件实时 Map）为主、`tasksStore.files`（Pinia）为兜底，保证切页/刷新后仍可检测。
  - `downloadOne()` 优先级调整：下载中 → 弹"重复下载"框；已下载 → 弹"重新下载"框；未下载 → 直接打开添加下载对话框。
  - `downloadSelected()` 批量路径：统计下载中数与已下载数，生成合并文案"选中文件中有 N 个正在下载中、M 个已下载，是否确认重复下载？"。
  - 修复 `wails.json` 中 `productVersion` 字段缺失逗号的语法错误。

## [0.10.0] - 2026-07-30 20:26:52

### Added
- **对话媒体下载状态增强**：已下载标记 + 重复下载确认框 + 双击系统默认程序打开 + 下载中实时进度角标（含跨页面状态恢复）
  - 后端：store 层新增 `idx_files_dialog_msg` 索引与 `ListDoneFilesByDialog`/`GetDoneFile` 查询；engine 层 `TaskFile` 补 `DialogID`/`MessageID` 并写库（修复原有关联缺口），`FileEvent` 携带最终路径；services 层新增 `ListDownloadedMessages`（os.Stat 过滤磁盘已删文件 + 同 messageId 去重）与 `OpenDownloadedFile`
  - 前端：`ChatsPage` 新增 `downloadedMap` 与已下载 chip、下载中角标文案改为"下载中 N%"、重复下载 ConfirmDialog（单文件/批量）、双击打开（缩略图区 250ms 延时区分单击/双击）、切换对话/路由时状态恢复（tasks store 回填 downloading 角标）
  - 前端：`MediaPreview` 新增 `downloadedIds` prop 与 `open` 事件，顶栏显示"已下载"Tag 与"打开文件"按钮
- 数据库迁移：schemaVersion 3→4，新增 `idx_files_dialog_msg` 索引

### Fixed
- 修复 engine 层写 files 表时未携带 `dialog_id`/`message_id` 的既有缺口，仅新下载记录具备对话关联。

## [0.8.0] - 2026-07-29 21:14:08

### Added
- **品牌视觉素材系统**：全套图标、Logo、宣传海报、空状态插图接入项目
  - 新增 `img/` 目录（Logo、导航图标、登录图标、状态图标、插图、宣传材料），含完整 README 规范文档
  - 新增 `AppIcon.vue` 组件：内联 11 个品牌 SVG 图标（currentColor 描边，自动适配明暗主题）
  - 侧边栏品牌位由文本 "tdl" 替换为品牌图标图片
  - 导航图标（对话/下载/脚本/日志/设置）、账号、主题切换全部接入品牌图标
  - 登录页顶部新增英雄插图，三个登录方式 Tab 图标替换为品牌图标（验证码/二维码/Desktop）
  - TasksPage 与 ChatsPage 空状态替换为专用插图（empty-tasks / empty-chats）
  - README.md / README_EN.md 顶部接入横幅头图，界面预览节加入宣传海报
- **Wails 构建图标**：替换 `src/build/appicon.png` 与 `src/build/windows/icon.ico`
- **前端 favicon**：`index.html` 新增 SVG + PNG 双格式 favicon
- **类型支持**：新增 `vite-env.d.ts` 提供 `?raw` SVG 导入类型声明

## [0.7.0] - 2026-07-29 20:50:59

### Removed
- **任务追加下载功能整条链路已撤回。** 此前误将 tdl CLI 的单任务限制套用到 GUI，实际上 GUI 的 `Manager.Create` 天然支持并发独立任务（无互斥，每任务独立 client + dcpool），无需追加功能。
  - 前端：移除下载页终态任务卡片上的「追加下载」加号按钮、`NewTaskDialog` 追加模式（`appendMode`/`taskId` props、追加提示块、条件渲染的三块表单）、`tasks.ts` 的 `appendTask` action、`api.ts` 的 `appendTaskItems` 封装、`types.ts` 的 `AppendOptions` 接口。
  - 后端：移除 `services.DownloadService.AppendTaskItems`、`engine.AppendOptions` / `Manager.AppendItems` / `Task.mergeOpts` / `unionIntSlice`、`store.AppendURLItem` / `MergeSelectionItem` / `unionInts` 及对应单测。
  - SQLite 表结构（`task_items`、`uq_items_url` 索引）与迁移语句保持不动；创建任务时写入消息项、重启恢复时重组装仍依赖它。

## [0.6.0] - 2026-07-29 02:01:16

### Added
- SQLite 持久化层（`internal/store`）：`tasks`/`task_items`/`files`/`resume_points` 四表，支持 WAL 模式、手写 PRAGMA user_version 迁移、旧 `tasks.json` 单事务导入并生成 `.bak`。
- 任务追加下载：已完成/暂停/失败/取消态任务可追加新消息链接（URLs）或选集（Selections），采用「重启式」实现（running 走 Pause→runDone→Resume，done 置 paused）。
- YAML 配置迁移：`settings.json` → `config.yaml`，旧文件重命名为 `.bak`；`Settings` 保留 JSON 标签，YAML 用内部 `yamlConfig` 互转。
- 可配置缓存目录：`Settings` 新增 `cacheDir`，空值默认 `<DataDir>/cache`；保存时校验目录可写；`thumbCache` 改为 `rootFn` 注入实现热生效。
- 设置页新增「存储」区块：数据目录展示、缓存目录输入+浏览、清空缓存按钮（二次确认）。
- 断点续传改造：从 BBolt KV 的「逻辑位置」改为 SQLite `resume_points` 表的「内容坐标 `dialogID:messageID`」；分组消息逐成员生效。

### Changed
- 引擎层 `Manager` 从 JSON 全量 `persist()` 改为 SQLite 行级写（`UpdateTaskStatus`/`UpdateTaskCounts`/`UpsertFile`/`FinishFile` 等）。
- `execute()` 每轮从 `task_items` 表重组装 URLs/Selections，覆盖 queued 窗口期追加。
- 删除 `engine/persist.go` / `persist_test.go`，断点逻辑迁入 `resume_repo.go`。

## [0.5.1] - 2026-07-29 00:56:53

### Fixed
- 修复 ChatsPage 媒体过滤工具栏 UI 组件重叠问题。
  - 约束 `InputNumber` / `DatePicker` 内部 input 宽度跟随包装器，消除溢出遮挡相邻的「查询」「重置」按钮。
  - 为固定宽度输入框、按钮、分隔符增加 `flex-shrink: 0` 保护，防止容器宽度紧张时被挤压。
  - `spacer-flex` 增加 `min-width: 0`，保证换行后右侧布局切换 / 月份选择器正常靠右。
- 加固 `MediaPreview.vue` 顶栏弹性布局：文件名优先收缩省略，下载/关闭按钮不被压缩。

## [0.5.0] - 2026-07-29 00:35:07

### Added
- 新增完整日志记录与浏览功能。
- 后端日志核心（`internal/logging`）：基于 zap + lumberjack，支持多目标输出（文件+UI）、格式模板、文件滚动（MaxSize/MaxAge/MaxBackups）、内存环形缓冲（5000 条）与 250ms 批量事件推送。
- 后端全面埋点：app/auth/chat/download/engine/script/settings/media 等模块公开方法入口与错误分支。
- 新增 `LogService` Wails 绑定：实时日志快照、历史日志文件列表与读取、YAML 配置导入/导出、打开日志目录。
- 前端新增「日志」标签页（`/logs`）：VirtualScroller 类终端视图、行号沟槽、语法高亮、正则/大小写/自动跳转搜索、级别过滤、跟随滚动、历史文件浏览。
- 设置页新增「日志」配置区：输出目标、日志级别、目录、格式模板、滚动策略，支持 YAML 导入/导出。
- 数据目录下 `logging.yaml` 启动时自动加载；设置保存即热生效。
## [0.4.0] - 2026-07-28 23:36:21

### Added
- 下载任务记录持久化到磁盘（`tasks.json`），重启后保留；任务内文件级跟踪（.tmp → 最终路径 → done/failed）。
- 下载管理页新增：清除已完成记录、删除所有文件（ConfirmDialog 二次确认）、打开目录、文件列表展开与多选删除。
- 媒体列表支持瀑布流 / 网格 / 时间流三种布局切换，选择持久化到 localStorage。
- 时间流布局：按月份分组、右侧月份索引条锚点跳转、DatePicker 任意月份跳转（OffsetDate 查询管线）。
- Lightbox 全屏预览：图片大图缩放（0.5x–5x）/ 平移 / 双击复位，左右键盘切换，尾部自动 loadMore；视频仅预览大缩略图。
- 缩略图磁盘缓存（`cache/thumbs/` + `cache/previews/`）+ 本地 HTTP 服务（`/media/thumb`、`/media/preview`），响应头 `Cache-Control: immutable`。
- 新增 `MediaItem.Width/Height` 支持，瀑布流按纵横比占位。

### Changed
- 缩略图通道整体替换为 HTTP 服务（旧 data URI 通道移除，禁止向后兼容），浏览器接管并发与缓存。
- 下载任务 `TaskView` 新增 `FileCount` 字段；`Task` 增加 `files` 与 `done channel` 支持。
- `thumb.go` 超时提升至 3 分钟，适配单 worker 串行队列。

### Fixed
- 修复 `wails dev` 下 `explorer` 不在 PATH 导致「打开目录」失败：改用 `%SystemRoot%\explorer.exe` 绝对路径。
- 修复 Vite SPA fallback 吞 `/media/*` 请求：新增 `media404` 插件让 Wails 回落后端 Handler。

## [0.3.0] - 2026-07-28 21:18:01

### Added
- 对话页支持选集下载：勾选媒体后按对话 + 消息 ID 直接创建任务，不再依赖 t.me 链接，私聊/普通群消息也可下载。
- 媒体列表新增缩略图：内嵌模糊占位图（stripped thumb）即时展示，清晰缩略图按需拉取并内存缓存（ChatService.GetThumbnail）。
- 下载任务支持自定义标签（label），任务列表优先展示标签而非链接。

### Changed
- ChatDetailPage 合并入 ChatsPage，对话列表与媒体浏览统一为单页交互。
- 对话类型常量与 peer 解析（ResolveDialogPeer）下沉至 engine 层，供 services 与下载引擎复用。

## [0.2.0] - 2026-07-28 20:46:24

### Added
- 设置页新增「账号」区块，显示登录状态并提供账号管理/前往登录入口。
- 对话页新增加载失败错误提示与重试按钮，空态区分加载中/失败/无数据。
- 新增 ChatService 并发与错误路径回归测试（死锁、连接超时、Stop 复位）。

### Changed
- ChatService 连接启动重构：可注入 runClient、连接超时保护、ready/dead 竞态处理。
- 页面宽度统一为全宽布局，移除脚本页独立宽度限制。
- Desktop 会话导入成功提示补充双端会话冲突风险说明，导入按钮增加 loading 态。

### Fixed
- 修复 Desktop 会话导入未校验有效性导致的假登录态：导入后建连校验，失败自动回滚会话。

## [0.1.0] - 2026-07-28

### Added
- 初始化 tdl UI 项目，集成 tdl 核心库作为子模块。
- 添加 Wails 桌面应用框架基础结构。
- 添加前端 Vue 3 + PrimeVue 基础架构。
- 添加项目文档与基础配置文件。


