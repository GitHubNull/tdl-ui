<template>
  <div 
    ref="containerRef" 
    class="virtual-scroll-container"
    :style="{ height: validatedProps.containerHeight + 'px', overflowY: 'auto' }"
    @scroll="onScroll"
  >
    <div 
      class="virtual-scroll-spacer" 
      :style="{ height: totalHeight + 'px', position: 'relative' }"
    >
      <div
        v-for="item in visibleItems"
        :key="validatedProps.getItemKey(item.data)"
        class="virtual-scroll-item"
        :style="{
          position: 'absolute',
          top: item.offset + 'px',
          left: 0,
          right: 0,
          height: validatedProps.itemHeight + 'px'
        }"
      >
        <slot :item="item.data" :index="item.index" />
      </div>
    </div>
    
    <!-- 加载更多触发器 -->
    <div 
      v-if="validatedProps.hasMore && !validatedProps.loading" 
      ref="loadMoreTrigger"
      class="load-more-trigger"
      :style="{ 
        position: 'absolute',
        top: Math.max(0, totalHeight - 100) + 'px',
        height: '1px',
        width: '100%'
      }"
    />
    
    <!-- 空状态 -->
    <div 
      v-if="validatedProps.items.length === 0" 
      class="virtual-scroll-empty"
      :style="{ 
        height: validatedProps.containerHeight + 'px',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        color: 'var(--p-text-muted-color)'
      }"
    >
      <slot name="empty">
        <span>暂无数据</span>
      </slot>
    </div>
  </div>
</template>

<script setup lang="ts" generic="T">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'

interface VirtualScrollItem<T> {
  data: T
  index: number
  offset: number
}

interface Props {
  items: T[]
  itemHeight: number
  containerHeight: number
  buffer?: number
  hasMore?: boolean
  loading?: boolean
  getItemKey: (item: T) => string | number
}

const props = withDefaults(defineProps<Props>(), {
  buffer: 5,
  hasMore: false,
  loading: false
})

const emit = defineEmits<{
  (e: 'load-more'): void
  (e: 'scroll', scrollTop: number): void
  (e: 'error', error: Error): void
}>()

const containerRef = ref<HTMLElement>()
const loadMoreTrigger = ref<HTMLElement>()
const scrollTop = ref(0)

// 错误处理：验证 props
const validatedProps = computed(() => {
  try {
    // 验证必需参数
    if (!Array.isArray(props.items)) {
      throw new Error('VirtualScroll: items must be an array')
    }
    
    if (typeof props.itemHeight !== 'number' || props.itemHeight <= 0) {
      throw new Error('VirtualScroll: itemHeight must be a positive number')
    }
    
    if (typeof props.containerHeight !== 'number' || props.containerHeight <= 0) {
      throw new Error('VirtualScroll: containerHeight must be a positive number')
    }
    
    if (typeof props.getItemKey !== 'function') {
      throw new Error('VirtualScroll: getItemKey must be a function')
    }
    
    return {
      items: props.items,
      itemHeight: Math.max(1, props.itemHeight), // 最小高度为1
      containerHeight: Math.max(100, props.containerHeight), // 最小容器高度为100
      buffer: Math.max(0, Math.min(20, props.buffer || 5)), // 限制buffer范围
      hasMore: Boolean(props.hasMore),
      loading: Boolean(props.loading),
      getItemKey: props.getItemKey
    }
  } catch (error) {
    emit('error', error as Error)
    // 返回安全的默认值
    return {
      items: [],
      itemHeight: 100,
      containerHeight: 400,
      buffer: 5,
      hasMore: false,
      loading: false,
      getItemKey: (item: T) => String(item)
    }
  }
})

// 计算总高度
const totalHeight = computed(() => {
  try {
    return validatedProps.value.items.length * validatedProps.value.itemHeight
  } catch (error) {
    emit('error', error as Error)
    return 0
  }
})

// 计算可见区域的项目
const visibleItems = computed(() => {
  try {
    const { items, itemHeight, containerHeight, buffer } = validatedProps.value
    
    // 边界情况：空数组
    if (items.length === 0) {
      return []
    }
    
    // 计算可见范围
    const startIndex = Math.max(0, Math.floor(scrollTop.value / itemHeight) - buffer)
    const endIndex = Math.min(
      items.length - 1,
      Math.ceil((scrollTop.value + containerHeight) / itemHeight) + buffer
    )
    
    // 边界情况：索引越界
    if (startIndex > endIndex || startIndex >= items.length) {
      return []
    }
    
    const result: VirtualScrollItem<T>[] = []
    for (let i = startIndex; i <= endIndex; i++) {
      if (i < items.length && items[i] !== undefined) {
        try {
          result.push({
            data: items[i],
            index: i,
            offset: i * itemHeight
          })
        } catch (error) {
          // 单个项目错误不影响其他项目
          console.warn('VirtualScroll: error processing item at index', i, error)
        }
      }
    }
    
    return result
  } catch (error) {
    emit('error', error as Error)
    return []
  }
})

// 滚动事件处理（带防抖）
let scrollTimer: ReturnType<typeof setTimeout> | null = null
function onScroll(event: Event) {
  try {
    const target = event.target as HTMLElement
    if (!target) return
    
    scrollTop.value = Math.max(0, target.scrollTop) // 确保不为负数
    
    // 防抖处理，避免过于频繁的滚动事件
    if (scrollTimer) {
      clearTimeout(scrollTimer)
    }
    
    scrollTimer = setTimeout(() => {
      emit('scroll', scrollTop.value)
    }, 16) // 约60fps
  } catch (error) {
    emit('error', error as Error)
  }
}

// Intersection Observer 用于触发加载更多
let observer: IntersectionObserver | null = null

onMounted(() => {
  try {
    if (loadMoreTrigger.value && containerRef.value) {
      observer = new IntersectionObserver(
        (entries) => {
          try {
            entries.forEach((entry) => {
              if (entry.isIntersecting && validatedProps.value.hasMore && !validatedProps.value.loading) {
                emit('load-more')
              }
            })
          } catch (error) {
            emit('error', error as Error)
          }
        },
        {
          root: containerRef.value,
          rootMargin: '100px'
        }
      )
      observer.observe(loadMoreTrigger.value)
    }
  } catch (error) {
    emit('error', error as Error)
  }
})

onBeforeUnmount(() => {
  try {
    if (observer) {
      observer.disconnect()
      observer = null
    }
    if (scrollTimer) {
      clearTimeout(scrollTimer)
      scrollTimer = null
    }
  } catch (error) {
    console.warn('VirtualScroll: error during cleanup', error)
  }
})

// 监听触发器元素变化
watch(loadMoreTrigger, (newEl, oldEl) => {
  try {
    if (oldEl && observer) {
      observer.unobserve(oldEl)
    }
    if (newEl && observer) {
      observer.observe(newEl)
    }
  } catch (error) {
    emit('error', error as Error)
  }
})

// 滚动到指定位置（带边界检查）
function scrollTo(position: number) {
  try {
    if (containerRef.value) {
      const maxScroll = Math.max(0, totalHeight.value - validatedProps.value.containerHeight)
      const targetPosition = Math.max(0, Math.min(maxScroll, position))
      containerRef.value.scrollTop = targetPosition
    }
  } catch (error) {
    emit('error', error as Error)
  }
}

// 滚动到指定项目（带边界检查）
function scrollToItem(index: number) {
  try {
    const { items, itemHeight } = validatedProps.value
    if (index < 0 || index >= items.length) {
      console.warn('VirtualScroll: scrollToItem index out of bounds', index)
      return
    }
    const position = index * itemHeight
    scrollTo(position)
  } catch (error) {
    emit('error', error as Error)
  }
}

// 获取当前可见范围信息（用于调试和监控）
function getVisibleRange() {
  try {
    const { items, itemHeight, containerHeight } = validatedProps.value
    const startIndex = Math.max(0, Math.floor(scrollTop.value / itemHeight))
    const endIndex = Math.min(
      items.length - 1,
      Math.ceil((scrollTop.value + containerHeight) / itemHeight)
    )
    return { startIndex, endIndex, totalItems: items.length }
  } catch (error) {
    emit('error', error as Error)
    return { startIndex: 0, endIndex: 0, totalItems: 0 }
  }
}

// 暴露方法给父组件
defineExpose({
  scrollTo,
  scrollToItem,
  getVisibleRange,
  containerRef
})
</script>

<style scoped>
.virtual-scroll-container {
  position: relative;
}

.virtual-scroll-spacer {
  width: 100%;
}

.virtual-scroll-item {
  width: 100%;
}

.load-more-trigger {
  pointer-events: none;
}
</style>
