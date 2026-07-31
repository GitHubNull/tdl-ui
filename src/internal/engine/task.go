package engine

import (
	"context"
	"fmt"
	"sync"
	"time"

	"tdl-ui/internal/events"
	"tdl-ui/internal/logging"
	"tdl-ui/internal/script"
	"tdl-ui/internal/scriptapi"
	"tdl-ui/internal/store"
)

var logEngine = logging.L("engine")

// Namespace 与 tdl CLI 默认命名空间保持一致，Telegram 会话数据互不干扰地存于自身 kv 目录。
const Namespace = "default"

// reconnectTimeout 客户端断线重连超时。
const reconnectTimeout = 5 * time.Minute

// 任务状态。
const (
	StatusQueued   = "queued"
	StatusRunning  = "running"
	StatusPaused   = "paused"
	StatusDone     = "done"
	StatusFailed   = "failed"
	StatusCanceled = "canceled"
)

// TaskOptions 创建任务的参数。URLs 与 Selections 至少一项非空，可混用。
type TaskOptions struct {
	URLs       []string    `json:"urls"`
	Selections []Selection `json:"selections"`
	Label      string      `json:"label"` // 任务显示名（选集下载时由前端提供，如「频道名 × 12 个文件」）
	Dir        string      `json:"dir"`
	ScriptName string      `json:"scriptName"`
	Template   string      `json:"template"`
	RewriteExt bool        `json:"rewriteExt"`
	SkipSame   bool        `json:"skipSame"`
	Group      bool        `json:"group"`
	Restart    bool        `json:"restart"`
}

// TaskFile 任务内单个文件记录（下载中为 .tmp 临时路径，完成后为最终路径）。
type TaskFile struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Size int64  `json:"size"`
	// State: downloading / done / failed
	State string `json:"state"`
	// 来源对话与消息（对话媒体页"已下载"标记依赖此关联）
	DialogID  int64 `json:"dialogId,omitempty"`
	MessageID int   `json:"messageId,omitempty"`
}

// TaskView 任务视图（前端展示 / task:update 事件负载）。
type TaskView struct {
	ID         string   `json:"id"`
	URLs       []string `json:"urls"`
	Label      string   `json:"label,omitempty"`
	Dir        string   `json:"dir"`
	ScriptName string   `json:"scriptName"`
	Status     string   `json:"status"`
	Error      string   `json:"error,omitempty"`
	Total      int      `json:"total"`
	Finished   int      `json:"finished"`
	Failed     int      `json:"failed"`
	FileCount  int      `json:"fileCount"`
	CreatedAt  string   `json:"createdAt"`
}

// Task 一次下载任务。
type Task struct {
	ID        string
	CreatedAt string

	opts      TaskOptions
	contracts *script.Contracts
	// scriptSrc 创建时的脚本源码快照，恢复时优先用它重建契约（SCR-04）
	scriptSrc string
	mgr       *Manager

	mu       sync.Mutex
	status   string
	errMsg   string
	total    int
	finished int
	failed   int
	paused   bool // 取消是否由“暂停”触发
	removed  bool // 已从管理器移除，阻断并发 Resume 复活僵尸下载
	cancel   context.CancelFunc
	files    []TaskFile    // 任务内文件记录（含 .tmp 未完成文件）
	runDone  chan struct{} // 本次 run 结束时关闭，供删除文件前等待任务真正停止
}

// stopWait 取消运行中的任务并等待其真正退出（Windows 下句柄占用会导致删除失败）。
func (t *Task) stopWait(timeout time.Duration) {
	t.mu.Lock()
	if t.cancel != nil {
		t.paused = false
		t.cancel()
	}
	ch := t.runDone
	t.mu.Unlock()

	if ch == nil {
		return
	}
	select {
	case <-ch:
	case <-time.After(timeout):
	}
}

// ---- Task 内部方法 ----

func (t *Task) view() TaskView {
	t.mu.Lock()
	defer t.mu.Unlock()
	return TaskView{
		ID:         t.ID,
		URLs:       t.opts.URLs,
		Label:      t.opts.Label,
		Dir:        t.opts.Dir,
		ScriptName: t.opts.ScriptName,
		Status:     t.status,
		Error:      t.errMsg,
		Total:      t.total,
		Finished:   t.finished,
		Failed:     t.failed,
		FileCount:  len(t.files),
		CreatedAt:  t.CreatedAt,
	}
}

func (t *Task) info() scriptapi.TaskInfo {
	v := t.view()
	return scriptapi.TaskInfo{
		ID:       v.ID,
		Dir:      v.Dir,
		Total:    v.Total,
		Finished: v.Finished,
		Failed:   v.Failed,
		Status:   v.Status,
	}
}

func (t *Task) emitUpdate() {
	t.persistState()
	t.mgr.deps.Emitter.Emit(events.Task, t.view())
}

func (t *Task) emitFile(ev FileEvent) {
	t.mgr.deps.Emitter.Emit(events.TaskFile, ev)
}

// ---- 文件记录维护（由 progress 回调，行级写 store） ----

// addFile 登记文件（同路径覆盖，断点重跑时天然去重）。
func (t *Task) addFile(f TaskFile) {
	t.mu.Lock()
	replaced := false
	for i := range t.files {
		if t.files[i].Path == f.Path {
			t.files[i] = f
			replaced = true
			break
		}
	}
	if !replaced {
		t.files = append(t.files, f)
	}
	t.mu.Unlock()
	if t.mgr.deps.Store != nil {
		// 文件记录写回可能发生在暂停/取消清理阶段，不得随任务 ctx 中断（STO-03）。
		if err := t.mgr.deps.Store.UpsertFile(context.Background(), store.File{
			TaskID: t.ID, Name: f.Name, Path: f.Path, Size: f.Size, State: f.State,
			DialogID: f.DialogID, MessageID: f.MessageID,
		}); err != nil {
			logStoreErr(err, "写入文件记录失败: id=%s err=%v", t.ID, err)
		}
	}
}

// finishFile 把 oldPath（.tmp）条目替换为最终文件记录；
// 若最终路径已存在旧条目（历史运行留下）先移除，避免重复。
func (t *Task) finishFile(oldPath string, f TaskFile) {
	t.mu.Lock()
	kept := t.files[:0]
	for _, x := range t.files {
		if x.Path == f.Path && x.Path != oldPath {
			continue
		}
		kept = append(kept, x)
	}
	t.files = kept
	replaced := false
	for i := range t.files {
		if t.files[i].Path == oldPath {
			t.files[i] = f
			replaced = true
			break
		}
	}
	if !replaced {
		t.files = append(t.files, f)
	}
	t.mu.Unlock()
	if t.mgr.deps.Store != nil {
		if err := t.mgr.deps.Store.FinishFile(context.Background(), t.ID, oldPath, store.File{
			TaskID: t.ID, Name: f.Name, Path: f.Path, Size: f.Size, State: f.State,
			DialogID: f.DialogID, MessageID: f.MessageID,
		}); err != nil {
			logStoreErr(err, "完成文件记录失败: id=%s err=%v", t.ID, err)
		}
	}
}

// markFileFailed 标记文件失败。
func (t *Task) markFileFailed(path string) {
	t.mu.Lock()
	for i := range t.files {
		if t.files[i].Path == path {
			t.files[i].State = "failed"
			break
		}
	}
	t.mu.Unlock()
	if t.mgr.deps.Store != nil {
		if err := t.mgr.deps.Store.MarkFileFailed(context.Background(), t.ID, path); err != nil {
			logStoreErr(err, "标记文件失败出错: id=%s err=%v", t.ID, err)
		}
	}
}

// dropFile 移除文件条目（临时文件已被清理）。
func (t *Task) dropFile(path string) {
	t.mu.Lock()
	kept := t.files[:0]
	for _, x := range t.files {
		if x.Path != path {
			kept = append(kept, x)
		}
	}
	t.files = kept
	t.mu.Unlock()
	if t.mgr.deps.Store != nil {
		if err := t.mgr.deps.Store.DropFile(context.Background(), t.ID, path); err != nil {
			logStoreErr(err, "移除文件记录失败: id=%s err=%v", t.ID, err)
		}
	}
}

// onFileDone 由 progress 回调：更新计数并触发脚本钩子。
func (t *Task) onFileDone(info scriptapi.FileInfo) {
	t.mu.Lock()
	t.finished++
	t.mu.Unlock()
	t.emitUpdate()

	if err := t.contracts.SafeOnFileDone(info); err != nil {
		t.mgr.deps.Emitter.Emit(events.ScriptLog,
			fmt.Sprintf("[%s] OnFileDone 钩子出错: %v", t.ID, err))
	}
}

func (t *Task) onFileFailed() {
	t.mu.Lock()
	t.failed++
	t.mu.Unlock()
	t.emitUpdate()
}
