# 脚本引擎安全约束

## 概述

脚本引擎位于 `src/internal/script/` + `src/internal/scriptapi/`，基于 [Yaegi](https://github.com/traefik/yaegi) Go 解释器实现。用户可编写 Go 脚本实现下载过滤、自定义文件名、任务生命周期钩子等功能。

引擎核心设计目标：**功能灵活** + **安全隔离**。脚本在沙箱中运行，禁止访问文件系统、网络、进程等敏感资源；同时通过 panic 恢复与超时保护防止恶意或错误脚本崩溃主程序。

## Yaegi 引擎封装

### 脚本契约

脚本为 `package main` 的 Go 源码，可选定义以下契约函数：

```go
package main

import "tdlui/api"

func Filter(f api.FileInfo) bool        // 返回 false 跳过该文件
func Rename(f api.FileInfo) string      // 返回新文件名，空串回退默认模板
func OnTaskStart(t api.TaskInfo)        // 任务开始时调用
func OnFileDone(f api.FileInfo)         // 单文件完成时调用
func OnTaskDone(t api.TaskInfo)         // 任务结束时调用
```

### 契约提取流程

`script.Load(src)` 的执行步骤：

1. 创建 Yaegi 解释器：`interp.New(interp.Options{})`
2. 注入白名单标准库：`i.Use(sandboxSymbols())`
3. 注入自定义 API 包：`i.Use(apiExports())` —— `tdlui/api` 提供 `FileInfo`、`TaskInfo`、`Log`、`Logf`
4. 编译执行脚本：`i.Eval(src)`
5. 反射提取契约函数：`i.Eval("main.Filter")` 等，类型断言后存入 `Contracts` 结构
6. 校验：至少定义一个契约函数，否则报错

### Contracts 结构

```go
type Contracts struct {
    Filter      func(scriptapi.FileInfo) bool
    Rename      func(scriptapi.FileInfo) string
    OnTaskStart func(scriptapi.TaskInfo)
    OnFileDone  func(scriptapi.FileInfo)
    OnTaskDone  func(scriptapi.TaskInfo)
    mu          sync.Mutex  // 串行化 Safe* 调用
}
```

所有契约函数通过 `Safe*` 方法调用，内部持有 `mu` 锁保证串行化（Yaegi 解释器不保证并发安全）。

## 沙箱安全策略

### 白名单标准库

`sandboxSymbols()` 仅导出纯计算类包，阻断所有具备进程/文件/网络能力的包：

| 允许 | 阻断 |
|------|------|
| bytes, errors, fmt, math, math/rand | os, os/exec |
| path, path/filepath, regexp | net, net/http |
| sort, strconv, strings | syscall, unsafe |
| time, unicode, unicode/utf8 | reflect（部分） |

实现方式：遍历 `stdlib.Symbols`，按 import 路径前缀过滤，仅保留白名单内的符号。

### 路径安全

`script.Store` 的脚本文件操作：

- 脚本名正则校验：`^[\w\p{Han}-]+$`（仅字母、数字、下划线、中划线、中文）
- Windows 保留设备名拒绝：`con`, `prn`, `aux`, `nul`, `com1-9`, `lpt1-9`
- 文件路径通过 `filepath.Join(dir, name+".go")` 拼接，防止路径穿越

### 脚本源码快照（SCR-04）

任务创建时，脚本源码被快照保存到 `Task.scriptSrc`。恢复任务时优先使用快照重建契约，而非按名重新加载当前磁盘上的脚本文件。这避免了：

- 用户编辑脚本后，恢复中的任务语义发生漂移
- 脚本被删除后，恢复任务无法找到原脚本

## Panic 恢复与超时保护

### 调用保护机制

`callWithGuard[T]()` 为每次脚本函数调用提供双重保护：

```go
func callWithGuard[T any](fn func() T, fallback T) (T, error) {
    ch := make(chan result, 1)
    go func() {
        defer func() {
            if r := recover(); r != nil {
                ch <- result{val: fallback, err: fmt.Errorf("脚本 panic: %v", r)}
            }
        }()
        ch <- result{val: fn()}
    }()
    select {
    case r := <-ch:
        return r.val, r.err
    case <-time.After(callTimeout):  // 10 秒超时
        return fallback, fmt.Errorf("超过 %s: %w", callTimeout, errScriptTimeout)
    }
}
```

1. **Panic 恢复**：`defer recover()` 捕获脚本 panic，返回 fallback 值 + 错误
2. **超时熔断**：10 秒超时后返回 `errScriptTimeout`，由调用方熔断对应契约函数

### 熔断机制

当某契约函数触发超时时，该函数被置为 `nil`，后续调用直接跳过：

```go
keep, err := callWithGuard(func() bool { return c.Filter(f) }, true)
if errors.Is(err, errScriptTimeout) {
    c.Filter = nil  // 熔断
    scriptapi.Logf("Filter 执行超时，已熔断禁用该脚本函数")
}
```

**设计意图**：避免死循环脚本逐文件重复超时并持续泄漏 goroutine。

### 超时常量

```go
const callTimeout = 10 * time.Second  // 单次脚本函数调用超时
```

## 脚本文件 CRUD

`script.Store`（`store.go`）管理数据目录 `scripts/` 下的 `.go` 文件：

| 操作 | 方法 | 说明 |
|------|------|------|
| 列出 | `List() ([]Meta, error)` | 按名称排序返回全部脚本元信息 |
| 读取 | `Read(name) (string, error)` | 读取脚本源码 |
| 保存 | `Write(name, src) error` | 保存脚本源码（覆盖） |
| 删除 | `Delete(name) error` | 删除脚本文件 |
| 加载 | `LoadByName(name) (*Contracts, error)` | 读取并解释指定脚本 |

脚本元信息：

```go
type Meta struct {
    Name      string // 不含扩展名
    Size      int64  // 字节
    UpdatedAt string // YYYY-MM-DD HH:MM:SS
}
```

## 契约函数提取

`script.Load()` 通过反射从 Yaegi 解释器中提取契约函数：

```go
if v, err := i.Eval("main.Filter"); err == nil {
    fn, ok := v.Interface().(func(scriptapi.FileInfo) bool)
    if !ok {
        return nil, fmt.Errorf("Filter 签名错误，应为 func(api.FileInfo) bool")
    }
    c.Filter = fn
}
```

签名严格校验：类型不匹配时报错，防止用户写错函数签名导致运行时 panic。

## 生命周期钩子

脚本钩子由下载引擎在特定时机调用：

| 钩子 | 触发时机 | 调用位置 |
|------|----------|----------|
| OnTaskStart | 任务开始运行前 | `run()` 中状态转为 running 后 |
| OnFileDone | 单个文件下载完成后 | `progress.OnDone()` → `task.onFileDone()` |
| OnTaskDone | 任务结束（done/failed/canceled）后 | `run()` 结束前 |

钩子错误（panic/超时）通过 `callWithGuard` 保护，不会中断下载流程。脚本日志通过 `scriptapi.Log` / `Logf` 转发到 GUI 的脚本日志面板。

## scriptapi 包

`src/internal/scriptapi/scriptapi.go` 定义脚本可见的数据类型与日志函数：

```go
type FileInfo struct {
    DialogID    int64
    MessageID   int
    MessageDate int64
    FileName    string
    FileCaption string
    FileSize    int64
}

type TaskInfo struct {
    ID       string
    Dir      string
    Total    int
    Finished int
    Failed   int
    Status   string
}

func Log(args ...any)     // 转发到 GUI 脚本日志面板
func Logf(format string, args ...any)
```

日志通过 `SetLogSink()` 注入回调，由宿主应用（`ScriptService`）将日志输出到前端 `script:log` 事件。
