package services

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"syscall"
)

// 缓存种类（对应语义化子目录）。
const (
	cacheKindThumb   = "thumbs"   // 清晰小图
	cacheKindPreview = "previews" // 预览大图
)

// thumbCache 缩略图磁盘缓存：缓存根由 rootFn 每次求值，配置变更即时生效。
// 文件名 <dialogID>_<messageID>.jpg 全局唯一，天然防重复防覆盖。
type thumbCache struct {
	rootFn func() string

	mu       sync.Mutex
	epoch    uint64                   // 清空代际号：Clear 时递增，使在途写盘自我作废（LOGIC-002）
	inflight map[string]chan struct{} // 进程内 singleflight，防并发重复拉取
}

// newThumbCache 缓存根由 rootFn 注入（通常为 config.Manager.CacheDir）。
func newThumbCache(rootFn func() string) *thumbCache {
	return &thumbCache{
		rootFn:   rootFn,
		inflight: make(map[string]chan struct{}),
	}
}

// path 缓存文件路径：<root>/<kind>/<dialogID>_<messageID>.jpg。
func (c *thumbCache) path(kind string, dialogID int64, messageID int) string {
	name := strconv.FormatInt(dialogID, 10) + "_" + strconv.Itoa(messageID) + ".jpg"
	return filepath.Join(c.rootFn(), kind, name)
}

// Get 命中磁盘缓存直接读；未命中经 fetch 拉取并原子落盘。
// 同一 key 的并发请求仅触发一次拉取，其余等待后重读缓存。
func (c *thumbCache) Get(kind string, dialogID int64, messageID int, fetch func() ([]byte, error)) ([]byte, error) {
	p := c.path(kind, dialogID, messageID)
	for {
		if b, err := os.ReadFile(p); err == nil {
			return b, nil
		}

		c.mu.Lock()
		if ch, ok := c.inflight[p]; ok {
			c.mu.Unlock()
			<-ch // 等拉取方结束后重读；若其失败则下一轮自己成为拉取方
			continue
		}
		ch := make(chan struct{})
		c.inflight[p] = ch
		e := c.epoch // LOGIC-002：快照清空代际，写盘前校验
		c.mu.Unlock()

		b, err := fetch()

		c.mu.Lock()
		if err == nil && len(b) > 0 {
			if c.epoch == e {
				// 写盘留在锁内：与 Clear 互斥，杜绝清空后幽灵文件与
				// Windows 下 RemoveAll 撞上 rename 的 Access is denied（LOGIC-002）。
				err = writeFileAtomic(p, b)
			}
			// 代际已变：本次拉取先于 Clear 发起，跳过落盘仅内存返回
		}
		delete(c.inflight, p)
		c.mu.Unlock()
		close(ch)

		if err != nil {
			return nil, err
		}
		return b, nil
	}
}

// Clear 在锁保护下清空 thumbs/previews 两个子目录后重建（不递归删根目录）。
// 持锁可阻止新拉取在清理期间启动；同时递增 epoch 使在途拉取的写盘自我作废，
// 写盘也持锁校验代际，二者互斥（SVC-18 + LOGIC-002）。
func (c *thumbCache) Clear() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.epoch++ // 作废所有清空前发起的在途写盘
	root := c.rootFn()
	for _, kind := range []string{cacheKindThumb, cacheKindPreview} {
		sub := filepath.Join(root, kind)
		if err := os.RemoveAll(sub); err != nil {
			return err
		}
		if err := os.MkdirAll(sub, 0o755); err != nil {
			return err
		}
	}
	return nil
}

// writeFileAtomic tmp + rename 原子写，避免读到半截文件。
func writeFileAtomic(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		// SVC-20 + CODE-003：Windows 上 rename 到已存在/被占用目标会失败。
		// 判据按失败原因收窄——仅"目标已存在/共享冲突"类错误视为并发写入者
		// 已完成同 key 内容（等价成功）；权限/磁盘等其他错误原样返回，
		// 避免 dst 恰为历史旧文件时误报成功。
		if isConcurrentWriteErr(err) {
			_ = os.Remove(tmp)
			return nil
		}
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// isConcurrentWriteErr 判定 rename 失败是否属于"并发写入者已占据目标"类错误：
// fs.ErrExist（EEXIST）或 Windows 的共享冲突/拒绝访问（文件被同 key 写入方占用）。
func isConcurrentWriteErr(err error) bool {
	if errors.Is(err, fs.ErrExist) {
		return true
	}
	if runtime.GOOS == "windows" {
		// ERROR_SHARING_VIOLATION(32) / ERROR_ACCESS_DENIED(5)：
		// 并发 rename 到同名目标或目标被读取方短暂占用的典型错误码
		var errno syscall.Errno
		if errors.As(err, &errno) {
			return errno == 32 || errno == 5
		}
	}
	return false
}
