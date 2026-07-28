package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRecordsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	recs := []taskRecord{
		{
			ID:        "a",
			CreatedAt: "2024-01-01 00:00:00",
			Status:    StatusRunning,
			Total:     3,
			Finished:  1,
			Files: []TaskFile{
				{Name: "x.mp4", Path: `C:\dl\x.mp4`, Size: 100, State: "done"},
				{Name: "y.mp4", Path: `C:\dl\y.mp4.tmp`, Size: 0, State: "downloading"},
			},
		},
		{ID: "b", Status: StatusDone},
	}
	if err := saveRecords(path, recs); err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	// 原子写不应残留 tmp 文件
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("残留临时文件: %v", err)
	}

	// 经 restore 恢复：running 应回退为 paused，文件记录完整保留
	m := &Manager{statePath: path, tasks: make(map[string]*Task)}
	m.restore()
	if len(m.order) != 2 {
		t.Fatalf("预期恢复 2 条任务，实际 %d", len(m.order))
	}
	a := m.tasks["a"]
	if a == nil || a.status != StatusPaused {
		t.Fatalf("running 任务应恢复为 paused，实际 %+v", a)
	}
	if len(a.files) != 2 || a.files[1].State != "downloading" {
		t.Fatalf("文件记录丢失: %+v", a.files)
	}
	if m.tasks["b"].status != StatusDone {
		t.Fatal("done 任务状态应原样保留")
	}
}

func TestLoadRecordsMissingFile(t *testing.T) {
	recs, err := loadRecords(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil || recs != nil {
		t.Fatalf("文件不存在应视为空列表，实际 recs=%v err=%v", recs, err)
	}
}

func TestDeleteFilesPathValidation(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "a.bin")
	if err := os.WriteFile(real, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := NewManager(Deps{})
	task := &Task{ID: "t1", mgr: m, status: StatusDone, files: []TaskFile{
		{Name: "a.bin", Path: real, Size: 1, State: "done"},
	}}
	m.tasks["t1"] = task
	m.order = []string{"t1"}

	// 未登记的路径必须拒绝，防止越权删除任意文件
	outside := filepath.Join(dir, "outside.bin")
	if err := os.WriteFile(outside, []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.DeleteFiles("t1", []string{outside}); err == nil {
		t.Fatal("删除未登记路径应报错")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatal("未登记文件不应被删除")
	}

	// 登记路径可删除；文件清空后整条任务记录随之移除
	if err := m.DeleteFiles("t1", []string{real}); err != nil {
		t.Fatalf("删除登记文件失败: %v", err)
	}
	if _, err := os.Stat(real); !os.IsNotExist(err) {
		t.Fatal("登记文件应被删除")
	}
	if _, err := m.get("t1"); err == nil {
		t.Fatal("文件清空后任务记录应被移除")
	}
}

func TestDeleteFilesRejectsRunning(t *testing.T) {
	m := NewManager(Deps{})
	m.tasks["t1"] = &Task{ID: "t1", mgr: m, status: StatusRunning, files: []TaskFile{{Path: "p"}}}
	m.order = []string{"t1"}
	if err := m.DeleteFiles("t1", []string{"p"}); err == nil {
		t.Fatal("运行中的任务应先暂停")
	}
}
