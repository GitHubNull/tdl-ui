package services

import (
	"os"
	"path/filepath"
	"testing"

	"tdl-ui/internal/config"
)

func newExportLogService(t *testing.T) *LogService {
	t.Helper()
	cfg, err := config.NewManagerAt(t.TempDir())
	if err != nil {
		t.Fatalf("NewManagerAt 失败: %v", err)
	}
	return NewLogService(cfg, nil)
}

// TestExportLogsWritesFile 正常导出写入目标目录并返回完整路径。
func TestExportLogsWritesFile(t *testing.T) {
	s := newExportLogService(t)
	dir := t.TempDir()

	got, err := s.ExportLogs(dir, "tdl-ui-log.csv", "time,level,module,source,message\n")
	if err != nil {
		t.Fatalf("ExportLogs 失败: %v", err)
	}
	want := filepath.Join(dir, "tdl-ui-log.csv")
	if got != want {
		t.Fatalf("返回路径应为 %q，实际 %q", want, got)
	}
	b, err := os.ReadFile(want)
	if err != nil {
		t.Fatalf("读取导出文件失败: %v", err)
	}
	if string(b) != "time,level,module,source,message\n" {
		t.Fatalf("导出内容不一致: %q", string(b))
	}
}

// TestExportLogsRejectsPathInFilename 文件名含路径分隔符被拒（防目录穿越）。
func TestExportLogsRejectsPathInFilename(t *testing.T) {
	s := newExportLogService(t)
	for _, name := range []string{"", filepath.Join("sub", "a.log"), "../a.log"} {
		if _, err := s.ExportLogs(t.TempDir(), name, "x"); err == nil {
			t.Errorf("文件名 %q 应被拒绝", name)
		}
	}
}

// TestExportLogsRejectsUnknownExt 非白名单扩展名被拒。
func TestExportLogsRejectsUnknownExt(t *testing.T) {
	s := newExportLogService(t)
	if _, err := s.ExportLogs(t.TempDir(), "a.exe", "x"); err == nil {
		t.Error("非白名单扩展名应被拒绝")
	}
	if _, err := s.ExportLogs(t.TempDir(), "noext", "x"); err == nil {
		t.Error("无扩展名应被拒绝")
	}
}

// TestExportLogsRejectsEmptyDir 空目录被拒。
func TestExportLogsRejectsEmptyDir(t *testing.T) {
	s := newExportLogService(t)
	if _, err := s.ExportLogs("", "a.log", "x"); err == nil {
		t.Error("空目录应被拒绝")
	}
}
