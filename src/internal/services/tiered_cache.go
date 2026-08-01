package services

import (
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// TieredCache 分层缓存架构：L1（内存）→ L2（磁盘）→ L3（网络）的三层缓存体系
// 提供高性能的媒体缓存访问，减少网络请求和磁盘 IO
type TieredCache struct {
	// L1: 高频访问的内存缓存
	memory *sync.Map // key -> *cacheEntry
	
	// L2: 磁盘持久化缓存
	disk *thumbCache
	
	// L3: 网络拉取函数
	fetcher func(key string) ([]byte, error)
	
	// 缓存统计
	stats *CacheStats
	
	// 配置
	maxMemoryEntries int64
	memoryTTL        time.Duration
}

// cacheEntry 内存缓存条目
type cacheEntry struct {
	data        []byte
	timestamp   time.Time
	accessCount int64
	lastAccess  time.Time
}

// CacheStats 缓存统计信息
type CacheStats struct {
	mu           sync.RWMutex
	L1Hits       int64 // L1 缓存命中次数
	L2Hits       int64 // L2 缓存命中次数
	L3Requests   int64 // L3 网络请求次数
	TotalRequests int64 // 总请求次数
	Evictions    int64 // 缓存淘汰次数
}

// NewTieredCache 创建分层缓存
func NewTieredCache(disk *thumbCache, fetcher func(key string) ([]byte, error)) *TieredCache {
	cache := &TieredCache{
		memory:           &sync.Map{},
		disk:             disk,
		fetcher:          fetcher,
		stats:            &CacheStats{},
		maxMemoryEntries: 1000, // 最多缓存1000个条目
		memoryTTL:        30 * time.Minute, // 内存缓存30分钟过期
	}
	
	// 启动清理 goroutine
	go cache.cleanupLoop()
	
	return cache
}

// Get 获取缓存数据，按 L1 → L2 → L3 顺序查找
func (c *TieredCache) Get(key string) ([]byte, error) {
	atomic.AddInt64(&c.stats.TotalRequests, 1)
	
	// L1 内存缓存查找
	if entry, ok := c.memory.Load(key); ok {
		e := entry.(*cacheEntry)
		// 检查是否过期
		if time.Since(e.timestamp) <= c.memoryTTL {
			atomic.AddInt64(&e.accessCount, 1)
			e.lastAccess = time.Now()
			atomic.AddInt64(&c.stats.L1Hits, 1)
			return e.data, nil
		}
		// 过期则删除
		c.memory.Delete(key)
		atomic.AddInt64(&c.stats.Evictions, 1)
	}
	
	// L2 磁盘缓存查找
	if c.disk != nil {
		// 从 key 解析 dialogID 和 messageID（假设 key 格式为 "dialogID:messageID"）
		if data, err := c.getFromDisk(key); err == nil {
			// 提升到 L1
			c.setMemoryEntry(key, data)
			atomic.AddInt64(&c.stats.L2Hits, 1)
			return data, nil
		}
	}
	
	// L3 网络拉取
	if c.fetcher != nil {
		atomic.AddInt64(&c.stats.L3Requests, 1)
		data, err := c.fetcher(key)
		if err != nil {
			return nil, err
		}
		
		// 写入 L1 和 L2
		c.setMemoryEntry(key, data)
		if c.disk != nil {
			c.setDiskEntry(key, data)
		}
		
		return data, nil
	}
	
	return nil, os.ErrNotExist
}

// getFromDisk 从磁盘缓存获取数据
func (c *TieredCache) getFromDisk(key string) ([]byte, error) {
	// 解析 key 获取 dialogID 和 messageID
	// 这里简化处理，实际应该根据具体的 key 格式解析
	// 暂时返回错误，表示需要从网络获取
	return nil, os.ErrNotExist
}

// setDiskEntry 写入磁盘缓存
func (c *TieredCache) setDiskEntry(key string, data []byte) {
	// 解析 key 获取 dialogID 和 messageID
	// 这里简化处理，实际应该根据具体的 key 格式解析
	// 暂时不实现磁盘写入
}

// setMemoryEntry 设置内存缓存条目
func (c *TieredCache) setMemoryEntry(key string, data []byte) {
	entry := &cacheEntry{
		data:        data,
		timestamp:   time.Now(),
		accessCount: 1,
		lastAccess:  time.Now(),
	}
	
	c.memory.Store(key, entry)
	
	// 检查是否需要淘汰
	c.evictIfNeeded()
}

// evictIfNeeded 如果需要则执行缓存淘汰
func (c *TieredCache) evictIfNeeded() {
	// 统计当前条目数
	var count int64
	c.memory.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	
	if count <= c.maxMemoryEntries {
		return
	}
	
	// 执行 LRU 淘汰：找到最少使用的条目删除
	var oldestKey interface{}
	var oldestTime time.Time = time.Now()
	var minAccess int64 = -1
	
	c.memory.Range(func(key, value interface{}) bool {
		entry := value.(*cacheEntry)
		accessCount := atomic.LoadInt64(&entry.accessCount)
		
		// 优先淘汰访问次数少的，其次淘汰最久未访问的
		if minAccess == -1 || accessCount < minAccess || 
		   (accessCount == minAccess && entry.lastAccess.Before(oldestTime)) {
			oldestKey = key
			oldestTime = entry.lastAccess
			minAccess = accessCount
		}
		return true
	})
	
	if oldestKey != nil {
		c.memory.Delete(oldestKey)
		atomic.AddInt64(&c.stats.Evictions, 1)
	}
}

// cleanupLoop 定期清理过期条目
func (c *TieredCache) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute) // 每5分钟清理一次
	defer ticker.Stop()
	
	for range ticker.C {
		c.cleanup()
	}
}

// cleanup 清理过期条目
func (c *TieredCache) cleanup() {
	now := time.Now()
	var keysToDelete []interface{}
	
	c.memory.Range(func(key, value interface{}) bool {
		entry := value.(*cacheEntry)
		if now.Sub(entry.timestamp) > c.memoryTTL {
			keysToDelete = append(keysToDelete, key)
		}
		return true
	})
	
	for _, key := range keysToDelete {
		c.memory.Delete(key)
		atomic.AddInt64(&c.stats.Evictions, 1)
	}
}

// GetStats 获取缓存统计信息
func (c *TieredCache) GetStats() CacheStats {
	c.stats.mu.RLock()
	defer c.stats.mu.RUnlock()
	return CacheStats{
		L1Hits:        c.stats.L1Hits,
		L2Hits:        c.stats.L2Hits,
		L3Requests:    c.stats.L3Requests,
		TotalRequests: c.stats.TotalRequests,
		Evictions:     c.stats.Evictions,
	}
}

// GetHitRate 获取缓存命中率
func (c *TieredCache) GetHitRate() float64 {
	stats := c.GetStats()
	if stats.TotalRequests == 0 {
		return 0
	}
	hits := stats.L1Hits + stats.L2Hits
	return float64(hits) / float64(stats.TotalRequests)
}

// Clear 清空所有缓存
func (c *TieredCache) Clear() {
	// 清空内存缓存
	c.memory.Range(func(key, _ interface{}) bool {
		c.memory.Delete(key)
		return true
	})
	
	// 清空磁盘缓存
	if c.disk != nil {
		_ = c.disk.Clear()
	}
	
	// 重置统计
	c.stats.mu.Lock()
	c.stats.L1Hits = 0
	c.stats.L2Hits = 0
	c.stats.L3Requests = 0
	c.stats.TotalRequests = 0
	c.stats.Evictions = 0
	c.stats.mu.Unlock()
}
