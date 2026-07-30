# 内置函数与类型参考

难度：高级

本章是脚本 API 的完整参考。脚本通过 `import "tdlui/api"` 使用以下全部内容。

## 日志函数

### api.Log

```go
func Log(args ...any)
```

| 项目 | 说明 |
| --- | --- |
| 参数 | `args ...any` —— 任意数量、任意类型的值，效果同 `fmt.Sprintln`（值之间自动加空格） |
| 返回值 | 无 |
| 适用场景 | 快速输出调试信息、拼接少量值 |

**使用示例**：

```go
api.Log("下载完成:", f.FileName, "大小:", f.FileSize)
// 输出：下载完成: video.mp4 大小: 10485760
```

**最佳实践与注意事项**：

- 输出实时推送到「脚本」页日志面板，无需换行符
- 日志仅在 GUI 运行期间可见，不会写入磁盘；重要统计建议在 `OnTaskDone` 中一次性汇总输出
- 高频调用（如在 Filter 中逐文件打印）会刷屏，定位问题后应移除

### api.Logf

```go
func Logf(format string, args ...any)
```

| 项目 | 说明 |
| --- | --- |
| 参数 | `format string` —— `fmt.Sprintf` 风格格式串；`args ...any` —— 格式参数 |
| 返回值 | 无 |
| 适用场景 | 需要控制数字精度、对齐或固定格式的输出 |

**使用示例**：

```go
api.Logf("任务 %s：成功 %d / 失败 %d（%.1f MB）",
	t.ID, t.Finished, t.Failed, float64(f.FileSize)/1024/1024)
```

**最佳实践与注意事项**：

- 格式动词与 Go 标准库一致：`%s` 字符串、`%d` 整数、`%.1f` 一位小数浮点等
- 格式串与参数数量不匹配时不会崩溃，但输出会包含 `%!d(MISSING)` 之类的错误标记，注意检查

## 契约函数参考

契约函数由**你来实现**、由下载引擎调用。全部可选，但至少要实现一个。

### Filter —— 文件过滤

```go
func Filter(f api.FileInfo) bool
```

| 项目 | 说明 |
| --- | --- |
| 参数 | `f api.FileInfo` —— 当前待下载文件的元信息 |
| 返回值 | `bool` —— `true` 下载该文件；`false` 跳过 |
| 调用时机 | 任务装配阶段，逐文件调用（在下载开始前） |

**适用场景**：按扩展名、大小、消息日期、说明文字等条件挑选文件。

**注意事项**：

- 脚本 panic 或超时时，该文件**默认保留**（不会因脚本错误而漏下载）
- 不实现该函数时全部文件都会下载

### Rename —— 自定义文件名

```go
func Rename(f api.FileInfo) string
```

| 项目 | 说明 |
| --- | --- |
| 参数 | `f api.FileInfo` —— 当前文件的元信息 |
| 返回值 | `string` —— 新文件名，可含相对子目录（如 `2026-07/a.mp4`）；返回**空串**回退到「设置」页的默认命名模板 |
| 调用时机 | 文件保存前逐文件调用 |

**适用场景**：按日期/对话分目录归档、用消息说明文字命名、统一扩展名大小写。

**注意事项**：

- 返回的名字会自动清理文件系统非法字符
- 建议保留原扩展名：`filepath.Ext(f.FileName)`
- 脚本出错时回退默认模板，不会中断下载

### OnTaskStart —— 任务开始钩子

```go
func OnTaskStart(t api.TaskInfo)
```

| 项目 | 说明 |
| --- | --- |
| 参数 | `t api.TaskInfo` —— 任务整体信息（此时 `Total` 已确定） |
| 返回值 | 无 |
| 调用时机 | 消息解析完成、第一个文件开始下载前，每任务一次 |

**适用场景**：记录任务开始时间、输出任务概况。

### OnFileDone —— 单文件完成钩子

```go
func OnFileDone(f api.FileInfo)
```

| 项目 | 说明 |
| --- | --- |
| 参数 | `f api.FileInfo` —— 刚下载完成的文件元信息 |
| 返回值 | 无 |
| 调用时机 | 每个文件下载完成时 |

**适用场景**：逐文件进度播报、累计统计。

**注意事项**：仅在文件**成功**下载后触发，失败的文件不会调用。

### OnTaskDone —— 任务结束钩子

```go
func OnTaskDone(t api.TaskInfo)
```

| 项目 | 说明 |
| --- | --- |
| 参数 | `t api.TaskInfo` —— 任务最终状态（`Status` 为 `done` / `failed` / `canceled`） |
| 返回值 | 无 |
| 调用时机 | 任务进入终态时，每任务一次；**取消和失败同样触发** |

**适用场景**：输出成功率汇总、结束提醒。

## 数据类型参考

### api.FileInfo

单个文件的元信息，供 `Filter` / `Rename` / `OnFileDone` 使用：

| 字段 | 类型 | 含义 |
| --- | --- | --- |
| `DialogID` | `int64` | 会话（频道/群组/私聊）ID |
| `MessageID` | `int` | 消息 ID |
| `MessageDate` | `int64` | 消息发送时间（Unix 秒），用 `time.Unix(f.MessageDate, 0)` 转换 |
| `FileName` | `string` | 原始文件名 |
| `FileCaption` | `string` | 消息文本/媒体说明（可能为空串） |
| `FileSize` | `int64` | 文件大小（字节） |

### api.TaskInfo

下载任务的整体信息，供 `OnTaskStart` / `OnTaskDone` 使用：

| 字段 | 类型 | 含义 |
| --- | --- | --- |
| `ID` | `string` | 任务 ID |
| `Dir` | `string` | 下载目录（绝对路径） |
| `Total` | `int` | 文件总数 |
| `Finished` | `int` | 已完成文件数 |
| `Failed` | `int` | 失败文件数 |
| `Status` | `string` | 任务状态：`running` / `done` / `failed` / `canceled` |
