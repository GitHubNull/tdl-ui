# tdl UI 项目知识卡

## 1. 项目定位

**tdl UI** 是基于 [Wails v2](https://wails.io) 的 Telegram 媒体下载桌面客户端，复用 [tdl](https://github.com/GitHubNull/tdl) CLI 的下载引擎。技术栈：Go 1.25.8 后端 + Vue 3.5.13 / PrimeVue 4.3.3 / Pinia / vue-router 前端，pnpm 管理依赖。

## 2. 目录结构约束

```
src/                    # 全部源代码
├── main.go             # Wails 应用组装入口
├── internal/           # Go 后端
│   ├── config/         # 设置持久化（settings.json）
│   ├── engine/         # 下载任务管理器（状态机、断点续传）
│   ├── events/         # Wails 事件契约与发射器
│   ├── logging/        # zap + lumberjack 结构化日志
│   ├── script/         # Yaegi 脚本引擎封装
│   ├── scriptapi/      # 脚本可见的 API 类型
│   ├── services/       # Wails 绑定服务（Auth/Chat/Download/Script/Settings）
│   └── store/          # SQLite 持久化（任务/消息项/文件/断点）
├── frontend/           # Vue 3 前端
│   ├── src/
│   │   ├── api.ts      # 后端调用统一封装
│   │   ├── theme.ts    # 主题状态管理
│   │   ├── types.ts    # 类型别名与事件负载
│   │   ├── router.ts   # hash 路由配置
│   │   ├── stores/     # Pinia stores
│   │   ├── pages/      # 6 大页面组件
│   │   └── components/ # 公共组件
│   └── wailsjs/        # Wails 生成绑定
└── wails.json          # Wails 构建配置

ref/tdl/                # Git 子模块（上游 tdl，只读！）
doc/                    # 文档体系（tutorials / dev-human / dev-ai）
tmp/                    # 临时文件（不入库）
img/                    # 视觉素材
```

## 3. ARC-05：子模块只读约束

`ref/tdl` 是 Git 子模块，通过 `go.mod replace` 引入：

```go
replace (
    github.com/iyear/tdl => ../ref/tdl
    github.com/iyear/tdl/core => ../ref/tdl/core
)
```

- **任何情况下禁止修改 `ref/tdl` 内文件**
- 仅允许 `git submodule update --remote` 同步上游
- 需要改动 tdl 行为时，在 `src/internal/` 复制改写（参考 `engine/` 对 `ref/tdl/app/dl` 的改写）
- 构建前须执行 `git submodule update --init`
- submodule commit 变更需随本仓库一起提交

## 4. 版本锁定

| 组件 | 版本 | 说明 |
|------|------|------|
| Go | 1.25.8 | 由 `ref/tdl/go.mod` 钉住，`go mod tidy` 自动回写 |
| Wails | v2.11.0 | 桌面壳框架 |
| Vue | 3.5.13 | 前端框架 |
| PrimeVue | 4.3.3 | UI 组件库（Aura 主题） |
| Pinia | 2.2.6 | 状态管理 |
| Yaegi | v0.16.1 | Go 解释器（脚本引擎） |
| gotd/td | v0.140.0 | Telegram MTProto 客户端 |
| modernc.org/sqlite | v1.54.0 | SQLite 驱动 |

## 5. 下载引擎核心知识

### 状态机六态

`queued → running ↔ paused → done / failed / canceled`

- **暂停 = 取消 + 保留 resume key**，恢复 = 重跑 + 断点续传
- `removed` 标志阻断并发 `Resume` 复活僵尸下载（ENG-03）
- 状态写回使用单条 UPDATE（ENG-10），避免状态与计数撕裂

### 断点续传

- 断点 key 为内容坐标：`dialogID:messageID`
- 存储于 SQLite `resume_keys` 表
- 行级追加写入（ENG-12：O(1)），替代旧版重写整个集合
- 断点写入使用 `context.Background()`（STO-03），避免 ctx 取消导致丢失

### 选集下载

- `Selection` = `DialogID` + `DialogType` + `MessageIDs`
- 与 URLs 可混用，持久化为 `task_items` 表行
- 恢复时从 `task_items` 重组，保证与原始输入一致

## 6. 脚本引擎安全

### Yaegi 沙箱

- 白名单标准库：bytes, errors, fmt, math, path, regexp, sort, strconv, strings, time, unicode 等
- **阻断**：os, os/exec, net, net/http, syscall, unsafe, reflect
- 脚本名正则：`^[\w\p{Han}-]+$`，拒绝 Windows 保留设备名

### 超时与熔断

- 单次脚本函数调用超时：**10 秒**
- `callWithGuard()`：独立 goroutine + panic recover + 超时 select
- 超时时熔断对应契约函数（置 nil），避免死循环脚本泄漏 goroutine

### 契约函数

```go
func Filter(f api.FileInfo) bool        // 过滤
func Rename(f api.FileInfo) string      // 命名
func OnTaskStart(t api.TaskInfo)        // 任务开始钩子
func OnFileDone(f api.FileInfo)         // 文件完成钩子
func OnTaskDone(t api.TaskInfo)         // 任务结束钩子
```

所有契约函数通过 `Safe*` 方法调用，内部 `sync.Mutex` 串行化（Yaegi 不保证并发安全）。

## 7. 事件系统

### 事件契约

| 事件 | 名称 | 负载类型 |
|------|------|----------|
| login:update | Login | LoginUpdate |
| task:update | Task | TaskView |
| task:file | TaskFile | FileEvent |
| script:log | ScriptLog | string |
| log:batch | Log | []LogEntry |

### Emitter 安全设计

- `OnStartup` 前 `Emit` 静默丢弃（安全）
- `OnShutdown` 后 `Emit` 静默丢弃（SVC-19）
- `RWMutex` 保护 ctx 读写

### 前端订阅模式

```typescript
// Store init() 中订阅
unsubs.push(on<TaskView>(EVENT_TASK, (t) => this.upsert(t)))

// HMR 安全注销
import.meta.hot?.dispose(() => {
  unsubs.splice(0).forEach((off) => off())
})
```

## 8. 主题切换

- 三模式：`light` | `dark` | `system`
- 默认检测 `matchMedia('(prefers-color-scheme: dark)')`
- 持久化键：`tdlui-theme`（localStorage）
- 切换方式：`document.documentElement.classList.toggle('app-dark', isDark)`
- 所有颜色通过 CSS 变量 `var(--p-*)` 引用，禁止硬编码

## 9. 构建命令

```bash
# 开发（src/ 目录下）
wails dev

# 生产构建
wails build -ldflags "-s -w" -trimpath

# 后端测试
go test ./...

# 前端单独构建（src/frontend/）
pnpm install && pnpm build
```

## 10. 验证要求

- 后端改动：`go build ./...` + `go test ./...` 必须通过
- 前端改动：`pnpm build` 必须通过；UI 验证使用 chrome-devtools MCP
- 全量验证：`wails build` 产出单一二进制
