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

// ---- UI 设置与上下限钳制 ----

// TestUIWithDefaults 零值回默认并钳制上下限。
func TestUIWithDefaults(t *testing.T) {
	cases := []struct {
		in   UISettings
		want UISettings
	}{
		{in: UISettings{}, want: UISettings{LogFontSize: 14, ScrollbarSize: 10, MaxLogLines: 128}},
		{in: UISettings{LogFontSize: 5, ScrollbarSize: 3, MaxLogLines: 1}, want: UISettings{LogFontSize: 10, ScrollbarSize: 6, MaxLogLines: 16}},
		{in: UISettings{LogFontSize: 50, ScrollbarSize: 100, MaxLogLines: 99999}, want: UISettings{LogFontSize: 28, ScrollbarSize: 24, MaxLogLines: 1024}},
		{in: UISettings{LogFontSize: 18, ScrollbarSize: 12, MaxLogLines: 256}, want: UISettings{LogFontSize: 18, ScrollbarSize: 12, MaxLogLines: 256}},
	}
	for _, c := range cases {
		got := c.in.withDefaults()
		if got != c.want {
			t.Errorf("withDefaults(%+v) = %+v, want %+v", c.in, got, c.want)
		}
	}
}

// TestUpdateUIClamped UI 字段经 Update 钳制。
func TestUpdateUIClamped(t *testing.T) {
	m := newTestManager(t)
	s := m.Get()
	s.UI = UISettings{LogFontSize: 50, ScrollbarSize: 1, MaxLogLines: 99999}
	if err := m.Update(s); err != nil {
		t.Fatalf("Update 失败: %v", err)
	}
	got := m.Get().UI
	if got.LogFontSize != 28 || got.ScrollbarSize != 6 || got.MaxLogLines != 1024 {
		t.Errorf("UI 应被钳制，实际 %+v", got)
	}
}

// ---- 目录历史 ----

// TestAddRecentDir 去重、置顶、上限 3、非法 kind 报错。
func TestAddRecentDir(t *testing.T) {
	m := newTestManager(t)

	// 非法 kind
	if _, err := m.AddRecentDir("unknown", "/tmp"); err == nil {
		t.Error("非法 kind 应报错")
	}

	// 正常添加
	list, err := m.AddRecentDir("download", "/a")
	if err != nil {
		t.Fatalf("AddRecentDir 失败: %v", err)
	}
	if len(list) != 1 || list[0] != "/a" {
		t.Errorf("首次添加后应为 [/a]，实际 %v", list)
	}

	// 置顶
	list, _ = m.AddRecentDir("download", "/b")
	list, _ = m.AddRecentDir("download", "/a")
	if list[0] != "/a" || list[1] != "/b" {
		t.Errorf("重复添加应置顶，实际 %v", list)
	}

	// 上限 3
	list, _ = m.AddRecentDir("download", "/c")
	list, _ = m.AddRecentDir("download", "/d")
	if len(list) != 3 || list[0] != "/d" {
		t.Errorf("应裁剪到 3 条且最新置顶，实际 %v", list)
	}

	// 不同 kind 独立
	logList, _ := m.AddRecentDir("logExport", "/logs")
	if len(logList) != 1 || logList[0] != "/logs" {
		t.Errorf("logExport 应独立，实际 %v", logList)
	}
}

// TestRecentDirsRoundTrip 目录历史经 Update 往返保留。
func TestRecentDirsRoundTrip(t *testing.T) {
	m := newTestManager(t)
	s := m.Get()
	s.RecentDirs = map[string][]string{
		"download": {"/a", "/b"},
	}
	if err := m.Update(s); err != nil {
		t.Fatalf("Update 失败: %v", err)
	}
	got := m.Get().RecentDirs["download"]
	if len(got) != 2 || got[0] != "/a" {
		t.Errorf("RecentDirs 往返失败，实际 %v", got)
	}
}

// ---- 脚本启用状态 ----

// TestSetScriptEnabled 设置、查询、删除清理。
func TestSetScriptEnabled(t *testing.T) {
	m := newTestManager(t)
	if m.IsScriptEnabled("foo") {
		t.Error("默认应未启用")
	}
	if err := m.SetScriptEnabled("foo", true); err != nil {
		t.Fatalf("SetScriptEnabled 失败: %v", err)
	}
	if !m.IsScriptEnabled("foo") {
		t.Error("启用后应为 true")
	}
	if err := m.SetScriptEnabled("foo", false); err != nil {
		t.Fatalf("SetScriptEnabled 失败: %v", err)
	}
	if m.IsScriptEnabled("foo") {
		t.Error("禁用后应为 false")
	}

	// 重新 NewManagerAt 读取持久化值
	m2, err := NewManagerAt(m.DataDir())
	if err != nil {
		t.Fatalf("NewManagerAt 失败: %v", err)
	}
	if m2.IsScriptEnabled("foo") {
		t.Error("持久化后禁用态应保留")
	}
}

// TestTemplatesSeededRoundTrip TemplatesSeeded 经 Update 往返保留。
func TestTemplatesSeededRoundTrip(t *testing.T) {
	m := newTestManager(t)
	s := m.Get()
	s.TemplatesSeeded = true
	if err := m.Update(s); err != nil {
		t.Fatalf("Update 失败: %v", err)
	}
	m2, err := NewManagerAt(m.DataDir())
	if err != nil {
		t.Fatalf("NewManagerAt 失败: %v", err)
	}
	if !m2.Get().TemplatesSeeded {
		t.Error("TemplatesSeeded 应持久化保留")
	}
}

// TestRecentDirsClampedTo3 Update 时超长的 RecentDirs 被裁剪。
func TestRecentDirsClampedTo3(t *testing.T) {
	m := newTestManager(t)
	s := m.Get()
	s.RecentDirs = map[string][]string{
		"download": {"/a", "/b", "/c", "/d", "/e"},
	}
	if err := m.Update(s); err != nil {
		t.Fatalf("Update 失败: %v", err)
	}
	got := m.Get().RecentDirs["download"]
	if len(got) != 3 {
		t.Errorf("应裁剪到 3 条，实际 %d 条", len(got))
	}
}
