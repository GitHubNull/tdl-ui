---
kind: frontend_style
name: 基于 PrimeVue Aura 的暗色主题前端样式系统
category: frontend_style
scope:
    - '**'
source_files:
    - src/frontend/src/main.ts
    - src/frontend/src/style.css
    - src/frontend/src/theme.ts
    - src/frontend/src/App.vue
    - src/frontend/vite.config.ts
    - src/frontend/package.json
---

## 1. 使用的系统与框架
- UI 组件库：PrimeVue 4（配合 @primeuix/themes 的 Aura 预设），图标使用 primeicons。
- 样式方案：以 CSS 变量 + 类名覆盖为主，结合 PrimeVue 的语义化主题变量（--p-*）实现亮/暗双主题。
- 构建与开发：Vite + Vue 3 SFC，TypeScript 编译；Wails 将前端资源嵌入 Go 后端。

## 2. 核心文件与包
- 入口与主题初始化：`src/frontend/src/main.ts`（注册 PrimeVue、定义 preset、启用 darkModeSelector）
- 全局样式与布局：`src/frontend/src/style.css`（CSS 变量、侧边栏、内容区、表格页、卡片、表单、空状态、任务卡片、代码编辑器等）
- 主题状态管理：`src/frontend/src/theme.ts`（响应式 themeMode/isDark、localStorage 持久化、system 跟随）
- 应用壳与导航：`src/frontend/src/App.vue`（侧边栏导航、主题切换 ToggleSwitch、Toast 容器）
- Vite 配置：`src/frontend/vite.config.ts`（Vue 插件、media 路由回退给 Wails）
- 依赖清单：`src/frontend/package.json`（Vue 3、Pinia、PrimeVue、@primeuix/themes/aura、vue-router）

## 3. 架构与约定
- 主题体系
  - 通过 `definePreset(Aura, { semantic: { primary: {...} } })` 将主色调统一为蓝色系，所有 PrimeVue 组件自动继承。
  - 使用 `darkModeSelector: '.app-dark'`，由 `theme.ts` 在 `<html>` 上切换 `app-dark` 类名，从而驱动亮/暗两套 CSS 变量与组件样式。
  - 主题模式支持 light/dark/system 三种，system 模式下监听 `prefers-color-scheme` 变化并同步。
- 布局结构
  - 全局采用 `.app-shell`（flex 横向）+ `.app-sidebar`（固定宽度 64px）+ `.app-content`（flex: 1）的经典三栏布局。
  - 页面级通用样式集中在 `style.css`：`.page`、`.page-table`、`.table-card`、`.table-toolbar`、`.table-status` 等，保证列表型页面一致体验。
- 组件样式策略
  - 业务组件（如 `DialogListPanel.vue`、`MediaPreview.vue` 等）主要复用 PrimeVue 组件并通过 CSS 变量（`--p-surface-*`、`--p-primary-*`、`--p-text-*`）进行明暗适配，避免硬编码颜色。
  - 自定义样式优先使用 BEM 风格类名（如 `.nav-item`、`.task-card`、`.panel-card`、`.form-field`），并在需要时通过 `.app-dark .xxx` 覆盖暗色变体。
- 图标与资源
  - 图标统一通过 `AppIcon.vue` 组件加载 `assets/icons/*.svg`，保持视觉一致性。
  - 插画与空状态使用 `assets/illustrations/*.svg`，配合 `.empty-state` 样式呈现一致的缺省体验。

## 4. 约定与约束
- 主题切换必须通过 `theme.ts` 暴露的 `setTheme()` / `initTheme()` 操作，确保 `app-dark` 类名与 localStorage 持久化保持一致。
- 所有颜色不得硬编码，应使用 `--p-*` 语义变量或 `color-mix()` 派生（如活跃态背景使用 `color-mix(in srgb, var(--p-primary-color) 20%, transparent)`）。
- 组件内样式遵循 scoped 原则，仅在必要时通过 `:deep(.p-*)` 穿透 PrimeVue 内部类进行微调（如 App.vue 中对 ToggleSwitch 的缩放）。
- 表格/列表页面统一使用 `.page-table` + `.table-card` + `.table-toolbar` + `.table-status` 结构，以保证滚动、边框、间距一致。
- 构建期通过 Vite 插件拦截 `/media/*` 返回 404，让 Wails 后端接管缩略图服务，前端不自行处理媒体路径。
- 设计令牌来源单一：`@primeuix/themes/aura` 提供基础 token，项目仅覆盖 `semantic.primary` 为蓝色系，其余沿用 Aura 默认值。