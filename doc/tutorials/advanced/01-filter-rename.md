# 过滤与命名脚本

> [← 教程目录](../README.md) | [项目主页](../../../README.md)

难度：🔴 高级 | 前置：[创建下载任务](../intermediate/01-download.md)

tdl UI 内置 [Yaegi](https://github.com/traefik/yaegi) Go 解释器，你可以用标准 Go 语法编写脚本，在下载装配阶段对每个文件执行**过滤**（决定是否下载）与**重命名**（决定保存文件名）。

## 脚本契约

脚本必须是 `package main`，两个函数均为**可选**，按需实现：

```go
// 返回 false 则跳过该文件
func Filter(f api.FileInfo) bool

// 返回新文件名（可含子目录）；返回空串则回退到默认命名模板
func Rename(f api.FileInfo) string
```

`api.FileInfo` 字段（导入 `"tdlui/api"`）：

| 字段 | 类型 | 含义 |
| --- | --- | --- |
| `DialogID` | int64 | 对话 ID |
| `MessageID` | int | 消息 ID |
| `MessageDate` | int64 | 消息时间戳（Unix 秒） |
| `FileName` | string | 原始文件名 |
| `FileCaption` | string | 消息文字说明 |
| `FileSize` | int64 | 文件大小（字节） |

## 编写与使用步骤

1. 打开「脚本」页 → 点击 ➕ 新建，编辑器会填入起始模板（也可点击「模板」按钮从内置示例开始）
2. 填写脚本名（如 `video-only`），编写代码
3. 点击「校验」确认语法正确、契约函数被识别
4. 点击「试运行」用内置示例文件预览 Filter/Rename 结果
5. 点击「保存」
6. 在脚本列表左侧勾选 **启用开关**，使该脚本出现在「添加下载」弹窗的脚本下拉中
7. 在「对话」页勾选媒体后点击「下载选中」，或在「下载」页创建任务时，从「过滤 / 命名脚本」下拉框选择该脚本

> 首次启动时应用会自动写入 4 个内置模板（`rename-by-date`、`filter-media`、`skip-duplicates`、`auto-archive`），默认全部禁用。删除后不再自动恢复，可从「模板」弹窗重新生成。

## 案例一：只下载大于 10MB 的视频

```go
package main

import (
	"regexp"

	"tdlui/api"
)

// 常见视频扩展名（不区分大小写）
var videoExt = regexp.MustCompile(`(?i)\.(mp4|mkv|avi|mov|webm)$`)

// 最小体积：10MB
const minSize = 10 * 1024 * 1024

func Filter(f api.FileInfo) bool {
	if !videoExt.MatchString(f.FileName) {
		api.Logf("跳过非视频: %s", f.FileName)
		return false
	}
	if f.FileSize < minSize {
		api.Logf("跳过小文件: %s (%d 字节)", f.FileName, f.FileSize)
		return false
	}
	return true
}
```

## 案例二：按消息日期分目录 + 说明文字做文件名

```go
package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"tdlui/api"
)

func Rename(f api.FileInfo) string {
	day := time.Unix(f.MessageDate, 0).Format("2006-01-02")

	name := strings.TrimSpace(f.FileCaption)
	if name == "" {
		return filepath.Join(day, f.FileName) // 无说明时保留原名
	}
	if len(name) > 40 {
		name = name[:40]
	}
	return filepath.Join(day, fmt.Sprintf("%s%s", name, filepath.Ext(f.FileName)))
}
```

保存后文件将按 `2024-06-01/说明文字.mp4` 的结构存放。

## 注意事项与安全边界

- 脚本中的 `panic` 会被捕获：Filter 出错默认**保留**文件，Rename 出错回退默认模板，错误信息显示在脚本日志中
- 单次调用有超时保护（10 秒），超时后该函数被熔断禁用，避免死循环卡住下载
- 脚本运行在白名单沙箱中，仅允许导入 14 个纯计算类标准库包（`strings`、`fmt`、`regexp`、`time`、`path/filepath` 等）；`os`、`net`、`syscall` 等被完全禁止，导入即编译失败
- 返回的文件名会自动清理非法字符

## 下一步

→ [任务自动化钩子](02-hooks.md)
