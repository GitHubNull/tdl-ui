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
	"time"

	"tdlui/api"
)

// 记录任务开始时间，用于计算耗时
var startAt time.Time

func OnTaskStart(t api.TaskInfo) {
	startAt = time.Now()
	api.Logf("[%s] 任务 %s 开始，共 %d 个文件，保存到 %s",
		startAt.Format("15:04:05"), t.ID, t.Total, t.Dir)
}

func OnFileDone(f api.FileInfo) {
	api.Logf("完成 %s（%.1f MB）", f.FileName, float64(f.FileSize)/1024/1024)
}

func OnTaskDone(t api.TaskInfo) {
	elapsed := time.Since(startAt).Round(time.Second)
	rate := 0.0
	if t.Total > 0 {
		rate = float64(t.Finished) / float64(t.Total) * 100
	}
	api.Logf("任务结束（%s）：状态=%s 成功=%d 失败=%d 成功率=%.1f%% 耗时=%s",
		t.ID, t.Status, t.Finished, t.Failed, rate, elapsed)
}
```

> 注意：沙箱禁止导入 `os` 包，无法直接写文件。所有统计信息通过 `api.Logf` 输出到脚本日志面板。

## 执行语义

- 钩子在**下载引擎所在的后端协程**中同步调用，请避免长耗时操作（有 10 秒超时保护）
- 钩子 panic 不会中断下载任务，错误会记录到脚本日志
- 任务被取消 / 失败时同样触发 `OnTaskDone`，可通过 `t.Status` 区分

## 错误处理与调试

### panic 自动恢复

脚本中的 `panic` 会被引擎捕获，**不会中断下载任务**。各函数的兜底行为：

| 函数 | panic 后的兜底行为 |
| --- | --- |
| `Filter` | 默认**保留**该文件（宁多勿漏） |
| `Rename` | 回退到默认命名模板 |
| `OnTaskStart` / `OnFileDone` / `OnTaskDone` | 忽略本次调用，任务继续 |

### 10 秒超时与熔断

每次契约函数调用有 **10 秒超时**。一旦超时，该函数被**熔断禁用**——本任务后续不再调用它。写循环时务必确认退出条件，避免无界循环。

> **已知限制**：超时的执行体无法被中断（Yaegi 解释器不支持强制终止），已在死循环中的那次执行会一直在后台占用一颗 CPU 核，直到应用退出。

### 常见错误

| 报错 / 现象 | 原因 | 解决 |
| --- | --- | --- |
| `脚本编译失败: ... undefined: os` | 导入了沙箱禁止的包 | 只使用白名单内的 14 个标准库包 |
| `Filter 签名错误` | 参数或返回值类型写错 | 对照 API 参考修正签名 |
| 日志出现 `脚本 panic: ...` | 运行期 panic | 根据 panic 信息定位，注意空串、除零、越界 |
| 日志出现 `[熔断] XX 执行超过 10s` | 函数单次执行超过 10 秒 | 排查死循环/重量级计算；修正后重启应用 |

## 相关文档

- 想扩展脚本 API？参见开发者文档：[Yaegi 脚本引擎扩展](../../dev-human/advanced/01-yaegi-extend.md)
- [← 返回教程目录](../README.md)
