<template>
  <Teleport to="body">
    <div v-if="visible" class="mp-overlay">
      <div class="mp-top">
        <span class="mp-name" :title="item?.name">{{ item?.name }}</span>
        <span class="mp-meta">{{ metaText }}</span>
        <div class="mp-actions">
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
import { computed, ref, watch } from 'vue'
import Button from 'primevue/button'

import { previewURL, thumbURL } from '../api'
import type { MediaItem } from '../types'

const props = defineProps<{
  visible: boolean
  items: MediaItem[]
  index: number
  dialogType: string
  hasMore: boolean
}>()

const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
  (e: 'update:index', v: number): void
  (e: 'download', item: MediaItem): void
  (e: 'load-more'): void
}>()

const item = computed<MediaItem | undefined>(() => props.items[props.index])

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
  emit('update:visible', false)
}

function go(delta: number) {
  const next = props.index + delta
  if (next < 0) return
  if (next >= props.items.length) {
    if (props.hasMore) emit('load-more') // 到尾部自动续拉，数据到达后可再切
    return
  }
  emit('update:index', next)
  // 预取尾部：临近末尾提前触发加载
  if (props.hasMore && next >= props.items.length - 3) emit('load-more')
}

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

// 切换条目时重置加载/缩放状态
watch(
  () => item.value?.messageId,
  () => {
    loaded.value = false
    previewFailed.value = false
    resetView()
  },
)

// ---- 展示辅助 ----

function kindIcon(k: string) {
  switch (k) {
    case 'video':
      return 'pi pi-video'
    case 'photo':
      return 'pi pi-image'
    case 'audio':
      return 'pi pi-volume-up'
    default:
      return 'pi pi-file'
  }
}

function fmtSize(n: number) {
  if (!n || n < 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  let v = n
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(v >= 100 || i === 0 ? 0 : 1)} ${units[i]}`
}

function fmtDate(unix: number) {
  if (!unix) return '-'
  const d = new Date(unix * 1000)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}
</script>

<style scoped>
.mp-overlay {
  position: fixed;
  inset: 0;
  z-index: 1100;
  display: flex;
  flex-direction: column;
  background: rgb(0 0 0 / 88%);
  color: #fff;
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
}

.mp-meta {
  color: rgb(255 255 255 / 65%);
  font-size: 12px;
  white-space: nowrap;
}

.mp-actions {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: 8px;
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
  color: rgb(255 255 255 / 80%);
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
  background: rgb(0 0 0 / 65%);
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
  color: rgb(255 255 255 / 55%);
}

.mp-file-name {
  margin: 16px 0 4px;
  font-size: 15px;
  font-weight: 500;
  word-break: break-all;
}

.mp-caption {
  color: rgb(255 255 255 / 70%);
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
  color: rgb(255 255 255 / 65%);
  font-size: 13px;
  margin-left: 16px;
}
</style>
