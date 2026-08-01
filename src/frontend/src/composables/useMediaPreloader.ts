// 媒体预加载策略 composable：在用户浏览当前媒体时，预加载相邻的媒体文件
// 提升媒体切换的响应速度和用户体验
import { ref, computed } from 'vue'
import { Chat, previewURL } from '../api'
import type { MediaItem } from '../types'

interface PreloadOptions {
  range?: number        // 预加载范围（前后几项）
  enableImages?: boolean // 是否预加载图片
  enableVideos?: boolean // 是否预加载视频元信息
  maxCacheSize?: number  // 最大缓存数量
}

interface PreloadTask {
  key: string
  promise: Promise<void>
  timestamp: number
}

export function useMediaPreloader(options: PreloadOptions = {}) {
  const {
    range = 2,
    enableImages = true,
    enableVideos = true,
    maxCacheSize = 50
  } = options

  // 预加载缓存
  const preloadCache = new Map<string, PreloadTask>()
  const isPreloading = ref(false)

  // 清理过期缓存
  function cleanupCache() {
    const now = Date.now()
    const maxAge = 5 * 60 * 1000 // 5分钟过期
    
    for (const [key, task] of preloadCache.entries()) {
      if (now - task.timestamp > maxAge) {
        preloadCache.delete(key)
      }
    }
    
    // 限制缓存大小
    if (preloadCache.size > maxCacheSize) {
      const entries = Array.from(preloadCache.entries())
      entries.sort((a, b) => a[1].timestamp - b[1].timestamp)
      
      const toDelete = entries.slice(0, entries.length - maxCacheSize)
      for (const [key] of toDelete) {
        preloadCache.delete(key)
      }
    }
  }

  // 预加载相邻媒体
  function preloadAdjacentMedia(
    currentIndex: number, 
    items: MediaItem[], 
    dialogType: string
  ) {
    if (items.length === 0) return
    
    isPreloading.value = true
    cleanupCache()
    
    const startIdx = Math.max(0, currentIndex - range)
    const endIdx = Math.min(items.length - 1, currentIndex + range)
    
    const preloadPromises: Promise<void>[] = []
    
    for (let i = startIdx; i <= endIdx; i++) {
      if (i === currentIndex) continue // 跳过当前项
      
      const item = items[i]
      const key = getPreloadKey(item)
      
      // 检查是否已缓存
      if (preloadCache.has(key)) {
        continue
      }
      
      // 创建预加载任务
      const promise = preloadMediaItem(item, dialogType)
      const task: PreloadTask = {
        key,
        promise,
        timestamp: Date.now()
      }
      
      preloadCache.set(key, task)
      preloadPromises.push(promise)
      
      // 任务完成后清理
      promise.finally(() => {
        // 延迟清理，避免立即删除可能需要的缓存
        setTimeout(() => {
          if (preloadCache.get(key) === task) {
            preloadCache.delete(key)
          }
        }, 30000) // 30秒后清理
      })
    }
    
    // 等待所有预加载完成
    Promise.allSettled(preloadPromises).finally(() => {
      isPreloading.value = false
    })
  }

  // 预加载单个媒体项
  async function preloadMediaItem(item: MediaItem, dialogType: string): Promise<void> {
    try {
      if (item.kind === 'photo' && enableImages) {
        await preloadImage(item, dialogType)
      } else if (item.kind === 'video' && enableVideos) {
        await preloadVideoMeta(item, dialogType)
      }
    } catch (error) {
      // 预加载失败静默处理，不影响正常功能
      console.debug('Media preload failed:', error)
    }
  }

  // 预加载图片
  function preloadImage(item: MediaItem, dialogType: string): Promise<void> {
    return new Promise((resolve, reject) => {
      const img = new Image()
      const src = previewURL(item.dialogId, dialogType, item.messageId)
      
      img.onload = () => resolve()
      img.onerror = () => reject(new Error(`Failed to preload image: ${src}`))
      
      img.src = src
    })
  }

  // 预加载视频元信息
  async function preloadVideoMeta(item: MediaItem, dialogType: string): Promise<void> {
    try {
      // 预加载视频元信息（通过触发视频流的第一字节请求）
      // 这样后端会缓存视频元信息，后续播放时会更快
      const videoUrl = `/media/video?d=${item.dialogId}&m=${item.messageId}&t=${encodeURIComponent(dialogType)}`
      
      // 发送 HEAD 请求来预加载元信息，不下载实际内容
      const response = await fetch(videoUrl, { method: 'HEAD' })
      if (!response.ok) {
        throw new Error(`Video meta preload failed: ${response.status}`)
      }
    } catch (error) {
      // 如果预加载失败，静默处理
      console.debug('Video meta preload failed:', error)
    }
  }

  // 预加载指定范围的媒体
  function preloadRange(
    items: MediaItem[], 
    dialogType: string, 
    startIdx: number, 
    endIdx: number
  ) {
    if (items.length === 0) return
    
    const validStartIdx = Math.max(0, startIdx)
    const validEndIdx = Math.min(items.length - 1, endIdx)
    
    if (validStartIdx > validEndIdx) return
    
    const preloadPromises: Promise<void>[] = []
    
    for (let i = validStartIdx; i <= validEndIdx; i++) {
      const item = items[i]
      const key = getPreloadKey(item)
      
      if (preloadCache.has(key)) {
        continue
      }
      
      const promise = preloadMediaItem(item, dialogType)
      const task: PreloadTask = {
        key,
        promise,
        timestamp: Date.now()
      }
      
      preloadCache.set(key, task)
      preloadPromises.push(promise)
    }
    
    return Promise.allSettled(preloadPromises)
  }

  // 预加载下一页媒体（用于无限滚动）
  function preloadNextPage(items: MediaItem[], dialogType: string, count: number = 10) {
    const startIdx = Math.max(0, items.length - count)
    return preloadRange(items, dialogType, startIdx, items.length - 1)
  }

  // 获取预加载键
  function getPreloadKey(item: MediaItem): string {
    return `${item.dialogId}:${item.messageId}:${item.kind}`
  }

  // 检查是否已预加载
  function isPreloaded(item: MediaItem): boolean {
    const key = getPreloadKey(item)
    return preloadCache.has(key)
  }

  // 获取预加载统计
  const preloadStats = computed(() => ({
    cacheSize: preloadCache.size,
    isPreloading: isPreloading.value,
    maxCacheSize
  }))

  // 清空预加载缓存
  function clearPreloadCache() {
    preloadCache.clear()
  }

  // 取消所有预加载任务
  function cancelAllPreloads() {
    // 注意：无法真正取消 Promise，但可以清空缓存
    preloadCache.clear()
    isPreloading.value = false
  }

  return {
    preloadAdjacentMedia,
    preloadRange,
    preloadNextPage,
    isPreloaded,
    preloadStats,
    clearPreloadCache,
    cancelAllPreloads
  }
}
