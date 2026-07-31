// Package script 封装 Yaegi 解释器，加载并执行用户 Go 脚本。
//
// 脚本契约（package main）：
//   - func Filter(f api.FileInfo) bool        文件过滤，返回 false 跳过该文件
//   - func Rename(f api.FileInfo) string      自定义文件名，返回空串回退默认模板
//   - func OnTaskStart(t api.TaskInfo)        任务开始钩子
//   - func OnFileDone(f api.FileInfo)         单文件完成钩子
//   - func OnTaskDone(t api.TaskInfo)         任务结束钩子
//
// 所有契约函数均为可选，脚本通过 import "tdlui/api" 访问类型与日志函数。
package script

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"

	"tdl-ui/internal/logging"
	"tdl-ui/internal/scriptapi"
)

var logScript = logging.L("script")

// callTimeout 单次脚本函数调用的超时时间；var 便于测试缩短。
var callTimeout = 10 * time.Second

// errScriptTimeout 脚本执行超时哨兵错误，用于触发契约函数熔断。
var errScriptTimeout = errors.New("脚本执行超时")

// fuseContract 熔断可观测化统一出口（CODE-001）：宿主日志按 Error 级别留痕，
// 并经 scriptapi 日志通道推送前端脚本面板；文案明确告知 yaegi 无法中断
// 已超时的执行体（已知限制），泄漏的 goroutine 会持续占用 CPU 直到进程退出。
func fuseContract(name string) {
	scriptapi.Logf("[熔断] %s 执行超过 %s，已熔断禁用该脚本函数；其执行体可能仍在后台占用 CPU，建议修正脚本后重启应用", name, callTimeout)
	logScript.Errorf("脚本函数 %s 超时熔断，执行体不可中断，可能残留 CPU 占用直到进程退出", name)
}

// Contracts 从脚本中解析出的契约函数集合，未定义的函数为 nil。
type Contracts struct {
	Filter      func(scriptapi.FileInfo) bool
	Rename      func(scriptapi.FileInfo) string
	OnTaskStart func(scriptapi.TaskInfo)
	OnFileDone  func(scriptapi.FileInfo)
	OnTaskDone  func(scriptapi.TaskInfo)

	// mu 串行化所有 Safe* 调用：yaegi 解释器不保证并发安全，
	// Filter/Rename（迭代器 goroutine）与 OnFileDone（下载 worker）会时间重叠；
	// 同时保护超时熔断置 nil 的写入。
	// CODE-001 核查结论：熔断置 nil 后 Safe* 拿锁仅做 nil 判断即返回（超时的
	// callWithGuard 已返回并释放锁，泄漏的执行体在锁外运行），后续调用为
	// 跳过语义而非等锁，无需 TryLock（见 TestFusedFilterSkipsImmediately）。
	mu sync.Mutex
}

// Load 解释执行脚本源码并提取契约函数。
func Load(src string) (*Contracts, error) {
	i := interp.New(interp.Options{})
	if err := i.Use(sandboxSymbols()); err != nil {
		return nil, fmt.Errorf("load stdlib: %w", err)
	}
	if err := i.Use(apiExports()); err != nil {
		return nil, fmt.Errorf("load tdlui/api: %w", err)
	}

	if _, err := i.Eval(src); err != nil {
		return nil, fmt.Errorf("脚本编译失败: %w", err)
	}

	c := &Contracts{}
	if v, err := i.Eval("main.Filter"); err == nil {
		fn, ok := v.Interface().(func(scriptapi.FileInfo) bool)
		if !ok {
			return nil, fmt.Errorf("Filter 签名错误，应为 func(api.FileInfo) bool")
		}
		c.Filter = fn
	}
	if v, err := i.Eval("main.Rename"); err == nil {
		fn, ok := v.Interface().(func(scriptapi.FileInfo) string)
		if !ok {
			return nil, fmt.Errorf("Rename 签名错误，应为 func(api.FileInfo) string")
		}
		c.Rename = fn
	}
	if v, err := i.Eval("main.OnTaskStart"); err == nil {
		fn, ok := v.Interface().(func(scriptapi.TaskInfo))
		if !ok {
			return nil, fmt.Errorf("OnTaskStart 签名错误，应为 func(api.TaskInfo)")
		}
		c.OnTaskStart = fn
	}
	if v, err := i.Eval("main.OnFileDone"); err == nil {
		fn, ok := v.Interface().(func(scriptapi.FileInfo))
		if !ok {
			return nil, fmt.Errorf("OnFileDone 签名错误，应为 func(api.FileInfo)")
		}
		c.OnFileDone = fn
	}
	if v, err := i.Eval("main.OnTaskDone"); err == nil {
		fn, ok := v.Interface().(func(scriptapi.TaskInfo))
		if !ok {
			return nil, fmt.Errorf("OnTaskDone 签名错误，应为 func(api.TaskInfo)")
		}
		c.OnTaskDone = fn
	}

	if c.Filter == nil && c.Rename == nil && c.OnTaskStart == nil && c.OnFileDone == nil && c.OnTaskDone == nil {
		return nil, fmt.Errorf("脚本未定义任何契约函数（Filter/Rename/OnTaskStart/OnFileDone/OnTaskDone）")
	}

	return c, nil
}

// HasFilter 是否定义了过滤函数（已熔断的视为未定义）。
func (c *Contracts) HasFilter() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Filter != nil
}

// HasRename 是否定义了命名函数（已熔断的视为未定义）。
func (c *Contracts) HasRename() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Rename != nil
}

// SafeFilter 带 panic 恢复与超时保护的过滤调用；出错时默认保留文件。
func (c *Contracts) SafeFilter(f scriptapi.FileInfo) (keep bool, err error) {
	if c == nil {
		return true, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Filter == nil {
		return true, nil
	}
	keep, err = callWithGuard(func() bool { return c.Filter(f) }, true)
	if errors.Is(err, errScriptTimeout) {
		c.Filter = nil
		fuseContract("Filter")
	}
	return keep, err
}

// SafeRename 带保护的命名调用；出错或返回空串时由调用方回退默认模板。
func (c *Contracts) SafeRename(f scriptapi.FileInfo) (name string, err error) {
	if c == nil {
		return "", nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Rename == nil {
		return "", nil
	}
	name, err = callWithGuard(func() string { return c.Rename(f) }, "")
	if errors.Is(err, errScriptTimeout) {
		c.Rename = nil
		fuseContract("Rename")
	}
	return name, err
}

// SafeOnTaskStart 带保护的任务开始钩子调用。
func (c *Contracts) SafeOnTaskStart(t scriptapi.TaskInfo) error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.OnTaskStart == nil {
		return nil
	}
	_, err := callWithGuard(func() struct{} { c.OnTaskStart(t); return struct{}{} }, struct{}{})
	if errors.Is(err, errScriptTimeout) {
		c.OnTaskStart = nil
		fuseContract("OnTaskStart")
	}
	return err
}

// SafeOnFileDone 带保护的文件完成钩子调用。
func (c *Contracts) SafeOnFileDone(f scriptapi.FileInfo) error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.OnFileDone == nil {
		return nil
	}
	_, err := callWithGuard(func() struct{} { c.OnFileDone(f); return struct{}{} }, struct{}{})
	if errors.Is(err, errScriptTimeout) {
		c.OnFileDone = nil
		fuseContract("OnFileDone")
	}
	return err
}

// SafeOnTaskDone 带保护的任务结束钩子调用。
func (c *Contracts) SafeOnTaskDone(t scriptapi.TaskInfo) error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.OnTaskDone == nil {
		return nil
	}
	_, err := callWithGuard(func() struct{} { c.OnTaskDone(t); return struct{}{} }, struct{}{})
	if errors.Is(err, errScriptTimeout) {
		c.OnTaskDone = nil
		fuseContract("OnTaskDone")
	}
	return err
}

// callWithGuard 在独立 goroutine 中执行脚本函数，捕获 panic 并附加超时。
// 超时时返回 errScriptTimeout，由调用方熔断对应契约函数，避免死循环脚本
// 逐文件重复超时并持续泄漏 goroutine。
func callWithGuard[T any](fn func() T, fallback T) (T, error) {
	type result struct {
		val T
		err error
	}
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
	case <-time.After(callTimeout):
		return fallback, fmt.Errorf("超过 %s: %w", callTimeout, errScriptTimeout)
	}
}

// sandboxSymbols 返回白名单标准库符号表：仅导出脚本契约所需的纯计算类包，
// 阻断 os、net、syscall、unsafe、reflect 等具备进程/文件/网络能力的包。
func sandboxSymbols() interp.Exports {
	allowed := map[string]struct{}{
		"bytes":         {},
		"errors":        {},
		"fmt":           {},
		"math":          {},
		"math/rand":     {},
		"path":          {},
		"path/filepath": {},
		"regexp":        {},
		"sort":          {},
		"strconv":       {},
		"strings":       {},
		"time":          {},
		"unicode":       {},
		"unicode/utf8":  {},
	}

	symbols := make(interp.Exports, len(allowed))
	for key, pkg := range stdlib.Symbols {
		// yaegi 符号表 key 形如 "importPath/pkgName"（如 "path/filepath/filepath"），
		// 去掉末段包名得到 import 路径后比对白名单。
		importPath := key
		if idx := strings.LastIndex(key, "/"); idx >= 0 {
			importPath = key[:idx]
		}
		if _, ok := allowed[importPath]; ok {
			symbols[key] = pkg
		}
	}
	return symbols
}

// apiExports 注入自定义符号包 tdlui/api。
func apiExports() interp.Exports {
	return interp.Exports{
		"tdlui/api/api": {
			"FileInfo": reflect.ValueOf((*scriptapi.FileInfo)(nil)),
			"TaskInfo": reflect.ValueOf((*scriptapi.TaskInfo)(nil)),
			"Log":      reflect.ValueOf(scriptapi.Log),
			"Logf":     reflect.ValueOf(scriptapi.Logf),
		},
	}
}
