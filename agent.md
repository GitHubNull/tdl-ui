# agent.md — AI 编程代理指南

面向 AI 编程代理的项目速览与硬性约束。详细任务配方见 [doc/dev-ai/](doc/dev-ai/README.md)。

## 项目概览

tdl UI：基于 Wails v2 的 Telegram 媒体下载桌面客户端，复用 tdl（CLI）的下载引擎。
功能范围（当前版本）：**账号登录 + 对话浏览 + 媒体下载 + Yaegi 脚本**。上传 / 转发 / 导出暂不在范围内。

## 目录结构与硬性约束

```
├── ref/tdl        # ⛔ 上游子模块，只读！禁止修改任何文件
├── src/           # ✅ 全部源代码在此
│   ├── main.go            # 应用组装入口
│   ├── internal/          # Go 后端（详见下文）
│   ├── frontend/          # Vue 3 前端
│   └── wails.json
├── doc/           # 文档（tutorials / dev-human / dev-ai 三系列）
└── tmp/           # ✅ 所有临时文件、实验脚本放这里（不入库）
```

**必须遵守：**

1. `ref/tdl` 是 Git 子模块，**任何情况下不得修改其中文件**；仅允许 `git submodule update --remote` 同步上游。需要改动 tdl 行为时：在 `src/internal/` 复制改写（参考 `src/internal/engine/` 对 `ref/tdl/app/dl` 的改写）。干净 clone 后构建前必须先执行 `git submodule update --init`（ARC-05：`src/go.mod` 的 replace 指向此子模块，require 版本号仅作记录）；submodule commit 变更必须随本仓库同一提交固化。
2. 所有源代码放 `src/`，所有临时文件放 `tmp/`。
3. 提交代码使用 **smart-commit** 技能，禁止手动 `git commit`。

## 技术栈

| 层 | 技术 |
| --- | --- |
| 桌面壳 | Wails v2（Go ↔ WebView 绑定 + 事件） |
| 后端 | Go 1.25+；`go.mod replace github.com/iyear/tdl => ../ref/tdl` |
| 下载引擎 | tdl `core/downloader` + `core/dcpool` + `pkg/tclient`（bolt 会话存储 `pkg/kv`） |
| 脚本引擎 | Yaegi（Go 解释器），沙箱禁用 `os/exec` |
| 前端 | Vue 3 + Vite + PrimeVue 4（Aura 主题）+ Pinia + vue-router，pnpm 管理 |

## 后端模块（src/internal/）

- `config/` — 设置持久化（`%AppData%\tdl-ui\settings.json`）
- `events/` — Wails 事件契约：`login:update`、`task:update`、`task:file`、`script:log`
- `scriptapi/` — 脚本可见的 API 类型（FileInfo/TaskInfo/Log/Logf）
- `script/` — Yaegi 引擎封装（契约函数提取、panic 恢复、10 秒超时保护）+ 脚本文件 CRUD
- `engine/` — 下载任务管理器（状态机 queued/running/paused/done/failed/canceled、断点续传、选集下载）
- `services/` — Wails 绑定服务：AuthService / ChatService / DownloadService / ScriptService / SettingsService

关键设计：bolt kv 全局唯一实例（bbolt 文件锁）；暂停 = 取消 + 保留 resume key，恢复 = 重跑 + 断点续传；登录交互 = channel + 事件；ChatService 常驻连接 = 懒启动 + 断线重建 + jobs 通道串行化。

## 前端约定

- Vue 组件三段式：`<template>` → `<script setup lang="ts">` → `<style scoped>`
- 后端调用统一走 `src/frontend/src/api.ts`（`window.go.services.*` 封装）；类型契约在 `types.ts`，修改 Go 结构体 JSON tag 后必须同步
- 事件订阅在 Pinia store 的 `init()` 中完成（App.vue onMounted 统一调用）
- UI 规范：简约美观大方；仅使用确认存在的 PrimeIcons；暗黑模式 localStorage 持久化 + 跟随系统；主按钮实心主色
- 页面路由（hash 模式）：`/` → `/chats`（对话浏览）, `/login`（账号）, `/tasks`（下载任务）, `/scripts`（脚本）, `/settings`（设置）

## 常用命令

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

## 验证要求

- 后端改动：`go build ./...` + `go test ./...` 必须通过
- 前端改动：`pnpm build` 必须通过；UI 验证使用 chrome-devtools MCP
- 全量验证：`wails build` 产出单一二进制
