package engine

import (
	"os"
	"path/filepath"
	"testing"
)

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
