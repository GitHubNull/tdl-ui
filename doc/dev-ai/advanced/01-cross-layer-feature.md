# 跨层功能新增流程

> [← AI 代理文档目录](../README.md) | [项目主页](../../../README.md)

新增一个"从后端能力到前端页面"的完整功能时，按本剧本自上而下执行。以假想需求"**转发消息到收藏夹**"为例演示各步骤的落点。

## 第 0 步：范围确认

- 确认能力是否可由 `ref/tdl` 现有公开包提供（Grep `ref/tdl/core/` 与 `ref/tdl/pkg/`）
- 若只存在于上游 CLI 层（`ref/tdl/app/`），则**复制到 `src/internal/` 改写**，绝不 import CLI 包、绝不改子模块
- 评估是否需要新服务，还是挂在现有五服务之一（原则：登录相关 → Auth，对话相关 → Chat，任务型长操作 → Download，纯配置 → Settings）

## 第 1 步：后端引擎层（如有长任务逻辑）

```
src/internal/engine/（或新建 src/internal/<域>/）
```

- 长操作实现为可取消任务：接收 `context.Context`，状态机对齐 `queued/running/.../canceled`
- 进度通过 `events.Emitter` 推送，高频事件节流 200ms
- 与 kv/session 交互复用 `pkg/kv`、`pkg/key`（key 格式与 tdl CLI 兼容）

## 第 2 步：服务绑定层

```
src/internal/services/forward.go（新服务）或现有服务加方法
```

- 导出方法 + JSON 可序列化参数/返回值
- 新服务需在 `src/main.go` 的 `Bind` 数组注册，并在 `OnStartup` 链路中完成依赖注入（参考现有五服务的构造顺序：config → emitter → kv → scripts → taskManager → auth → download → script → settings → chat）

## 第 3 步：事件契约

```
src/internal/events/events.go
```

- 新增事件名常量（命名风格 `域:动作`，如 `forward:update`）与负载结构

## 第 4 步：前端契约层

```
src/frontend/src/types.ts + api.ts
```

- types.ts：负载/参数接口（字段名 = JSON tag）
- api.ts：服务封装对象 + 事件常量

## 第 5 步：状态层

```
src/frontend/src/stores/forward.ts（或扩展现有 store）
```

- 事件订阅写在 `init()`；若新建 store，需在 `App.vue` 的 `onMounted` 中追加 `init()` 调用

## 第 6 步：页面层

```
src/frontend/src/pages/ForwardPage.vue + router.ts
```

- `router.ts` 加路由（hash 模式，`meta` 提供导航 label 与 PrimeIcons 图标名）
- 侧边栏导航由 `App.vue` 从路由 meta 自动生成，无需改导航组件
- 页面遵循三段式 + 既有设计规范（页头标题 + 说明、卡片布局、语义色 token）
- `ChatsPage.vue` 类型的双面板布局需额外注意左右面板比例与滚动容器

## 第 7 步：测试与文档

- 纯逻辑（解析、脚本、状态机）补 Go 单元测试；`go test ./...` 全过
- `wails dev` + 浏览器工具走通完整用户路径（含亮/暗主题）
- 文档同步：`doc/tutorials/` 加用法教程、`doc/dev-human/` 补机制说明、本系列补契约表、README 功能列表更新

## 完成检查清单

- [ ] `go build ./...` / `go test ./...` / `pnpm build` 全通过
- [ ] 未修改 `ref/tdl/` 任何文件
- [ ] 事件名/类型三处一致（events.go / api.ts / types.ts）
- [ ] 新 store 已在 App.vue 初始化
- [ ] 新服务已在 main.go Bind 注册
- [ ] 文档四处同步（tutorials / dev-human / dev-ai / README）
- [ ] smart-commit 提交

## 下一步

→ [升级迁移剧本](02-upgrade-playbook.md)
