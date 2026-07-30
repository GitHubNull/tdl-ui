---
kind: dependency_management
name: Go + Node.js 双栈依赖管理与子模块替换策略
category: dependency_management
scope:
    - '**'
source_files:
    - src/go.mod
    - src/frontend/package.json
    - src/frontend/pnpm-lock.yaml
    - src/wails.json
    - .gitmodules
---

本项目采用 Go（后端）与 Node.js（前端）双栈管理依赖，核心策略如下：

**Go 依赖管理**
- 使用 `go.mod` + `go.sum` 声明依赖，Go 版本锁定为 1.25.8。
- 通过 `replace` 指令将 `github.com/iyear/tdl` 及其 `core` 子模块重定向到本地 `ref/tdl` 子模块，使 tdl 引擎以只读 fork 形式内嵌。
- `.gitmodules` 定义 `ref/tdl` 子模块指向 `https://github.com/GitHubNull/tdl`，构建前需执行 `git submodule update --init`。
- `go.mod` 中注释明确约定：require 中的版本号仅作记录，实际版本由子模块当前 checkout 决定；go 指令的 patch 版本由 ref/tdl 的 go.mod 钉住，`go mod tidy` 会自动回写。
- 未配置 GOPRIVATE、GONOSUMCHECK 或私有代理，依赖均从公共仓库获取。

**Node.js 前端依赖管理**
- 使用 pnpm 作为包管理器，`src/frontend/package.json` 声明运行时依赖（vue、pinia、primevue 等）与开发依赖（vite、typescript、vue-tsc 等）。
- `pnpm-lock.yaml` 锁定所有依赖的确切版本与 integrity hash，确保可重复构建。
- Wails 配置（`src/wails.json`）通过 `frontend:install` 和 `frontend:build` 脚本集成 pnpm 流程。

**构建集成**
- Wails 在 `wails.json` 中指定 `frontend:install: "pnpm install"`、`frontend:build: "pnpm build"`，Go 构建时自动触发前端资源打包。
- 前端产物被嵌入 Go 二进制，形成单一可分发文件。

**约束与约定**
- tdl 引擎必须通过子模块引入，禁止直接 require 远程版本（见 go.mod 注释 ARC-05）。
- 子模块提交变更需随主仓库一起提交，保证依赖快照一致性。
- 前端依赖更新需同步提交 `pnpm-lock.yaml`，避免构建不一致。