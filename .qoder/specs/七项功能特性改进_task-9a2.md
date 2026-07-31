# 七项功能特性改进实施计划

## Summary

七项需求按"后端持久化 → 绑定重生成 → 前端页面"三层推进。已确认的设计取舍（来自用户确认）：

- 脚本启用开关**仅控制可选性**：启用的脚本才出现在「添加下载」弹窗的脚本下拉中，引擎执行链路（`opts.ScriptName` 单脚本）不改。
- 界面偏好（日志字号、滚动条尺寸）持久化到 **config.yaml 的 `ui` 段**，并在设置页提供表单项。
- 对话页恢复范围：**对话选中 + 已加载媒体 + 筛选条件 + 滚动位置**，不自动重开 MediaPreview 浮层。
- 目录历史**按用途分组**，每组 3 条：`download` / `logExport` / `cache` / `temp` / `logDir`。

---

## 一、Go 后端：配置持久化

### `src/internal/config/config.go`

`Settings` 新增三组字段，并在 `yamlConfig` 中落到对应段：

```go
// UISettings 界面偏好（日志页字号与滚动条尺寸）
type UISettings struct {
    LogFontSize   int `json:"logFontSize" yaml:"logFontSize"`     // 默认 14，钳制 10..28
    ScrollbarSize int `json:"scrollbarSize" yaml:"scrollbarSize"` // 默认 10，钳制 6..24
}
```

- `Settings.UI UISettings`（json `ui`）→ yamlConfig 新增 `UI UISettings \`yaml:"ui"\``。
- `Settings.RecentDirs map[string][]string`（json `recentDirs`）→ yamlConfig `Storage` 之后新增 `RecentDirs map[string][]string \`yaml:"recentDirs"\``。
- `Settings.EnabledScripts []string`（json `enabledScripts`）+ `Settings.TemplatesSeeded bool`（json `templatesSeeded`）→ yamlConfig 新增
  ```go
  Scripts struct {
      Enabled         []string `yaml:"enabled"`
      TemplatesSeeded bool     `yaml:"templatesSeeded"`
  } `yaml:"scripts"`
  ```
- `settingsToYAML` / `yamlToSettings` 双向补齐上述字段。
- `defaultSettings` 中 `UI: UISettings{LogFontSize: 14, ScrollbarSize: 10}`。
- `Update()` 内新增 `s.UI = s.UI.withDefaults()`（零值回默认 + 上下限钳制）、`RecentDirs` nil 兜底为空 map、每个 kind 裁剪至 3 条。
- `verifyWritableDir` 提升为导出 `VerifyWritableDir`（日志导出复用同一口径）。

新增 Manager 方法（均在 `mu` 保护下操作后调用 `save()`）：

```go
const MaxRecentDirs = 3
var recentDirKinds = map[string]struct{}{"download":{}, "logExport":{}, "cache":{}, "temp":{}, "logDir":{}}

func (m *Manager) RecentDirs(kind string) []string
func (m *Manager) AddRecentDir(kind, dir string) ([]string, error) // 校验 kind 白名单；去重、置顶、裁剪 3
func (m *Manager) IsScriptEnabled(name string) bool
func (m *Manager) SetScriptEnabled(name string, on bool) error
func (m *Manager) ForgetScript(name string) error                  // 删除脚本时清理启用记录
func (m *Manager) MarkTemplatesSeeded() error
```

非法 kind 返回 `errors.Errorf("未知目录用途: %q", kind)`。

---

## 二、Go 后端：内置脚本模板

### 新增 `src/internal/script/templates/` （四个 `.go.txt` 文件，Go 工具链不编译）

| 文件 | 用途 |
| --- | --- |
| `rename-by-date.go.txt` | 文件重命名：按年月分目录 + 对话 ID/消息 ID 前缀 |
| `filter-media.go.txt` | 过滤规则：类型白名单 + 大小区间 + 关键词 |
| `skip-duplicates.go.txt` | 过滤规则：扩展名/关键词黑名单跳过 |
| `auto-archive.go.txt` | 自动化处理：`OnTaskStart` / `OnFileDone` / `OnTaskDone` 钩子清单与汇总输出 |

每个文件头部为固定格式元信息注释块（保存为脚本后即成为该脚本的说明来源）：

```go
// 【功能】按消息日期归档并重命名下载文件。
// 【参数】subDirLayout：目录层级模板；keepOriginalName：是否保留原文件名。
// 【注意】Yaegi 沙箱内仅可用 tdlui/api 与部分标准库；单次调用超时保护生效，勿做阻塞操作。
package main
```

### 新增 `src/internal/script/templates.go`

```go
//go:embed templates/*.go.txt
var templateFS embed.FS

type Template struct {
    ID, Name, Category, Description, Notes, Source string
}
func Templates() []Template          // manifest 内联元信息 + 从 embed 读 Source
```

### `src/internal/script/store.go`

- `Meta` 新增 `Enabled bool \`json:"enabled"\`` 与 `Description string \`json:"description"\``。
- `List()` 读取每个脚本文件前 4KB，提取 `package` 之前连续 `//` 行首段（截断 200 字符）作为 `Description`；`Enabled` 留给服务层填充。
- 新增 `SeedTemplates() (int, error)`：把 `Templates()` 逐个写入 `<ScriptDir>/<ID>.go`，**已存在同名文件则跳过**，返回新建数量。

---

## 三、Go 后端：服务层

### `src/internal/services/script.go`

- `NewScriptService(store *script.Store, cfg *config.Manager)`。
- `List()`：调 `store.List()` 后按 `cfg.IsScriptEnabled(name)` 填 `Enabled`。
- 新增 `SetEnabled(name string, enabled bool) error`（Info 日志 + `cfg.SetScriptEnabled`）。
- 新增 `Templates() []script.Template`。
- `Delete()` 成功后调 `cfg.ForgetScript(name)`。

### `src/internal/services/download.go`

`SelectDirectory` 改签名（无需向后兼容）：

```go
func (s *DownloadService) SelectDirectory(kind, current string) (string, error)
```

`DefaultDirectory` 取值优先级：`current` → `cfg.RecentDirs(kind)[0]` → `cfg.Get().DownloadDir`；用户选定后 `cfg.AddRecentDir(kind, picked)`（失败仅 Warn，不阻断返回）。

### `src/internal/services/logsvc.go`

新增：

```go
// ExportLogs 把前端组装的日志文本写入 dir/filename，返回完整路径。
func (s *LogService) ExportLogs(dir, filename, content string) (string, error)
```

校验顺序：`filename != "" && filename == filepath.Base(filename)`；扩展名白名单 `.log/.txt/.csv`；`dir != ""`；`config.VerifyWritableDir(dir)`；`os.WriteFile(..., 0o644)`；成功后 `cfg.AddRecentDir("logExport", dir)` 并 `logLog.Infof`。

### `src/internal/services/settings.go`

新增 `RecentDirs(kind string) []string`、`AddRecentDir(kind, dir string) ([]string, error)`（手工输入目录的入库通道）。

### `src/main.go`

- `scriptSvc := services.NewScriptService(scriptStore, cfg)`。
- 在 `scriptStore` 创建后、`taskManager` 之前插入首次播种：
  ```go
  if !cfg.Get().TemplatesSeeded {
      if n, err := scriptStore.SeedTemplates(); err != nil {
          logApp.Warnf("写入内置脚本模板失败: %v", err)
      } else {
          _ = cfg.MarkTemplatesSeeded()
          logApp.Infof("已写入 %d 个内置脚本模板（默认禁用）", n)
      }
  }
  ```
  已播种标记保证用户删除后不会被重新塞回。

---

## 四、Go 单元测试

- `config_test.go` 追加：`AddRecentDir` 去重/置顶/上限 3/非法 kind 报错；`UI` 零值回默认与上下限钳制；`SetScriptEnabled` + 重新 `NewManagerAt` 往返读取；`TemplatesSeeded` 往返。
- 新增 `src/internal/script/templates_test.go`：遍历 `Templates()`，断言 `Source` 非空、`script.Load(Source)` 成功且至少含一个契约函数、元信息注释三要素（功能/参数/注意）齐全。
- 新增 `src/internal/services/logsvc_export_test.go`：`t.TempDir()` 下正常写入；`filename` 含路径分隔符被拒；非白名单扩展名被拒。

---

## 五、绑定重生成与 api.ts

1. `cd src; wails generate module`（或跑一次 `wails dev`）刷新 `src/frontend/wailsjs/`。
2. `src/frontend/src/api.ts` 补充导出：
   - `Script.setEnabled`、`Script.templates`
   - `SettingsApi.recentDirs`、`SettingsApi.addRecentDir`
   - `LogApi.exportLogs`
   - `Download.selectDirectory` 全部调用点改为传 `(kind, current)`
3. `src/frontend/src/types.ts` 新增别名 `export type ScriptTemplate = script.Template`。

---

## 六、前端：目录历史组件

### 新增 `src/frontend/src/components/DirSelect.vue`

- props：`modelValue: string`、`kind: string`、`placeholder?: string`、`inputId?: string`；emits `update:modelValue`。
- 结构：`<Select editable>`（options 为最近 3 条历史）+ 尾随 `pi pi-folder-open` 浏览按钮，沿用现有 `.form-row > .grow` 布局。
- `onMounted`：`SettingsApi.recentDirs(kind)` 填充 options；若 `modelValue` 为空则默认选中 `options[0]`（满足"默认选中最近一次"）。
- 浏览：`Download.selectDirectory(kind, modelValue)` → 非空则写回并刷新 options。
- 暴露 `defineExpose({ commit })`：`commit()` 调 `SettingsApi.addRecentDir(kind, value)` 并刷新 options，由调用方在"确认/保存"动作成功后调用，保证手工输入的目录也进历史。
- 所有后端调用 `try/catch` 静默（非 Wails 环境）。

### 接入点

- `components/NewTaskDialog.vue`：保存目录输入替换为 `<DirSelect kind="download" v-model="dir" />`，`create()` 成功后 `dirSelect.value?.commit()`；删除原 `browse()`。
- `pages/SettingsPage.vue`：`downloadDir`（kind `download`）、`cacheDir`（`cache`）、`tempDir`（`temp`）、`log.dir`（`logDir`）四处改用 DirSelect，`save()` 成功后依次 `commit()`；删除 `browse/browseCacheDir/browseTempDir/browseLogDir`。

---

## 七、前端：日志页字号 / 滚动条 / 导出

### `pages/LogsPage.vue`

字号（需求 2）：

- 字号来源 `settings.settings.ui.logFontSize`，默认值由后端给到 14px（比现状 12px 增大）。
- 新增 `const rowHeight = computed(() => Math.round(fontSize.value * 1.8))`，`VirtualScroller :item-size="rowHeight"`；`.log-row` 的 `height/line-height` 与 `.logs-body` 的 `font-size` 改由内联 CSS 变量 `--log-font-size` / `--log-row-height` 驱动，保证虚拟滚动不错位。
- `.logs-body-wrap` 上 `@wheel="onWheel"`：`if (!ev.ctrlKey) return; ev.preventDefault();` 按 `deltaY` 符号 ±1，`clamp(10, 28)`；改动即时写入 store（实时生效），并以 600ms 防抖调 `settings.save()` 持久化（`onBeforeUnmount` 清定时器）。
- 工具栏增加字号 `-` / `A` / `+` 三键组（tooltip「Ctrl + 滚轮调整字号」），供无滚轮场景使用；`A` 复位到 14。

滚动条（需求 3）：

- `--logs-scrollbar-size` 由 `settings.settings.ui.scrollbarSize` 注入到 `.logs-body-wrap` 内联 style。
- 新增样式：
  ```css
  .logs-body :deep(.p-virtualscroller)::-webkit-scrollbar { width: var(--logs-scrollbar-size); height: var(--logs-scrollbar-size); }
  .logs-body :deep(.p-virtualscroller)::-webkit-scrollbar-thumb { background: var(--p-surface-600); border-radius: calc(var(--logs-scrollbar-size) / 2); }
  .logs-body :deep(.p-virtualscroller)::-webkit-scrollbar-track { background: transparent; }
  ```
- 设置页「界面」段新增两个 `InputNumber`：日志字体大小（10–28）、滚动条尺寸（6–24），hint 注明 Ctrl+滚轮亦可调整字号。

导出（需求 4）：

- 工具栏「打开日志目录」左侧新增 `pi pi-file-export` 按钮，打开导出弹窗，透传 `rows`（当前过滤结果）与全量行（实时源 `logs.entries` / 历史 `historyLines`）。

### 新增 `src/frontend/src/components/ExportLogsDialog.vue`

- 字段：范围（当前过滤结果 / 当前数据源全部）、格式（`.log` / `.txt` / `.csv`）、目录（`<DirSelect kind="logExport">`）、文件名（默认 `tdl-ui-log-<yyyyMMdd-HHmmss>`，扩展名随格式自动拼接并禁止用户手填扩展名）。
- 内容组装：`.log/.txt` 为逐行原文；`.csv` 表头 `time,level,module,source,message`，字段按 RFC4180 加引号转义（含逗号/引号/换行时），未结构化解析的行整行落入 `message`。
- 确认 → `LogApi.exportLogs(dir, filename, content)` → 成功 toast 展示返回的完整路径，失败 toast 错误；随后 `dirSelect.commit()`。

---

## 八、前端：对话页状态保持（需求 1）

### `composables/useMediaPager.ts`

- `Snapshot` 增加 `scrollTop: number`。
- options 增加 `scrollTop?: () => number`，`saveSnapshot()` 内部写入 `options.scrollTop?.() ?? 0`。
- 新增返回 `restoredScrollTop: Ref<number>`：`switchTo` 命中缓存时置为快照值，未命中置 0。

### `pages/ChatsPage.vue`

- 常量 `const LAST_DIALOG_KEY = 'chats.lastDialogId'`（与既有 `chats.mediaLayout` 同一 localStorage 约定）。
- `watch(selectedId)`：非空写 `localStorage.setItem`，为空则 `removeItem`。
- `onMounted`：`route.params.id` 优先；无 id 时读 localStorage，命中则 `selectedId = last` 且 `router.replace({ path: '/chats/'+last, query: { type, title } })`（type/title 由 `chats.byId` 命中时带上，未命中留空由既有兜底逻辑处理）。
- 滚动恢复：`switchTo` 返回 true 时置 `restoring = true`，`await nextTick()` → `requestAnimationFrame(() => { scrollEl.value!.scrollTop = restoredScrollTop.value; restoring = false })`；`autoContinue` 回调改为 `() => sentinelVisible && !restoring`，避免恢复前哨兵在 `scrollTop=0` 处抢先触发 `loadMore` 打乱分页。
- `useMediaPager` 传入 `scrollTop: () => scrollEl.value?.scrollTop ?? 0`，`onBeforeUnmount` 现有 `saveSnapshot()` 即自动带上滚动位置。
- 登出 watch 中追加 `localStorage.removeItem(LAST_DIALOG_KEY)`。
- 筛选条件回填已由 `MediaToolbar` 的 `applied` watch 覆盖，无需改动。

---

## 九、前端：脚本页模板与启用开关（需求 6、7）

### `stores/scripts.ts`

- 新增 action `setEnabled(name: string, v: boolean)`：调 `Script.setEnabled` 后 `refresh()`，抛错交页面 toast。
- 新增 getter `enabled`：`scripts.filter(s => s.enabled)`。

### `pages/ScriptsPage.vue`

- 列表项左侧加 `<Checkbox binary @click.stop>` 绑定 `s.enabled`，`@update:model-value` 调 `scripts.setEnabled`；失败 toast 并 `scripts.refresh()` 回滚显示。
- 列表项 `meta` 行下方展示 `s.description`（两行截断 + `:title` 全文），作为脚本元信息说明。
- 列表头新增「模板」按钮（`pi pi-copy`）→ 打开模板选择弹窗（新增 `components ScriptTemplateDialog.vue`，或以 `Select` + 说明面板实现于本页；采用弹窗，内容为模板卡片列表：名称、分类 Tag、描述、注意事项）。选中后走 `confirmDiscard`：`source = tpl.source`、`currentName = tpl.id`、`editing = true`。
- 空态文案改为「还没有脚本，可点击「模板」从内置示例开始」。
- 编辑器上方 hint 增加一行：未启用的脚本不会出现在「添加下载」弹窗的脚本下拉中。

### `components/NewTaskDialog.vue`

- `scriptOptions` 改为仅取 `scripts.scripts.filter(s => s.enabled)`；无可选项时 `placeholder` 显示「无已启用脚本（到脚本页启用）」。

---

## 十、Test Plan

1. `cd src; go build ./...; go test ./...`（新增 config / templates / logsvc 三组用例全绿）。
2. `cd src/frontend; npm run build`（vue-tsc 类型检查通过，含新绑定类型）。
3. `cd src; wails dev`，用 chrome-devtools mcp 逐项验收：
   - 对话页选中某对话并向下滚动 → 切到「下载」「设置」→ 回到「对话」：对话、媒体列表、筛选、滚动位置一致，无重新拉取（Network 无新增 ListMedia）。
   - 日志页默认字号明显大于旧 12px；Ctrl+滚轮上下调整实时生效且不出现行错位；切页返回后字号保持。
   - 设置页调整滚动条尺寸并保存 → 日志页滚动条宽度随之变化。
   - 日志页导出：弹窗选择格式 `.csv` 与目录 → 成功 toast 显示路径，文件存在且内容含表头与命中行。
   - 任一目录弹窗：连选 3 个不同目录后下拉出现 3 条历史，最近一次置顶且默认选中；重启应用后历史仍在（校验 `%AppData%\tdl-ui\config.yaml` 的 `recentDirs` 段）。
   - 脚本页首启动可见 4 个内置脚本，全部未勾选；勾选一个 → 「添加下载」弹窗下拉仅出现该脚本；重启后勾选态保持（校验 config.yaml `scripts.enabled`）。
   - 「模板」弹窗选中模板 → 编辑器灌入完整可编辑代码，「校验」返回契约函数列表。
4. 文档与提交：CHANGELOG.md 追加条目（日期用 `Get-Date` 取真实时间到秒）；`doc/tutorials` 与 `doc/dev-human` 中脚本页、日志页相关章节补充新功能说明；提交走 smart-commit 技能，版本号变更时先用 `python -c "import json; json.load(...)"` 校验 wails.json / package.json 语法。

---

## Assumptions

- 日志导出内容取自前端当前已加载数据（实时源环形缓冲、历史文件尾部各 5000 行上限不变），不新增后端全量文件导出通道。
- 导出格式限定 `.log` / `.txt` / `.csv` 三种。
- 内置模板首次启动播种为普通脚本文件，默认禁用；用户删除后不再自动恢复，可从「模板」弹窗重新生成。
- 滚动条自定义仅作用于日志页日志主体（含横向滚动条），不改全局滚动条样式；`::-webkit-scrollbar` 在 Wails WebView2 / Chromium 下有效。
- `SelectDirectory` 改签名后不保留旧调用形式（遵循项目禁止向后兼容的约定）。