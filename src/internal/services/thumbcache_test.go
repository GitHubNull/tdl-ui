package services

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newTestThumbCache(t *testing.T) *thumbCache {
	t.Helper()
	return &thumbCache{
		root:     filepath.Join(t.TempDir(), "cache"),
		inflight: make(map[string]chan struct{}),
	}
}

func TestThumbCacheGetWritesDisk(t *testing.T) {
	c := newTestThumbCache(t)
	want := []byte("jpeg-bytes")
	var calls int32

	got, err := c.Get(cacheKindThumb, 42, 7, func() ([]byte, error) {
		atomic.AddInt32(&calls, 1)
		return want, nil
	})
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("首次拉取失败: %v %q", err, got)
	}

	// 落盘后无 tmp 残留，二次读取直接命中磁盘、不再触发 fetch
	p := c.path(cacheKindThumb, 42, 7)
	if _, err := os.Stat(p + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("原子写不应残留 tmp 文件")
	}
	got, err = c.Get(cacheKindThumb, 42, 7, func() ([]byte, error) {
		atomic.AddInt32(&calls, 1)
		return nil, errors.New("不应再拉取")
	})
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("缓存命中失败: %v", err)
	}
	if calls != 1 {
		t.Fatalf("预期只拉取 1 次，实际 %d", calls)
	}
}

func TestThumbCacheSingleflight(t *testing.T) {
	c := newTestThumbCache(t)
	var calls int32
	started := make(chan struct{})
	release := make(chan struct{})

	// 首个拉取方：在 fetch 内阻塞，撑开窗口让其余请求进入等待
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		b, err := c.Get(cacheKindPreview, 1, 2, func() ([]byte, error) {
			atomic.AddInt32(&calls, 1)
			close(started)
			<-release
			return []byte("data"), nil
		})
		if err != nil || string(b) != "data" {
			t.Errorf("拉取方 Get 失败: %v %q", err, b)
		}
	}()
	<-started

	for range 7 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b, err := c.Get(cacheKindPreview, 1, 2, func() ([]byte, error) {
				atomic.AddInt32(&calls, 1)
				return []byte("data"), nil
			})
			if err != nil || string(b) != "data" {
				t.Errorf("并发 Get 失败: %v %q", err, b)
			}
		}()
	}
	time.Sleep(50 * time.Millisecond) // 等待方就位后再放行拉取
	close(release)
	wg.Wait()
	if calls != 1 {
		t.Fatalf("并发请求应只触发 1 次拉取，实际 %d", calls)
	}
}

func TestThumbCacheKindIsolation(t *testing.T) {
	c := newTestThumbCache(t)
	tp := c.path(cacheKindThumb, 5, 6)
	pp := c.path(cacheKindPreview, 5, 6)
	if tp == pp {
		t.Fatal("thumbs 与 previews 路径不应相同")
	}
	if filepath.Base(tp) != "5_6.jpg" {
		t.Fatalf("文件名应为 <dialogID>_<messageID>.jpg，实际 %s", filepath.Base(tp))
	}
}
