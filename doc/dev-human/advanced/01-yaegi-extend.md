# Yaegi 脚本引擎扩展

> [← 开发者文档目录](../README.md) | [项目主页](../../../README.md)

本文讲解脚本引擎的实现原理与扩展方法。代码：`src/internal/script/engine.go` + `src/internal/scriptapi/`。

## 工作原理

1. **解释器构建**：每次 `Load(src)` 创建全新的 `yaegi/interp.Interpreter`（脚本之间完全隔离）
2. **符号注入**：
   - `sandboxSymbols()`：从 `yaegi/stdlib.Symbols` 拷贝标准库符号表，**删除 `os/exec`**
   - `apiExports()`：注入自定义包 `tdlui/api`，包含 `FileInfo`、`TaskInfo`、`Log`、`Logf`
3. **契约提取**：`i.Eval("main.Filter")` 逐个取出函数值并做类型断言（`func(api.FileInfo) bool` 等），签名不符时报错
4. **安全执行**：所有调用经 `callWithGuard`（Go 泛型）包裹 —— `recover()` 捕获 panic + 10 秒超时保护，出错返回 error 而非崩溃

## 扩展脚本 API 的步骤

以"给脚本增加 HTTP 通知函数 `api.Notify(url, msg)`"为例：

### 1. 在 scriptapi 中实现

```go
// src/internal/scriptapi/scriptapi.go
func Notify(url, msg string) error {
    resp, err := http.Post(url, "text/plain", strings.NewReader(msg))
    if err != nil { return err }
    defer resp.Body.Close()
    return nil
}
```

### 2. 注册到符号表

```go
// src/internal/script/engine.go 的 apiExports()
"Notify": reflect.ValueOf(scriptapi.Notify),
```

### 3. 更新文档与起始模板

- `services/script.go` 的 `StarterTemplate()`（用户可见的示例）
- `doc/tutorials/advanced/` 教程中的 API 表格

### 4. 补测试

在 `engine_test.go` 中添加调用新函数的脚本用例。

## 新增契约函数的步骤

以增加 `OnFileFailed(f api.FileInfo, errMsg string)` 为例：

1. `engine.go`：`Contracts` 结构加字段，`Load` 中提取 `main.OnFileFailed` 并断言类型，加 `SafeOnFileFailed` 包装
2. `engine/progress.go`：在文件失败回调处调用
3. `services/script.go`：`Validate` 的 funcs 列表加名字
4. 教程与测试同步更新

## 沙箱策略

当前策略：**仅移除 `os/exec`**，保留 `os`（文件操作）与 `net/http`（供钩子做通知），信任脚本作者是用户本人。

若需收紧（例如分发场景），可在 `sandboxSymbols()` 中继续删除：

```go
delete(symbols, "os/os")          // 全部文件系统操作
delete(symbols, "net/http/http")  // 网络
delete(symbols, "syscall/syscall")
```

注意：删除 `os` 会导致教程中"写下载报告"的案例失效，需同步修订文档。

## 已知限制

- Yaegi 对泛型的支持不完整，脚本内避免使用泛型
- 每次 Load 会重新解释源代码（毫秒级），任务创建时 Load 一次、整个任务复用同一 `Contracts`
- 脚本内启动的 goroutine 不受超时保护管控，文档已提醒用户避免

## 下一步

→ [上游子模块同步与适配升级](02-upstream-sync.md)
