# 升级迁移剧本

> [← AI 代理文档目录](../README.md) | [项目主页](../../../README.md) | [人类版：上游同步](../../dev-human/advanced/02-upstream-sync.md)

AI 代理执行"同步上游 tdl 子模块 / 升级依赖"任务的逐步剧本。每一步都有明确的成功判据，失败时按对应分支处理。

## 剧本 1：同步 ref/tdl 子模块

### 前置约束

- ⛔ 全程不得修改 `ref/tdl/` 内文件；子模块内只允许 `git fetch` / `git checkout <tag>`
- 升级目标版本由用户指定；未指定时取上游最新 release tag 并在回复中明确告知所选版本

### 步骤

```bash
# 1. 记录当前版本（回滚锚点）
git submodule status                      # 记下当前 commit

# 2. 更新子模块到目标版本
git submodule update --remote ref/tdl     # 最新
# 或指定 tag：cd ref/tdl && git fetch --tags && git checkout <tag> && cd ../..

# 3. 依赖收敛
cd src && go mod tidy
```

**判据**：`go mod tidy` 无报错。若报缺包/版本冲突 → 检查 go.mod 三条 replace 是否完好；上游若拆分/合并了模块，需相应增删 replace 条目。

```bash
# 4. 编译定位破坏性变更
go build ./...
```

**判据**：编译通过 → 跳到步骤 6。编译失败 → 进入步骤 5 适配。

### 5. 适配破坏性变更（按错误类型分诊）

| 编译错误位置 | 处理方式 |
| --- | --- |
| `services/auth.go`（pkg/tclient、pkg/tpath、core/storage） | 读上游对应包新签名，最小改动适配调用处 |
| `services/chat.go/thumb.go`（core/tmedia、core/util/tutil、core/storage） | 读上游对应包新签名，最小改动适配调用处 |
| `engine/`（core/downloader、dcpool、tmessage、tplfunc） | 先 `cd ref/tdl && git diff <旧>..<新> -- app/dl/ core/` 看上游同类调用如何改，再对照移植 |
| `main.go`（pkg/kv） | 对照上游 kv 驱动初始化新用法 |

移植上游 `app/dl` 逻辑变更时逐文件对照 `src/internal/engine/{iter,progress,elem,task}.go`，只移植行为性差异（bug 修复、新特性、fingerprint 算法、flood-wait 处理），不引入 CLI 特有代码（cobra/prog 进度条）。

**高危检查**（改完必须人工确认并在总结中报告）：

- [ ] `fingerprint` 算法是否变化（变化 = 用户旧断点失效，需在提交信息与发布说明注明）
- [ ] `key.Resume` / session key 格式是否变化（影响已登录会话兼容性）
- [ ] tdl 内置 App ID/Hash（`pkg/consts`）是否变化

### 6. 全量验证

```bash
go test ./...                                  # 必须全过
cd frontend && pnpm build && cd ..             # 前端契约未受影响
wails build -ldflags "-s -w" -trimpath         # 产出可执行文件
```

**判据**：三条全绿 + `src/build/bin/tdl-ui.exe` 生成且体积无异常膨胀（基线约 50 MB，涨幅 >20% 需排查）。

### 7. 收尾

- 更新 `go.mod` 中 tdl require 版本号与子模块实际 tag 一致（replace 生效下仅为可读性）
- 文档同步：若 API 面变化，更新 `dev-human/advanced/02-upstream-sync.md` 的依赖表与本剧本
- smart-commit 提交，提交信息注明版本跨度（如 `chore: sync ref/tdl v0.20.3 -> v0.21.0`）

### 回滚

任一步无法收敛时：`cd ref/tdl && git checkout <锚点 commit> && cd ../.. && cd src && go mod tidy`，恢复后向用户报告阻塞原因与上游破坏点清单。

## 剧本 2：升级直接依赖（Wails / Yaegi / 前端包）

### Go 依赖

```bash
cd src
go get <module>@<version>
go mod tidy && go build ./... && go test ./...
```

- **Yaegi 升级**必须重点跑 `go test ./internal/script/...`（符号表结构变化会破坏 `sandboxSymbols()` 的 `os/exec` 删除逻辑）
- **Wails v2 → v3 属重大迁移**，不适用本剧本，需单独规划（绑定与事件 API 全变）

### 前端依赖

```bash
cd src/frontend
pnpm update <pkg> --latest
pnpm build
```

- **PrimeVue 主版本升级**：核对 `@primeuix/themes` 的 definePreset API 与 `darkModeSelector` 配置是否兼容（见 `main.ts`），逐页面目视验证组件渲染
- **Vite/Vue 升级**：`pnpm build` 通过后仍需 `wails dev` 确认嵌入式 WebView 下运行正常

## 完成报告模板

升级任务结束时向用户报告：版本跨度、破坏性变更及适配点清单、高危检查三项结论、验证命令输出摘要、（如有）断点/会话兼容性影响。

## 相关文档

- [人类开发者版：上游子模块同步与适配升级](../../dev-human/advanced/02-upstream-sync.md)
- [← 返回 AI 代理文档目录](../README.md)
