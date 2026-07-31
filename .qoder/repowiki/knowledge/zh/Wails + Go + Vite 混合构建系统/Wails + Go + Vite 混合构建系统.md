---
kind: build_system
name: Wails + Go + Vite 混合构建系统
category: build_system
scope:
    - '**'
source_files:
    - src/go.mod
    - src/wails.json
    - src/frontend/package.json
    - src/frontend/vite.config.ts
    - src/main.go
---

本项目采用 **Wails v2** 作为桌面应用壳，将 Go 后端与 Vue 3 前端打包为单一跨平台二进制。构建体系由三个层次组成：Go 模块依赖管理、前端资源构建、以及 Wails 最终产物组装。

### 1. 构建系统与工具链
- **Go 后端**：`src/go.mod` 声明 `go 1.25.8`（patch 版本钉住），通过 `replace` 指令将 `github.com/iyear/tdl` 及其 `core` 子模块替换为本地 `../ref/tdl` Git submodule，实际编译代码以 submodule checkout 为准，require 中的版本号仅做记录。
- **前端构建**：`src/frontend/package.json` 使用 pnpm 管理依赖，Vite 6 + Vue 3 + TypeScript，构建脚本为 `pnpm build`（先执行 `vue-tsc --noEmit` 类型检查再打包）。
- **Wails 集成**：`src/wails.json` 定义 `frontend:install`、`frontend:build`、`frontend:dev:watcher` 等钩子，Wails 在构建时自动调用这些命令完成前端资源编译并嵌入 Go 二进制。
- **生产构建命令**：`wails build -ldflags "-s -w" -trimpath`（见 README.md / agent.md / doc 文档），输出名为 `tdl-ui` 的单一可执行文件。

### 2. 关键文件与职责
- `src/go.mod`：Go 依赖声明与 tdl 子模块 replace 规则，注释明确说明 submodule 必须提前初始化。
- `src/wails.json`：Wails 应用元数据（名称、版本 0.11.0）、前后端构建脚本绑定、产品信息。
- `src/frontend/package.json`：前端依赖与构建脚本（dev/build/preview）。
- `src/frontend/vite.config.ts`：Vite 配置，包含自定义插件 `media404()` 用于开发模式下让 `/media/*` 回落到 Wails 后端处理器。
- `src/main.go`：Go 入口，通过 `//go:embed all:frontend/dist` 嵌入前端静态资源，并启动 Wails 应用。

### 3. 架构与约定
- **单二进制分发**：Wails 将前端 dist 目录 embed 进 Go 二进制，运行时通过 AssetServer 提供静态资源与 `/media/thumb`、`/media/preview` 等动态接口。
- **子模块依赖隔离**：tdl 下载引擎作为只读 submodule 引入，构建前必须执行 `git submodule update --init`，否则 `go build` 失败。
- **版本同步**：Go 模块版本（`go.mod` 中 go 指令）与前端 package.json version（0.11.0）及 wails.json info.productVersion 需保持一致，均由上游 tdl 的 go.mod patch 版本驱动。
- **开发工作流**：`wails dev` 自动启动 Vite 开发服务器并监听前端变更，Go 侧通过 `frontend:dev:serverUrl: auto` 连接。

### 4. 约束与规范
- **必须预先初始化子模块**：`src/go.mod` 注释明确要求 `git submodule update --init`，否则构建失败。
- **Go 工具链锁定**：`go 1.25.8` 精确到 patch 版本，Go toolchain 会自动切换至该版本，所有构建环境必须 ≥ 1.25.8。
- **生产构建参数固定**：统一使用 `-ldflags "-s -w" -trimpath` 去除调试符号与路径信息。
- **前端类型检查前置**：`pnpm build` 先执行 `vue-tsc --noEmit` 再进行 Vite 打包，类型错误会阻断构建。
- **无独立 Makefile/Dockerfile/CI**：项目未提供顶层 Makefile、Dockerfile 或 GitHub Actions 工作流；构建完全依赖 Wails CLI 与 pnpm 脚本，发布流程由 ref/tdl 子模块的 `.github/workflows/release.yml` 通过 Goreleaser 管理（但这是 tdl 引擎的发布，非 tdl-ui 本身）。
- **审计指出风险**：doc/audit/2026-07-29-full-audit/01-architecture.md 指出 replace 机制缺少 CI 校验，submodule commit 未固化，存在构建不可重现风险。