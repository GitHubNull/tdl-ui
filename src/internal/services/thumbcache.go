package services

import (
	"os"
	"path/filepath"
	"strconv"
	"sync"
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
		c.mu.Unlock()

		b, err := fetch()
		if err == nil && len(b) > 0 {
			err = writeFileAtomic(p, b)
		}

		c.mu.Lock()
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
// 持锁可阻止新拉取在清理期间启动，降低与 in-flight 写入的竞争窗口（SVC-18）。
func (c *thumbCache) Clear() error {
	c.mu.Lock()
	defer c.mu.Unlock()
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
		// SVC-20：Windows 上 rename 到已存在目标会失败；
		// 目标已存在说明他方已写入同 key 内容，视为成功
		if _, serr := os.Stat(path); serr == nil {
			_ = os.Remove(tmp)
			return nil
		}
		return err
	}
	return nil
}
