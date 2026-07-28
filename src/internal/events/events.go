// Package events 封装 Wails 事件推送，统一前后端事件契约。
package events

import (
	"context"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// 事件名称契约，前端通过 EventsOn 订阅。
const (
	// Login 登录流程事件，payload 见 LoginUpdate
	Login = "login:update"
	// TaskUpdate 任务级状态事件，payload 见 TaskUpdate
	Task = "task:update"
	// TaskFile 文件级进度事件，payload 见 FileUpdate
	TaskFile = "task:file"
	// ScriptLog 脚本日志事件，payload 为字符串
	ScriptLog = "script:log"
	// Log 应用日志批量事件，payload 为 []logging.LogEntry
	Log = "log:batch"
)

// LoginUpdate 登录事件负载。
type LoginUpdate struct {
	// Stage: qr / need_code / need_password / success / error / logout
	Stage string `json:"stage"`
	QRURL string `json:"qrUrl,omitempty"`
	User  *User  `json:"user,omitempty"`
	Error string `json:"error,omitempty"`
}

// User 登录成功后的账号信息。
type User struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
}

// Emitter 持有 Wails 运行时上下文并发射事件。
// 在应用 OnStartup 前调用 Emit 是安全的（静默丢弃）。
type Emitter struct {
	mu  sync.RWMutex
	ctx context.Context
}

// Bind 在 Wails OnStartup 中绑定运行时上下文。
func (e *Emitter) Bind(ctx context.Context) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.ctx = ctx
}

// Emit 发射事件；未绑定上下文时静默忽略。
func (e *Emitter) Emit(event string, payload any) {
	e.mu.RLock()
	ctx := e.ctx
	e.mu.RUnlock()
	if ctx == nil {
		return
	}
	runtime.EventsEmit(ctx, event, payload)
}

// Ctx 返回 Wails 运行时上下文（用于对话框等运行时 API），未绑定时为 nil。
func (e *Emitter) Ctx() context.Context {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.ctx
}
