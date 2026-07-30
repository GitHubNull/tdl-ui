package services

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestVideoDiskCache(t *testing.T) *videoDiskCache {
	t.Helper()
	dir := t.TempDir()
	return newVideoDiskCache(func() string { return dir })
}

// TestVideoDiskCacheRoundTrip put 后 get 应命中且内容一致，未写分段应未命中。
func TestVideoDiskCacheRoundTrip(t *testing.T) {
	c := newTestVideoDiskCache(t)

	data := []byte("part-bytes-0123456789")
	c.put(100, 7, 3, data)

	got, ok := c.get(100, 7, 3)
	if !ok {
		t.Fatal("已落盘分段应命中")
	}
	if !bytes.Equal(got, data) {
		t.Errorf("分段内容不一致: got=%q want=%q", got, data)
	}

	if _, ok := c.get(100, 7, 4); ok {
		t.Error("未写入的分段不应命中")
	}
	if _, ok := c.get(101, 7, 3); ok {
		t.Error("其他视频的分段不应命中")
	}
}

// TestVideoDiskCacheSweep 超限后按 mtime 从旧到新淘汰，最新分段保留。
func TestVideoDiskCacheSweep(t *testing.T) {
	c := newTestVideoDiskCache(t)
	c.max = 25 // 收缩上限便于触发淘汰

	old := bytes.Repeat([]byte("a"), 10)
	c.put(1, 1, 0, old)
	// 拉开 mtime 差距，避免文件系统时间精度导致排序不稳定
	oldPath := c.path(1, 1, 0)
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(oldPath, past, past); err != nil {
		t.Fatalf("设置旧分段 mtime 失败: %v", err)
	}

	c.put(1, 1, 1, bytes.Repeat([]byte("b"), 10))
	c.put(1, 1, 2, bytes.Repeat([]byte("c"), 10))

	c.mu.Lock()
	c.sweepLocked()
	c.mu.Unlock()

	if _, ok := c.get(1, 1, 0); ok {
		t.Error("最旧分段应被淘汰")
	}
	if _, ok := c.get(1, 1, 2); !ok {
		t.Error("最新分段应保留")
	}
}

// TestVideoDiskCacheClear Clear 后全部分段消失且根目录仍存在。
func TestVideoDiskCacheClear(t *testing.T) {
	c := newTestVideoDiskCache(t)
	c.put(2, 3, 0, []byte("x"))
	c.put(2, 3, 1, []byte("y"))

	if err := c.Clear(); err != nil {
		t.Fatalf("Clear 失败: %v", err)
	}
	if _, ok := c.get(2, 3, 0); ok {
		t.Error("Clear 后分段不应命中")
	}
	if st, err := os.Stat(c.dir()); err != nil || !st.IsDir() {
		t.Errorf("Clear 后根目录应重建: err=%v", err)
	}
}

// TestVideoDiskCachePathLayout 分段路径遵循 <root>/video/<dialog>_<msg>/<part>.part 布局。
func TestVideoDiskCachePathLayout(t *testing.T) {
	dir := t.TempDir()
	c := newVideoDiskCache(func() string { return dir })
	want := filepath.Join(dir, "video", "42_9", "5.part")
	if got := c.path(42, 9, 5); got != want {
		t.Errorf("分段路径应为 %q，实际 %q", want, got)
	}
}
