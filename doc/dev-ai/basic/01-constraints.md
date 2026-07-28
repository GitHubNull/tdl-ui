# 项目约束与目录规范

> [← AI 代理文档目录](../README.md) | [项目主页](../../../README.md)

## 硬性约束清单（违反任一条即任务失败）

- [ ] **禁止修改 `ref/tdl/` 内任何文件** —— 它是 Git 子模块（上游只读镜像）。需要不同行为时，将相关代码复制到 `src/internal/` 后改写
- [ ] **禁止编辑自动生成产物**：`src/frontend/dist/`、`src/build/bin/`、`node_modules/`
- [ ] **所有源码只能放在 `src/` 下**；临时文件/实验脚本一律放 `tmp/`（已被 .gitignore 排除）
- [ ] **提交必须使用 smart-commit 技能**，禁止手动 `git commit`
- [ ] **协议为 AGPL-3.0**（继承自 tdl），新文件不得引入不兼容协议的代码
- [ ] Go 结构体 JSON tag 变更后，**必须同步** `src/frontend/src/types.ts`
- [ ] 事件名/负载变更后，**必须同步** `src/internal/events/events.go` 与 `src/frontend/src/api.ts` 的事件常量
- [ ] 前端组件遵循三段式：`<template>` → `<script setup lang="ts">` → `<style scoped>`
- [ ] 只使用经验证存在的 PrimeIcons 图标类名（`pi pi-*`）

## 目录职责边界

| 路径 | 职责 | 修改策略 |
| --- | --- | --- |
| `ref/tdl/` | 上游 tdl 子模块 | ⛔ 只读，仅 `git submodule update --remote` 同步 |
| `src/main.go` | Wails 入口、服务绑定注册 | 新增服务时追加 Bind |
| `src/internal/config/` | 设置持久化（JSON 文件） | 加设置项在此扩展 Settings 结构 |
| `src/internal/events/` | 事件名常量 + Emitter | 新事件在此定义常量 |
| `src/internal/services/` | Wails 绑定服务（Auth/Download/Script/Settings） | 前端可调用的方法都在这里 |
| `src/internal/engine/` | 下载任务引擎（改写自 `ref/tdl/app/dl`） | 对照上游升级，见升级剧本 |
| `src/internal/script/` | Yaegi 引擎 + 脚本文件存储 | 契约函数/沙箱策略在此 |
| `src/internal/scriptapi/` | 暴露给用户脚本的 `tdlui/api` 包 | 加脚本 API 在此 |
| `src/frontend/src/api.ts` | 后端调用唯一入口 | 页面禁止直接用 `window.go` |
| `src/frontend/src/types.ts` | 与 Go JSON 契约对应的 TS 类型 | 与后端结构体同步维护 |
| `src/frontend/src/stores/` | Pinia stores（auth/tasks/scripts/settings） | 事件订阅统一放 store 的 `init()` |
| `src/frontend/src/pages/` | 五个页面组件 | UI 改动主要发生地 |
| `doc/` | 三大文档系列 | 行为变更后同步相关篇目 |

## 环境与命令速查

| 目的 | 命令（工作目录） |
| --- | --- |
| 后端编译 | `go build ./...`（`src/`） |
| 后端测试 | `go test ./...`（`src/`） |
| 前端依赖 | `pnpm install`（`src/frontend/`） |
| 前端构建 | `pnpm build`（`src/frontend/`） |
| 开发联调 | `wails dev`（`src/`） |
| 生产构建 | `wails build -ldflags "-s -w" -trimpath`（`src/`） |

## go.mod 关键事实

- 模块名 `tdl-ui`；tdl 为三模块仓库，存在 **三条 replace**（`github.com/iyear/tdl`、`…/core`、`…/extension` → `../ref/tdl` 相对路径），删除任何一条都会导致编译失败
- 修改依赖后运行 `go mod tidy` 并重新 `go build ./...` 验证

## 下一步

→ [服务绑定与事件契约](../intermediate/01-binding-events.md)
