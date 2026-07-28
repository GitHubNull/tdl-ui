# 上游子模块同步与适配升级

> [← 开发者文档目录](../README.md) | [项目主页](../../../README.md)

`ref/tdl` 是指向上游 tdl 仓库的 Git 子模块。本文描述如何安全地同步上游更新并适配破坏性变更。

## 同步流程

```bash
# 1. 查看当前锁定的子模块版本
git submodule status

# 2. 拉取上游最新（或指定 tag）
git submodule update --remote ref/tdl
# 或手动检出指定版本：
#   cd ref/tdl && git fetch && git checkout v0.21.0 && cd ../..

# 3. 后端重新解析依赖并全量验证
cd src
go mod tidy
go build ./...
go test ./...

# 4. 全量构建验证
wails build -ldflags "-s -w" -trimpath

# 5. 一切通过后，提交子模块指针变更（使用 smart-commit）
```

> ⛔ 永远不要直接修改 `ref/tdl` 内的文件。上游行为不满足需求时，将相关代码复制到 `src/internal/` 改写。

## 我们依赖的上游 API 面

升级后重点检查以下引用点（编译错误会直接暴露）：

| 上游包 | 我们的使用处 | 用途 |
| --- | --- | --- |
| `pkg/tclient` | services/auth.go、engine/task.go | 客户端构建、App 凭据 |
| `pkg/kv` | main.go | bolt 存储 |
| `pkg/key` | services/auth.go、engine/task.go | kv key 生成（app/resume） |
| `pkg/tmessage` | engine/task.go | 消息链接解析 |
| `pkg/tplfunc` | engine/iter.go | 命名模板函数 |
| `pkg/tpath` | services/auth.go | Desktop 路径探测 |
| `pkg/utils` | engine/iter.go | 字节格式化 |
| `core/downloader` | engine/ | 下载器本体 |
| `core/dcpool` / `core/tclient` | engine/task.go | 连接池与中间件 |
| `core/storage` | services/auth.go、engine/、services/chat.go | 会话与 peer 存储 |
| `core/tmedia` | engine/iter.go、engine/elem.go、services/chat.go | 媒体信息提取 |
| `core/util/fsutil` | engine/progress.go | 文件名处理 |
| `core/util/tutil` | engine/iter.go、services/chat.go、services/thumb.go | 消息/缩略图工具 |

## 改写代码的对照升级

`src/internal/engine/` 改写自 `ref/tdl/app/dl`（iter/progress/elem/dl 四个文件）。上游该目录变更时：

```bash
# 查看上游两个版本间 app/dl 的差异
cd ref/tdl
git diff <旧版本>..<新版本> -- app/dl/
```

将有意义的差异（bug 修复、新特性、fingerprint 算法变化等）手工移植到 `src/internal/engine/` 对应文件。**特别注意**：

- `fingerprint` 算法若变化，用户的旧断点将失效（可接受，但需在发布说明中注明）
- `key.Resume` / `keygen.New("session")` 的 key 格式若变化，会影响会话与断点兼容性

## go.mod 注意事项

- tdl 是三模块仓库，`replace` 必须三条同时存在（主模块 / core / extension）
- `require` 的版本号（如 v0.20.3）在 replace 生效时仅作占位，但建议与子模块实际 tag 保持一致以免误导
- 上游 `go` 指令升级（如 1.25 → 1.26）时，本地 Go 工具链需满足或依赖 `GOTOOLCHAIN=auto` 自动下载

## 升级检查清单

- [ ] `go build ./...` 与 `go test ./...` 通过
- [ ] `wails build` 产出可运行二进制
- [ ] 登录（三种方式任一）冒烟测试
- [ ] 对话列表加载 + 媒体浏览冒烟测试
- [ ] 创建下载任务 + 暂停恢复冒烟测试
- [ ] 二进制体积无异常膨胀
- [ ] 用 smart-commit 提交，注明上游版本跨度

## 相关文档

- [AI 代理版升级剧本](../../dev-ai/advanced/02-upgrade-playbook.md)
- [← 返回开发者文档目录](../README.md)
