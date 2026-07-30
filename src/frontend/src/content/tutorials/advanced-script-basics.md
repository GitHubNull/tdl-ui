# 脚本语法基础与编写规范

难度：高级 · 前置：创建下载任务

tdl UI 内置 Yaegi Go 解释器，你可以用**标准 Go 语法**编写脚本，在下载流程的关键节点介入：过滤文件、自定义文件名、挂接任务生命周期钩子。

## 脚本结构

每个脚本都是一个独立的 Go 源文件，必须满足：

1. 包名必须是 `package main`
2. 通过 `import "tdlui/api"` 访问内置类型与日志函数
3. 至少定义一个契约函数（见下文），否则保存时报错

最小可用脚本：

```go
package main

import "tdlui/api"

// 返回 false 则跳过该文件
func Filter(f api.FileInfo) bool {
	api.Log("检查文件:", f.FileName)
	return true
}
```

## 五个契约函数

所有契约函数均为**可选**，按需实现，可写在同一个脚本中：

| 函数签名 | 调用时机 | 返回值语义 |
| --- | --- | --- |
| `func Filter(f api.FileInfo) bool` | 每个文件下载前 | `false` 跳过该文件 |
| `func Rename(f api.FileInfo) string` | 每个文件保存前 | 新文件名（可含子目录）；空串回退默认模板 |
| `func OnTaskStart(t api.TaskInfo)` | 消息解析完成、开始下载前 | 无 |
| `func OnFileDone(f api.FileInfo)` | 每个文件下载完成时 | 无 |
| `func OnTaskDone(t api.TaskInfo)` | 任务结束（完成/失败/取消） | 无 |

> 函数名与签名必须**完全一致**（大小写敏感），签名不匹配时保存会报错并提示正确签名。

## 可用的标准库（白名单沙箱）

出于安全考虑，脚本运行在白名单沙箱中，仅允许导入以下 14 个纯计算类标准库包：

| 类别 | 包 |
| --- | --- |
| 文本处理 | `strings`、`bytes`、`unicode`、`unicode/utf8` |
| 格式化 | `fmt`、`strconv` |
| 正则 | `regexp` |
| 路径 | `path`、`path/filepath` |
| 数学 | `math`、`math/rand` |
| 时间 | `time` |
| 其他 | `errors`、`sort` |

以下能力被**完全禁止**，导入即编译失败：

- `os`、`os/exec` —— 不能读写文件、执行外部命令
- `net`、`net/http` —— 不能发起网络请求
- `syscall`、`unsafe`、`reflect` —— 不能进行系统调用与反射

## 编写规范建议

- **保持轻量**：契约函数在下载引擎协程中同步调用，单次调用超过 10 秒会被熔断禁用（详见「脚本调试与错误处理」）
- **避免全局可变状态的并发假设**：Filter/Rename 与 OnFileDone 由引擎内部串行化调用，无需自行加锁
- **Rename 返回相对路径**：可用 `filepath.Join("子目录", "文件名.mp4")` 创建子目录归档；返回的文件名会自动清理非法字符
- **善用日志**：`api.Log` / `api.Logf` 输出实时显示在「脚本」页日志面板，是最主要的调试手段

## 编写与使用步骤

1. 打开「脚本」页 → 点击工具栏的 **+**（新建脚本）按钮，编辑器会填入起始模板
2. 填写脚本名（如 `video-only`），编写代码
3. 点击「校验」确认语法正确、契约函数被识别
4. 点击「试运行」用内置示例文件预览 Filter/Rename 结果
5. 点击「保存」
6. 创建下载任务时，从「过滤 / 命名脚本」下拉框选择该脚本
