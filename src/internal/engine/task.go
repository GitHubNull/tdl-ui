package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/go-faster/errors"
	"github.com/gotd/td/telegram/peers"

	"github.com/iyear/tdl/core/dcpool"
	"github.com/iyear/tdl/core/downloader"
	"github.com/iyear/tdl/core/storage"
	coretclient "github.com/iyear/tdl/core/tclient"
	"github.com/iyear/tdl/pkg/key"
	"github.com/iyear/tdl/pkg/kv"
	pkgtclient "github.com/iyear/tdl/pkg/tclient"
	"github.com/iyear/tdl/pkg/tmessage"

	"tdl-ui/internal/config"
	"tdl-ui/internal/events"
	"tdl-ui/internal/script"
	"tdl-ui/internal/scriptapi"
)

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
	CreatedAt  string   `json:"createdAt"`
}

// Task 一次下载任务。
type Task struct {
	ID        string
	CreatedAt string

	opts      TaskOptions
	contracts *script.Contracts
	mgr       *Manager

	mu       sync.Mutex
	status   string
	errMsg   string
	total    int
	finished int
	failed   int
	paused   bool // 取消是否由“暂停”触发
	cancel   context.CancelFunc
}

// Deps 任务管理器依赖。
type Deps struct {
	Cfg     *config.Manager
	KV      kv.Storage
	Emitter *events.Emitter
	Scripts *script.Store
}

// Manager 任务管理器：维护任务列表并驱动执行。
type Manager struct {
	deps Deps

	mu    sync.Mutex
	tasks map[string]*Task
	order []string // 创建顺序
}

// NewManager 创建任务管理器。
func NewManager(deps Deps) *Manager {
	return &Manager{
		deps:  deps,
		tasks: make(map[string]*Task),
	}
}

// Create 创建并启动下载任务，返回任务 ID。
func (m *Manager) Create(opts TaskOptions) (string, error) {
	if len(opts.URLs) == 0 && len(opts.Selections) == 0 {
		return "", errors.New("至少需要一条消息链接或一个选集")
	}
	if len(opts.URLs) > 0 {
		urls, err := NormalizeURLs(opts.URLs)
		if err != nil {
			return "", err
		}
		opts.URLs = urls
	}
	if err := validateSelections(opts.Selections); err != nil {
		return "", err
	}
	if opts.Dir == "" {
		opts.Dir = m.deps.Cfg.Get().DownloadDir
	}
	if opts.Template == "" {
		opts.Template = m.deps.Cfg.Get().Template
	}
	if err := os.MkdirAll(opts.Dir, 0o755); err != nil {
		return "", errors.Wrap(err, "创建下载目录失败")
	}

	// 提前加载脚本，配置错误在创建时即反馈
	var contracts *script.Contracts
	if opts.ScriptName != "" {
		c, err := m.deps.Scripts.LoadByName(opts.ScriptName)
		if err != nil {
			return "", errors.Wrapf(err, "加载脚本 %q 失败", opts.ScriptName)
		}
		contracts = c
	}

	t := &Task{
		ID:        fmt.Sprintf("t%d", time.Now().UnixNano()),
		CreatedAt: time.Now().Format("2006-01-02 15:04:05"),
		opts:      opts,
		contracts: contracts,
		mgr:       m,
		status:    StatusQueued,
	}

	m.mu.Lock()
	m.tasks[t.ID] = t
	m.order = append(m.order, t.ID)
	m.mu.Unlock()

	t.emitUpdate()
	go m.run(t)

	return t.ID, nil
}

// List 按创建时间倒序返回任务视图。
func (m *Manager) List() []TaskView {
	m.mu.Lock()
	defer m.mu.Unlock()

	views := make([]TaskView, 0, len(m.order))
	for i := len(m.order) - 1; i >= 0; i-- {
		views = append(views, m.tasks[m.order[i]].view())
	}
	return views
}

// Pause 暂停任务（进度保留，可恢复）。
func (m *Manager) Pause(id string) error {
	t, err := m.get(id)
	if err != nil {
		return err
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	if t.status != StatusRunning || t.cancel == nil {
		return errors.New("任务未在运行中")
	}
	t.paused = true
	t.cancel()
	return nil
}

// Resume 恢复已暂停/失败/已取消的任务（断点续传）。
func (m *Manager) Resume(id string) error {
	t, err := m.get(id)
	if err != nil {
		return err
	}

	t.mu.Lock()
	switch t.status {
	case StatusPaused, StatusFailed, StatusCanceled:
	default:
		t.mu.Unlock()
		return errors.New("仅暂停/失败/已取消的任务可恢复")
	}
	t.status = StatusQueued
	t.errMsg = ""
	t.failed = 0
	t.paused = false
	t.opts.Restart = false // 恢复必然走断点续传
	t.mu.Unlock()

	t.emitUpdate()
	go m.run(t)
	return nil
}

// Cancel 取消任务（已下载进度保留，可通过恢复继续）。
func (m *Manager) Cancel(id string) error {
	t, err := m.get(id)
	if err != nil {
		return err
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	if t.status != StatusRunning || t.cancel == nil {
		return errors.New("任务未在运行中")
	}
	t.paused = false
	t.cancel()
	return nil
}

// Remove 从列表中移除处于终态的任务。
func (m *Manager) Remove(id string) error {
	t, err := m.get(id)
	if err != nil {
		return err
	}

	t.mu.Lock()
	running := t.status == StatusRunning || t.status == StatusQueued
	t.mu.Unlock()
	if running {
		return errors.New("请先暂停或取消运行中的任务")
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.tasks, id)
	for i, oid := range m.order {
		if oid == id {
			m.order = append(m.order[:i], m.order[i+1:]...)
			break
		}
	}
	return nil
}

func (m *Manager) get(id string) (*Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[id]
	if !ok {
		return nil, errors.Errorf("任务不存在: %s", id)
	}
	return t, nil
}

// run 驱动任务执行并根据结果迁移状态。
func (m *Manager) run(t *Task) {
	ctx, cancel := context.WithCancel(context.Background())

	t.mu.Lock()
	t.cancel = cancel
	t.status = StatusRunning
	t.mu.Unlock()
	t.emitUpdate()

	_ = t.contracts.SafeOnTaskStart(t.info())

	err := m.execute(ctx, t)
	cancel()

	t.mu.Lock()
	t.cancel = nil
	switch {
	case err == nil:
		t.status = StatusDone
	case errors.Is(err, context.Canceled):
		if t.paused {
			t.status = StatusPaused
		} else {
			t.status = StatusCanceled
		}
	default:
		t.status = StatusFailed
		t.errMsg = err.Error()
	}
	t.mu.Unlock()

	t.emitUpdate()
	_ = t.contracts.SafeOnTaskDone(t.info())
}

// execute 单次任务执行：建连 → 解析链接 → 装配迭代器 → 断点续传 → 下载。
// 装配流程对应 ref/tdl/app/dl/dl.go 的 Run。
func (m *Manager) execute(ctx context.Context, t *Task) (rerr error) {
	settings := m.deps.Cfg.Get()

	kvd, err := m.deps.KV.Open(Namespace)
	if err != nil {
		return errors.Wrap(err, "open kv")
	}

	c, err := pkgtclient.New(ctx, pkgtclient.Options{
		KV:               kvd,
		Proxy:            settings.Proxy,
		ReconnectTimeout: reconnectTimeout,
	}, false)
	if err != nil {
		return errors.Wrap(err, "create client")
	}

	return coretclient.RunWithAuth(ctx, c, func(ctx context.Context) error {
		pool := dcpool.NewPool(c,
			int64(settings.PoolSize),
			coretclient.NewDefaultMiddlewares(ctx, reconnectTimeout)...)
		defer func() { _ = pool.Close() }()

		var dialogs []*tmessage.Dialog
		if len(t.opts.URLs) > 0 {
			d, err := tmessage.Parse(tmessage.FromURL(ctx, pool, kvd, t.opts.URLs))
			if err != nil {
				return errors.Wrap(err, "解析消息链接失败")
			}
			dialogs = d
		}

		manager := peers.Options{Storage: storage.NewPeers(kvd)}.Build(pool.Default(ctx))

		// 选集下载：按对话 + 消息 ID 直接解析，与链接结果合并
		if len(t.opts.Selections) > 0 {
			d, err := selectionsToDialogs(ctx, manager, t.opts.Selections)
			if err != nil {
				return err
			}
			dialogs = append(dialogs, d...)
		}

		it, err := newIter(pool, manager, dialogs, IterOptions{
			Dir:        t.opts.Dir,
			RewriteExt: t.opts.RewriteExt,
			SkipSame:   t.opts.SkipSame,
			Template:   t.opts.Template,
			Group:      t.opts.Group,
			Contracts:  t.contracts,
			OnSkip: func(info scriptapi.FileInfo, reason string) {
				m.deps.Emitter.Emit(events.ScriptLog,
					fmt.Sprintf("[%s] %s: %s", t.ID, info.FileName, reason))
			},
		})
		if err != nil {
			return err
		}

		t.mu.Lock()
		t.total = it.Total()
		t.mu.Unlock()

		// 断点续传：加载已完成集合；Restart 则清空进度
		if t.opts.Restart {
			_ = kvd.Delete(ctx, key.Resume(it.Fingerprint()))
		} else if err = loadResume(ctx, kvd, it); err != nil {
			return err
		}

		t.mu.Lock()
		t.finished = len(it.Finished())
		t.mu.Unlock()
		t.emitUpdate()

		defer func() { // 保存或清理断点
			if rerr != nil {
				saveErr := saveResume(ctx, kvd, it)
				if saveErr != nil {
					rerr = errors.Wrapf(rerr, "save resume: %v", saveErr)
				}
			} else {
				_ = kvd.Delete(ctx, key.Resume(it.Fingerprint()))
			}
		}()

		return downloader.New(downloader.Options{
			Pool:     pool,
			Threads:  settings.Threads,
			Iter:     it,
			Progress: newProgress(t, it, t.opts.RewriteExt),
		}).Download(ctx, settings.Limit)
	})
}

// loadResume 读取断点数据（对应 ref/tdl/app/dl/dl.go 的 resume，GUI 下无需询问，直接续传）。
func loadResume(ctx context.Context, kvd storage.Storage, it *iter) error {
	b, err := kvd.Get(ctx, key.Resume(it.Fingerprint()))
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return err
	}
	if len(b) == 0 {
		return nil
	}

	finished := make(map[int]struct{})
	if err = json.Unmarshal(b, &finished); err != nil {
		return err
	}
	if len(finished) == 0 {
		return nil
	}

	it.SetFinished(finished)
	return nil
}

func saveResume(ctx context.Context, kvd storage.Storage, it *iter) error {
	b, err := json.Marshal(it.Finished())
	if err != nil {
		return err
	}
	return kvd.Set(ctx, key.Resume(it.Fingerprint()), b)
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
	t.mgr.deps.Emitter.Emit(events.Task, t.view())
}

func (t *Task) emitFile(ev FileEvent) {
	t.mgr.deps.Emitter.Emit(events.TaskFile, ev)
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
