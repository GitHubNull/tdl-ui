package services

import (
	"context"
	"sync"
	"time"
)

// SmartCacheWarmer 智能缓存预热器：根据用户浏览模式，智能预加载可能需要的媒体
// 通过分析用户行为模式，预测并预加载用户可能查看的媒体内容
type SmartCacheWarmer struct {
	chatService *ChatService
	tieredCache *TieredCache
	
	// 用户行为跟踪
	mu            sync.RWMutex
	recentDialogs []int64                    // 最近访问的对话列表
	accessPattern map[string]*AccessPattern  // 访问模式统计
	
	// 配置
	maxRecentDialogs int
	preloadEnabled   bool
}

// AccessPattern 访问模式统计
type AccessPattern struct {
	DialogID     int64
	AccessCount  int64
	LastAccess   time.Time
	CommonKinds  map[string]int // 媒体类型访问频率
	TimePattern  []int          // 访问时间模式（小时）
}

// NewSmartCacheWarmer 创建智能缓存预热器
func NewSmartCacheWarmer(chatService *ChatService, tieredCache *TieredCache) *SmartCacheWarmer {
	return &SmartCacheWarmer{
		chatService:      chatService,
		tieredCache:      tieredCache,
		recentDialogs:    make([]int64, 0, 10),
		accessPattern:    make(map[string]*AccessPattern),
		maxRecentDialogs: 10,
		preloadEnabled:   true,
	}
}

// WarmupForDialog 为指定对话预热缓存
func (w *SmartCacheWarmer) WarmupForDialog(dialogID int64, dialogType string, visibleItems []MediaItem) {
	if !w.preloadEnabled {
		return
	}
	
	// 记录访问行为
	w.recordDialogAccess(dialogID, visibleItems)
	
	// 预加载当前可见区域的缩略图
	go w.preloadVisibleThumbs(dialogID, dialogType, visibleItems)
	
	// 基于访问模式预测性预加载
	go w.predictivePreload(dialogID, dialogType)
	
	// 预加载相关对话
	go w.preloadRelatedDialogs(dialogID)
}

// recordDialogAccess 记录对话访问行为
func (w *SmartCacheWarmer) recordDialogAccess(dialogID int64, items []MediaItem) {
	w.mu.Lock()
	defer w.mu.Unlock()
	
	// 更新最近访问对话列表
	w.updateRecentDialogs(dialogID)
	
	// 更新访问模式
	key := w.getDialogKey(dialogID)
	pattern, exists := w.accessPattern[key]
	if !exists {
		pattern = &AccessPattern{
			DialogID:    dialogID,
			CommonKinds: make(map[string]int),
			TimePattern: make([]int, 24), // 24小时
		}
		w.accessPattern[key] = pattern
	}
	
	pattern.AccessCount++
	pattern.LastAccess = time.Now()
	
	// 统计媒体类型偏好
	for _, item := range items {
		pattern.CommonKinds[item.Kind]++
	}
	
	// 记录访问时间模式
	hour := time.Now().Hour()
	pattern.TimePattern[hour]++
}

// updateRecentDialogs 更新最近访问对话列表（LRU）
func (w *SmartCacheWarmer) updateRecentDialogs(dialogID int64) {
	// 移除已存在的对话 ID
	for i, id := range w.recentDialogs {
		if id == dialogID {
			w.recentDialogs = append(w.recentDialogs[:i], w.recentDialogs[i+1:]...)
			break
		}
	}
	
	// 添加到最前面
	w.recentDialogs = append([]int64{dialogID}, w.recentDialogs...)
	
	// 限制列表长度
	if len(w.recentDialogs) > w.maxRecentDialogs {
		w.recentDialogs = w.recentDialogs[:w.maxRecentDialogs]
	}
}

// preloadVisibleThumbs 预加载可见区域的缩略图
func (w *SmartCacheWarmer) preloadVisibleThumbs(dialogID int64, dialogType string, items []MediaItem) {
	if w.chatService.thumbPreloader == nil {
		return
	}
	
	// 预加载前 20 个可见项的缩略图
	endIdx := len(items)
	if endIdx > 20 {
		endIdx = 20
	}
	
	w.chatService.thumbPreloader.PreloadVisibleThumbs(dialogID, dialogType, items, 0, endIdx-1)
}

// predictivePreload 预测性预加载
func (w *SmartCacheWarmer) predictivePreload(dialogID int64, dialogType string) {
	w.mu.RLock()
	pattern, exists := w.accessPattern[w.getDialogKey(dialogID)]
	w.mu.RUnlock()
	
	if !exists || pattern.AccessCount < 3 {
		// 访问次数太少，无法进行有效预测
		return
	}
	
	// 基于用户偏好的媒体类型进行预测性预加载
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		
		// 获取用户最常访问的媒体类型
		preferredKind := w.getPreferredMediaKind(pattern)
		
		// 构建查询条件
		query := MediaQuery{
			DialogID:   dialogID,
			DialogType: dialogType,
			Limit:      50, // 预加载50个媒体项
			Kinds:      []string{preferredKind},
		}
		
		// 异步查询并预加载
		page, err := w.chatService.ListMedia(query)
		if err != nil || page == nil {
			return
		}
		
		// 预加载这些媒体的缩略图
		var messageIDs []int
		for _, item := range page.Items {
			if item.Kind == KindPhoto || item.Kind == KindVideo {
				messageIDs = append(messageIDs, item.MessageID)
			}
		}
		
		if len(messageIDs) > 0 && w.chatService.thumbPreloader != nil {
			w.chatService.thumbPreloader.PreloadThumbs(dialogID, dialogType, messageIDs)
		}
		
		_ = ctx // 避免未使用变量警告
	}()
}

// preloadRelatedDialogs 预加载相关对话
func (w *SmartCacheWarmer) preloadRelatedDialogs(currentDialogID int64) {
	w.mu.RLock()
	relatedDialogs := make([]int64, 0, 3)
	
	// 选择最近访问的其他对话
	for _, dialogID := range w.recentDialogs {
		if dialogID != currentDialogID && len(relatedDialogs) < 3 {
			relatedDialogs = append(relatedDialogs, dialogID)
		}
	}
	w.mu.RUnlock()
	
	// 为相关对话预加载少量媒体
	for _, dialogID := range relatedDialogs {
		go func(id int64) {
			// 这里需要知道对话类型，暂时跳过
			// 实际实现中应该从缓存或数据库中获取对话类型
		}(dialogID)
	}
}

// getPreferredMediaKind 获取用户偏好的媒体类型
func (w *SmartCacheWarmer) getPreferredMediaKind(pattern *AccessPattern) string {
	maxCount := 0
	preferredKind := KindPhoto // 默认偏好图片
	
	for kind, count := range pattern.CommonKinds {
		if count > maxCount {
			maxCount = count
			preferredKind = kind
		}
	}
	
	return preferredKind
}

// getDialogKey 获取对话键
func (w *SmartCacheWarmer) getDialogKey(dialogID int64) string {
	return string(rune(dialogID))
}

// GetAccessPattern 获取访问模式统计
func (w *SmartCacheWarmer) GetAccessPattern(dialogID int64) *AccessPattern {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.accessPattern[w.getDialogKey(dialogID)]
}

// GetRecentDialogs 获取最近访问的对话列表
func (w *SmartCacheWarmer) GetRecentDialogs() []int64 {
	w.mu.RLock()
	defer w.mu.RUnlock()
	result := make([]int64, len(w.recentDialogs))
	copy(result, w.recentDialogs)
	return result
}

// SetPreloadEnabled 设置是否启用预加载
func (w *SmartCacheWarmer) SetPreloadEnabled(enabled bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.preloadEnabled = enabled
}

// IsPreloadEnabled 检查是否启用预加载
func (w *SmartCacheWarmer) IsPreloadEnabled() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.preloadEnabled
}

// ClearPattern 清除指定对话的访问模式
func (w *SmartCacheWarmer) ClearPattern(dialogID int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.accessPattern, w.getDialogKey(dialogID))
}

// ClearAllPatterns 清除所有访问模式
func (w *SmartCacheWarmer) ClearAllPatterns() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.accessPattern = make(map[string]*AccessPattern)
	w.recentDialogs = w.recentDialogs[:0]
}
