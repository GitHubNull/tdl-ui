# 安装与启动

难度：基础

## 下载

从项目 Releases 页面下载 `tdl-ui.exe`（Windows x64）。单一二进制文件，**无需安装任何依赖**。

## 启动

双击 `tdl-ui.exe` 即可启动。首次启动时：

1. Windows 可能弹出 SmartScreen 提示，点击「更多信息 → 仍要运行」
2. 应用会在用户数据目录创建配置与会话存储

## 界面总览

窗口左侧是图标导航栏，从上到下依次为：

| 图标 | 页面 | 用途 |
| --- | --- | --- |
| {{icon:chats}} | 对话 | 浏览对话列表，查看媒体文件，勾选下载 |
| {{icon:download}} | 下载 | 下载任务的实时进度与控制 |
| {{icon:scripts}} | 脚本 | 编写过滤 / 命名 / 自动化脚本 |
| {{icon:logs}} | 日志 | 查看应用运行日志 |
| {{icon:settings}} | 设置 | 代理、下载目录、主题等 |
| {{icon:tutorial}} | 教程 | 你正在看的这份使用教程 |
| {{icon:about}} | 关于 | 版本、协议与反馈渠道 |

导航栏底部还有 {{icon:account}} 账号入口（登录 / 登出）和 {{icon:theme-dark}} / {{icon:theme-light}} 主题快速切换按钮。

## 数据目录

应用数据存放在 `%AppData%\tdl-ui`（可在「设置」页底部查看实际路径）：

```
tdl-ui/
├── config.yaml     # 应用设置
├── logging.yaml    # 日志配置
├── tasks.db        # 任务数据库（SQLite）
├── cache/          # 缩略图与预览缓存
├── kv/             # Telegram 会话存储（bolt 数据库）
└── scripts/        # 用户脚本（.go 文件）
```

> ⚠️ `kv/` 目录包含登录凭据，请勿分享给他人。

## 卸载

删除 `tdl-ui.exe` 与 `%AppData%\tdl-ui` 目录即可完全清除。
