# Changelog

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


