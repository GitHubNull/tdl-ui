package services

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func newTestThumbCache(t *testing.T) *thumbCache {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "cache")
	return &thumbCache{
		rootFn:   func() string { return dir },
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

// LOGIC-002 回归：Clear 期间在途的 fetch 完成后不得把旧代际文件写回刚清空的目录。
func TestThumbCacheClearInvalidatesInflightWrite(t *testing.T) {
	c := newTestThumbCache(t)
	fetching := make(chan struct{})
	release := make(chan struct{})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		b, err := c.Get(cacheKindThumb, 9, 9, func() ([]byte, error) {
			close(fetching)
			<-release // 撑开窗口：fetch 进行中时触发 Clear
			return []byte("stale"), nil
		})
		// 旧代际拉取仍应内存返回成功，只是不落盘
		if err != nil || string(b) != "stale" {
			t.Errorf("在途 Get 应内存返回成功: %v %q", err, b)
		}
	}()
	<-fetching

	if err := c.Clear(); err != nil {
		t.Fatalf("Clear 失败: %v", err)
	}
	close(release)
	wg.Wait()

	// 清空前发起的拉取不得在目录中留下幽灵文件
	if _, err := os.Stat(c.path(cacheKindThumb, 9, 9)); !os.IsNotExist(err) {
		t.Fatal("Clear 后旧代际写盘应被跳过，目录中不应出现幽灵文件")
	}

	// 新代际拉取正常落盘
	if _, err := c.Get(cacheKindThumb, 9, 9, func() ([]byte, error) {
		return []byte("fresh"), nil
	}); err != nil {
		t.Fatalf("新代际 Get 失败: %v", err)
	}
	if b, err := os.ReadFile(c.path(cacheKindThumb, 9, 9)); err != nil || string(b) != "fresh" {
		t.Fatalf("新代际应正常落盘: %v %q", err, b)
	}
}

// LOGIC-002 并发压测：多 Get 与 Clear 交错，无 panic、Clear 不报错（Windows 上无 Access is denied）。
func TestThumbCacheConcurrentGetAndClear(t *testing.T) {
	c := newTestThumbCache(t)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := range 5 {
				_, _ = c.Get(cacheKindThumb, int64(id), j, func() ([]byte, error) {
					time.Sleep(5 * time.Millisecond)
					return []byte("x"), nil
				})
			}
		}(i)
	}
	for range 3 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			time.Sleep(10 * time.Millisecond)
			if err := c.Clear(); err != nil {
				t.Errorf("并发 Clear 失败: %v", err)
			}
		}()
	}
	wg.Wait()
}

// CODE-003 回归：rename 失败判据按错误类型收窄，而非"目标存在即成功"。
func TestIsConcurrentWriteErr(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"EEXIST", &os.LinkError{Op: "rename", Old: "a", New: "b", Err: fs.ErrExist}, true},
		{"共享冲突(32)", &os.LinkError{Op: "rename", Old: "a", New: "b", Err: syscall.Errno(32)}, true},
		{"拒绝访问(5)", &os.LinkError{Op: "rename", Old: "a", New: "b", Err: syscall.Errno(5)}, true},
		{"文件不存在(2)", &os.LinkError{Op: "rename", Old: "a", New: "b", Err: syscall.Errno(2)}, false},
		{"普通错误", errors.New("disk full"), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isConcurrentWriteErr(c.err); got != c.want {
				t.Fatalf("预期 %v，实际 %v", c.want, got)
			}
		})
	}
}

// CODE-003：真实失败（父目录路径被文件占据）必须返回错误，不得误报成功。
func TestWriteFileAtomicPropagatesRealError(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("准备失败: %v", err)
	}
	// blocker 是文件，无法作为目录创建其子路径
	if err := writeFileAtomic(filepath.Join(blocker, "sub", "a.jpg"), []byte("data")); err == nil {
		t.Fatal("路径不可用时应返回错误")
	}
}
