// 媒体分页 composable（FE-11 拆分自 ChatsPage）：
// - epoch 代际防乱序（FE-01）：切换对话/重置后在途响应一律丢弃
// - dialogId 为 key 的模块级 LRU 快照缓存（FE-16）：离开页面/切换对话后返回免重拉
import { ref } from 'vue'
import { Chat } from '../api'
import type { MediaItem, MediaQuery } from '../types'

export interface AppliedFilters {
  query: string
  kinds: string[]
  exts: string[]
  minSize: number
  maxSize: number
}

export const emptyFilters = (): AppliedFilters => ({
  query: '',
  kinds: [],
  exts: [],
  minSize: 0,
  maxSize: 0,
})

interface Snapshot {
  items: MediaItem[]
  offset: number
  hasMore: boolean
  applied: AppliedFilters
  jumpOffsetDate: number
}

// 模块级 LRU：跨组件卸载保留最近浏览对话的媒体状态
const cache = new Map<number, Snapshot>()
const CACHE_LIMIT = 5

const PAGE_SIZE = 50
// 连续空页（过滤后无命中）自动续拉上限，避免筛选苛刻时假死
const MAX_EMPTY_PAGES = 3

export function useMediaPager(options: {
  /** 当前对话类型（查询参数用，切换后由调用方保证与 switchTo 的 id 匹配） */
  dialogType: () => string
  /** 加载失败回调（toast 等由调用方决定） */
  onError?: (e: unknown) => void
  /** 一轮加载结束后是否自动继续（如滚动哨兵仍在视口内） */
  autoContinue?: () => boolean
}) {
  const items = ref<MediaItem[]>([])
  const offset = ref(0)
  const hasMore = ref(true)
  const loading = ref(false)
  const failed = ref(false)
  const applied = ref<AppliedFilters>(emptyFilters())
  const jumpOffsetDate = ref(0)

  let currentId = 0
  // 请求代际：每次重置/切换递增，在途请求返回后校验不等则丢弃
  let epoch = 0

  function resetState() {
    epoch++
    items.value = []
    offset.value = 0
    hasMore.value = true
    loading.value = false
    failed.value = false
    jumpOffsetDate.value = 0
  }

  /** 保存当前对话的浏览快照（切换对话或组件卸载时调用） */
  function saveSnapshot() {
    if (!currentId || !items.value.length) return
    cache.delete(currentId)
    cache.set(currentId, {
      items: [...items.value],
      offset: offset.value,
      hasMore: hasMore.value,
      applied: { ...applied.value, kinds: [...applied.value.kinds], exts: [...applied.value.exts] },
      jumpOffsetDate: jumpOffsetDate.value,
    })
    while (cache.size > CACHE_LIMIT) {
      cache.delete(cache.keys().next().value!)
    }
  }

  /** 切换到目标对话；命中缓存返回 true（无需 loadMore），未命中已重置状态 */
  function switchTo(dialogId: number | null): boolean {
    saveSnapshot()
    currentId = dialogId ?? 0
    const snap = dialogId ? cache.get(dialogId) : undefined
    if (snap && dialogId) {
      // LRU touch
      cache.delete(dialogId)
      cache.set(dialogId, snap)
      epoch++
      items.value = [...snap.items]
      offset.value = snap.offset
      hasMore.value = snap.hasMore
      applied.value = { ...snap.applied, kinds: [...snap.applied.kinds], exts: [...snap.applied.exts] }
      jumpOffsetDate.value = snap.jumpOffsetDate
      loading.value = false
      failed.value = false
      return true
    }
    resetState()
    applied.value = emptyFilters()
    return false
  }

  /** 应用筛选条件并重新拉取（保持当前对话） */
  function applyFilters(f: AppliedFilters) {
    applied.value = f
    resetState()
    void loadMore()
  }

  /** 月份跳转：以 unix 秒为起点重新拉取（保持筛选条件） */
  function jumpToDate(unixSec: number) {
    resetState()
    jumpOffsetDate.value = unixSec
    void loadMore()
  }

  function buildQuery(dialogId: number, dialogType: string): MediaQuery {
    return {
      dialogId,
      dialogType,
      offsetId: offset.value,
      offsetDate: jumpOffsetDate.value,
      limit: PAGE_SIZE,
      query: applied.value.query,
      kinds: applied.value.kinds,
      exts: applied.value.exts,
      minSize: applied.value.minSize,
      maxSize: applied.value.maxSize,
    }
  }

  async function loadMore() {
    if (loading.value || !hasMore.value || !currentId) return
    const myEpoch = epoch
    // 固定本轮查询的对话快照，避免循环中途切换对话混入两个对话的查询
    const dialogId = currentId
    const dialogType = options.dialogType()
    loading.value = true
    failed.value = false

    try {
      let emptyPages = 0
      for (;;) {
        const page = await Chat.listMedia(buildQuery(dialogId, dialogType))
        if (myEpoch !== epoch) return // 代际已作废：丢弃过期响应
        items.value.push(...(page.items ?? []))
        offset.value = page.nextOffset
        hasMore.value = page.nextOffset !== 0

        if (!hasMore.value || (page.items?.length ?? 0) > 0 || ++emptyPages >= MAX_EMPTY_PAGES) {
          break
        }
      }
    } catch (e) {
      if (myEpoch !== epoch) return
      failed.value = true
      options.onError?.(e)
    } finally {
      if (myEpoch === epoch) {
        loading.value = false
        // 哨兵仍在视口内（如首屏未铺满）则继续拉取
        if (options.autoContinue?.() && hasMore.value && !failed.value) {
          setTimeout(() => loadMore(), 0)
        }
      }
    }
  }

  return {
    items,
    offset,
    hasMore,
    loading,
    failed,
    applied,
    jumpOffsetDate,
    switchTo,
    saveSnapshot,
    applyFilters,
    jumpToDate,
    loadMore,
  }
}
