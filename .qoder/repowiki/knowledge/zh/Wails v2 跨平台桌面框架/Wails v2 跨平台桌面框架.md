---
kind: external_dependency
name: Wails v2 跨平台桌面框架
slug: wails-v2
category: external_dependency
category_hints:
    - vendor_identity
scope:
    - '**'
---

### Wails v2 集成
- **角色**：Go 后端与 Vue 前端之间的桥接框架，提供原生应用打包能力
- **集成点**：`src/wails.json` 配置构建脚本（`pnpm install/build/dev`）、`main.go` 初始化、`go.mod` 依赖声明
- **版本锁定**：v2.11.0，由 `go.mod` 固定；产品版本号在 `wails.json` 的 `productVersion` 字段同步管理
- **构建流程**：`wails build -ldflags "-s -w" -trimpath` 产出单一二进制文件，前端资源通过 Vite 构建后嵌入
- **约束**：前端开发服务器端口自动分配（`frontend:dev:serverUrl: auto`），生产环境需重新构建前端资源
- **验证**：构建前必须执行 `pnpm install` 安装前端依赖，确保 `node_modules` 完整