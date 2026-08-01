package services

import (
	"context"
	"sync"
	"time"
)

// ThumbPreloader 缩略图批量预加载器：在媒体列表加载时预加载可见区域的缩略图。
// 使用信号量控制并发数，避免触发 Telegram API 限制。
type ThumbPreloader struct {
	chatService *ChatService
	semaphore   chan struct{} // 控制并发数
	
	// 预加载统计
	stats *PreloadStats
}

// PreloadStats 预加载统计信息
type PreloadStats struct {
	mu           sync.RWMutex
	TotalRequests int64
	SuccessCount  int64
	FailureCount  int64
	CacheHits     int64
}

// NewThumbPreloader 创建缩略图预加载器
func NewThumbPreloader(chatService *ChatService) *ThumbPreloader {
	return &ThumbPreloader{
		chatService: chatService,
		semaphore:   make(chan struct{}, 5), // 最多5个并发预加载
		stats:       &PreloadStats{},
	}
}

// PreloadThumbs 批量预加载指定消息的缩略图
// 异步执行，不阻塞主流程；失败静默处理，不影响正常功能
func (p *ThumbPreloader) PreloadThumbs(dialogID int64, dialogType string, messageIDs []int) {
	if len(messageIDs) == 0 {
		return
	}

	// 在后台 goroutine 中执行预加载
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		var wg sync.WaitGroup
		for _, msgID := range messageIDs {
			// 检查上下文是否已取消
			select {
			case <-ctx.Done():
				return
			default:
			}

			wg.Add(1)
			go func(messageID int) {
				defer wg.Done()
				
				// 获取信号量，控制并发数
				select {
				case p.semaphore <- struct{}{}:
					defer func() { <-p.semaphore }()
				case <-ctx.Done():
					return
				}

				p.preloadSingleThumb(ctx, dialogID, dialogType, messageID)
			}(msgID)
		}
		wg.Wait()
	}()
}

// PreloadVisibleThumbs 预加载可见区域的缩略图（智能预加载）
func (p *ThumbPreloader) PreloadVisibleThumbs(dialogID int64, dialogType string, items []MediaItem, startIdx, endIdx int) {
	if startIdx < 0 {
		startIdx = 0
	}
	if endIdx >= len(items) {
		endIdx = len(items) - 1
	}
	if startIdx > endIdx {
		return
	}

	// 提取需要预加载的消息 ID
	var messageIDs []int
	for i := startIdx; i <= endIdx; i++ {
		item := items[i]
		// 只预加载图片和视频的缩略图
		if item.Kind == KindPhoto || item.Kind == KindVideo {
			messageIDs = append(messageIDs, item.MessageID)
		}
	}

	p.PreloadThumbs(dialogID, dialogType, messageIDs)
}

// preloadSingleThumb 预加载单个缩略图
func (p *ThumbPreloader) preloadSingleThumb(ctx context.Context, dialogID int64, dialogType string, messageID int) {
	p.stats.mu.Lock()
	p.stats.TotalRequests++
	p.stats.mu.Unlock()

	// 检查 chatService 和 thumbs 是否为 nil
	if p.chatService == nil || p.chatService.thumbs == nil {
		p.stats.mu.Lock()
		p.stats.FailureCount++
		p.stats.mu.Unlock()
		return
	}

	// 检查是否已缓存（快速路径）
	kind := cacheKindThumb
	cachePath := p.chatService.thumbs.path(kind, dialogID, messageID)
	if _, err := p.chatService.thumbs.Get(kind, dialogID, messageID, func() ([]byte, error) {
		// 如果缓存存在，这个 fetch 函数不会被调用
		return nil, nil
	}); err == nil {
		// 缓存命中
		p.stats.mu.Lock()
		p.stats.CacheHits++
		p.stats.mu.Unlock()
		_ = cachePath // 避免未使用变量警告
		return
	}

	// 执行预加载
	_, err := p.chatService.thumbJPEG(dialogID, dialogType, messageID, false)
	
	p.stats.mu.Lock()
	if err != nil {
		p.stats.FailureCount++
	} else {
		p.stats.SuccessCount++
	}
	p.stats.mu.Unlock()
}

// GetStats 获取预加载统计信息
func (p *ThumbPreloader) GetStats() PreloadStats {
	p.stats.mu.RLock()
	defer p.stats.mu.RUnlock()
	return PreloadStats{
		TotalRequests: p.stats.TotalRequests,
		SuccessCount:  p.stats.SuccessCount,
		FailureCount:  p.stats.FailureCount,
		CacheHits:     p.stats.CacheHits,
	}
}

// ResetStats 重置统计信息
func (p *ThumbPreloader) ResetStats() {
	p.stats.mu.Lock()
	defer p.stats.mu.Unlock()
	p.stats.TotalRequests = 0
	p.stats.SuccessCount = 0
	p.stats.FailureCount = 0
	p.stats.CacheHits = 0
}
