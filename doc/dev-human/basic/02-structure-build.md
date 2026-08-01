# 项目结构与构建

> [← 开发者文档目录](../README.md) | [项目主页](../../../README.md)

## 顶层目录

```
├── ref/tdl        # tdl 上游 Git 子模块 —— 只读！
├── src/           # Wails 应用全部源码
├── doc/           # 文档（tutorials / dev-human / dev-ai）
└── tmp/           # 临时文件，不入库
```

**核心约束**：`ref/tdl` 中的文件禁止修改，只允许 `git submodule update --remote` 同步上游。需要定制行为时，把相关代码复制到 `src/internal/` 改写（现例：`src/internal/engine/` 改写自 `ref/tdl/app/dl`）。

## src/ 结构

```
src/
├── main.go              # 组装入口：kv 单例、服务实例化、wails.Run
├── wails.json           # Wails 项目配置（pnpm 命令）
├── go.mod               # module tdl-ui；replace 指向 ../ref/tdl
├── internal/
│   ├── config/          # 设置持久化（%AppData%\tdl-ui\config.yaml）
│   ├── events/          # 事件名常量与 Emitter（Wails EventsEmit 封装）
│   ├── scriptapi/       # 脚本可见类型：FileInfo / TaskInfo / Log
│   ├── script/          # Yaegi 引擎封装 + 脚本文件 CRUD + 内置模板（templates/）
│   ├── engine/          # 下载任务引擎（iter/progress/elem/task/selection/links）
│   ├── store/           # SQLite 任务持久化层（tasks/task_items/files/resume_points，TaskRepo 接口）
│   ├── logging/         # 日志核心（zap + lumberjack，多目标输出、滚动、环形缓冲）
│   └── services/        # Wails 绑定服务（Auth/Chat/Download/Script/Settings/Log）
└── frontend/
    ├── src/
    │   ├── api.ts       # window.go 绑定封装（唯一后端调用入口）
    │   ├── types.ts     # 与 Go 结构体对应的 TS 类型契约
    │   ├── theme.ts     # 亮暗主题（localStorage + 跟随系统）
    │   ├── router.ts    # 八页面路由（hash 模式）
    │   ├── stores/      # Pinia：auth / chats / tasks / scripts / settings / logs
    │   └── pages/       # 八个页面组件（三段式 SFC）
    └── dist/            # 构建产物（go:embed 嵌入）
```

## 关键机制：go.mod replace

```
require github.com/iyear/tdl v0.20.3
replace (
    github.com/iyear/tdl => ../ref/tdl
    github.com/iyear/tdl/core => ../ref/tdl/core
    github.com/iyear/tdl/extension => ../ref/tdl/extension
)
```

tdl 仓库包含三个 Go module（主模块、core、extension），必须同时 replace，否则版本解析失败。

## 构建

```bash
cd src

# 开发模式：Go 热重载 + Vite dev server
wails dev

# 生产构建（-s -w 去符号表，-trimpath 去路径信息）
wails build -ldflags "-s -w" -trimpath
# 产物：src/build/bin/tdl-ui.exe
```

`wails build` 流程：生成绑定 → `pnpm install` → `pnpm build` → 前端产物经 `//go:embed all:frontend/dist` 嵌入 → Go 编译。

### 体积说明

当前二进制约 50 MB，主要来自 gotd（Telegram MTProto 实现，含全量 API schema）与 Yaegi（内嵌标准库符号表）。若需要减小体积：

```bash
upx --best build/bin/tdl-ui.exe   # 约压缩至一半，启动时间略增
```

## 测试

```bash
cd src
go test ./...                     # 全部后端测试
go test ./internal/script/ -v    # 脚本引擎测试（契约/panic 恢复/沙箱）
go test ./internal/engine/ -v    # 引擎测试（链接解析/选集验证）
go test ./internal/services/ -v  # 服务测试（对话/缩略图）
```

## 下一步

→ [Wails 前后端通信机制](../intermediate/01-wails-ipc.md)
