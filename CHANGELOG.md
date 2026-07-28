# Changelog

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

