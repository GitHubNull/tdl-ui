package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/go-faster/errors"

	"github.com/iyear/tdl/pkg/kv"

	"tdl-ui/internal/config"
	"tdl-ui/internal/events"
	"tdl-ui/internal/script"
	"tdl-ui/internal/store"
)

// TaskRepo 任务/消息项/文件/断点的持久化窄接口，按 engine 实际调用集合裁剪（ARC-004）。
// *store.Store 天然满足；单测可注入内存 fake，无需真实 SQLite 文件。
type TaskRepo interface {
	// 任务
	InsertTask(ctx context.Context, t store.Task, items []store.Item) error
	UpdateTaskStatus(ctx context.Context, id, status, errMsg string) error
	UpdateTaskState(ctx context.Context, id, status, errMsg string, total, finished, failed int) error
	DeleteTask(ctx context.Context, id string) error
	LoadAllTasks(ctx context.Context) ([]store.Task, error)
	ListItems(ctx context.Context, taskID string) ([]store.Item, error)
	// 文件
	UpsertFile(ctx context.Context, f store.File) error
	FinishFile(ctx context.Context, taskID, oldPath string, f store.File) error
	MarkFileFailed(ctx context.Context, taskID, path string) error
	DropFile(ctx context.Context, taskID, path string) error
	ListFiles(ctx context.Context, taskID string) ([]store.File, error)
	DeleteFilesByPath(ctx context.Context, taskID string, paths []string) error
	ListDoneFilesByDialog(ctx context.Context, dialogID int64) ([]store.File, error)
	GetDoneFile(ctx context.Context, dialogID int64, messageID int) (store.File, bool, error)
	// 断点
	LoadFinished(ctx context.Context, taskID string) (map[string]struct{}, error)
	AddFinished(ctx context.Context, taskID, key string) error
	SaveFinished(ctx context.Context, taskID string, finished map[string]struct{}) error
	DeleteResume(ctx context.Context, taskID string) error
}

// Deps 任务管理器依赖。
type Deps struct {
	Cfg     *config.Manager
	KV      kv.Storage
	Emitter *events.Emitter
	Scripts *script.Store
	Store   TaskRepo // 任务/消息项/文件/断点持久化（单测可为 nil 或内存 fake，ARC-004）
}

// Manager 任务管理器：维护任务列表并驱动执行。
type Manager struct {
	deps Deps

	// executor 单次任务执行函数，可注入以便测试；nil 时使用真实下载执行（execute）
	executor func(ctx context.Context, t *Task) error

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

// newTaskID 生成任务 ID：时间戳保证可读性与大致有序，随机后缀避免
// Windows 粗粒度时钟下连建任务的碰撞（ENG-11）。
func newTaskID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("t%d-%s", time.Now().UnixNano(), hex.EncodeToString(b))
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

	// 提前加载脚本，配置错误在创建时即反馈；同时快照源码，
	// 恢复时用快照而非按名重载，避免脚本被编辑后语义漂移（SCR-04）
	var contracts *script.Contracts
	var scriptSrc string
	if opts.ScriptName != "" {
		src, err := m.deps.Scripts.Read(opts.ScriptName)
		if err != nil {
			return "", errors.Wrapf(err, "读取脚本 %q 失败", opts.ScriptName)
		}
		c, err := script.Load(src)
		if err != nil {
			return "", errors.Wrapf(err, "加载脚本 %q 失败", opts.ScriptName)
		}
		contracts = c
		scriptSrc = src
	}

	t := &Task{
		ID:        newTaskID(),
		CreatedAt: time.Now().Format("2006-01-02 15:04:05"),
		opts:      opts,
		contracts: contracts,
		scriptSrc: scriptSrc,
		mgr:       m,
		status:    StatusQueued,
	}

	if m.deps.Store != nil {
		// Manager 公开 API 无调用方 ctx，持久化统一用 Background（STO-03）。
		if err := m.deps.Store.InsertTask(context.Background(), toStoreTask(t), optionsToItems(opts)); err != nil {
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

// Pause 暂停任务（进度保留，可恢复）。排队中的任务直接落为暂停，
// run 启动时检测到非排队态会放弃执行。
func (m *Manager) Pause(id string) error {
	t, err := m.get(id)
	if err != nil {
		return err
	}

	t.mu.Lock()
	switch {
	case t.status == StatusRunning && t.cancel != nil:
		t.paused = true
		t.cancel()
		t.mu.Unlock()
		logEngine.Infof("任务暂停中: id=%s", id)
		return nil
	case t.status == StatusQueued:
		t.status = StatusPaused
		t.mu.Unlock()
		logEngine.Infof("排队任务已暂停: id=%s", id)
		t.emitUpdate()
		return nil
	default:
		t.mu.Unlock()
		return errors.New("任务未在运行中")
	}
}

// Resume 恢复已暂停/失败/已取消的任务（断点续传）。
func (m *Manager) Resume(id string) error {
	t, err := m.get(id)
	if err != nil {
		return err
	}

	t.mu.Lock()
	if t.removed { // 已被并发 Remove 移除，不得复活
		t.mu.Unlock()
		return errors.Errorf("任务不存在: %s", id)
	}
	switch t.status {
	case StatusPaused, StatusFailed, StatusCanceled:
	default:
		t.mu.Unlock()
		return errors.New("仅暂停/失败/已取消的任务可恢复")
	}
	// 重启后恢复的任务未携带脚本契约，按需补加载：
	// 优先用创建时的源码快照，无快照（旧记录）才按名重载（SCR-04）
	if t.contracts == nil && t.opts.ScriptName != "" {
		var c *script.Contracts
		var err error
		if t.scriptSrc != "" {
			c, err = script.Load(t.scriptSrc)
		} else {
			c, err = m.deps.Scripts.LoadByName(t.opts.ScriptName)
		}
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

// Cancel 取消任务（已下载进度保留，可通过恢复继续）。排队中的任务直接落为已取消。
func (m *Manager) Cancel(id string) error {
	t, err := m.get(id)
	if err != nil {
		return err
	}

	t.mu.Lock()
	switch {
	case t.status == StatusRunning && t.cancel != nil:
		t.paused = false
		t.cancel()
		t.mu.Unlock()
		logEngine.Infof("任务取消中: id=%s", id)
		return nil
	case t.status == StatusQueued:
		t.status = StatusCanceled
		t.mu.Unlock()
		logEngine.Infof("排队任务已取消: id=%s", id)
		t.emitUpdate()
		return nil
	default:
		t.mu.Unlock()
		return errors.New("任务未在运行中")
	}
}

// Remove 从列表中移除处于终态的任务。
func (m *Manager) Remove(id string) error {
	t, err := m.get(id)
	if err != nil {
		return err
	}

	t.mu.Lock()
	running := t.status == StatusRunning || t.status == StatusQueued
	if !running {
		// 与状态判定同一临界区标记已删除，阻断并发 Resume 复活僵尸下载（ENG-03）
		t.removed = true
	}
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
		if err := m.deps.Store.DeleteTask(context.Background(), id); err != nil {
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

// DownloadedFiles 返回对话内全部已完成的文件记录（跨任务，"已下载"标记数据源）。
func (m *Manager) DownloadedFiles(dialogID int64) ([]store.File, error) {
	if m.deps.Store == nil {
		return nil, nil
	}
	return m.deps.Store.ListDoneFilesByDialog(context.Background(), dialogID)
}

// DownloadedFile 返回指定消息最新的已完成文件记录；未下载过时第二返回值为 false。
func (m *Manager) DownloadedFile(dialogID int64, messageID int) (store.File, bool, error) {
	if m.deps.Store == nil {
		return store.File{}, false, nil
	}
	return m.deps.Store.GetDoneFile(context.Background(), dialogID, messageID)
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
			if err := m.deps.Store.DeleteTask(context.Background(), id); err != nil {
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
			_ = m.deps.Store.DeleteTask(context.Background(), id)
		} else if len(deleted) > 0 {
			_ = m.deps.Store.DeleteFilesByPath(context.Background(), id, deleted)
		}
	}
	return firstErr
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
			_ = m.deps.Store.DeleteTask(context.Background(), t.ID)
		}
	}
	return firstErr
}

// HasActive 是否存在运行中或排队中的任务（重新登录前的安全检查用）。
func (m *Manager) HasActive() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.tasks {
		t.mu.Lock()
		active := t.status == StatusRunning || t.status == StatusQueued
		t.mu.Unlock()
		if active {
			return true
		}
	}
	return false
}

// StopAndWait 以“暂停”语义停止全部运行中的任务并等待其 goroutine 退出（应用关闭编排用）。
// 断点由 progress 实时持久化，任务落库为 paused，下次启动可断点续传。
func (m *Manager) StopAndWait(timeout time.Duration) {
	m.mu.Lock()
	snapshot := make([]*Task, 0, len(m.order))
	for _, id := range m.order {
		snapshot = append(snapshot, m.tasks[id])
	}
	m.mu.Unlock()

	waits := make([]chan struct{}, 0, len(snapshot))
	for _, t := range snapshot {
		t.mu.Lock()
		if t.cancel != nil {
			t.paused = true // 关闭视同暂停：进度保留，重启后恢复为已暂停
			t.cancel()
		} else if t.status == StatusQueued {
			t.status = StatusPaused // 阻断尚未启动的 run
		}
		if t.runDone != nil {
			waits = append(waits, t.runDone)
		}
		t.mu.Unlock()
	}
	if len(waits) == 0 {
		return
	}

	deadline := time.After(timeout)
	for _, ch := range waits {
		select {
		case <-ch:
		case <-deadline:
			logEngine.Warnf("等待下载任务退出超时（%s），继续关闭流程", timeout)
			return
		}
	}
	logEngine.Infof("全部下载任务已停止")
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
