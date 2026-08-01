package engine

// TaskRepo 接口化后的可测试性验证（ARC-004）：
// 用纯内存 fake 替代真实 SQLite 文件跑通任务恢复与断点写入路径。

import (
	"context"
	"testing"

	"tdl-ui/internal/events"
	"tdl-ui/internal/store"
)

// memRepo TaskRepo 的最小内存实现（无并发保护，仅供单测串行使用）。
type memRepo struct {
	tasks    map[string]store.Task
	items    map[string][]store.Item
	files    map[string][]store.File
	finished map[string]map[string]struct{}
}

func newMemRepo() *memRepo {
	return &memRepo{
		tasks:    make(map[string]store.Task),
		items:    make(map[string][]store.Item),
		files:    make(map[string][]store.File),
		finished: make(map[string]map[string]struct{}),
	}
}

func (r *memRepo) InsertTask(_ context.Context, t store.Task, items []store.Item) error {
	r.tasks[t.ID] = t
	r.items[t.ID] = items
	return nil
}

func (r *memRepo) UpdateTaskStatus(_ context.Context, id, status, errMsg string) error {
	t := r.tasks[id]
	t.Status, t.Error = status, errMsg
	r.tasks[id] = t
	return nil
}

func (r *memRepo) UpdateTaskState(_ context.Context, id, status, errMsg string, total, finished, failed int) error {
	t := r.tasks[id]
	t.Status, t.Error = status, errMsg
	t.Total, t.Finished, t.Failed = total, finished, failed
	r.tasks[id] = t
	return nil
}

func (r *memRepo) DeleteTask(_ context.Context, id string) error {
	delete(r.tasks, id)
	delete(r.items, id)
	delete(r.files, id)
	delete(r.finished, id)
	return nil
}

func (r *memRepo) LoadAllTasks(_ context.Context) ([]store.Task, error) {
	out := make([]store.Task, 0, len(r.tasks))
	for _, t := range r.tasks {
		out = append(out, t)
	}
	return out, nil
}

func (r *memRepo) ListItems(_ context.Context, taskID string) ([]store.Item, error) {
	return r.items[taskID], nil
}

func (r *memRepo) UpsertFile(_ context.Context, f store.File) error {
	r.files[f.TaskID] = append(r.files[f.TaskID], f)
	return nil
}

func (r *memRepo) FinishFile(_ context.Context, taskID, _ string, f store.File) error {
	r.files[taskID] = append(r.files[taskID], f)
	return nil
}

func (r *memRepo) MarkFileFailed(_ context.Context, _, _ string) error { return nil }
func (r *memRepo) DropFile(_ context.Context, _, _ string) error       { return nil }

func (r *memRepo) ListFiles(_ context.Context, taskID string) ([]store.File, error) {
	return r.files[taskID], nil
}

func (r *memRepo) DeleteFilesByPath(_ context.Context, _ string, _ []string) error { return nil }

func (r *memRepo) ListDoneFilesByDialog(_ context.Context, _ int64) ([]store.File, error) {
	return nil, nil
}

func (r *memRepo) GetDoneFile(_ context.Context, _ int64, _ int) (store.File, bool, error) {
	return store.File{}, false, nil
}

func (r *memRepo) LoadFinished(_ context.Context, taskID string) (map[string]struct{}, error) {
	out := make(map[string]struct{}, len(r.finished[taskID]))
	for k := range r.finished[taskID] {
		out[k] = struct{}{}
	}
	return out, nil
}

func (r *memRepo) AddFinished(_ context.Context, taskID, key string) error {
	if r.finished[taskID] == nil {
		r.finished[taskID] = make(map[string]struct{})
	}
	r.finished[taskID][key] = struct{}{}
	return nil
}

func (r *memRepo) SaveFinished(_ context.Context, taskID string, finished map[string]struct{}) error {
	r.finished[taskID] = finished
	return nil
}

func (r *memRepo) DeleteResume(_ context.Context, taskID string) error {
	delete(r.finished, taskID)
	return nil
}

func (r *memRepo) DeleteResumeKey(_ context.Context, taskID, key string) error {
	if r.finished[taskID] != nil {
		delete(r.finished[taskID], key)
	}
	return nil
}

// 编译期确认 *store.Store 满足 TaskRepo（main.go 装配零改动的保证）。
var _ TaskRepo = (*store.Store)(nil)

// TestManagerWithMemRepo 内存 fake 跑通任务恢复与断点写入，全程不落 SQLite 文件。
func TestManagerWithMemRepo(t *testing.T) {
	repo := newMemRepo()
	repo.tasks["t1"] = store.Task{ID: "t1", Status: StatusRunning, Dir: t.TempDir()}

	// 恢复：running 任务重启后应落为 paused，且状态写回 fake
	m := NewManager(Deps{Emitter: &events.Emitter{}, Store: repo})
	task, err := m.get("t1")
	if err != nil {
		t.Fatalf("恢复任务失败: %v", err)
	}
	if got := taskStatus(task); got != StatusPaused {
		t.Fatalf("重启后 running 任务应为 paused，实际 %s", got)
	}
	if repo.tasks["t1"].Status != StatusPaused {
		t.Fatalf("状态写回 fake 应为 paused，实际 %s", repo.tasks["t1"].Status)
	}

	// 断点写入：saveResumeKey 应进入 fake 并可读回
	task.saveResumeKey("1_2")
	got, err := repo.LoadFinished(context.Background(), "t1")
	if err != nil {
		t.Fatalf("LoadFinished 失败: %v", err)
	}
	if _, ok := got["1_2"]; !ok {
		t.Fatal("断点集合缺少 1_2")
	}
}
