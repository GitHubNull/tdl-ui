---
kind: logging_system
name: 日志系统（基于 zap + lumberjack 的双目标结构化日志）
category: logging_system
scope:
    - '**'
source_files:
    - src/internal/logging/logger.go
    - src/internal/logging/settings.go
    - src/internal/logging/sink.go
    - src/internal/services/logsvc.go
    - src/main.go
---

## 系统与架构
本应用使用 `go.uber.org/zap` 作为核心日志框架，配合 `gopkg.in/natefinch/lumberjack.v2` 实现文件滚动输出，并通过自定义 `zapcore.Core`（`sink`）将日志同时推送到前端 UI。日志系统位于 `src/internal/logging/`，由 `main.go` 在应用启动时初始化，通过 Wails 事件总线向 Vue 前端的 LogsPage 实时推送。

## 核心组件
- `logger.go`：包级全局 `root` logger、`Init`/`Reconfigure`/`Close` 生命周期管理、`L(module)` 获取带模块名的 SugaredLogger、级别解析与 `Recent()` 环形缓冲快照。
- `settings.go`：`LogSettings` 配置结构体（targets/level/dir/format/maxSizeMb/maxAgeDays/maxBackups），默认值填充、YAML 导入导出、目录可写探测与回退逻辑。
- `sink.go`：自定义 `zapcore.Core` 实现，维护内存环形缓冲（5000 条）、批量推送（250ms 周期）、lumberjack 文件写入、字段合并与模板渲染。
- `logsvc.go`：Wails 暴露的日志服务，提供读取历史文件、导入/导出 YAML 配置、打开日志目录等能力。

## 设计决策与约定
- **双目标输出**：`targets` 支持 `file` / `ui` / `both`，默认 `both`。UI 目标通过 `emit` 回调批量推送，文件目标由 lumberjack 处理。
- **结构化字段**：通过 `appendFields` 将 zap 字段按 `key=value` 追加到消息末尾，键名排序保证输出稳定。
- **格式模板**：支持 `{datetime} {level} {module} {file} {func} {line} {msg}` 占位符，默认模板包含完整调用点信息。
- **热更新**：`Reconfigure` 可在运行时切换级别、目标、目录与滚动策略，无需重启。
- **容错机制**：日志目录不可写时自动回退到用户缓存目录（Windows 为 `%LOCALAPPDATA%`），首次写入失败仅告警一次避免刷屏。
- **并发安全**：`sink` 使用互斥锁保护环形缓冲与 pending 队列，文件写入移出临界区避免阻塞。
- **脚本日志转发**：`scriptapi.SetLogSink` 将 Yaegi 脚本输出统一接入 logging 系统并额外通过事件推送。

## 前端集成
Vue 前端通过 `wailsjs/go/services/LogService.js` 调用后端方法，LogsPage 订阅 `events.Log` 事件接收批量日志条目，显示在表格中并提供分页与搜索。

## 约束与规则
- 日志级别仅支持 `debug/info/warn/error`，非法值回退到 `info`。
- 文件名固定为 `tdl-ui.log`，滚动备份由 lumberjack 自动命名。
- 读取日志文件时仅允许日志目录内的 `.log` 扩展名文件，防止目录穿越攻击。
- 环形缓冲上限 5000 条，超出时丢弃最旧条目。
- 批量推送间隔 250ms，避免频繁事件风暴。
