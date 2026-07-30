# 主题切换系统

## 概述

主题切换系统为 tdl UI 提供明亮、暗黑、跟随系统三种视觉模式。所有 UI 组件基于 CSS 变量实现适配，禁止硬编码颜色值。主题状态持久化至 `localStorage`，刷新后保持用户选择。

核心文件：

| 文件 | 职责 |
|------|------|
| `src/frontend/src/theme.ts` | 主题状态管理（响应式 + localStorage） |
| `src/frontend/src/style.css` | CSS 变量适配规则 |
| `src/frontend/src/stores/settings.ts` | 设置页主题选项同步 |
| `src/frontend/src/App.vue` | 主题切换控件（ToggleSwitch） |

## 系统偏好检测

```typescript
const media = window.matchMedia('(prefers-color-scheme: dark)')
const systemDark = ref(media.matches)

media.addEventListener('change', (e) => {
  systemDark.value = e.matches
  apply()
})
```

- 应用启动时通过 `matchMedia` 检测操作系统当前主题偏好
- 监听系统主题变化事件，实时响应（无需刷新页面）
- 当 `themeMode === 'system'` 时，`isDark` 的值随系统偏好自动变化

## localStorage 持久化

```typescript
const STORAGE_KEY = 'tdlui-theme'

export function initTheme() {
  const saved = localStorage.getItem(STORAGE_KEY) as ThemeMode | null
  if (saved === 'light' || saved === 'dark' || saved === 'system') {
    themeMode.value = saved
  }
  // ...
}

export function setTheme(mode: ThemeMode) {
  themeMode.value = mode
  localStorage.setItem(STORAGE_KEY, mode)
  apply()
}
```

- 键名：`tdlui-theme`
- 有效值：`light` | `dark` | `system`
- 非法值（如旧版本残留）被忽略，回退默认 `system`

## CSS 变量定义

### PrimeVue Aura 主题变量

PrimeVue 4 的 Aura 主题通过 CSS 变量提供完整的颜色体系，主题切换只需切换变量值：

```css
/* 亮色（默认） */
--p-surface-0: #ffffff
--p-surface-50: #f8fafc
--p-surface-100: #f1f5f9
--p-surface-200: #e2e8f0
--p-surface-700: #334155
--p-surface-900: #0f172a
--p-surface-950: #020617
--p-primary-color: #3b82f6
--p-text-color: #1e293b
--p-text-muted-color: #64748b
```

暗色模式下，PrimeVue 自动切换为暗色变量值（通过 `.app-dark` 类前缀覆盖）。

### 自定义适配规则

`style.css` 中所有颜色引用均通过 CSS 变量：

```css
body {
  background: var(--p-surface-50);
  color: var(--p-text-color);
}

.app-dark body {
  background: var(--p-surface-950);
}

.panel-card {
  background: var(--p-surface-0);
  border: 1px solid var(--p-surface-200);
  border-radius: 12px;
}

.app-dark .panel-card {
  background: var(--p-surface-900);
  border-color: var(--p-surface-700);
}

.nav-item.active {
  background: var(--p-primary-50);
  color: var(--p-primary-color);
}

.app-dark .nav-item.active {
  background: color-mix(in srgb, var(--p-primary-color) 20%, transparent);
}
```

**关键规则**：
- 所有颜色值使用 `var(--p-*)` 引用
- 暗色覆盖统一使用 `.app-dark` 前缀选择器
- `color-mix()` 用于半透明混合色（如 active 状态背景）

### 应用/移除暗色类

```typescript
function apply() {
  document.documentElement.classList.toggle('app-dark', isDark.value)
}
```

- `app-dark` 类加在 `<html>` 元素上
- 通过 `classList.toggle` 原子切换，避免闪烁
- 所有后代元素通过 `.app-dark` 前缀匹配暗色规则

## PrimeVue Aura 主题适配

PrimeVue 4 的 Aura 主题本身支持明暗切换，但需要通过 PrimeVue 的 ThemeProvider 或 CSS 变量注入。tdl UI 采用**简化方案**：

1. 引入 PrimeVue 的 Aura 主题 CSS（含完整变量定义）
2. 通过 `.app-dark` 类覆盖需要自定义的变量和组件样式
3. 大部分 PrimeVue 组件（Button、Input、Table 等）自动响应 CSS 变量变化

无需动态加载/卸载主题文件，切换仅涉及一个 CSS 类的增删。

## ToggleSwitch 控件实现

App.vue 侧边栏底部提供快捷主题切换：

```vue
<div class="nav-item theme-switch" v-tooltip.right="themeTip" aria-label="切换主题">
  <ToggleSwitch :model-value="isDark" @update:model-value="toggleTheme">
    <template #handle>
      <AppIcon class="handle-icon" :name="isDark ? 'theme-dark' : 'theme-light'" />
    </template>
  </ToggleSwitch>
</div>
```

```typescript
function toggleTheme() {
  settings.applyTheme(isDark.value ? 'light' : 'dark')
}
```

- 使用 PrimeVue `ToggleSwitch` 组件
- handle 槽内显示太阳/月亮图标，提供视觉反馈
- 快捷开关只在 `light`/`dark` 间切换；设置页提供完整三选项

### 设置页主题选择

`SettingsPage.vue` 提供完整的主题模式选择（Dropdown 或 Radio）：

```typescript
// stores/settings.ts
applyTheme(mode: ThemeMode) {
  this.settings.theme = mode
  setTheme(mode)  // 更新 theme.ts 状态 + localStorage
}
```

设置保存时，`theme` 字段随其他设置一并写入后端 `settings.json`，但运行时以 `localStorage` 为准（启动即生效）。

## 各页面主题适配检查点

| 页面 | 暗色适配要点 |
|------|-------------|
| LoginPage | 登录表单卡片背景、输入框边框、hero 插图 |
| ChatsPage | 对话列表背景、媒体卡片边框、工具栏分隔线、空状态插图 |
| TasksPage | 任务卡片背景/边框、进度条颜色、文件行文本 |
| ScriptsPage | 代码编辑器背景（固定深色）、日志控制台（固定深色） |
| LogsPage | 日志表格行背景交替、级别标签颜色 |
| SettingsPage | 设置卡片背景、表单字段标签、开关组件 |

**特殊处理**：
- 代码编辑器（`code-editor`）和日志控制台（`log-console`）固定深色主题，不受全局切换影响
- 空状态插图（`empty-chats.svg`、`empty-tasks.svg`）使用 SVG 的 `currentColor`，自动跟随文本颜色
