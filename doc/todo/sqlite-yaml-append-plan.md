# 下载引擎重构方案：SQLite 持久化 + YAML 配置 + 任务追加

> 状态：待实施  
> 优先级：P1（架构级改造）  
> 预估工期：3-5 天  

---

## 1. 改造目标

| 目标 | 现状 | 目标态 |
|------|------|--------|
| 下载记录持久化 | JSON 文件（`tasks.json`）全量读写 | SQLite 关系型存储 |
| 应用配置 | JSON（`settings.json`） | YAML（`config.yaml`） |
| 任务追加 | 不支持（一个 Task 对应固定 URLs/Selections） | 支持向已有任务动态追加消息 |
| 断点续传 | BBolt KV 存储（`key.Resume(fingerprint)`） | SQLite 表内维护，支持追加后增量续传 |

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
| 前端任务页 | `src/frontend/src/pages/TasksPage.vue` | 任务列表展示、操作按钮 |
| 前端任务 Store | `src/frontend/src/stores/tasks.ts` | 任务列表缓存、`task:update` / `task:file` 事件订阅 |
| 新建任务弹窗 | `src/frontend/src/components/NewTaskDialog.vue` | 创建任务表单 |

### 2.3 当前限制

1. **Task 创建后不可变**：`TaskOptions`（URLs / Selections）在 `Create()` 时固定，无修改入口
2. **JSON 全量覆盖**：每次文件状态变化都重写整个 `tasks.json`，任务量大时性能下降
3. **断点与消息列表双重耦合**：① `fingerprint(dialogs)` 基于完整消息列表，追加后指纹变化导致断点 key 失效；② `finished` 集合存的是遍历序号 `logicalPos`，插入新消息会整体错位（见 5.1）
4. **配置格式单一**：JSON 不支持注释，用户手动编辑体验差

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
# config.yaml（替代 settings.json，字段与现有 Settings 结构一一对应，不得遗漏）
app:
  theme: system           # light / dark / system
  proxy: ""               # socks5://127.0.0.1:1080

download:
  dir: "~/Downloads/tdl-ui"
  template: "{{ .DialogID }}_{{ .MessageID }}_{{ filenamify .FileName }}"
  threads: 4              # 单文件下载线程数
  limit: 2                # 同时下载最大文件数
  pool_size: 8            # DC 连接池大小

# 日志配置（对应 Settings.Log；logging.LogSettings 已带 yaml 标签，直接内嵌复用）
log:
  targets: file
  level: info
  dir: ""
  format: text
  maxSizeMb: 10
  maxAgeDays: 7
  maxBackups: 5

# 保留：最近一次登录用户信息（仅展示用）
session:
  logged_in_user_id: 0
  logged_in_username: ""
```

> 决策说明：
> - YAML 库沿用项目已直接依赖的 `gopkg.in/yaml.v2`（`internal/logging` 已在用），**不引入** `gopkg.in/yaml.v3`，避免同仓库双 YAML 依赖。
> - 数据目录下独立的 `logging.yaml` 启动覆盖机制（`main.go`）与 LogService 的导入/导出功能保持不变；`config.yaml` 的 `log` 段是持久化配置源，`logging.yaml` 仅作一次性外部覆盖入口。

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
| `src/internal/engine/task.go` | ① `Manager` 注入 `*store.Store` 替代 `statePath`，删除 `restore()` / `persist()`（启动时从 DB 加载，运行中行级写入）；② `Task` 运行时仍持有 `opts` 与内存态（view/计数仍要用），但 `opts` 改为创建/恢复时由 `task_items` 表组装；③ `Create()` 改为事务插入 tasks + task_items 后启动 goroutine；④ 新增 `AppendItems(taskID, items)` 方法；⑤ `loadResume` / `saveResume` 改走 `resume_repo`，断点集合类型由 `map[int]struct{}` 改为 `map[string]struct{}`（key 为 `"dialogID:messageID"`） |
| `src/internal/engine/persist.go` | **删除**。JSON 读写逻辑迁移到 `store/`；注意文件内的 `TaskFile` 结构体被 task.go / progress.go / services 引用，需先迁入 `task.go`（同步更新 `persist_test.go`） |
| `src/internal/engine/iter.go` | ① 删除 `fingerprint()` / `Fingerprint()`（断点改为任务级，见 5.1）；② `finished` / `Finish()` / `SetFinished()` 改为 `"dialogID:messageID"` 字符串 key，`logicalPos` 相关字段同步移除；③ `sortDialogs` 保留（稳定遍历顺序，与指纹无关） |
| `src/internal/engine/elem.go` | `iterElem.logicalPos` 替换为断点 key（或直接复用 `info.DialogID/MessageID` 拼 key） |
| `src/internal/engine/progress.go` | ① `OnDone` 中 `it.Finish(e.logicalPos)` 改为按 `dialogID:messageID` 标记；② 文件状态变更在现有内存维护基础上增加 `file_repo` 行级写入，取代每次全量 `persist()` |
| `src/internal/config/config.go` | ① 读写逻辑改为 YAML（库用已有的 `gopkg.in/yaml.v2`）；② `Settings` 字段补加 `yaml` 标签，JSON 标签保留（Wails 前后端传输仍是 JSON）；③ 保留 `DataDir()` / `KVDir()` / `ScriptDir()` 与现有兼底逻辑 |
| `src/internal/services/download.go` | 新增 `AppendTaskItems(taskID string, opts engine.AppendOptions)` 方法暴露给前端 |
| `src/frontend/src/pages/TasksPage.vue` | ① 非运行态任务（paused/done/failed/canceled）增加「追加下载」按钮；② 打开 NewTaskDialog 时传入 `appendMode=true` 和 `taskID` |
| `src/frontend/src/components/NewTaskDialog.vue` | ① 支持 `appendMode` prop；② 追加模式下隐藏目录/脚本/选项选择（继承原任务），仅允许输入新 URLs/Selections |
| `src/frontend/src/stores/tasks.ts` | 新增 `appendTask(taskID, items)` action，调用 `Download.AppendTaskItems` |
| `src/frontend/src/types.ts` | 新增 `AppendOptions` 类型定义（与后端结构对齐） |

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
    // 3. 若任务 running/queued：复用 Pause 路径（paused=true + cancel），
    //    等 runDone 关闭后自动调用 Resume → 新一轮 execute() 从 DB 加载
    //    全量 task_items，断点集合（dialogID:messageID）保证已完成部分不重下
    // 4. 若任务 paused/done/failed/canceled：仅写 DB，用户手动 Resume
    //    （done 任务追加后状态置回 paused，前端展示「可继续」）
}
```

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

## 6. 实施步骤（建议顺序）

### Phase 1：SQLite 基础设施（1 天）

1. 引入依赖：`modernc.org/sqlite`（纯 Go，无需 CGO；Wails Windows 构建链不引入 gcc 依赖，不选 `mattn/go-sqlite3`）
2. 创建 `src/internal/store/` 包，实现 `Store` 结构体（含 PRAGMA 初始化、`user_version` 迁移）
3. 实现 `tasks.json` → 三表的一次性导入（见 5.3）
4. 编写 `task_repo.go`、`file_repo.go`、`resume_repo.go` 的基础 CRUD + 单测

### Phase 2：YAML 配置（0.5 天）

1. 复用已有依赖 `gopkg.in/yaml.v2`（不新增 YAML 库）
2. 重写 `config/config.go`：`Settings` 补 yaml 标签，读写改 YAML，JSON 自动迁移
3. 验证设置页正常读写（SettingsService 接口签名不变，前端无需改动）

### Phase 3：引擎层改造（2 天）

1. `TaskFile` 结构迁入 `task.go`，删除 `persist.go` / `persist_test.go`
2. 改造 `Manager`：注入 `*store.Store`，删除 `statePath` / `restore()` / `persist()`
3. 改造 `iter.go` / `elem.go` / `progress.go`：断点 key 改为 `dialogID:messageID`，删除 fingerprint / logicalPos
4. `loadResume` / `saveResume` 改走 `resume_repo`；`Restart` 与任务完成路径改为删行
5. 实现 `AppendItems()`（重启式追加，见 5.2）

### Phase 4：前端改造（1 天）

1. `types.ts` 新增 `AppendOptions`；`NewTaskDialog.vue` 支持 `appendMode`
2. `TasksPage.vue` 添加「追加下载」入口（非运行态任务）
3. `stores/tasks.ts` 新增 `appendTask()` action

### Phase 5：测试与验证（0.5 天）

1. 断点续传功能回归测试（含分组消息场景：新断点 key 对分组展开成员逐个生效）
2. 追加下载功能测试（运行中追加自动重启、暂停后追加、完成后追加再恢复、重复追加去重）
3. 迁移测试（settings.json → config.yaml；tasks.json → tasks.db；无旧数据首次启动）
4. 大数据量性能测试（1000+ 任务，重点对比原全量 JSON 写盘路径）

---

## 7. 风险与回滚

| 风险 | 缓解措施 |
|------|---------|
| SQLite 在 Windows 下文件锁定问题 | 使用 `modernc.org/sqlite` + WAL + `busy_timeout`；单进程单连接池，风险可控 |
| 旧断点不迁移导致升级后重复下载 | 旧格式无法离线映射（见 5.1）；靠 `SkipSame` 兼底 + 发布说明提示升级前先完成或重启任务 |
| 任务记录迁移失败 | 迁移在单事务内完成，失败则删除半成品 tasks.db 并保留 tasks.json，下次启动重试 |
| 配置迁移失败 | 保留 `settings.json.bak`；迁移失败时回退读取旧配置并上报日志 |
| 追加时任务状态不一致 | `AppendItems()` 遵守 `m.mu → t.mu` 锁序，等待 runDone 时不持锁，DB 操作在事务内完成 |
| 行级写入频率过高（每文件多次 update） | 进度计数（total/finished/failed）可延迟到状态变更/退出时落盘；文件行写入本身频率 = 文件数量级，远低于现状每次全量重写 tasks.json |

> 已否决「新旧存储双写观察期」：断点 key 语义已变（逻辑位置 → 内容坐标），
> 双写无法互验，只会增加复杂度；回滚靠 `.bak` 文件 + 旧版本二进制即可。

---

## 8. 文件变更汇总

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
src/internal/engine/progress.go     -- Finish 改 key，文件状态行级写 DB
src/internal/config/config.go       -- YAML 化 + 迁移
src/internal/services/download.go   -- 新增 AppendTaskItems 接口
src/frontend/src/pages/TasksPage.vue           -- 追加入口
src/frontend/src/components/NewTaskDialog.vue  -- 追加模式
src/frontend/src/stores/tasks.ts               -- appendTask action
src/frontend/src/types.ts                      -- AppendOptions 类型
```

### 删除文件

```
src/internal/engine/persist.go       -- TaskFile 迁入 task.go，JSON 读写迁入 store/（仅作一次性导入）
src/internal/engine/persist_test.go  -- 随 persist.go 删除，用例迁入 store_test.go
```

---

## 9. 附录：追加下载的交互流程

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

*文档版本：v1.1（经与 src/internal 实际代码核对后修订）*  
*创建时间：2026-07-28*  
*修订时间：2026-07-28*  
*作者：AI 助手*  
