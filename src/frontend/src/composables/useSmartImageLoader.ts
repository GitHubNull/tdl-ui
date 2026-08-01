// 智能图片懒加载 composable：基于 Intersection Observer 的高性能图片加载
// 支持预加载、错误重试、内存缓存等功能
import { ref, onBeforeUnmount } from 'vue'

interface ImageLoadOptions {
  rootMargin?: string
  threshold?: number
  enableCache?: boolean
  maxRetries?: number
  retryDelay?: number
}

interface LoadState {
  loading: boolean
  error: boolean
  loaded: boolean
  retryCount: number
}

export function useSmartImageLoader(options: ImageLoadOptions = {}) {
  const {
    rootMargin = '50px',
    threshold = 0.1,
    enableCache = true,
    maxRetries = 3,
    retryDelay = 1000
  } = options

  // 图片缓存
  const imageCache = new Map<string, HTMLImageElement>()
  // 加载状态跟踪
  const loadStates = new Map<string, LoadState>()
  // 进行中的加载 Promise
  const loadingPromises = new Map<string, Promise<void>>()
  // Intersection Observer
  const observer = ref<IntersectionObserver | null>(null)

  // 初始化 Observer
  function initObserver() {
    if (observer.value) return

    observer.value = new IntersectionObserver(
      (entries) => {
        entries.forEach((entry) => {
          if (entry.isIntersecting) {
            const img = entry.target as HTMLImageElement
            const src = img.dataset.src
            if (src) {
              loadImage(img, src)
              observer.value?.unobserve(img)
            }
          }
        })
      },
      {
        rootMargin,
        threshold
      }
    )
  }

  // 加载图片
  async function loadImage(img: HTMLImageElement, src: string): Promise<void> {
    // 检查缓存
    if (enableCache && imageCache.has(src)) {
      const cachedImg = imageCache.get(src)!
      img.src = cachedImg.src
      img.classList.add('loaded')
      updateLoadState(src, { loaded: true })
      return
    }

    // 防止重复加载
    if (loadingPromises.has(src)) {
      return loadingPromises.get(src)
    }

    // 初始化加载状态
    if (!loadStates.has(src)) {
      loadStates.set(src, {
        loading: false,
        error: false,
        loaded: false,
        retryCount: 0
      })
    }

    const state = loadStates.get(src)!
    if (state.loading) return

    state.loading = true
    state.error = false

    const promise = performLoad(img, src)
    loadingPromises.set(src, promise)

    try {
      await promise
      state.loaded = true
      state.loading = false
    } catch (error) {
      state.error = true
      state.loading = false
      console.warn('Image load failed:', src, error)
      
      // 自动重试
      if (state.retryCount < maxRetries) {
        state.retryCount++
        setTimeout(() => {
          state.error = false
          loadImage(img, src)
        }, retryDelay * state.retryCount)
      }
    } finally {
      loadingPromises.delete(src)
    }
  }

  // 执行实际的图片加载
  function performLoad(img: HTMLImageElement, src: string): Promise<void> {
    return new Promise((resolve, reject) => {
      const image = new Image()
      
      image.onload = () => {
        // 缓存图片
        if (enableCache) {
          imageCache.set(src, image)
        }
        
        // 更新目标图片
        img.src = src
        img.classList.add('loaded')
        img.classList.remove('error')
        
        resolve()
      }
      
      image.onerror = () => {
        img.classList.add('error')
        img.classList.remove('loaded')
        reject(new Error(`Failed to load image: ${src}`))
      }
      
      // 设置图片源开始加载
      image.src = src
    })
  }

  // 更新加载状态
  function updateLoadState(src: string, updates: Partial<LoadState>) {
    const state = loadStates.get(src)
    if (state) {
      Object.assign(state, updates)
    }
  }

  // 观察图片元素
  function observe(img: HTMLImageElement) {
    initObserver()
    if (observer.value) {
      observer.value.observe(img)
    }
  }

  // 停止观察图片元素
  function unobserve(img: HTMLImageElement) {
    if (observer.value) {
      observer.value.unobserve(img)
    }
  }

  // 预加载图片（不绑定到特定元素）
  async function preloadImage(src: string): Promise<void> {
    if (enableCache && imageCache.has(src)) {
      return
    }

    if (loadingPromises.has(src)) {
      return loadingPromises.get(src)
    }

    const promise = new Promise<void>((resolve, reject) => {
      const image = new Image()
      image.onload = () => {
        if (enableCache) {
          imageCache.set(src, image)
        }
        resolve()
      }
      image.onerror = () => reject(new Error(`Failed to preload image: ${src}`))
      image.src = src
    })

    loadingPromises.set(src, promise)
    
    try {
      await promise
    } finally {
      loadingPromises.delete(src)
    }
  }

  // 批量预加载图片
  async function preloadImages(srcs: string[]): Promise<void> {
    const promises = srcs.map(src => preloadImage(src).catch(() => {
      // 静默处理预加载失败
    }))
    await Promise.allSettled(promises)
  }

  // 获取加载状态
  function getLoadState(src: string): LoadState | undefined {
    return loadStates.get(src)
  }

  // 清理缓存
  function clearCache() {
    imageCache.clear()
    loadStates.clear()
    loadingPromises.clear()
  }

  // 清理过期缓存（可选）
  function cleanupCache(maxAge: number = 30 * 60 * 1000) { // 默认30分钟
    const now = Date.now()
    for (const [src, img] of imageCache.entries()) {
      // 这里可以添加时间戳检查，暂时简化处理
      // 实际实现中需要在缓存时记录时间戳
    }
  }

  // 组件卸载时清理
  onBeforeUnmount(() => {
    if (observer.value) {
      observer.value.disconnect()
      observer.value = null
    }
  })

  return {
    observe,
    unobserve,
    loadImage,
    preloadImage,
    preloadImages,
    getLoadState,
    clearCache,
    cleanupCache
  }
}
