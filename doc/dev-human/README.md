# 开发者文档（人类维护者）

> [← 返回项目主页](../../README.md)

本系列面向维护 tdl UI 的人类开发者，按深度分为三级。AI 编程代理请优先阅读 [dev-ai 系列](../dev-ai/README.md)与根目录 [AGENTS.md](../../AGENTS.md)。

## 目录

### 🟢 基础（basic）

| 文档 | 内容 |
| --- | --- |
| [环境搭建](basic/01-environment.md) | Go / Node / pnpm / Wails CLI 安装与验证 |
| [项目结构与构建](basic/02-structure-build.md) | 目录职责、子模块机制、开发与生产构建 |

### 🟡 进阶（intermediate）

| 文档 | 内容 |
| --- | --- |
| [Wails 前后端通信机制](intermediate/01-wails-ipc.md) | 服务绑定、事件系统、类型契约同步 |
| [下载引擎装配](intermediate/02-download-engine.md) | 从消息链接到 downloader 的完整装配链路 |

### 🔴 高级（advanced）

| 文档 | 内容 |
| --- | --- |
| [Yaegi 脚本引擎扩展](advanced/01-yaegi-extend.md) | 契约提取原理、扩展脚本 API、沙箱策略 |
| [上游子模块同步与适配升级](advanced/02-upstream-sync.md) | 同步 ref/tdl、破坏性变更适配流程 |

## 快速命令参考

```bash
cd src
wails dev                                  # 开发模式（热重载）
wails build -ldflags "-s -w" -trimpath     # 生产构建
go test ./...                              # 后端测试
```
