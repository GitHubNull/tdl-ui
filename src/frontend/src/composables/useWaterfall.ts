// 瀑布流布局 composable（FE-11 拆分自 ChatsPage）：
// 监听滚动容器宽度，按媒体纵横比计算 grid 跨行数
import { computed, onBeforeUnmount, ref, watch, type Ref } from 'vue'
import type { MediaItem } from '../types'

export type MediaLayout = 'waterfall' | 'grid' | 'timeline'

// FE-24：网格几何常量单一来源——JS 计算与 .media-grid 的 CSS 变量共用（见 gridVars）
/** 网格最小列宽（px），对应 minmax(Xpx, 1fr) */
export const GRID_MIN_COL = 150
/** 网格间距（px） */
export const GRID_GAP = 12
/** 瀑布流行步长基数（px），对应 grid-auto-rows */
export const GRID_ROW = 8
/** 卡片信息区（文件名/大小两行）高度（px） */
const CARD_INFO_H = 52
/** 滚动容器左右 padding 合计（px），对应 .media-grid padding: 16px */
const SCROLL_PADDING_X = 32
/** 纵横比钳制范围，防止极端长图占满整列 */
const RATIO_MIN = 0.5
const RATIO_MAX = 2.2

/** 注入模板 :style 的 CSS 变量，保证 CSS 与 JS 几何参数同源。 */
export const gridVars = {
  '--grid-min-col': `${GRID_MIN_COL}px`,
  '--grid-gap': `${GRID_GAP}px`,
  '--grid-row': `${GRID_ROW}px`,
}

export function useWaterfall(scrollEl: Ref<HTMLElement | null>, layout: Ref<MediaLayout>) {
  const gridWidth = ref(0)
  let scrollRO: ResizeObserver | null = null

  watch(scrollEl, (el) => {
    scrollRO?.disconnect()
    scrollRO = null
    if (el) {
      scrollRO = new ResizeObserver(() => {
        gridWidth.value = el.clientWidth - SCROLL_PADDING_X
      })
      scrollRO.observe(el)
      gridWidth.value = el.clientWidth - SCROLL_PADDING_X
    }
  })

  onBeforeUnmount(() => {
    scrollRO?.disconnect()
  })

  // 与 minmax(GRID_MIN_COL, 1fr) + GRID_GAP 对应的实际列宽
  const colWidth = computed(() => {
    const w = gridWidth.value
    if (w <= 0) return GRID_MIN_COL
    const n = Math.max(1, Math.floor((w + GRID_GAP) / (GRID_MIN_COL + GRID_GAP)))
    return (w - (n - 1) * GRID_GAP) / n
  })

  function cardStyle(it: MediaItem) {
    if (layout.value === 'grid') return undefined
    const w = it.width ?? 0
    const h = it.height ?? 0
    const ratio = w > 0 && h > 0 ? Math.min(RATIO_MAX, Math.max(RATIO_MIN, h / w)) : 1
    // 行步长 = grid-auto-rows + gap
    const span = Math.ceil((colWidth.value * ratio + CARD_INFO_H + GRID_GAP) / (GRID_ROW + GRID_GAP))
    return { gridRowEnd: `span ${span}` }
  }

  return { cardStyle }
}
