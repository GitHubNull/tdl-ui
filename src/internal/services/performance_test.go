package services

import (
	"sync"
	"testing"
	"time"
)

// TestThumbPreloader 测试缩略图预加载器
func TestThumbPreloader(t *testing.T) {
	// 创建带有基本字段的 ChatService
	chatService := &ChatService{
		thumbs: newThumbCache(func() string { return t.TempDir() }),
	}
	
	preloader := NewThumbPreloader(chatService)
	if preloader == nil {
		t.Fatal("Failed to create ThumbPreloader")
	}

	// 测试预加载功能
	messageIDs := []int{1, 2, 3, 4, 5}
	preloader.PreloadThumbs(123, "private", messageIDs)

	// 等待一段时间让预加载完成
	time.Sleep(100 * time.Millisecond)

	// 检查统计信息
	stats := preloader.GetStats()
	if stats.TotalRequests < 0 {
		t.Error("TotalRequests should be non-negative")
	}
}

// TestTieredCache 测试分层缓存
func TestTieredCache(t *testing.T) {
	// 创建带有临时目录的磁盘缓存
	diskCache := newThumbCache(func() string { return t.TempDir() })
	
	// 创建网络拉取函数
	fetcher := func(key string) ([]byte, error) {
		return []byte("test data for " + key), nil
	}

	cache := NewTieredCache(diskCache, fetcher)
	if cache == nil {
		t.Fatal("Failed to create TieredCache")
	}

	// 测试缓存获取
	data, err := cache.Get("test:key")
	if err != nil {
		t.Errorf("Failed to get from cache: %v", err)
	}
	if string(data) != "test data for test:key" {
		t.Errorf("Unexpected data: %s", string(data))
	}

	// 再次获取应该命中 L1 缓存
	data2, err := cache.Get("test:key")
	if err != nil {
		t.Errorf("Failed to get from cache on second call: %v", err)
	}
	if string(data2) != string(data) {
		t.Error("Data should be consistent")
	}

	// 检查统计信息
	stats := cache.GetStats()
	if stats.TotalRequests != 2 {
		t.Errorf("Expected 2 total requests, got %d", stats.TotalRequests)
	}
	if stats.L3Requests != 1 {
		t.Errorf("Expected 1 L3 request, got %d", stats.L3Requests)
	}
	if stats.L1Hits != 1 {
		t.Errorf("Expected 1 L1 hit, got %d", stats.L1Hits)
	}

	// 测试命中率
	hitRate := cache.GetHitRate()
	if hitRate != 0.5 { // 1 hit out of 2 requests
		t.Errorf("Expected hit rate 0.5, got %f", hitRate)
	}
}

// TestSmartCacheWarmer 测试智能缓存预热器
func TestSmartCacheWarmer(t *testing.T) {
	chatService := &ChatService{
		thumbs: newThumbCache(func() string { return t.TempDir() }),
	}
	tieredCache := NewTieredCache(chatService.thumbs, nil)
	
	warmer := NewSmartCacheWarmer(chatService, tieredCache)
	if warmer == nil {
		t.Fatal("Failed to create SmartCacheWarmer")
	}

	// 测试预热功能
	items := []MediaItem{
		{DialogID: 123, MessageID: 1, Kind: KindPhoto},
		{DialogID: 123, MessageID: 2, Kind: KindVideo},
		{DialogID: 123, MessageID: 3, Kind: KindFile},
	}

	warmer.WarmupForDialog(123, "private", items)

	// 检查最近对话列表
	recentDialogs := warmer.GetRecentDialogs()
	if len(recentDialogs) == 0 {
		t.Error("Recent dialogs should not be empty")
	}
	if recentDialogs[0] != 123 {
		t.Errorf("Expected first recent dialog to be 123, got %d", recentDialogs[0])
	}

	// 测试预加载开关
	if !warmer.IsPreloadEnabled() {
		t.Error("Preload should be enabled by default")
	}

	warmer.SetPreloadEnabled(false)
	if warmer.IsPreloadEnabled() {
		t.Error("Preload should be disabled after SetPreloadEnabled(false)")
	}
}

// TestConcurrentAccess 测试并发访问安全性
func TestConcurrentAccess(t *testing.T) {
	diskCache := newThumbCache(func() string { return t.TempDir() })
	cache := NewTieredCache(diskCache, func(key string) ([]byte, error) {
		// 模拟网络延迟
		time.Sleep(10 * time.Millisecond)
		return []byte("data for " + key), nil
	})

	const numGoroutines = 10
	const numRequestsPerGoroutine = 5

	var wg sync.WaitGroup
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numRequestsPerGoroutine; j++ {
				key := "concurrent:test"
				_, err := cache.Get(key)
				if err != nil {
					t.Errorf("Goroutine %d failed to get from cache: %v", id, err)
				}
			}
		}(i)
	}
	wg.Wait()

	stats := cache.GetStats()
	expectedTotal := int64(numGoroutines * numRequestsPerGoroutine)
	if stats.TotalRequests != expectedTotal {
		t.Errorf("Expected %d total requests, got %d", expectedTotal, stats.TotalRequests)
	}

	// 由于并发访问同一个 key，应该有缓存命中
	if stats.L1Hits == 0 {
		t.Error("Expected some L1 cache hits due to concurrent access")
	}
}

// TestCacheEviction 测试缓存淘汰
func TestCacheEviction(t *testing.T) {
	diskCache := newThumbCache(func() string { return t.TempDir() })
	cache := NewTieredCache(diskCache, func(key string) ([]byte, error) {
		return []byte("data for " + key), nil
	})

	// 设置小的缓存容量进行测试
	cache.maxMemoryEntries = 3

	// 添加超过容量的条目
	for i := 0; i < 5; i++ {
		key := string(rune('a' + i))
		_, err := cache.Get(key)
		if err != nil {
			t.Errorf("Failed to get from cache: %v", err)
		}
	}

	stats := cache.GetStats()
	if stats.Evictions == 0 {
		t.Error("Expected some cache evictions")
	}
}

// BenchmarkCacheGet 缓存获取性能基准测试
func BenchmarkCacheGet(b *testing.B) {
	diskCache := newThumbCache(func() string { return b.TempDir() })
	cache := NewTieredCache(diskCache, func(key string) ([]byte, error) {
		return []byte("benchmark data"), nil
	})

	// 预热缓存
	cache.Get("benchmark:key")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := cache.Get("benchmark:key")
		if err != nil {
			b.Errorf("Failed to get from cache: %v", err)
		}
	}
}

// BenchmarkConcurrentAccess 并发访问性能基准测试
func BenchmarkConcurrentAccess(b *testing.B) {
	diskCache := newThumbCache(func() string { return b.TempDir() })
	cache := NewTieredCache(diskCache, func(key string) ([]byte, error) {
		return []byte("concurrent benchmark data"), nil
	})

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := cache.Get("concurrent:benchmark")
			if err != nil {
				b.Errorf("Failed to get from cache: %v", err)
			}
		}
	})
}
