---
kind: configuration_system
name: 配置系统：YAML 持久化 + 迁移与热更新
category: configuration_system
scope:
    - '**'
source_files:
    - src/internal/config/config.go
    - src/internal/config/migrate.go
    - src/internal/logging/settings.go
    - src/internal/services/settings.go
    - src/main.go
    - src/wails.json
---

## 1. 使用的系统与方案
- 采用 Go 自定义 `config.Manager` 管理应用全局设置，以 YAML（gopkg.in/yaml.v2）格式持久化。
- 数据目录位于操作系统用户配置目录下（Windows 为 `%AppData%\tdl-ui`），通过 `os.UserConfigDir()` 获取。
- 日志配置独立为 `logging.LogSettings`，支持从 `logging.yaml` 一次性导入并热更新。
- Wails 构建配置由根级 `wails.json` 管理，不纳入运行时配置体系。

## 2. 核心文件与包
- `src/internal/config/config.go`：Settings 结构、YAML 映射、Manager 生命周期、默认值、校验与保存。
- `src/internal/config/migrate.go`：旧版 `settings.json` 的 JSON 反序列化桥接。
- `src/internal/logging/settings.go`：日志配置结构、默认值填充、YAML 解析/序列化。
- `src/internal/services/settings.go`：暴露给前端的 SettingsService，封装读取/保存与缓存清理。
- `src/main.go`：启动时初始化 config、导入 logging.yaml、装配各服务。
- `src/wails.json`：Wails 元信息（产品名称、版本等），非运行时配置。

## 3. 架构与约定
- **单一数据源**：`config.Manager` 持有内存中的 `Settings` 副本，所有读写经其完成，并发安全使用 `sync.RWMutex`。
- **分层结构**：
  - 对外暴露扁平 `Settings`（仅 JSON 标签，供前端/服务层使用）。
  - 内部用分组 `yamlConfig`（app/download/storage/session/log）映射到 YAML 层级。
- **默认值策略**：`defaultSettings(dataDir)` 提供下载目录、线程数、连接池大小、主题、日志默认值；`LogSettings.WithDefaults()` 对非法字段做白名单回填。
- **路径约定**：
  - 主配置：`<DataDir>/config.yaml`
  - 旧配置迁移：`<DataDir>/settings.json` → 迁移后重命名为 `.bak`
  - 日志导入：`<DataDir>/logging.yaml` 启动时只读一次，成功后改名为 `.imported`
  - 子目录：`kv/`（bolt 会话）、`scripts/`（用户脚本）、`cache/`（缩略图/预览）、`downloads/`（默认下载目标）
- **校验与保护**：
  - 代理地址仅允许 `socks5/http/https` 且必须含主机名，否则保存时报错（SVC-14）。
  - 自定义缓存目录需可创建并可写，写入探针文件验证后立即删除。
  - 线程数上限 16、连接池上限 64，超出即钳制。
- **热更新**：`SettingsService.Save` 调用 `cfg.Update` 持久化后，立即调用 `logging.Reconfigure` 使日志级别/目标即时生效。

## 4. 约定与约束
- **配置文件优先级**：启动时按顺序尝试加载 `config.yaml` → 若不存在则尝试 `settings.json` 并迁移 → 两者皆无则写出默认配置。
- **向后兼容**：旧版扁平 JSON 结构仍可通过 `legacyJSONUnmarshal` 正确导入，迁移过程自动保留已登录用户信息与代理设置。
- **日志配置导入幂等**：`logging.yaml` 仅在首次存在时导入一次，避免覆盖用户在界面修改的配置；失败或重命名失败会记录警告并在下次启动重试。
- **数据目录权限**：所有关键目录在 `NewManager` 时统一 `MkdirAll(0o755)` 创建，确保后续写入不会失败。
- **前端交互契约**：Settings 结构仅暴露 JSON 标签，前端通过 Wails 绑定的 `SettingsService` 进行 Get/Save，不直接操作文件系统。
- **配置不可变快照**：`Get()` 返回当前设置的副本，避免外部修改影响内部状态。

## 5. 与其他子系统关系
- `engine`、`store`、`script`、`services` 均通过注入 `*config.Manager` 获取数据目录与运行参数。
- `logging` 子系统既被配置系统消费（作为 Settings 的子配置），又反向被用于记录配置迁移与校验日志。
- `main.go` 中 bolt KV、SQLite 任务存储、脚本引擎等均依赖 `cfg.DataDir()` 派生路径，保证数据集中存放。