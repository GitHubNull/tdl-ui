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
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"

	"tdl-ui/internal/scriptapi"
)

// callTimeout 单次脚本函数调用的超时时间。
const callTimeout = 10 * time.Second

// Contracts 从脚本中解析出的契约函数集合，未定义的函数为 nil。
type Contracts struct {
	Filter      func(scriptapi.FileInfo) bool
	Rename      func(scriptapi.FileInfo) string
	OnTaskStart func(scriptapi.TaskInfo)
	OnFileDone  func(scriptapi.FileInfo)
	OnTaskDone  func(scriptapi.TaskInfo)
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

// HasFilter 是否定义了过滤函数。
func (c *Contracts) HasFilter() bool { return c != nil && c.Filter != nil }

// HasRename 是否定义了命名函数。
func (c *Contracts) HasRename() bool { return c != nil && c.Rename != nil }

// SafeFilter 带 panic 恢复与超时保护的过滤调用；出错时默认保留文件。
func (c *Contracts) SafeFilter(f scriptapi.FileInfo) (keep bool, err error) {
	if !c.HasFilter() {
		return true, nil
	}
	return callWithGuard(func() bool { return c.Filter(f) }, true)
}

// SafeRename 带保护的命名调用；出错或返回空串时由调用方回退默认模板。
func (c *Contracts) SafeRename(f scriptapi.FileInfo) (name string, err error) {
	if !c.HasRename() {
		return "", nil
	}
	return callWithGuard(func() string { return c.Rename(f) }, "")
}

// SafeOnTaskStart 带保护的任务开始钩子调用。
func (c *Contracts) SafeOnTaskStart(t scriptapi.TaskInfo) error {
	if c == nil || c.OnTaskStart == nil {
		return nil
	}
	_, err := callWithGuard(func() struct{} { c.OnTaskStart(t); return struct{}{} }, struct{}{})
	return err
}

// SafeOnFileDone 带保护的文件完成钩子调用。
func (c *Contracts) SafeOnFileDone(f scriptapi.FileInfo) error {
	if c == nil || c.OnFileDone == nil {
		return nil
	}
	_, err := callWithGuard(func() struct{} { c.OnFileDone(f); return struct{}{} }, struct{}{})
	return err
}

// SafeOnTaskDone 带保护的任务结束钩子调用。
func (c *Contracts) SafeOnTaskDone(t scriptapi.TaskInfo) error {
	if c == nil || c.OnTaskDone == nil {
		return nil
	}
	_, err := callWithGuard(func() struct{} { c.OnTaskDone(t); return struct{}{} }, struct{}{})
	return err
}

// callWithGuard 在独立 goroutine 中执行脚本函数，捕获 panic 并附加超时。
// 超时后放弃等待（goroutine 泄漏风险由脚本编写者文档约束）。
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
		return fallback, fmt.Errorf("脚本执行超时（%s）", callTimeout)
	}
}

// sandboxSymbols 返回受限的标准库符号表：移除 os/exec 等危险包。
func sandboxSymbols() interp.Exports {
	banned := []string{"os/exec"}

	symbols := make(interp.Exports, len(stdlib.Symbols))
	for path, pkg := range stdlib.Symbols {
		blocked := false
		for _, b := range banned {
			if strings.HasPrefix(path, b) {
				blocked = true
				break
			}
		}
		if !blocked {
			symbols[path] = pkg
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
