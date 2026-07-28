# 代理与设置

> [← 教程目录](../README.md) | [项目主页](../../../README.md)

难度：🟡 进阶

「设置」页集中管理网络、下载与界面选项。修改后点击「保存设置」生效。

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

创建任务时未指定目录时的默认保存位置。

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

主题支持 **亮色 / 暗色 / 跟随系统** 三种模式，选择立即生效并持久保存。左侧导航栏底部的按钮可快速切换亮暗。

## 下一步

→ [过滤与命名脚本](../advanced/01-filter-rename.md)
