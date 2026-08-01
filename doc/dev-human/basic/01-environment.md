# 环境搭建

> [← 开发者文档目录](../README.md) | [项目主页](../../../README.md)

## 依赖清单

| 工具 | 最低版本 | 说明 |
| --- | --- | --- |
| Go | 1.25+ | tdl 子模块声明 go 1.25.x，Go 工具链会按 `GOTOOLCHAIN=auto` 自动下载匹配版本 |
| Node.js | 20+ | 前端构建 |
| pnpm | 9+ | 前端包管理（项目约定，勿用 npm/yarn） |
| Wails CLI | v2.11+ | `go install github.com/wailsapp/wails/v2/cmd/wails@latest` |
| WebView2 | 任意 | Windows 10/11 一般已内置 |

## 克隆与初始化

```bash
# 必须带子模块
git clone --recurse-submodules <仓库地址>
cd tdl_UI

# 若已克隆但缺少子模块
git submodule update --init
```

## 验证环境

```bash
wails doctor        # 检查 Wails 依赖是否齐全
cd src
go build ./...      # 后端编译（首次会下载依赖与工具链，耗时较长）
cd frontend
pnpm install && pnpm build
```

三步均无报错即环境就绪。

## 常见问题

- **go build 报 module 找不到**：确认 `ref/tdl` 子模块已拉取（`git submodule update --init`），`src/go.mod` 的 replace 指向 `../ref/tdl`
- **pnpm install 提示 Ignored build scripts**：esbuild 等依赖的构建脚本被 pnpm 默认拦截，不影响构建；如需放行执行 `pnpm approve-builds`
- **wails dev 白屏**：先手动执行一次 `pnpm install`，或检查 34115 端口占用

## 下一步

→ [项目结构与构建](02-structure-build.md)
