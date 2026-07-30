package config

import (
	"path/filepath"
	"testing"
)

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	dir := t.TempDir()
	return &Manager{dataDir: dir, settings: defaultSettings(dir)}
}

// TestDefaultSettingsDownloadDirAbsolute 默认下载目录必须是绝对路径（SVC-13）。
func TestDefaultSettingsDownloadDirAbsolute(t *testing.T) {
	s := defaultSettings(t.TempDir())
	if !filepath.IsAbs(s.DownloadDir) {
		t.Fatalf("默认下载目录应为绝对路径，实际: %q", s.DownloadDir)
	}
}

// TestUpdateClampsUpperLimits 超限参数钳到上限而非原样保存（SVC-14）。
func TestUpdateClampsUpperLimits(t *testing.T) {
	m := newTestManager(t)
	s := m.Get()
	s.Threads = 100000
	s.PoolSize = 99999
	if err := m.Update(s); err != nil {
		t.Fatalf("Update 失败: %v", err)
	}
	got := m.Get()
	if got.Threads != maxThreads {
		t.Errorf("Threads 应钳到 %d，实际 %d", maxThreads, got.Threads)
	}
	if got.PoolSize != maxPoolSize {
		t.Errorf("PoolSize 应钳到 %d，实际 %d", maxPoolSize, got.PoolSize)
	}
}

// TestUpdateDefaultsOnNonPositive 非正值与空模板回落默认。
func TestUpdateDefaultsOnNonPositive(t *testing.T) {
	m := newTestManager(t)
	s := m.Get()
	s.Threads = 0
	s.Limit = -1
	s.PoolSize = 0
	s.Template = ""
	if err := m.Update(s); err != nil {
		t.Fatalf("Update 失败: %v", err)
	}
	got := m.Get()
	if got.Threads != 4 || got.Limit != 2 || got.PoolSize != 8 {
		t.Errorf("非正值应回落默认 4/2/8，实际 %d/%d/%d", got.Threads, got.Limit, got.PoolSize)
	}
	if got.Template != DefaultTemplate {
		t.Errorf("空模板应回落默认模板，实际 %q", got.Template)
	}
}

// TestUpdateValidatesProxy 代理格式预校验（SVC-14）。
func TestUpdateValidatesProxy(t *testing.T) {
	m := newTestManager(t)
	for _, p := range []string{
		"ftp://127.0.0.1:21", // 协议不在白名单
		"://bad",             // 无法解析
		"socks5://",          // 缺主机名
	} {
		s := m.Get()
		s.Proxy = p
		if err := m.Update(s); err == nil {
			t.Errorf("非法代理 %q 应被拒绝", p)
		}
	}
	for _, p := range []string{
		"",
		"socks5://127.0.0.1:1080",
		"http://127.0.0.1:8080",
		"https://proxy.local:443",
	} {
		s := m.Get()
		s.Proxy = p
		if err := m.Update(s); err != nil {
			t.Errorf("合法代理 %q 不应被拒绝: %v", p, err)
		}
	}
}

// TestTempDirFallback 视频临时目录：空值回落 <DataDir>/tmp，非空值原样返回。
func TestTempDirFallback(t *testing.T) {
	m := newTestManager(t)
	if got, want := m.TempDir(), filepath.Join(m.DataDir(), "tmp"); got != want {
		t.Errorf("空 TempDir 应回落 %q，实际 %q", want, got)
	}
	custom := filepath.Join(t.TempDir(), "video-tmp")
	s := m.Get()
	s.TempDir = custom
	if err := m.Update(s); err != nil {
		t.Fatalf("Update 失败: %v", err)
	}
	if got := m.TempDir(); got != custom {
		t.Errorf("自定义 TempDir 应为 %q，实际 %q", custom, got)
	}
}

// TestTempDirYAMLRoundTrip TempDir 经 yamlConfig 往返不丢失。
func TestTempDirYAMLRoundTrip(t *testing.T) {
	s := defaultSettings(t.TempDir())
	s.TempDir = `D:\video-tmp`
	got := yamlToSettings(settingsToYAML(s))
	if got.TempDir != s.TempDir {
		t.Errorf("TempDir 往返应保留 %q，实际 %q", s.TempDir, got.TempDir)
	}
}
