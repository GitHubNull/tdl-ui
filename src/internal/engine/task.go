package engine

import (
	"context"
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
	"github.com/iyear/tdl/pkg/kv"
	pkgtclient "github.com/iyear/tdl/pkg/tclient"
	"github.com/iyear/tdl/pkg/tmessage"

	"tdl-ui/internal/config"
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
	mgr       *Manager

	mu       sync.Mutex
	status   string
	errMsg   string
	total    int
	finished int
	failed   int
	paused   bool // 取消是否由“暂停”触发
	cancel   context.CancelFunc
	files    []TaskFile    // 任务内文件记录（含 .tmp 未完成文件）
	runDone  chan struct{} // 本次 run 结束时关闭，供删除文件前等待任务真正停止
}

// Deps 任务管理器依赖。
type Deps struct {
	Cfg     *config.Manager
	KV      kv.Storage
	Emitter *events.Emitter
	Scripts *script.Store
	Store   *store.Store // 任务/消息项/文件/断点的 SQLite 持久化（单测可为 nil）
}

// Manager 任务管理器：维护任务列表并驱动执行。
type Manager struct {
	deps Deps

	mu    sync.Mutex
	tasks map[string]*Task
	order []string // 创建顺序
}

// NewManager 创建任务管理器，并从 SQLite 恢复历史任务记录。
func NewManager(deps Deps) *Manager {
	m := &Manager{
		deps:  deps,
		tasks: make(map[string]*Task),
	}
	m.loadFromStore()
	return m
}

// loadFromStore 从 SQLite 恢复任务；上次退出时仍在运行/排队的任务恢复为已暂停（可断点续传）。
func (m *Manager) loadFromStore() {
	if m.deps.Store == nil { // 单测可能不提供 store
		return
	}
	tasks, err := m.deps.Store.LoadAllTasks()
	if err != nil {
		logEngine.Errorf("加载任务记录失败: %v", err)
		return
	}
	for _, st := range tasks {
		status := st.Status
		if status == StatusRunning || status == StatusQueued {
			status = StatusPaused
			_ = m.deps.Store.UpdateTaskStatus(st.ID, status, st.Error)
		}
		items, err := m.deps.Store.ListItems(st.ID)
		if err != nil {
			logEngine.Errorf("加载任务 %s 消息项失败: %v", st.ID, err)
			continue
		}
		urls, selections := itemsToOptions(items)
		files, err := m.deps.Store.ListFiles(st.ID)
		if err != nil {
			logEngine.Errorf("加载任务 %s 文件失败: %v", st.ID, err)
		}
		t := &Task{
			ID:        st.ID,
			CreatedAt: st.CreatedAt,
			opts: TaskOptions{
				URLs:       urls,
				Selections: selections,
				Label:      st.Label,
				Dir:        st.Dir,
				ScriptName: st.ScriptName,
				Template:   st.Template,
				RewriteExt: st.RewriteExt,
				SkipSame:   st.SkipSame,
				Group:      st.GroupMedia,
			},
			mgr:      m,
			status:   status,
			errMsg:   st.Error,
			total:    st.Total,
			finished: st.Finished,
			failed:   st.Failed,
			files:    storeFilesToTaskFiles(files),
		}
		m.tasks[t.ID] = t
		m.order = append(m.order, t.ID)
	}
}

// ---- store 组装辅助 ----

// itemsToOptions 把消息项行还原为 URLs / Selections。
func itemsToOptions(items []store.Item) ([]string, []Selection) {
	var urls []string
	var sels []Selection
	for _, it := range items {
		switch it.ItemType {
		case store.ItemTypeURL:
			if it.URL != "" {
				urls = append(urls, it.URL)
			}
		case store.ItemTypeSelection:
			sels = append(sels, Selection{
				DialogID:   it.DialogID,
				DialogType: it.DialogType,
				MessageIDs: it.MessageIDs,
			})
		}
	}
	return urls, sels
}

// optionsToItems 把任务选项拆成消息项行（用于插入）。
func optionsToItems(opts TaskOptions) []store.Item {
	items := make([]store.Item, 0, len(opts.URLs)+len(opts.Selections))
	for _, u := range opts.URLs {
		items = append(items, store.Item{ItemType: store.ItemTypeURL, URL: u})
	}
	for _, sel := range opts.Selections {
		items = append(items, store.Item{
			ItemType:   store.ItemTypeSelection,
			DialogID:   sel.DialogID,
			DialogType: sel.DialogType,
			MessageIDs: sel.MessageIDs,
		})
	}
	return items
}

// toStoreTask 把任务主字段转为 store.Task 行。
func toStoreTask(t *Task) store.Task {
	return store.Task{
		ID:         t.ID,
		Label:      t.opts.Label,
		Dir:        t.opts.Dir,
		ScriptName: t.opts.ScriptName,
		Template:   t.opts.Template,
		RewriteExt: t.opts.RewriteExt,
		SkipSame:   t.opts.SkipSame,
		GroupMedia: t.opts.Group,
		Status:     t.status,
		Error:      t.errMsg,
		Total:      t.total,
		Finished:   t.finished,
		Failed:     t.failed,
		CreatedAt:  t.CreatedAt,
	}
}

// storeFilesToTaskFiles 把文件表行转为内存文件记录。
func storeFilesToTaskFiles(files []store.File) []TaskFile {
	out := make([]TaskFile, 0, len(files))
	for _, f := range files {
		out = append(out, TaskFile{Name: f.Name, Path: f.Path, Size: f.Size, State: f.State})
	}
	return out
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

	if m.deps.Store != nil {
		if err := m.deps.Store.InsertTask(toStoreTask(t), optionsToItems(opts)); err != nil {
			return "", errors.Wrap(err, "保存任务失败")
		}
	}

	m.mu.Lock()
	m.tasks[t.ID] = t
	m.order = append(m.order, t.ID)
	m.mu.Unlock()

	logEngine.Infof("任务已创建: id=%s 链接数=%d 选集数=%d 目录=%s", t.ID, len(opts.URLs), len(opts.Selections), opts.Dir)
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
	logEngine.Infof("任务暂停中: id=%s", id)
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
	// 重启后恢复的任务未携带脚本契约，按需补加载
	if t.contracts == nil && t.opts.ScriptName != "" {
		c, err := m.deps.Scripts.LoadByName(t.opts.ScriptName)
		if err != nil {
			t.mu.Unlock()
			return errors.Wrapf(err, "加载脚本 %q 失败", t.opts.ScriptName)
		}
		t.contracts = c
	}
	t.status = StatusQueued
	t.errMsg = ""
	t.failed = 0
	t.paused = false
	t.opts.Restart = false // 恢复必然走断点续传
	t.mu.Unlock()

	logEngine.Infof("任务恢复: id=%s", id)
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
	logEngine.Infof("任务取消中: id=%s", id)
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
	delete(m.tasks, id)
	for i, oid := range m.order {
		if oid == id {
			m.order = append(m.order[:i], m.order[i+1:]...)
			break
		}
	}
	m.mu.Unlock()
	if m.deps.Store != nil {
		if err := m.deps.Store.DeleteTask(id); err != nil {
			logEngine.Errorf("删除任务记录失败: id=%s err=%v", id, err)
		}
	}
	return nil
}

// Files 返回任务的文件记录副本。
func (m *Manager) Files(id string) ([]TaskFile, error) {
	t, err := m.get(id)
	if err != nil {
		return nil, err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]TaskFile(nil), t.files...), nil
}

// TaskDir 返回任务的保存目录。
func (m *Manager) TaskDir(id string) (string, error) {
	t, err := m.get(id)
	if err != nil {
		return "", err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.opts.Dir, nil
}

// ClearFinished 移除全部已完成状态的任务记录（不动磁盘文件）。
func (m *Manager) ClearFinished() {
	m.mu.Lock()
	var removed []string
	kept := m.order[:0]
	for _, id := range m.order {
		t := m.tasks[id]
		t.mu.Lock()
		done := t.status == StatusDone
		t.mu.Unlock()
		if done {
			delete(m.tasks, id)
			removed = append(removed, id)
		} else {
			kept = append(kept, id)
		}
	}
	m.order = kept
	m.mu.Unlock()
	if m.deps.Store != nil {
		for _, id := range removed {
			if err := m.deps.Store.DeleteTask(id); err != nil {
				logEngine.Errorf("清理任务记录失败: id=%s err=%v", id, err)
			}
		}
	}
}

// DeleteFiles 删除任务内指定文件（paths 必须严格匹配记录内登记的路径，
// 防止删除任意路径）；文件全部删光后整条任务记录一并移除。
func (m *Manager) DeleteFiles(id string, paths []string) error {
	t, err := m.get(id)
	if err != nil {
		return err
	}

	t.mu.Lock()
	if t.status == StatusRunning || t.status == StatusQueued {
		t.mu.Unlock()
		return errors.New("请先暂停或取消运行中的任务")
	}
	registered := make(map[string]struct{}, len(t.files))
	for _, f := range t.files {
		registered[f.Path] = struct{}{}
	}
	for _, p := range paths {
		if _, ok := registered[p]; !ok {
			t.mu.Unlock()
			return errors.Errorf("文件不属于该任务: %s", p)
		}
	}

	toDelete := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		toDelete[p] = struct{}{}
	}
	var firstErr error
	var deleted []string
	kept := t.files[:0]
	for _, f := range t.files {
		if _, ok := toDelete[f.Path]; !ok {
			kept = append(kept, f)
			continue
		}
		if err := os.Remove(f.Path); err != nil && !os.IsNotExist(err) {
			if firstErr == nil {
				firstErr = errors.Wrapf(err, "删除 %s 失败", f.Name)
			}
			kept = append(kept, f) // 删除失败的文件保留记录
			continue
		}
		deleted = append(deleted, f.Path)
	}
	t.files = kept
	empty := len(t.files) == 0
	t.mu.Unlock()

	if empty {
		m.mu.Lock()
		delete(m.tasks, id)
		for i, oid := range m.order {
			if oid == id {
				m.order = append(m.order[:i], m.order[i+1:]...)
				break
			}
		}
		m.mu.Unlock()
	}
	if m.deps.Store != nil {
		if empty {
			_ = m.deps.Store.DeleteTask(id)
		} else if len(deleted) > 0 {
			_ = m.deps.Store.DeleteFilesByPath(id, deleted)
		}
	}
	return firstErr
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

// DeleteAllFiles 停止全部任务，删除所有登记文件（含 .tmp 未完成文件），并清空全部记录。
func (m *Manager) DeleteAllFiles() error {
	m.mu.Lock()
	snapshot := make([]*Task, 0, len(m.order))
	for _, id := range m.order {
		snapshot = append(snapshot, m.tasks[id])
	}
	// 先清空登记，迟到的 run() 会因任务不存在而直接退出
	m.tasks = make(map[string]*Task)
	m.order = nil
	m.mu.Unlock()

	for _, t := range snapshot {
		t.stopWait(10 * time.Second)
	}

	var firstErr error
	for _, t := range snapshot {
		t.mu.Lock()
		files := append([]TaskFile(nil), t.files...)
		t.files = nil
		t.mu.Unlock()
		for _, f := range files {
			if err := os.Remove(f.Path); err != nil && !os.IsNotExist(err) && firstErr == nil {
				firstErr = errors.Wrapf(err, "删除 %s 失败", f.Name)
			}
		}
	}

	if m.deps.Store != nil {
		for _, t := range snapshot {
			_ = m.deps.Store.DeleteTask(t.ID)
		}
	}
	return firstErr
}

// exists 任务是否仍在管理器中（删除全部文件时用于阻断迟到的 run）。
func (m *Manager) exists(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.tasks[id]
	return ok
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
	if !m.exists(t.ID) { // 记录已被删除（如删除全部文件），不再执行
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan struct{})

	t.mu.Lock()
	t.cancel = cancel
	t.runDone = runDone
	t.status = StatusRunning
	t.mu.Unlock()
	logEngine.Infof("任务开始运行: id=%s", t.ID)
	t.emitUpdate()

	_ = t.contracts.SafeOnTaskStart(t.info())

	err := m.execute(ctx, t)
	cancel()

	t.mu.Lock()
	t.cancel = nil
	t.runDone = nil
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
	status := t.status
	t.mu.Unlock()
	close(runDone)

	if status == StatusFailed {
		logEngine.Errorf("任务失败: id=%s err=%v", t.ID, err)
	} else {
		logEngine.Infof("任务结束: id=%s 状态=%s", t.ID, status)
	}

	t.emitUpdate()
	_ = t.contracts.SafeOnTaskDone(t.info())
}

// execute 单次任务执行：建连 → 解析链接 → 装配迭代器 → 断点续传 → 下载。
// 装配流程对应 ref/tdl/app/dl/dl.go 的 Run。
func (m *Manager) execute(ctx context.Context, t *Task) (rerr error) {
	settings := m.deps.Cfg.Get()

	// 每轮从 task_items 表重组装 URLs/Selections，与持久化的消息项保持一致。
	urls, selections := t.opts.URLs, t.opts.Selections
	if m.deps.Store != nil {
		items, err := m.deps.Store.ListItems(t.ID)
		if err != nil {
			return errors.Wrap(err, "读取任务消息项失败")
		}
		if len(items) > 0 {
			urls, selections = itemsToOptions(items)
			t.mu.Lock()
			t.opts.URLs = urls
			t.opts.Selections = selections
			t.mu.Unlock()
		}
	}

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
		if len(urls) > 0 {
			d, err := tmessage.Parse(tmessage.FromURL(ctx, pool, kvd, urls))
			if err != nil {
				return errors.Wrap(err, "解析消息链接失败")
			}
			dialogs = d
		}

		manager := peers.Options{Storage: storage.NewPeers(kvd)}.Build(pool.Default(ctx))

		// 选集下载：按对话 + 消息 ID 直接解析，与链接结果合并
		if len(selections) > 0 {
			d, err := selectionsToDialogs(ctx, manager, selections)
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

		// 断点续传：从 resume_points 加载已完成集合（内容坐标）；Restart 则清空进度
		if t.opts.Restart {
			if m.deps.Store != nil {
				_ = m.deps.Store.DeleteResume(t.ID)
			}
		} else if m.deps.Store != nil {
			finished, err := m.deps.Store.LoadFinished(t.ID)
			if err != nil {
				return errors.Wrap(err, "加载断点失败")
			}
			it.SetFinished(finished)
		}

		t.mu.Lock()
		t.finished = len(it.Finished())
		t.mu.Unlock()
		t.emitUpdate()

		defer func() { // 保存或清理断点
			if m.deps.Store == nil {
				return
			}
			if rerr != nil {
				if saveErr := m.deps.Store.SaveFinished(t.ID, it.Finished()); saveErr != nil {
					rerr = errors.Wrapf(rerr, "save resume: %v", saveErr)
				}
			} else {
				_ = m.deps.Store.DeleteResume(t.ID)
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

// saveResume 实时持久化断点集合（由 progress.OnDone 每完成一个文件调用）。
func (t *Task) saveResume(finished map[string]struct{}) {
	if t.mgr.deps.Store == nil {
		return
	}
	if err := t.mgr.deps.Store.SaveFinished(t.ID, finished); err != nil {
		logEngine.Errorf("保存断点失败: id=%s err=%v", t.ID, err)
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

// persistState 行级写回任务状态与计数（替代旧的全量 persist）。
func (t *Task) persistState() {
	if t.mgr.deps.Store == nil {
		return
	}
	t.mu.Lock()
	status, errMsg := t.status, t.errMsg
	total, finished, failed := t.total, t.finished, t.failed
	t.mu.Unlock()
	if err := t.mgr.deps.Store.UpdateTaskStatus(t.ID, status, errMsg); err != nil {
		logEngine.Errorf("更新任务状态失败: id=%s err=%v", t.ID, err)
	}
	if err := t.mgr.deps.Store.UpdateTaskCounts(t.ID, total, finished, failed); err != nil {
		logEngine.Errorf("更新任务计数失败: id=%s err=%v", t.ID, err)
	}
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
		if err := t.mgr.deps.Store.UpsertFile(store.File{
			TaskID: t.ID, Name: f.Name, Path: f.Path, Size: f.Size, State: f.State,
		}); err != nil {
			logEngine.Errorf("写入文件记录失败: id=%s err=%v", t.ID, err)
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
		if err := t.mgr.deps.Store.FinishFile(t.ID, oldPath, store.File{
			TaskID: t.ID, Name: f.Name, Path: f.Path, Size: f.Size, State: f.State,
		}); err != nil {
			logEngine.Errorf("完成文件记录失败: id=%s err=%v", t.ID, err)
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
		if err := t.mgr.deps.Store.MarkFileFailed(t.ID, path); err != nil {
			logEngine.Errorf("标记文件失败出错: id=%s err=%v", t.ID, err)
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
		if err := t.mgr.deps.Store.DropFile(t.ID, path); err != nil {
			logEngine.Errorf("移除文件记录失败: id=%s err=%v", t.ID, err)
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
