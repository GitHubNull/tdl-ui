# 下载引擎重构方案：SQLite 持久化 + YAML 配置 + 任务追加

> 状态：待实施  
> 优先级：P1（架构级改造）  
> 预估工期：3.5-5.5 天（含新增缓存目录配置约 +0.5 天）  

> **撤回说明**：本文档中「任务追加下载」相关设计（`AppendItems` / `AppendTaskItems` / `AppendOptions`、`NewTaskDialog` 追加模式、下载页加号入口、`AppendURLItem` / `MergeSelectionItem`）已按需求撤回，对应代码均已从仓库移除；该需求源自误解 tdl CLI 的单任务限制，GUI 本身支持在已有任务运行时创建全新独立下载任务。SQLite 持久化、YAML 配置迁移、缓存目录可配置等其余部分仍然有效，`task_items` 表结构与 `uq_items_url` 索引保留（创建任务时写入消息项、重启恢复时重组装仍依赖它）。

---

## 1. 改造目标

| 目标 | 现状 | 目标态 |
|------|------|--------|
| 下载记录持久化 | JSON 文件（`tasks.json`）全量读写 | SQLite 关系型存储 |
| 应用配置 | JSON（`settings.json`） | YAML（`config.yaml`） |
| 任务追加 | 不支持（一个 Task 对应固定 URLs/Selections） | 支持向已有任务动态追加消息 |
| 断点续传 | BBolt KV 存储（`key.Resume(fingerprint)`） | SQLite 表内维护，支持追加后增量续传 |
| 缓存目录 | exe 目录下 `cache/` 硬编码（不可配置） | 可配置（设置页 + `config.yaml`），默认迁至数据目录 |

---

## 2. 现状诊断

### 2.1 当前数据流

```
前端 TasksPage.vue / NewTaskDialog.vue
    │
    ▼
DownloadService (services/download.go)
    │
    ▼
Manager.Create(opts) ──→ Task（内存）
    │                        │
    ▼                        ▼
persist() ──→ tasks.json   run() ──→ execute()
                              │
                              ▼
                        BBolt KV（断点：key.Resume）
```

### 2.2 关键代码位置

| 模块 | 文件 | 职责 |
|------|------|------|
| 任务管理器 | `src/internal/engine/task.go` | `Manager` / `Task` 结构体、生命周期管理 |
| 持久化层 | `src/internal/engine/persist.go` | `taskRecord` JSON 序列化、原子写、`TaskFile` 结构定义 |
| 迭代器 | `src/internal/engine/iter.go` | `iter` 遍历 dialogs、生成 `fingerprint` |
| 进度回调 | `src/internal/engine/progress.go` | `progress` 接口实现、文件状态维护 |
| 配置管理 | `src/internal/config/config.go` | `Settings` JSON 读写 |
| 缩略图缓存 | `src/internal/services/thumbcache.go` | 缩略图/预览磁盘缓存（根目录硬编码为 exe 目录下 `cache/`） |
| 前端任务页 | `src/frontend/src/pages/TasksPage.vue` | 任务列表展示、操作按钮 |
| 前端任务 Store | `src/frontend/src/stores/tasks.ts` | 任务列表缓存、`task:update` / `task:file` 事件订阅 |
| 新建任务弹窗 | `src/frontend/src/components/NewTaskDialog.vue` | 创建任务表单 |

### 2.3 当前限制

1. **Task 创建后不可变**：`TaskOptions`（URLs / Selections）在 `Create()` 时固定，无修改入口
2. **JSON 全量覆盖**：每次文件状态变化都重写整个 `tasks.json`，任务量大时性能下降
3. **断点与消息列表双重耦合**：① `fingerprint(dialogs)` 基于完整消息列表，追加后指纹变化导致断点 key 失效；② `finished` 集合存的是遍历序号 `logicalPos`，插入新消息会整体错位（见 5.1）
4. **配置格式单一**：JSON 不支持注释，用户手动编辑体验差
5. **缓存目录硬编码**：`thumbCache` 根目录固定为可执行文件目录下 `cache/`，不可配置；Windows 安装到 `Program Files` 时 exe 目录通常不可写，缓存会静默失效（详见第 6 节）

---

## 3. 架构设计

### 3.1 改造后数据流

```
┌─────────────────────────────────────────────────────────────┐
│                        前端层                                │
│  TasksPage.vue          NewTaskDialog.vue (新增追加模式)      │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                      Wails 服务层                            │
│  DownloadService ──→ TaskManager（SQLite 版）                │
│  SettingsService ──→ ConfigManager（YAML 版）                │
└─────────────────────────────────────────────────────────────┘
                              │
              ┌───────────────┼───────────────┐
              ▼               ▼               ▼
        ┌─────────┐    ┌──────────┐    ┌──────────┐
        │ SQLite  │    │   YAML   │    │  BBolt   │
        │  (新)   │    │  (新)    │    │ (保留)   │
        └─────────┘    └──────────┘    └──────────┘
        tasks.db       config.yaml     kv/*.db
        ├─ tasks                      (Telegram 会话)
        ├─ task_items
        ├─ files
        └─ resume_points
```

### 3.2 SQLite 表结构

```sql
-- 连接初始化（每次打开连接执行，不属于建表迁移）
PRAGMA journal_mode = WAL;   -- 写入不阻塞 UI 侧读查询
PRAGMA foreign_keys = ON;    -- SQLite 默认关闭外键，级联删除依赖此开关
PRAGMA busy_timeout = 5000;  -- Windows 下偶发锁竞争时等待而非直接报错

-- 任务主表（替代 tasks.json）
CREATE TABLE tasks (
    id          TEXT PRIMARY KEY,           -- 如 "t1722180000000000000"
    label       TEXT,                       -- 显示名称
    dir         TEXT NOT NULL,              -- 保存目录
    script_name TEXT,                       -- 关联脚本
    template    TEXT,                       -- 文件名模板
    rewrite_ext INTEGER NOT NULL DEFAULT 0, -- SQLite 无 BOOLEAN 类型，统一 INTEGER 0/1
    skip_same   INTEGER NOT NULL DEFAULT 0,
    group_media INTEGER NOT NULL DEFAULT 1,
    status      TEXT NOT NULL,              -- queued/running/paused/done/failed/canceled
    error       TEXT,                       -- 失败原因
    total       INTEGER NOT NULL DEFAULT 0, -- 对应 taskRecord.Total/Finished/Failed，
    finished    INTEGER NOT NULL DEFAULT 0, -- 重启后不跑迭代器也能展示历史进度
    failed      INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT NOT NULL,              -- 沿用现有 "2006-01-02 15:04:05" 格式
    updated_at  TEXT NOT NULL
);

-- 任务消息项表（支持追加的核心：一个任务可有多条消息记录）
CREATE TABLE task_items (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id     TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    item_type   TEXT NOT NULL,              -- 'url' | 'selection'
    -- URL 类型时填充：
    url         TEXT,
    -- Selection 类型时填充：
    dialog_id   INTEGER,
    dialog_type TEXT,
    message_ids TEXT,                       -- JSON 数组，如 "[123,124,125]"
    created_at  TEXT NOT NULL
);

-- URL 去重：部分唯一索引。
-- 不能用 UNIQUE(task_id, item_type, url, dialog_id)：
--   ① SQLite 中 UNIQUE 索引内 NULL 彼此不相等，url 行的 dialog_id 恒为 NULL，重复 URL 拦不住；
--   ② selection 行的 url 恒为 NULL，同一对话第二次追加不同消息反而会被该约束误拒。
CREATE UNIQUE INDEX uq_items_url ON task_items(task_id, url) WHERE item_type = 'url';
-- Selection 去重在应用层完成：追加时把同 (task_id, dialog_id) 的行
-- 与新 message_ids 求并集后合并为一行（事务内 SELECT → 合并 → UPDATE/INSERT）。

-- 文件记录表（替代 Task.files 的持久化职责）
CREATE TABLE files (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id     TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,              -- 最终文件名
    path        TEXT NOT NULL,              -- 完整路径（下载中为 .tmp 路径）
    size        INTEGER,
    state       TEXT NOT NULL,              -- downloading / done / failed
    dialog_id   INTEGER,                    -- 来源对话 ID
    message_id  INTEGER,                    -- 来源消息 ID
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    UNIQUE(task_id, path)                   -- 对应现有 addFile 的「同路径覆盖」语义
);

-- 断点续传表（替代 BBolt key.Resume；断点改为任务级后每任务至多一行，
-- 不再需要 fingerprint 字段，详见 5.1）
CREATE TABLE resume_points (
    task_id     TEXT PRIMARY KEY REFERENCES tasks(id) ON DELETE CASCADE,
    finished    TEXT NOT NULL,              -- JSON 数组，元素为 "dialogID:messageID"
    updated_at  TEXT NOT NULL
);

-- 索引
CREATE INDEX idx_files_task ON files(task_id);
CREATE INDEX idx_items_task ON task_items(task_id);

-- 版本管理：PRAGMA user_version 递增 + 手写迁移函数，不引入 golang-migrate
```

### 3.3 YAML 配置结构

```yaml
# config.yaml（替代 settings.json，字段与现有 Settings 结构一一对应，不得遗漏；
# 样例值即 defaultSettings() 的实际默认值，实现时须逐一核对）
app:
  theme: system           # light / dark / system
  proxy: ""               # socks5://127.0.0.1:1080

download:
  dir: "~/Downloads/tdl-ui"
  template: "{{ .DialogID }}_{{ .MessageID }}_{{ filenamify .FileName }}"
  threads: 4              # 单文件下载线程数
  limit: 2                # 同时下载最大文件数
  pool_size: 8            # DC 连接池大小

# 存储目录（新增需求，设计见第 6 节）
storage:
  cache_dir: ""           # 缩略图/预览缓存目录，空 = <数据目录>/cache

# 日志配置（对应 Settings.Log；logging.LogSettings 已带 yaml 标签，直接内嵌复用）
log:
  targets: both           # file / ui / both（WithDefaults 默认 both）
  level: info
  dir: ""                 # 空 = 程序目录下 logs 子目录
  format: ""              # 空 = DefaultFormat
  maxSizeMb: 10
  maxAgeDays: 7
  maxBackups: 10          # 注意：默认 10，非 5（见 settings_test.go 断言）

# 保留：最近一次登录用户信息（仅展示用）
session:
  logged_in_user_id: 0
  logged_in_username: ""
```

> 决策说明：
> - YAML 库沿用项目已直接依赖的 `gopkg.in/yaml.v2`（`internal/logging` 已在用），**不引入** `gopkg.in/yaml.v3`，避免同仓库双 YAML 依赖。
> - 数据目录下独立的 `logging.yaml` 启动覆盖机制（`main.go`）与 LogService 的导入/导出功能保持不变；`config.yaml` 的 `log` 段是持久化配置源，`logging.yaml` 仅作一次性外部覆盖入口。
> - `Settings` 结构体**不加 yaml 标签**：现有 `Settings` 是扁平结构，直接加标签只能得到扁平 YAML，与上述分组样例矛盾。改为在 `config` 包内新增映射结构体 `yamlConfig`（分组 app / download / storage / log / session），load/save 时与扁平 `Settings` 互转；`Settings` 的 JSON 标签与 Wails 前后端 API 保持原样。

---

## 4. 模块改造清单

### 4.1 新增模块

| 模块 | 文件 | 职责 |
|------|------|------|
| SQLite 存储层 | `src/internal/store/store.go` | DB 连接、迁移、通用查询 |
| 任务仓库 | `src/internal/store/task_repo.go` | Task CRUD、事务 |
| 文件仓库 | `src/internal/store/file_repo.go` | File 记录操作 |
| 断点仓库 | `src/internal/store/resume_repo.go` | Resume point 读写 |
| YAML 配置 | `src/internal/config/yaml.go` | YAML 解析、保存、热重载 |

### 4.2 改造模块

| 原文件 | 改造内容 |
|--------|---------|
| `src/internal/engine/task.go` | ① `Manager` 注入 `*store.Store` 替代 `statePath`，删除 `restore()` / `persist()`（启动时从 DB 加载，运行中行级写入）；② `Task` 运行时仍持有 `opts` 与内存态（view/计数仍要用），但 `opts` 改为创建/恢复时由 `task_items` 表组装；③ `Create()` 改为事务插入 tasks + task_items 后启动 goroutine；④ 新增 `AppendItems(taskID, items)` 方法；⑤ `loadResume` / `saveResume` 改走 `resume_repo`，断点集合类型由 `map[int]struct{}` 改为 `map[string]struct{}`（key 为 `"dialogID:messageID"`）；⑥ `addFile` / `finishFile` / `markFileFailed` / `dropFile` / `emitUpdate` 内的 `mgr.persist()` 调用点逐个替换为对应仓库的行级写，其中 `finishFile` 在同一事务内先删除已存在的最终路径行、再改写 `.tmp` 行（对齐现有「同路径覆盖」语义） |
| `src/internal/engine/persist.go` | **删除**。JSON 读写逻辑迁移到 `store/`；注意文件内的 `TaskFile` 结构体被 task.go / progress.go / services 引用，需先迁入 `task.go`（同步更新 `persist_test.go`） |
| `src/internal/engine/iter.go` | ① 删除 `fingerprint()` / `Fingerprint()`（断点改为任务级，见 5.1）；② `finished` / `Finish()` / `SetFinished()` 改为 `"dialogID:messageID"` 字符串 key，`logicalPos` 相关字段同步移除；③ `sortDialogs` 保留（稳定遍历顺序，与指纹无关） |
| `src/internal/engine/elem.go` | `iterElem.logicalPos` 替换为断点 key（或直接复用 `info.DialogID/MessageID` 拼 key） |
| `src/internal/engine/progress.go` | ① `OnDone` 中 `it.Finish(e.logicalPos)` 改为按 `dialogID:messageID` 标记；② 文件状态变更在现有内存维护基础上增加 `file_repo` 行级写入，取代每次全量 `persist()`；③ `OnDone` 标记完成时同步 upsert `resume_points`（断点实时落盘——现状仅在 `execute` 出错返回时才保存断点，进程崩溃会丢失全部断点） |
| `src/internal/config/config.go` | ① 读写逻辑改为 YAML（库用已有的 `gopkg.in/yaml.v2`）；② 新增内部映射结构 `yamlConfig`（分组结构见 3.3）负责与扁平 `Settings` 互转，`Settings` 不加 yaml 标签、JSON 标签保留（Wails 前后端传输仍是 JSON）；③ `Settings` 新增 `CacheDir` 字段，`Manager` 新增 `CacheDir()` 方法（空值回退 `<DataDir>/cache`，见第 6 节）；④ 保留 `DataDir()` / `KVDir()` / `ScriptDir()` 与现有兼底逻辑 |
| `src/internal/services/thumbcache.go` | `newThumbCache` 改为接收根目录提供函数 `rootFn func() string`，`path()` 每次求值（缓存目录热生效，见 6.4）；`thumbcache_test.go` 同步注入固定 `t.TempDir()` |
| `src/internal/services/chat.go` | 构造 `thumbCache` 时注入 `func() string { return cfg.CacheDir() }` |
| `src/internal/services/download.go` | 新增 `AppendTaskItems(taskID string, opts engine.AppendOptions)` 方法暴露给前端 |
| `src/frontend/src/pages/TasksPage.vue` | ① 非运行态任务（paused/done/failed/canceled）增加「追加下载」按钮；② 打开 NewTaskDialog 时传入 `appendMode=true` 和 `taskID` |
| `src/frontend/src/components/NewTaskDialog.vue` | ① 支持 `appendMode` prop；② 追加模式下隐藏目录/脚本/选项选择（继承原任务），仅允许输入新 URLs/Selections |
| `src/frontend/src/stores/tasks.ts` | 新增 `appendTask(taskID, items)` action，调用 `Download.AppendTaskItems` |
| `src/frontend/src/types.ts` | 新增 `AppendOptions` 类型定义（与后端结构对齐）；`Settings` 增加 `cacheDir: string` |
| `src/frontend/src/pages/SettingsPage.vue` | 新增「存储」区块：缓存目录输入 + 浏览按钮（见 6.5） |
| `src/frontend/src/stores/settings.ts` | `defaults()` 增加 `cacheDir: ''`（旧配置缺该字段时兜底空串） |

### 4.3 保留不变

| 模块 | 原因 |
|------|------|
| BBolt KV（`ref/tdl/pkg/kv`） | Telegram 会话/peer 缓存存储由上游 tdl 决定，不改动（仅不再写入 `key.Resume` 断点数据） |
| `core/downloader` 下载核心 | 迭代器接口（`Next/Value/Err`）不变，仅 `iter` 实现层调整 |
| `src/internal/engine/selection.go` / `links.go` | URL/Selection 解析验证逻辑不变，`AppendItems` 直接复用 `NormalizeURLs` / `validateSelections` |
| `main.go` 的 `logging.yaml` 启动覆盖 | 与本次改造正交，保持现状 |

---

## 5. 关键设计决策

### 5.1 断点续传与追加的兼容性

**问题**：现有断点机制有两层耦合，追加后都会失效：

1. `fingerprint(dialogs)` 基于完整消息列表计算，追加后指纹变化，旧断点 key 查不到；
2. `finished` 集合的元素是 **逻辑位置** `logicalPos`（遍历序号，分组消息展开时动态推进），
   插入新消息会整体错位，即使指纹不变也会漏下/重下。

**方案**：彻底去掉 fingerprint，断点改为**任务级 + 内容坐标**：

```
断点定位：resume_points.task_id 主键（一任务一行，无需指纹）
finished 元素："dialogID:messageID"（内容坐标，与遍历顺序无关）
```

这样追加后：
- 已下载消息的 `dialogID:messageID` 仍在 `finished` 集合中，自然跳过
- 新消息不在集合中，自然会被下载
- 分组消息展开后每个成员有独立 message ID，比逻辑位置更稳健

**代码落点**：`iter.finished` 改为 `map[string]struct{}`；`progress.OnDone` 中
`it.Finish(e.logicalPos)` 改为用 `e.info.DialogID/MessageID` 拼 key；
`Task.opts.Restart` 路径改为删除 `resume_points` 行；任务成功完成后同样删行。

**旧断点数据不迁移**：旧格式（BBolt、逻辑位置集合）无法离线映射为内容坐标
（分组展开位置只在运行时可知）。接受一次性代价：升级后未完成任务首次恢复时，
已落盘文件由 `SkipSame`（同名同大小跳过）兼底避免重复下载；风险限于升级时
恰有未完成任务且未开 `SkipSame` 的场景，在发布说明中提示。

### 5.2 追加时的并发安全

**场景**：任务正在运行中，用户追加新消息。

**方案（v1 采用「重启式追加」，不做运行中热更新）**：

> 否决早期设想的「向 `iter.elem` chan 发 reload 信号」：`elem` 是 `chan downloader.Elem`，
> 由 `core/downloader` 消费，无法传递控制信号；且 dialogs 在 `execute()` 入口一次性
> 解析装配，运行中重建迭代器需侵入 `core/downloader` 内部，风险远大于收益。

```go
func (m *Manager) AppendItems(taskID string, items AppendOptions) error {
    // 1. 复用 NormalizeURLs / validateSelections 预检
    // 2. DB 事务：插入/合并 task_items（URL 靠部分唯一索引去重，
    //    Selection 按 dialog_id 合并 message_ids 并集）
    // 3. 若任务 running：复用 Pause 路径（paused=true + cancel），
    //    等 runDone 关闭后自动调用 Resume → 新一轮 execute() 从 DB 加载
    //    全量 task_items，断点集合（dialogID:messageID）保证已完成部分不重下
    // 4. 若任务 queued：仅写 DB——queued 阶段 t.cancel 尚为 nil（run() 未把状态
    //    置为 running），现有 Pause 会直接报错；由「execute() 每轮启动时从 DB
    //    重新组装 items」兜底，本轮自然包含新项（窗口期内无需重启）
    // 5. 若任务 paused/done/failed/canceled：仅写 DB，用户手动 Resume
    //    （done 任务追加后状态置回 paused，前端展示「可继续」）
}
```

> 前置约束：`execute()` 每轮入口处从 `task_items` 表重新组装 URLs/Selections
> （而非沿用内存 `t.opts`），这既是追加生效的基础，也自然覆盖 queued 窗口期。

锁约束：与现有代码一致，保持 `m.mu → t.mu` 的锁序；等待 `runDone` 时**不得持有任何锁**
（参考 `stopWait` 的先取 chan 再解锁模式）；DB 操作在事务内完成。
运行中热追加（不中断当前下载）作为后续优化项，不入本期范围。

### 5.3 配置与任务记录的迁移策略

**首次启动检测（config.Manager）**：

```go
func NewManager() (*Manager, error) {
    // 1. 检测 config.yaml 是否存在，存在则直接加载
    // 2. 若不存在但 settings.json 存在 → 读入后写出 config.yaml
    // 3. 迁移成功后把 settings.json 重命名为 settings.json.bak（不直接删除）
    // 4. 两者都不存在 → defaultSettings() 写出 config.yaml
}
```

**任务记录迁移（store.Store 初始化时）**：

```go
// tasks.db 不存在且 tasks.json 存在时：
// 1. loadRecords(tasks.json) → 单事务写入 tasks / task_items / files 三表
//    （taskRecord.Opts.URLs → url 行；Opts.Selections → selection 行）
// 2. running/queued 状态按现有 restore() 逻辑降级为 paused
// 3. 成功后 tasks.json 重命名为 tasks.json.bak
// 旧 BBolt 断点不迁移（理由见 5.1），遗留的 resume key 随旧数据自然废弃
```

---

## 6. 新增：缓存目录等存储配置

### 6.1 现状与问题

`services/thumbcache.go` 的 `newThumbCache()` 把缓存根硬编码为可执行文件目录下
`cache/`（子目录 `thumbs/`、`previews/`），由 `ChatService` 持有、经
`/media/thumb`、`/media/preview` 两个 AssetServer 路由对外服务：

1. **不可配置**：用户无法把缓存指向大容量磁盘，也无法把它从系统盘移走
2. **可写性风险**：Windows 安装到 `Program Files` 后 exe 目录通常只读，
   `writeFileAtomic` 持续失败，缓存退化为每次重新拉取（静默性能劣化）
3. **目录职责混乱**：配置/会话/脚本已统一在数据目录（`%AppData%\tdl-ui`），
   唯独缓存落在程序目录，升级/搬迁时容易遗漏或残留

### 6.2 配置设计

| 层 | 变更 |
|----|------|
| `Settings`（Go） | 新增 `CacheDir string \`json:"cacheDir"\``，空串 = 默认值 |
| `config.yaml` | `storage.cache_dir`（由 `yamlConfig` 映射，见 3.3） |
| 默认值 | 空 → `<DataDir>/cache`（**默认位置从 exe 目录迁至数据目录**，解决 6.1-2/3） |
| 校验 | `Manager.Update()` 对非空 `CacheDir` 执行 `MkdirAll` + 写探针文件校验，失败返回错误阻断保存 |

**旧缓存不迁移**：缩略图/预览均可重新拉取，代价仅为首次浏览时重建缓存；
不做 exe 目录旧 `cache/` 的探测与搬迁（与项目「禁止向后兼容」原则一致），
发布说明提示可手动删除旧目录。

### 6.3 后端改造

```go
// config/config.go
func (m *Manager) CacheDir() string {
    m.mu.RLock()
    dir := m.settings.CacheDir
    m.mu.RUnlock()
    if dir == "" {
        return filepath.Join(m.dataDir, "cache")
    }
    return dir
}

// services/thumbcache.go：根目录改为提供函数，path() 每次求值
func newThumbCache(rootFn func() string) *thumbCache { ... }

// services/chat.go
NewChatService: thumbs: newThumbCache(func() string { return cfg.CacheDir() })
```

可选增强（同期实现，成本低）：`SettingsService` 新增 `ClearCache() error`，
删除 `CacheDir()` 下 `thumbs/`、`previews/` 两个子目录并重建，供设置页「清空缓存」按钮调用；
只删这两个固定子目录，不对 `CacheDir()` 根做递归删除，避免用户误指向非专用目录时误删。

### 6.4 热生效与并发

- `path()` 每次调用 `rootFn()` 取当前配置，保存设置后新请求立即落到新目录，**无需重启**
- `inflight` singleflight 以完整路径为 key，切换目录前后的并发请求互不干扰，无需额外锁
- 切换瞬间正在写入旧目录的文件照常完成（路径已在任务开始时固定），仅成为孤儿缓存，无正确性风险

### 6.5 前端设置页

`SettingsPage.vue` 在「界面」与「日志」之间新增「存储」区块（并把现有「数据目录」只读项从「界面」移入该区块）：

```
存储
  数据目录        [只读 hint，现有 store.dataDir]
  缓存目录        [InputText v-model=settings.cacheDir] [📁 浏览]
                  hint：留空使用默认目录（<数据目录>\cache），保存后立即生效
  [清空缓存]      二次确认后调用 Settings.ClearCache，toast 反馈
```

- 浏览按钮复用现有 `Download.selectDirectory()`（与下载目录/日志目录同模式）
- `stores/settings.ts` 的 `defaults()` 补 `cacheDir: ''`，旧后端返回缺字段时表单不绑 `undefined`
- `types.ts` 的 `Settings` 同步增加 `cacheDir: string`

---

## 7. 实施步骤（建议顺序）

### Phase 1：SQLite 基础设施（1 天）

1. 引入依赖：`modernc.org/sqlite`（纯 Go，无需 CGO；Wails Windows 构建链不引入 gcc 依赖，不选 `mattn/go-sqlite3`）
2. 创建 `src/internal/store/` 包，实现 `Store` 结构体（含 PRAGMA 初始化、`user_version` 迁移）
3. 实现 `tasks.json` → 三表的一次性导入（见 5.3）
4. 编写 `task_repo.go`、`file_repo.go`、`resume_repo.go` 的基础 CRUD + 单测

### Phase 2：YAML 配置与存储目录（1 天）

1. 复用已有依赖 `gopkg.in/yaml.v2`（不新增 YAML 库）
2. 重写 `config/config.go`：新增 `yamlConfig` 分组映射结构，读写改 YAML，JSON 自动迁移
3. `Settings` 增加 `CacheDir` 字段 + `Manager.CacheDir()` + `Update()` 可写校验（见 6.2/6.3）
4. `thumbcache.go` 改 `rootFn` 注入，`chat.go` 接入 `cfg.CacheDir`，同步更新 `thumbcache_test.go`
5. 验证设置页正常读写（SettingsService 接口签名不变；可选 `ClearCache` 为新增方法）

### Phase 3：引擎层改造（2 天）

1. `TaskFile` 结构迁入 `task.go`，删除 `persist.go` / `persist_test.go`
2. 改造 `Manager`：注入 `*store.Store`，删除 `statePath` / `restore()` / `persist()`
3. 改造 `iter.go` / `elem.go` / `progress.go`：断点 key 改为 `dialogID:messageID`，删除 fingerprint / logicalPos
4. `loadResume` / `saveResume` 改走 `resume_repo`；`Restart` 与任务完成路径改为删行
5. 实现 `AppendItems()`（重启式追加，见 5.2）

### Phase 4：前端改造（1 天）

1. `types.ts` 新增 `AppendOptions`、`Settings.cacheDir`；`NewTaskDialog.vue` 支持 `appendMode`
2. `TasksPage.vue` 添加「追加下载」入口（非运行态任务）
3. `stores/tasks.ts` 新增 `appendTask()` action
4. `SettingsPage.vue` 新增「存储」区块（缓存目录 + 清空缓存，见 6.5）；`stores/settings.ts` 补默认值

### Phase 5：测试与验证（0.5 天）

1. 断点续传功能回归测试（含分组消息场景：新断点 key 对分组展开成员逐个生效）
2. 追加下载功能测试（运行中追加自动重启、暂停后追加、完成后追加再恢复、重复追加去重）
3. 迁移测试（settings.json → config.yaml；tasks.json → tasks.db；无旧数据首次启动）
4. 缓存目录测试（默认路径落在数据目录、切换目录保存后新请求热生效、不可写目录被拒、清空缓存）
5. 大数据量性能测试（1000+ 任务，重点对比原全量 JSON 写盘路径）

---

## 8. 风险与回滚

| 风险 | 缓解措施 |
|------|---------|
| SQLite 在 Windows 下文件锁定问题 | 使用 `modernc.org/sqlite` + WAL + `busy_timeout`；单进程单连接池，风险可控 |
| 旧断点不迁移导致升级后重复下载 | 旧格式无法离线映射（见 5.1）；靠 `SkipSame` 兼底 + 发布说明提示升级前先完成或重启任务 |
| 任务记录迁移失败 | 迁移在单事务内完成，失败则删除半成品 tasks.db 并保留 tasks.json，下次启动重试 |
| 配置迁移失败 | 保留 `settings.json.bak`；迁移失败时回退读取旧配置并上报日志 |
| 追加时任务状态不一致 | `AppendItems()` 遵守 `m.mu → t.mu` 锁序，等待 runDone 时不持锁，DB 操作在事务内完成 |
| 行级写入频率过高（每文件多次 update） | 进度计数（total/finished/failed）可延迟到状态变更/退出时落盘；文件行写入本身频率 = 文件数量级，远低于现状每次全量重写 tasks.json |
| 用户配置的缓存目录不可写 | `Update()` 保存前 `MkdirAll` + 写探针文件校验，失败拒绝保存并提示（见 6.2） |
| 缓存默认位置变更/切换目录后旧缓存失效 | 缓存均可重新拉取，不迁移；发布说明提示可手动删除旧 `cache/` 目录 |

> 已否决「新旧存储双写观察期」：断点 key 语义已变（逻辑位置 → 内容坐标），
> 双写无法互验，只会增加复杂度；回滚靠 `.bak` 文件 + 旧版本二进制即可。

---

## 9. 文件变更汇总

### 新增文件

```
src/internal/store/
    store.go          -- DB 连接、PRAGMA、user_version 迁移、tasks.json 导入
    task_repo.go      -- 任务表 + task_items 表操作
    file_repo.go      -- 文件表操作
    resume_repo.go    -- 断点表操作
    store_test.go     -- 建表/迁移/CRUD 单测
doc/todo/
    sqlite-yaml-append-plan.md  -- 本方案
```

### 修改文件

```
src/go.mod                          -- 新增 modernc.org/sqlite 依赖（YAML 复用现有 yaml.v2）
src/internal/engine/task.go         -- 重构 Manager/Task，迁入 TaskFile，新增 AppendItems
src/internal/engine/iter.go         -- 删 fingerprint，断点 key 改内容坐标
src/internal/engine/elem.go         -- logicalPos 替换为断点 key
src/internal/engine/progress.go     -- Finish 改 key，文件状态行级写 DB，断点实时落盘
src/internal/config/config.go       -- YAML 化（yamlConfig 映射）+ 迁移 + CacheDir
src/internal/services/thumbcache.go -- 缓存根改 rootFn 注入（含 thumbcache_test.go）
src/internal/services/chat.go       -- thumbCache 接入 cfg.CacheDir
src/internal/services/settings.go   -- 可选：新增 ClearCache 方法
src/internal/services/download.go   -- 新增 AppendTaskItems 接口
src/frontend/src/pages/TasksPage.vue           -- 追加入口
src/frontend/src/pages/SettingsPage.vue        -- 新增「存储」区块（缓存目录）
src/frontend/src/components/NewTaskDialog.vue  -- 追加模式
src/frontend/src/stores/tasks.ts               -- appendTask action
src/frontend/src/stores/settings.ts            -- cacheDir 默认值
src/frontend/src/types.ts                      -- AppendOptions 类型、Settings.cacheDir
```

### 删除文件

```
src/internal/engine/persist.go       -- TaskFile 迁入 task.go，JSON 读写迁入 store/（仅作一次性导入）
src/internal/engine/persist_test.go  -- 随 persist.go 删除，用例迁入 store_test.go
```

---

## 10. 附录：追加下载的交互流程

```
用户点击「追加下载」
    │
    ▼
NewTaskDialog 以 appendMode 打开（目录/脚本/选项置灰，继承原任务）
    │
    ▼
用户输入新 URLs / 选择新消息
    │
    ▼
前端调用 Download.AppendTaskItems(taskID, items)
    │
    ▼
后端 AppendItems()：
    ├─ 预检（NormalizeURLs / validateSelections）
    ├─ DB 事务插入/合并 task_items（重复项去重）
    ├─ 若任务 running → 暂停并等待退出 → 自动 Resume
    │   （新一轮 execute() 从 DB 加载全量 items，
    │     断点集合保证已完成消息不重下）
    └─ 若任务非运行态 → 仅写 DB，用户手动 Resume 时自然包含新消息
    │
    ▼
前端收到 task:update 事件，进度条更新
```

---

*文档版本：v1.2（二次与 src/internal、src/frontend 实际代码核对后修订，并新增缓存目录配置方案）*  
*创建时间：2026-07-28*  
*修订时间：2026-07-29 00:44:04*  
*作者：AI 助手*  
