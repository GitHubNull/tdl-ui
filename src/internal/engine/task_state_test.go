package engine

// 状态机转换与断点持久化的单元测试（ENG-05）。
// 通过 Manager.executor 注入打桩执行器，不触碰真实 Telegram 客户端。

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"tdl-ui/internal/events"
	"tdl-ui/internal/store"
)

// newStubManager 返回注入打桩执行器的管理器。
func newStubManager(exec func(ctx context.Context, t *Task) error) *Manager {
	m := NewManager(Deps{Emitter: &events.Emitter{}})
	m.executor = exec
	return m
}

// addQueuedTask 直接登记一个排队任务（绕过 Create 的 URL/目录校验）。
func addQueuedTask(m *Manager, id string) *Task {
	t := &Task{ID: id, mgr: m, status: StatusQueued}
	m.tasks[id] = t
	m.order = append(m.order, id)
	return t
}

func taskStatus(t *Task) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.status
}

// waitStatus 轮询等待任务进入目标状态。
func waitStatus(t *testing.T, task *Task, want string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if taskStatus(task) == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("等待状态 %s 超时，当前 %s", want, taskStatus(task))
}

// 执行成功 → done。
func TestRunTransitionsToDone(t *testing.T) {
	m := newStubManager(func(_ context.Context, _ *Task) error { return nil })
	task := addQueuedTask(m, "t1")
	m.run(task)
	if got := taskStatus(task); got != StatusDone {
		t.Fatalf("预期 done，实际 %s", got)
	}
}

// 执行失败 → failed 且记录错误信息。
func TestRunTransitionsToFailed(t *testing.T) {
	m := newStubManager(func(_ context.Context, _ *Task) error {
		return os.ErrPermission
	})
	task := addQueuedTask(m, "t1")
	m.run(task)
	if got := taskStatus(task); got != StatusFailed {
		t.Fatalf("预期 failed，实际 %s", got)
	}
	task.mu.Lock()
	errMsg := task.errMsg
	task.mu.Unlock()
	if errMsg == "" {
		t.Fatal("failed 状态应携带错误信息")
	}
}

// 运行中 Pause → paused；Resume 后重新执行 → done。
func TestPauseRunningThenResume(t *testing.T) {
	release := make(chan struct{})
	m := newStubManager(func(ctx context.Context, _ *Task) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-release:
			return nil
		}
	})
	task := addQueuedTask(m, "t1")
	go m.run(task)
	waitStatus(t, task, StatusRunning)

	if err := m.Pause("t1"); err != nil {
		t.Fatalf("Pause 失败: %v", err)
	}
	waitStatus(t, task, StatusPaused)

	close(release) // 之后的执行立即成功
	if err := m.Resume("t1"); err != nil {
		t.Fatalf("Resume 失败: %v", err)
	}
	waitStatus(t, task, StatusDone)
}

// 运行中 Cancel → canceled。
func TestCancelRunningTask(t *testing.T) {
	m := newStubManager(func(ctx context.Context, _ *Task) error {
		<-ctx.Done()
		return ctx.Err()
	})
	task := addQueuedTask(m, "t1")
	go m.run(task)
	waitStatus(t, task, StatusRunning)

	if err := m.Cancel("t1"); err != nil {
		t.Fatalf("Cancel 失败: %v", err)
	}
	waitStatus(t, task, StatusCanceled)
}

// 排队态直接 Pause/Cancel（ENG-04）；run 启动时放弃执行。
func TestQueuedDirectTransitions(t *testing.T) {
	executed := false
	m := newStubManager(func(_ context.Context, _ *Task) error {
		executed = true
		return nil
	})

	p := addQueuedTask(m, "tp")
	if err := m.Pause("tp"); err != nil {
		t.Fatalf("排队态 Pause 失败: %v", err)
	}
	if got := taskStatus(p); got != StatusPaused {
		t.Fatalf("预期 paused，实际 %s", got)
	}
	m.run(p) // 迟到的 run 必须放弃执行
	if executed {
		t.Fatal("已暂停的排队任务不应被执行")
	}

	c := addQueuedTask(m, "tc")
	if err := m.Cancel("tc"); err != nil {
		t.Fatalf("排队态 Cancel 失败: %v", err)
	}
	if got := taskStatus(c); got != StatusCanceled {
		t.Fatalf("预期 canceled，实际 %s", got)
	}
}

// Remove 后任务不可 Resume 复活（ENG-03）。
func TestRemoveBlocksResume(t *testing.T) {
	m := newStubManager(func(_ context.Context, _ *Task) error { return nil })
	task := addQueuedTask(m, "t1")
	task.status = StatusPaused

	if err := m.Remove("t1"); err != nil {
		t.Fatalf("Remove 失败: %v", err)
	}
	task.mu.Lock()
	removed := task.removed
	task.mu.Unlock()
	if !removed {
		t.Fatal("Remove 后应标记 removed")
	}
	// 持有旧引用直接调 run（模拟并发窗口）：不得执行
	executed := false
	m.executor = func(_ context.Context, _ *Task) error { executed = true; return nil }
	m.run(task)
	if executed {
		t.Fatal("已移除的任务不应被执行")
	}
}

// 终态限制：done 任务不可 Pause/Cancel；running 任务不可 Resume。
func TestInvalidTransitionsRejected(t *testing.T) {
	m := newStubManager(nil)
	d := addQueuedTask(m, "td")
	d.status = StatusDone
	if err := m.Pause("td"); err == nil {
		t.Fatal("done 任务 Pause 应报错")
	}
	if err := m.Cancel("td"); err == nil {
		t.Fatal("done 任务 Cancel 应报错")
	}
	r := addQueuedTask(m, "tr")
	r.status = StatusRunning
	if err := m.Resume("tr"); err == nil {
		t.Fatal("running 任务 Resume 应报错")
	}
}

// Resume 复位错误信息与失败计数，并强制断点续传。
func TestResumeResetsState(t *testing.T) {
	done := make(chan struct{})
	m := newStubManager(func(_ context.Context, _ *Task) error {
		close(done)
		return nil
	})
	task := addQueuedTask(m, "t1")
	task.status = StatusFailed
	task.errMsg = "boom"
	task.failed = 3
	task.opts.Restart = true

	if err := m.Resume("t1"); err != nil {
		t.Fatalf("Resume 失败: %v", err)
	}
	task.mu.Lock()
	errMsg, failed, restart := task.errMsg, task.failed, task.opts.Restart
	task.mu.Unlock()
	if errMsg != "" || failed != 0 {
		t.Fatalf("Resume 应复位错误与失败计数，实际 errMsg=%q failed=%d", errMsg, failed)
	}
	if restart {
		t.Fatal("Resume 应强制断点续传（Restart=false）")
	}
	<-done
	waitStatus(t, task, StatusDone)
}

// 断点持久化：saveResumeKey 逐条落库后 LoadFinished 能读回同一集合；
// 带 store 的管理器重启后运行/排队任务恢复为 paused。
func TestResumePersistenceRoundtrip(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "tasks.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("打开 store 失败: %v", err)
	}
	defer func() { _ = st.Close() }()

	m := NewManager(Deps{Emitter: &events.Emitter{}, Store: st})
	id, err0 := func() (string, error) {
		task := &Task{
			ID: "t1", CreatedAt: "2026-01-01 00:00:00", mgr: m, status: StatusRunning,
			opts: TaskOptions{Dir: t.TempDir(), URLs: []string{"https://t.me/c/1/2"}},
		}
		if err := st.InsertTask(context.Background(), toStoreTask(task), optionsToItems(task.opts)); err != nil {
			return "", err
		}
		m.tasks[task.ID] = task
		m.order = append(m.order, task.ID)

		task.saveResumeKey("1_2")
		task.saveResumeKey("1_3")
		return task.ID, nil
	}()
	if err0 != nil {
		t.Fatalf("准备任务失败: %v", err0)
	}

	got, err := st.LoadFinished(context.Background(), id)
	if err != nil {
		t.Fatalf("LoadFinished 失败: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("断点集合应有 2 项，实际 %d", len(got))
	}
	if _, ok := got["1_2"]; !ok {
		t.Fatal("断点集合缺少 1_2")
	}

	// 模拟重启：新管理器从 store 恢复，running 任务应落为 paused（可断点续传）
	m2 := NewManager(Deps{Emitter: &events.Emitter{}, Store: st})
	t2, err := m2.get(id)
	if err != nil {
		t.Fatalf("重启后任务丢失: %v", err)
	}
	if got := taskStatus(t2); got != StatusPaused {
		t.Fatalf("重启后 running 任务应为 paused，实际 %s", got)
	}
}
