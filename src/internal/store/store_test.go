package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

var testCtx = context.Background()

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "tasks.db"))
	if err != nil {
		t.Fatalf("打开测试库失败: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestMigrateCreatesSchema(t *testing.T) {
	s := newTestStore(t)
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("读取版本失败: %v", err)
	}
	if version != schemaVersion {
		t.Fatalf("期望版本 %d，实际 %d", schemaVersion, version)
	}
}

func TestInsertAndLoadTask(t *testing.T) {
	s := newTestStore(t)
	task := Task{ID: "t1", Label: "示例", Dir: "/tmp", Status: "queued", GroupMedia: true, Total: 3}
	items := []Item{
		{ItemType: ItemTypeURL, URL: "https://t.me/a/1"},
		{ItemType: ItemTypeSelection, DialogID: 42, DialogType: "channel", MessageIDs: []int{10, 11}},
	}
	if err := s.InsertTask(testCtx, task, items); err != nil {
		t.Fatalf("插入任务失败: %v", err)
	}

	tasks, err := s.LoadAllTasks(testCtx)
	if err != nil || len(tasks) != 1 {
		t.Fatalf("加载任务失败: %v 数量=%d", err, len(tasks))
	}
	if tasks[0].ID != "t1" || !tasks[0].GroupMedia || tasks[0].Total != 3 {
		t.Fatalf("任务字段不符: %+v", tasks[0])
	}

	got, err := s.ListItems(testCtx, "t1")
	if err != nil || len(got) != 2 {
		t.Fatalf("加载消息项失败: %v 数量=%d", err, len(got))
	}
	if got[1].DialogID != 42 || len(got[1].MessageIDs) != 2 {
		t.Fatalf("选集项字段不符: %+v", got[1])
	}
}

func TestFileUpsertAndFinish(t *testing.T) {
	s := newTestStore(t)
	if err := s.InsertTask(testCtx, Task{ID: "t1", Dir: "/tmp", Status: "running"}, nil); err != nil {
		t.Fatal(err)
	}
	tmp := "/tmp/a.mp4.tmp"
	if err := s.UpsertFile(testCtx, File{TaskID: "t1", Name: "a.mp4", Path: tmp, Size: 100, State: "downloading"}); err != nil {
		t.Fatal(err)
	}
	// 同路径覆盖
	if err := s.UpsertFile(testCtx, File{TaskID: "t1", Name: "a.mp4", Path: tmp, Size: 200, State: "downloading"}); err != nil {
		t.Fatal(err)
	}
	files, _ := s.ListFiles(testCtx, "t1")
	if len(files) != 1 || files[0].Size != 200 {
		t.Fatalf("同路径应覆盖为 1 行 size=200，实际 %+v", files)
	}

	final := "/tmp/a.mp4"
	if err := s.FinishFile(testCtx, "t1", tmp, File{TaskID: "t1", Name: "a.mp4", Path: final, Size: 200, State: "done"}); err != nil {
		t.Fatal(err)
	}
	files, _ = s.ListFiles(testCtx, "t1")
	if len(files) != 1 || files[0].Path != final || files[0].State != "done" {
		t.Fatalf("完成后应为最终路径 done，实际 %+v", files)
	}
}

func TestResumePoints(t *testing.T) {
	s := newTestStore(t)
	if err := s.InsertTask(testCtx, Task{ID: "t1", Dir: "/tmp", Status: "running"}, nil); err != nil {
		t.Fatal(err)
	}
	set := map[string]struct{}{"42:10": {}, "42:11": {}}
	if err := s.SaveFinished(testCtx, "t1", set); err != nil {
		t.Fatal(err)
	}
	got, err := s.LoadFinished(testCtx, "t1")
	if err != nil || len(got) != 2 {
		t.Fatalf("断点读取失败: %v 数量=%d", err, len(got))
	}
	if _, ok := got["42:10"]; !ok {
		t.Fatal("断点集合应包含 42:10")
	}
	if err := s.DeleteResume(testCtx, "t1"); err != nil {
		t.Fatal(err)
	}
	got, _ = s.LoadFinished(testCtx, "t1")
	if len(got) != 0 {
		t.Fatalf("删除后应为空，实际 %d", len(got))
	}
}

func TestCascadeDelete(t *testing.T) {
	s := newTestStore(t)
	if err := s.InsertTask(testCtx, Task{ID: "t1", Dir: "/tmp", Status: "done"},
		[]Item{{ItemType: ItemTypeURL, URL: "u1"}}); err != nil {
		t.Fatal(err)
	}
	_ = s.UpsertFile(testCtx, File{TaskID: "t1", Name: "f", Path: "/tmp/f", State: "done"})
	_ = s.SaveFinished(testCtx, "t1", map[string]struct{}{"1:1": {}})

	if err := s.DeleteTask(testCtx, "t1"); err != nil {
		t.Fatal(err)
	}
	if items, _ := s.ListItems(testCtx, "t1"); len(items) != 0 {
		t.Fatal("级联删除应清空消息项")
	}
	if files, _ := s.ListFiles(testCtx, "t1"); len(files) != 0 {
		t.Fatal("级联删除应清空文件")
	}
	if fin, _ := s.LoadFinished(testCtx, "t1"); len(fin) != 0 {
		t.Fatal("级联删除应清空断点")
	}
}

func TestImportLegacyJSON(t *testing.T) {
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "tasks.json")
	legacy := `[{"id":"t1","createdAt":"2026-01-01 00:00:00","opts":{"urls":["u1","u2"],
		"selections":[{"dialogId":42,"dialogType":"channel","messageIds":[1,2]}],
		"label":"L","dir":"/d","scriptName":"s","template":"tpl","group":true},
		"status":"running","total":4,"finished":1,"files":[{"name":"a","path":"/d/a","size":9,"state":"done"}]}]`
	if err := os.WriteFile(jsonPath, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}

	s, err := Open(filepath.Join(dir, "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if err := s.ImportLegacyJSON(testCtx, jsonPath); err != nil {
		t.Fatalf("导入失败: %v", err)
	}

	tasks, _ := s.LoadAllTasks(testCtx)
	if len(tasks) != 1 || tasks[0].Status != "paused" {
		t.Fatalf("running 应降级为 paused，实际 %+v", tasks)
	}
	items, _ := s.ListItems(testCtx, "t1")
	if len(items) != 3 { // 2 url + 1 selection
		t.Fatalf("应导入 3 条消息项，实际 %d", len(items))
	}
	files, _ := s.ListFiles(testCtx, "t1")
	if len(files) != 1 {
		t.Fatalf("应导入 1 条文件，实际 %d", len(files))
	}
	// tasks.json 应重命名为 .bak
	if _, err := os.Stat(jsonPath); !os.IsNotExist(err) {
		t.Fatal("tasks.json 应已重命名")
	}
	if _, err := os.Stat(jsonPath + ".bak"); err != nil {
		t.Fatal("应生成 tasks.json.bak")
	}
}
