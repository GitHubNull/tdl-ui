package services

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"
)

// 视频磁盘分段缓存参数（与 video.go 的流式常量同口径集中具名）。
const (
	// videoDiskCacheMax 磁盘分段总量上限（2GB），超限按 mtime LRU 淘汰。
	videoDiskCacheMax int64 = 2 << 30
	// videoDiskSweepEvery 每累计写入该字节数触发一次超限淘汰，避免逐写扫盘。
	videoDiskSweepEvery int64 = 64 << 20
)

// videoDiskCache 在线视频分段磁盘缓存（Telegram 式边下边播的临时文件层）：
// 分段落盘为 <root>/video/<dialogID>_<msgID>/<partIdx>.part，
// 重看/回拖直接命中磁盘，不重复向 Telegram 拉取。
// 根目录经 rootFn 每次求值（config.Manager.TempDir），配置变更即时生效。
type videoDiskCache struct {
	rootFn func() string
	// max 磁盘总量上限，默认 videoDiskCacheMax（测试可注入小值）
	max int64

	mu      sync.Mutex
	written int64 // 距上次淘汰后的累计写入字节数
}

// newVideoDiskCache 构造时清理一次历史残留，避免上次运行遗留超限占用。
func newVideoDiskCache(rootFn func() string) *videoDiskCache {
	c := &videoDiskCache{rootFn: rootFn, max: videoDiskCacheMax}
	c.mu.Lock()
	c.sweepLocked()
	c.mu.Unlock()
	return c
}

// dir 分段根目录：<TempDir>/video。
func (c *videoDiskCache) dir() string { return filepath.Join(c.rootFn(), "video") }

// path 分段文件路径：<root>/video/<dialogID>_<msgID>/<partIdx>.part。
func (c *videoDiskCache) path(dialogID int64, messageID int, partIdx int64) string {
	sub := strconv.FormatInt(dialogID, 10) + "_" + strconv.Itoa(messageID)
	return filepath.Join(c.dir(), sub, strconv.FormatInt(partIdx, 10)+".part")
}

// get 读取磁盘分段；命中时刷新 mtime 作为 LRU 依据。
func (c *videoDiskCache) get(dialogID int64, messageID int, partIdx int64) ([]byte, bool) {
	p := c.path(dialogID, messageID, partIdx)
	b, err := os.ReadFile(p)
	if err != nil || len(b) == 0 {
		return nil, false
	}
	now := time.Now()
	_ = os.Chtimes(p, now, now) // LRU 触碰失败不影响读取
	return b, true
}

// has 仅探测分段是否已落盘（预取跳过判定，不读内容不触碰 mtime）。
func (c *videoDiskCache) has(dialogID int64, messageID int, partIdx int64) bool {
	st, err := os.Stat(c.path(dialogID, messageID, partIdx))
	return err == nil && !st.IsDir() && st.Size() > 0
}

// put 原子落盘一个分段；累计写入超过阈值时触发一次超限淘汰。
func (c *videoDiskCache) put(dialogID int64, messageID int, partIdx int64, data []byte) {
	if len(data) == 0 {
		return
	}
	p := c.path(dialogID, messageID, partIdx)
	if err := writeFileAtomic(p, data); err != nil {
		logMedia.Debugf("视频分段落盘失败: %s err=%v", p, err)
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.written += int64(len(data))
	if c.written >= videoDiskSweepEvery {
		c.written = 0
		c.sweepLocked()
	}
}

// Clear 清空全部磁盘分段后重建根目录（设置页「清空缓存」入口）。
func (c *videoDiskCache) Clear() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	dir := c.dir()
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	return os.MkdirAll(dir, 0o755)
}

// diskPart 淘汰扫描的候选条目。
type diskPart struct {
	path string
	size int64
	mod  time.Time
}

// sweepLocked 扫描分段目录，总量超限时按 mtime 从旧到新删除，需持有 c.mu。
func (c *videoDiskCache) sweepLocked() {
	var (
		parts []diskPart
		total int64
	)
	_ = filepath.Walk(c.dir(), func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil // 目录不存在或单文件异常均跳过，不中断扫描
		}
		parts = append(parts, diskPart{path: path, size: info.Size(), mod: info.ModTime()})
		total += info.Size()
		return nil
	})
	if total <= c.max {
		return
	}

	sort.Slice(parts, func(i, j int) bool { return parts[i].mod.Before(parts[j].mod) })
	for _, p := range parts {
		if total <= c.max {
			break
		}
		if err := os.Remove(p.path); err != nil {
			logMedia.Debugf("视频分段淘汰失败: %s err=%v", p.path, err)
			continue
		}
		total -= p.size
	}
	logMedia.Infof("视频临时分段已按 LRU 淘汰至 %d 字节以内", c.max)
}
