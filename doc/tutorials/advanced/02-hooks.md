# 任务自动化钩子

> [← 教程目录](../README.md) | [项目主页](../../../README.md)

难度：🔴 高级 | 前置：[过滤与命名脚本](01-filter-rename.md)

除了过滤与命名，脚本还可以挂接任务生命周期的三个节点，实现下载自动化。

## 钩子契约

三个函数均为可选，与 `Filter` / `Rename` 写在同一个脚本中：

```go
// 任务开始（消息解析完成、开始下载前）
func OnTaskStart(t api.TaskInfo)

// 每个文件下载完成时
func OnFileDone(f api.FileInfo)

// 任务结束（完成 / 失败 / 取消时都会调用）
func OnTaskDone(t api.TaskInfo)
```

`api.TaskInfo` 字段：

| 字段 | 类型 | 含义 |
| --- | --- | --- |
| `ID` | string | 任务 ID |
| `Dir` | string | 保存目录 |
| `Total` | int | 文件总数 |
| `Finished` | int | 已完成数 |
| `Failed` | int | 失败数 |
| `Status` | string | 任务状态：running / done / failed / canceled |

## 日志输出

`api.Log(args...)` 与 `api.Logf(format, args...)` 会把消息实时推送到「脚本」页的日志面板：

```go
api.Log("下载完成:", f.FileName)
api.Logf("任务 %s：成功 %d / 失败 %d", t.ID, t.Finished, t.Failed)
```

## 案例：任务统计与完成记录

```go
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"tdlui/api"
)

func OnTaskStart(t api.TaskInfo) {
	api.Logf("[%s] 任务开始，共 %d 个文件", time.Now().Format("15:04:05"), t.Total)
}

func OnFileDone(f api.FileInfo) {
	api.Logf("完成 %s（%.1f MB）", f.FileName, float64(f.FileSize)/1024/1024)
}

// 任务结束时在下载目录写一份清单
func OnTaskDone(t api.TaskInfo) {
	api.Logf("任务结束：成功 %d，失败 %d", t.Finished, t.Failed)

	report := fmt.Sprintf("task=%s time=%s ok=%d fail=%d\n",
		t.ID, time.Now().Format(time.RFC3339), t.Finished, t.Failed)
	path := filepath.Join(t.Dir, "download-report.txt")

	fd, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		api.Log("写报告失败:", err.Error())
		return
	}
	defer fd.Close()
	fd.WriteString(report)
}
```

## 执行语义

- 钩子在**下载引擎所在的后端协程**中同步调用，请避免长耗时操作（有 10 秒超时保护）
- 钩子 panic 不会中断下载任务，错误会记录到脚本日志
- 任务被取消 / 失败时同样触发 `OnTaskDone`，可通过 `t.Status` 区分

## 相关文档

- 想扩展脚本 API？参见开发者文档：[Yaegi 脚本引擎扩展](../../dev-human/advanced/01-yaegi-extend.md)
- [← 返回教程目录](../README.md)
