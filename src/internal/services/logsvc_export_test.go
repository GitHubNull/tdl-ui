package services

import (
	"path/filepath"
	"strings"
	"testing"

	"tdl-ui/internal/config"
	"tdl-ui/internal/events"
	"tdl-ui/internal/logging"
)

func newTestLogService(t *testing.T) (*LogService, *config.Manager) {
	t.Helper()
	cfg, err := config.NewManagerAt(t.TempDir())
	if err != nil {
		t.Fatalf("NewManagerAt 失败: %v", err)
	}
	emitter := &events.Emitter{}
	return NewLogService(cfg, emitter), cfg
}

// TestExportLogsNormal 正常写入文件。
func TestExportLogsNormal(t *testing.T) {
	svc, _ := newTestLogService(t)
	dir := t.TempDir()
	path, err := svc.ExportLogs(dir, "test.log", "hello world")
	if err != nil {
		t.Fatalf("ExportLogs 失败: %v", err)
	}
	if path != filepath.Join(dir, "test.log") {
		t.Errorf("路径应为 %q，实际 %q", filepath.Join(dir, "test.log"), path)
	}
}

// TestExportLogsRejectsPathSeparator filename 含路径分隔符应被拒。
func TestExportLogsRejectsPathSeparator(t *testing.T) {
	svc, _ := newTestLogService(t)
	for _, name := range []string{"../etc/passwd.log", "a/b.txt", `c\d.txt`} {
		_, err := svc.ExportLogs(t.TempDir(), name, "x")
		if err == nil || !strings.Contains(err.Error(), "非法") {
			t.Errorf("%q 应被拒，实际 err=%v", name, err)
		}
	}
}

// TestExportLogsRejectsBadExt 非白名单扩展名应被拒。
func TestExportLogsRejectsBadExt(t *testing.T) {
	svc, _ := newTestLogService(t)
	for _, name := range []string{"data.exe", "dump.zip", "file"} {
		_, err := svc.ExportLogs(t.TempDir(), name, "x")
		if err == nil || !strings.Contains(err.Error(), "不支持") {
			t.Errorf("%q 应被拒，实际 err=%v", name, err)
		}
	}
}

// TestExportLogsRecordsRecentDir 成功后记录 logExport 历史。
func TestExportLogsRecordsRecentDir(t *testing.T) {
	svc, cfg := newTestLogService(t)
	dir := t.TempDir()
	_, err := svc.ExportLogs(dir, "test.txt", "x")
	if err != nil {
		t.Fatalf("ExportLogs 失败: %v", err)
	}
	recent := cfg.RecentDirs("logExport")
	if len(recent) != 1 || recent[0] != dir {
		t.Errorf("导出目录应被记录，实际 %v", recent)
	}
}

// 初始化日志系统（避免 nil logger）
func init() {
	logging.Init(logging.DefaultSettings(), nil)
}
