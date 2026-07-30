# 前端页面架构

## 概述

前端位于 `src/frontend/src/`，基于 Vue 3 + Vite + PrimeVue 4（Aura 主题）+ Pinia + vue-router 构建。采用**三段式组件结构**：`<template>` → `<script setup lang="ts">` → `<style scoped>`。

整体布局为**侧边栏导航 + 内容区**的左右结构，所有页面共享 `App.vue` 提供的 shell 框架。

## 路由配置

`src/frontend/src/router.ts` 使用 hash 路由（`createWebHashHistory`），适配 Wails 嵌入环境避免刷新丢路径：

| 路径 | 组件 | 导航显示 | 说明 |
|------|------|----------|------|
| `/` | redirect `/chats` | - | 默认重定向到对话页 |
| `/chats` | ChatsPage | 对话 | 双面板对话浏览 |
| `/chats/:id` | ChatsPage | - | 带对话 ID 的深层链接 |
| `/login` | LoginPage | - | 账号登录（隐藏导航） |
| `/tasks` | TasksPage | 下载 | 任务列表与管理 |
| `/scripts` | ScriptsPage | 脚本 | 脚本编辑器 |
| `/logs` | LogsPage | 日志 | 日志查看 |
| `/settings` | SettingsPage | 设置 | 应用设置 |

## 页面组件层级

```
App.vue (shell)
├── aside (侧边栏导航)
│   ├── brand logo
│   ├── nav-items (路由驱动)
│   ├── nav-spacer
│   ├── account entry (登录入口)
│   └── theme switch (ToggleSwitch)
└── main (app-content)
    └── router-view
        ├── LoginPage.vue
        ├── ChatsPage.vue
        │   ├── DialogListPanel.vue
        │   ├── MediaToolbar.vue
        │   ├── SearchBox.vue
        │   └── MediaPreview.vue
        ├── TasksPage.vue
        │   └── NewTaskDialog.vue
        ├── ScriptsPage.vue
        ├── LogsPage.vue
        └── SettingsPage.vue
```

### ChatsPage 双面板布局

ChatsPage 是最复杂的页面，采用**左右双面板**结构：

- **左侧面板**：对话列表（`DialogListPanel.vue`），显示全部对话，支持搜索过滤
- **右侧面板**：媒体浏览区，顶部工具栏（`MediaToolbar.vue`）+ 瀑布流/表格视图 + 分页
- 选中对话后右侧加载该对话的媒体文件，支持按类型/关键词/大小过滤

### 公共组件

| 组件 | 文件 | 用途 |
|------|------|------|
| AppIcon | `components/AppIcon.vue` | 统一图标组件，支持自定义 SVG 图标 |
| DialogListPanel | `components/DialogListPanel.vue` | 对话列表面板 |
| MediaPreview | `components/MediaPreview.vue` | 媒体预览弹窗 |
| MediaToolbar | `components/MediaToolbar.vue` | 媒体过滤工具栏 |
| NewTaskDialog | `components/NewTaskDialog.vue` | 新建下载任务弹窗 |
| SearchBox | `components/SearchBox.vue` | 搜索框（带清除按钮） |

## Pinia Store 依赖关系

```
App.vue onMounted
  └── Promise.all([init all stores])
      ├── authStore (stores/auth.ts)
      ├── tasksStore (stores/tasks.ts)
      ├── scriptsStore (stores/scripts.ts)
      ├── settingsStore (stores/settings.ts)
      └── logsStore (stores/logs.ts)
```

### Store 职责

| Store | 状态 | 事件订阅 | 说明 |
|-------|------|----------|------|
| auth | loggedIn, userId, username, stage, qrUrl, error | `login:update` | 登录状态与流程管理 |
| tasks | tasks[], files{} | `task:update`, `task:file` | 任务列表与实时进度 |
| scripts | - | - | 脚本列表（简单委托 API） |
| settings | settings, dataDir | - | 应用设置，主题切换入口 |
| logs | entries[], files[] | `log:batch` | 日志条目与文件列表 |
| chats | - | - | 对话数据（页面级状态，非全局 store） |

### 事件订阅模式

每个需要事件订阅的 store 在 `init()` 中完成订阅：

```typescript
// stores/tasks.ts
async init() {
  unsubs.push(on<TaskView>(EVENT_TASK, (t) => this.upsert(t)))
  unsubs.push(on<FileEvent>(EVENT_TASK_FILE, (f) => this.onFile(f)))
  await this.refresh()
}
```

**HMR 安全**：使用 `import.meta.hot?.dispose()` 在模块热替换时注销旧回调，避免事件双触发（FE-17）。

### AuthStore 登录流程状态

```
idle → pending ──30s 超时──> error
  ↑      │
  │      ├── StartCodeLogin / StartQRLogin 成功 ──> need_code / qr
  │      │
  │      ├── SubmitCode ──> need_password / success
  │      │
  │      └── SubmitPassword ──> success
  │
  └── CancelLogin / Logout ──> idle
```

`pending` 态有 30 秒超时兜底（FE-19）：后端既不 reject 也不发事件时，自动转为 error。

## 响应式布局策略

全局采用 **8px 间距栅格** 与 **CSS 变量** 实现响应式：

- 侧边栏固定宽度 `64px`，内容区 `flex: 1` 自适应
- 表格页（对话/媒体列表）使用 `flex: 1` + `min-height: 0` 实现全高填充
- 卡片式布局：`panel-card` 类统一圆角（12px）、边框、阴影
- 空状态：`empty-state` 类统一居中占位图 + 提示文本

### 关键 CSS 变量（PrimeVue Aura 主题）

```css
/* 亮色 */
--p-surface-0: #ffffff      /* 卡片背景 */
--p-surface-50: #f8fafc     /* 页面背景 */
--p-surface-200: #e2e8f0    /* 边框 */
--p-primary-color: #3b82f6  /* 主色 */
--p-text-color: #1e293b     /* 主文本 */
--p-text-muted-color: #64748b /* 次要文本 */

/* 暗色（.app-dark 前缀） */
--p-surface-900: #0f172a
--p-surface-950: #020617
--p-surface-700: #334155
```

## 主题适配实现

主题系统由 `theme.ts` + `style.css` + `stores/settings.ts` 协同实现：

### theme.ts —— 核心状态

```typescript
const STORAGE_KEY = 'tdlui-theme'
export const themeMode = ref<ThemeMode>('system')  // light | dark | system
export const isDark = computed(() => 
  themeMode.value === 'dark' || (themeMode.value === 'system' && systemDark.value)
)
```

- `initTheme()`：从 `localStorage` 恢复偏好，监听 `matchMedia` 系统变化
- `setTheme(mode)`：切换模式并持久化
- `apply()`：切换 `document.documentElement.classList` 上的 `app-dark` 类

### style.css —— 适配规则

所有颜色值通过 CSS 变量引用，禁止硬编码：

```css
body { background: var(--p-surface-50); color: var(--p-text-color); }
.app-dark body { background: var(--p-surface-950); }

.panel-card { background: var(--p-surface-0); border: 1px solid var(--p-surface-200); }
.app-dark .panel-card { background: var(--p-surface-900); border-color: var(--p-surface-700); }
```

### App.vue 主题切换控件

```vue
<ToggleSwitch :model-value="isDark" @update:model-value="toggleTheme">
  <template #handle>
    <AppIcon :name="isDark ? 'theme-dark' : 'theme-light'" />
  </template>
</ToggleSwitch>
```

快捷开关在亮/暗间切换；设置页提供完整的三选项（light/dark/system）。

## 公共组件复用

### api.ts —— 后端调用统一封装

所有后端调用集中走 `api.ts`，避免页面组件直接访问 `window.go`：

```typescript
// 服务分组 + 事件常量 + 媒体图 URL 生成
export const Auth = { ... }
export const Download = { ... }
export const EVENT_TASK = 'task:update'
export function thumbURL(...): string
export function on<T>(event, cb): () => void
```

### types.ts —— 类型单一事实来源

```typescript
// wailsjs/go/models.ts 的命名别名 + 事件负载补充
export type Settings = config.Settings
export type TaskView = engine.TaskView
export interface FileEvent { ... }
export interface LoginUpdate { ... }
```

**约束**：修改 Go 结构体 JSON tag 后必须同步更新前端类型。
