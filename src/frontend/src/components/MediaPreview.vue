<template>
  <Teleport to="body">
    <div v-if="visible" class="mp-overlay">
      <div class="mp-top">
        <span class="mp-name" :title="item?.name">{{ item?.name }}</span>
        <span class="mp-meta">{{ metaText }}</span>
        <Tag v-if="downloaded" value="已下载" severity="success" icon="pi pi-check" />
        <div class="mp-actions">
          <Button
            v-if="downloaded"
            label="打开文件"
            icon="pi pi-external-link"
            size="small"
            severity="secondary"
            @click="item && $emit('open', item)"
          />
          <Button label="下载此文件" icon="pi pi-download" size="small" @click="item && $emit('download', item)" />
          <Button icon="pi pi-times" text rounded severity="contrast" v-tooltip.bottom="'关闭 (Esc)'" @click="close" />
        </div>
      </div>

      <div
        ref="stage"
        class="mp-stage"
        @wheel.prevent="onWheel"
        @dblclick="resetView"
        @mousedown.self="close"
      >
        <template v-if="item">
          <!-- 图片：磁盘缓存预览大图，加载中先垫已有缩略图 -->
          <div v-if="item.kind === 'photo' || item.kind === 'video'" class="mp-img-wrap">
            <img
              v-if="!previewFailed"
              :key="previewSrc"
              :src="previewSrc"
              :style="imgStyle"
              class="mp-img"
              :class="{ grabbing: panning }"
              alt=""
              draggable="false"
              @load="loaded = true"
              @error="previewFailed = true"
              @mousedown.prevent="startPan"
            />
            <img v-if="(!loaded || previewFailed) && thumbSrc" :src="thumbSrc" class="mp-placeholder" alt="" draggable="false" />
            <i v-if="previewFailed && !thumbSrc" :class="kindIcon(item.kind)" class="mp-bigicon" />
            <div v-if="!loaded && !previewFailed" class="mp-loading"><i class="pi pi-spin pi-spinner" /></div>
            <div v-if="item.kind === 'video'" class="mp-video-hint">
              <i class="pi pi-play-circle" /> 视频请下载后观看
            </div>
          </div>

          <!-- 音频 / 文件：大图标 + 元信息 -->
          <div v-else class="mp-file">
            <i :class="kindIcon(item.kind)" class="mp-bigicon" />
            <p class="mp-file-name">{{ item.name }}</p>
            <p class="mp-meta">{{ metaText }}</p>
            <p v-if="item.caption" class="mp-caption">{{ item.caption }}</p>
          </div>
        </template>

        <Button
          class="mp-nav prev"
          icon="pi pi-chevron-left"
          rounded
          severity="contrast"
          :disabled="index <= 0"
          v-tooltip.right="'上一个 (←)'"
          @mousedown.stop
          @click.stop="go(-1)"
        />
        <Button
          class="mp-nav next"
          icon="pi pi-chevron-right"
          rounded
          severity="contrast"
          :disabled="index >= items.length - 1 && !hasMore"
          v-tooltip.left="'下一个 (→)'"
          @mousedown.stop
          @click.stop="go(1)"
        />
      </div>

      <div class="mp-bottom">
        <template v-if="zoomable">
          <Button icon="pi pi-search-minus" text rounded severity="contrast" @click="zoomBy(-0.25)" />
          <span class="mp-zoom">{{ Math.round(zoom * 100) }}%</span>
          <Button icon="pi pi-search-plus" text rounded severity="contrast" @click="zoomBy(0.25)" />
          <Button label="复位" size="small" text severity="contrast" @click="resetView" />
        </template>
        <span class="mp-pos">{{ index + 1 }} / {{ items.length }}{{ hasMore ? '+' : '' }}</span>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import Button from 'primevue/button'
import Tag from 'primevue/tag'

import { previewURL, thumbURL } from '../api'
import { fmtDate, fmtSize, kindIcon } from '../utils/format'
import type { MediaItem } from '../types'

const props = defineProps<{
  visible: boolean
  items: MediaItem[]
  index: number
  dialogType: string
  hasMore: boolean
  /** 已下载消息 ID 集合（可选，用于"已下载"Tag 与打开文件按钮） */
  downloadedIds?: Set<number>
}>()

const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
  (e: 'update:index', v: number): void
  (e: 'download', item: MediaItem): void
  (e: 'open', item: MediaItem): void
  (e: 'load-more'): void
}>()

const item = computed<MediaItem | undefined>(() => props.items[props.index])

const downloaded = computed(() => !!item.value && !!props.downloadedIds?.has(item.value.messageId))

const loaded = ref(false)
const previewFailed = ref(false)

const previewSrc = computed(() =>
  item.value ? previewURL(item.value.dialogId, props.dialogType, item.value.messageId) : '',
)
// 占位：优先列表页已加载的清晰小图（同 URL 命中浏览器缓存），否则内嵌模糊图
const thumbSrc = computed(() => {
  if (!item.value) return ''
  if (item.value.kind === 'photo' || item.value.kind === 'video') {
    return thumbURL(item.value.dialogId, props.dialogType, item.value.messageId)
  }
  return item.value.thumb ?? ''
})

const zoomable = computed(() => item.value?.kind === 'photo' || item.value?.kind === 'video')

const metaText = computed(() => {
  if (!item.value) return ''
  const parts = [fmtSize(item.value.size)]
  if (item.value.width && item.value.height) parts.push(`${item.value.width}×${item.value.height}`)
  parts.push(fmtDate(item.value.date))
  return parts.join(' · ')
})

// ---- 缩放与平移 ----

const zoom = ref(1)
const pan = ref({ x: 0, y: 0 })
const panning = ref(false)
let panStart = { x: 0, y: 0, px: 0, py: 0 }

const imgStyle = computed(() => ({
  transform: `translate(${pan.value.x}px, ${pan.value.y}px) scale(${zoom.value})`,
  transition: panning.value ? 'none' : 'transform 0.15s ease',
  cursor: zoom.value > 1 ? (panning.value ? 'grabbing' : 'grab') : 'default',
}))

function clampZoom(z: number) {
  return Math.min(5, Math.max(0.5, z))
}

function zoomBy(delta: number) {
  zoom.value = clampZoom(zoom.value + delta)
  if (zoom.value <= 1) pan.value = { x: 0, y: 0 }
}

function onWheel(e: WheelEvent) {
  if (!zoomable.value) return
  zoomBy(e.deltaY < 0 ? 0.25 : -0.25)
}

function resetView() {
  zoom.value = 1
  pan.value = { x: 0, y: 0 }
}

function startPan(e: MouseEvent) {
  if (zoom.value <= 1) return
  panning.value = true
  panStart = { x: e.clientX, y: e.clientY, px: pan.value.x, py: pan.value.y }
  window.addEventListener('mousemove', onPanMove)
  window.addEventListener('mouseup', endPan)
}

function onPanMove(e: MouseEvent) {
  if (!panning.value) return
  pan.value = { x: panStart.px + (e.clientX - panStart.x), y: panStart.py + (e.clientY - panStart.y) }
}

function endPan() {
  panning.value = false
  window.removeEventListener('mousemove', onPanMove)
  window.removeEventListener('mouseup', endPan)
}

// ---- 切换与关闭 ----

function close() {
  pendingAdvance = false
  emit('update:visible', false)
}

// FE-26：末项续拉时标记待前进，数据到达后自动切到下一张，避免“按了没反应”
let pendingAdvance = false

function go(delta: number) {
  const next = props.index + delta
  if (next < 0) return
  if (next >= props.items.length) {
    if (props.hasMore) {
      pendingAdvance = true
      emit('load-more') // 到尾部自动续拉，数据到达后自动前进
    }
    return
  }
  pendingAdvance = false
  emit('update:index', next)
  // 预取尾部：临近末尾提前触发加载
  if (props.hasMore && next >= props.items.length - 3) emit('load-more')
}

watch(
  () => props.items.length,
  (len) => {
    if (!props.visible) return
    // FE-26：外部 items 被清空（如登出重置）时自动关闭，避免空遮罩
    if (len === 0) {
      close()
      return
    }
    if (pendingAdvance && props.index + 1 < len) {
      pendingAdvance = false
      emit('update:index', props.index + 1)
    }
  },
)

function onKey(e: KeyboardEvent) {
  if (!props.visible) return
  switch (e.key) {
    case 'Escape':
      close()
      break
    case 'ArrowLeft':
      go(-1)
      break
    case 'ArrowRight':
      go(1)
      break
  }
}

watch(
  () => props.visible,
  (v) => {
    if (v) {
      window.addEventListener('keydown', onKey)
    } else {
      window.removeEventListener('keydown', onKey)
    }
    resetView()
  },
)

// 预览打开状态下整页卸载（如切换路由）时，清理挂在 window 上的监听，避免闭包泄漏
onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKey)
  endPan()
})

// 切换条目时重置加载/缩放状态
watch(
  () => item.value?.messageId,
  () => {
    loaded.value = false
    previewFailed.value = false
    resetView()
  },
)
</script>

<style scoped>
.mp-overlay {
  /* lightbox 暗色遮罩场景：明暗主题下均为黑底白字，属功能性固定配色，豁免主题 token，集中为组件级变量 */
  --mp-overlay-bg: rgb(0 0 0 / 88%);
  --mp-chip-bg: rgb(0 0 0 / 65%);
  --mp-fg: #fff;
  position: fixed;
  inset: 0;
  z-index: 1100;
  display: flex;
  flex-direction: column;
  background: var(--mp-overlay-bg);
  color: var(--mp-fg);
}

.mp-top {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 16px;
  flex-shrink: 0;
}

.mp-name {
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 40%;
  flex-shrink: 1;
  min-width: 0;
}

.mp-meta {
  color: color-mix(in srgb, var(--mp-fg) 65%, transparent);
  font-size: 12px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.mp-actions {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}

.mp-stage {
  position: relative;
  flex: 1;
  min-height: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
}

.mp-img-wrap {
  position: relative;
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  pointer-events: none;
}

.mp-img {
  max-width: 92%;
  max-height: 92%;
  object-fit: contain;
  user-select: none;
  pointer-events: auto;
}

.mp-placeholder {
  position: absolute;
  max-width: 92%;
  max-height: 92%;
  object-fit: contain;
  filter: blur(2px);
  opacity: 0.6;
  pointer-events: none;
}

.mp-loading {
  position: absolute;
  font-size: 2rem;
  color: color-mix(in srgb, var(--mp-fg) 80%, transparent);
}

.mp-video-hint {
  position: absolute;
  bottom: 24px;
  left: 50%;
  transform: translateX(-50%);
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 8px 16px;
  border-radius: 20px;
  background: var(--mp-chip-bg);
  font-size: 13px;
  pointer-events: none;
}

.mp-video-hint .pi {
  font-size: 1.2rem;
}

.mp-file {
  text-align: center;
  max-width: 60%;
}

.mp-bigicon {
  font-size: 5rem;
  color: color-mix(in srgb, var(--mp-fg) 55%, transparent);
}

.mp-file-name {
  margin: 16px 0 4px;
  font-size: 15px;
  font-weight: 500;
  word-break: break-all;
}

.mp-caption {
  color: color-mix(in srgb, var(--mp-fg) 70%, transparent);
  font-size: 13px;
  margin-top: 12px;
  white-space: pre-wrap;
  word-break: break-word;
  max-height: 20vh;
  overflow-y: auto;
}

.mp-nav {
  position: absolute;
  top: 50%;
  transform: translateY(-50%);
}

.mp-nav.prev {
  left: 16px;
}

.mp-nav.next {
  right: 16px;
}

.mp-bottom {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 8px 16px;
  flex-shrink: 0;
}

.mp-zoom {
  font-size: 13px;
  min-width: 44px;
  text-align: center;
}

.mp-pos {
  color: color-mix(in srgb, var(--mp-fg) 65%, transparent);
  font-size: 13px;
  margin-left: 16px;
}
</style>
