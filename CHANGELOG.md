# Changelog

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


