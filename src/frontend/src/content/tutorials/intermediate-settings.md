# 代理与设置

难度：中级

「设置」页集中管理网络、下载与界面选项。修改后点击「保存设置」生效。

## 账号

显示当前登录状态，提供「账号管理」/「前往登录」快捷入口。

## 网络

### 代理地址

支持 SOCKS5 与 HTTP 代理：

```
socks5://127.0.0.1:1080
socks5://user:pass@127.0.0.1:1080
http://127.0.0.1:8080
```

留空表示直连。代理对**登录流程和新创建的任务**生效；修改代理不影响进行中的任务。

## 下载

### 默认下载目录

创建任务时未指定目录时的默认保存位置。默认值为用户主目录下的 `Downloads/tdl-ui`。

### 默认命名模板

Go text/template 语法，与 tdl CLI 完全兼容。默认值：

```
{{ .DialogID }}_{{ .MessageID }}_{{ filenamify .FileName }}
```

可用变量：

| 变量 | 含义 |
| --- | --- |
| `.DialogID` | 对话（频道/群组）ID |
| `.MessageID` | 消息 ID |
| `.MessageDate` | 消息时间戳（Unix 秒） |
| `.FileName` | 原始文件名 |
| `.FileCaption` | 消息文字说明 |
| `.FileSize` | 文件大小（字节） |

常用函数：`filenamify`（清理非法字符）、`now`、`formatDate` 等。

示例 —— 按日期归类：

```
{{ formatDate .MessageDate }}_{{ filenamify .FileName }}
```

### 性能参数

| 参数 | 默认 | 说明 |
| --- | --- | --- |
| 单文件线程数 | 4 | 单个文件的并行分片数，大文件可调高 |
| 并发文件数 | 2 | 同时下载的文件数量 |
| DC 连接池大小 | 8 | Telegram 数据中心连接池，一般无需修改 |

> 参数过大可能触发 Telegram 限流（FLOOD_WAIT），建议保持默认或小幅调整。

## 界面

主题支持 **亮色 / 暗色 / 跟随系统** 三种模式：

- 选择后立即生效并持久保存
- 「跟随系统」模式会自动响应操作系统的主题偏好变化
- 左侧导航栏底部的 {{icon:theme-dark}} / {{icon:theme-light}} 按钮可快速在亮/暗之间切换

## 数据目录

设置页底部显示应用数据目录的实际路径（Windows 下为 `%AppData%\tdl-ui`），包含 `settings.json`、会话存储 `kv/` 和脚本目录 `scripts/`。
