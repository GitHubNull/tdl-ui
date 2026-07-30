<template>
  <div
    ref="rootEl"
    class="vp"
    :class="{ 'vp-idle': !controlsVisible }"
    @mousedown.stop
    @mousemove="pokeControls"
    @mouseleave="onMouseLeave"
  >
    <video
      ref="videoEl"
      :src="src"
      :poster="poster || undefined"
      class="vp-video"
      preload="auto"
      playsinline
      @click="togglePlay"
      @dblclick.stop="toggleFullscreen"
      @loadedmetadata="syncDuration"
      @durationchange="syncDuration"
      @timeupdate="onTimeUpdate"
      @progress="refreshBuffered"
      @play="playing = true"
      @pause="onPause"
      @waiting="buffering = true"
      @playing="buffering = false"
      @canplay="onCanPlay"
      @seeked="seeking = false"
      @volumechange="syncVolume"
      @error="onError"
    />

    <div v-if="buffering" class="vp-loading"><i class="pi pi-spin pi-spinner" /></div>

    <!-- 自定义控制栏（PrimeVue Button/Slider），鼠标静止自动隐藏，暂停时常显 -->
    <div class="vp-controls" @click.stop @dblclick.stop>
      <div class="vp-progress">
        <!-- buffered 区段（Slider 轨道下层） -->
        <div class="vp-buffered">
          <span
            v-for="(seg, i) in bufferedSegs"
            :key="i"
            class="vp-buffered-seg"
            :style="{ left: seg.left, width: seg.width }"
          />
        </div>
        <Slider
          v-model="progressValue"
          class="vp-slider"
          :min="0"
          :max="duration || 0.1"
          :step="0.1"
          :disabled="!duration"
          @change="onSeekChange"
          @slideend="onSeekEnd"
        />
      </div>

      <div class="vp-bar">
        <Button
          :icon="playing ? 'pi pi-pause' : 'pi pi-play'"
          text
          rounded
          severity="contrast"
          v-tooltip.top="playing ? '暂停 (空格)' : '播放 (空格)'"
          @click="togglePlay"
        />
        <span class="vp-time">{{ fmtDuration(currentTime) }} / {{ fmtDuration(duration) }}</span>

        <span class="vp-spacer" />

        <Button
          :icon="muted || volumeValue === 0 ? 'pi pi-volume-off' : 'pi pi-volume-up'"
          text
          rounded
          severity="contrast"
          v-tooltip.top="muted ? '取消静音 (M)' : '静音 (M)'"
          @click="toggleMute"
        />
        <Slider v-model="volumeValue" class="vp-volume" :min="0" :max="100" :step="1" @change="onVolumeChange" />

        <Button
          :icon="fullscreen ? 'pi pi-window-minimize' : 'pi pi-window-maximize'"
          text
          rounded
          severity="contrast"
          v-tooltip.top="fullscreen ? '退出全屏 (F)' : '全屏 (F)'"
          @click="toggleFullscreen"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import Slider from 'primevue/slider'

import { fmtDuration } from '../utils/format'

defineProps<{
  /** 视频字节流地址（/media/local 或 /media/video） */
  src: string
  /** 封面占位图（可选） */
  poster?: string
}>()

const emit = defineEmits<{
  /** 播放失败，透传 HTMLMediaElement.error.code */
  (e: 'error', code?: number): void
}>()

// 同会话内跨条目记忆音量/静音（组件按 :key 重建时沿用上次设置）
let lastVolume = 1
let lastMuted = false

const rootEl = ref<HTMLDivElement | null>(null)
const videoEl = ref<HTMLVideoElement | null>(null)

const playing = ref(false)
const buffering = ref(true)
const duration = ref(0)
const currentTime = ref(0)
const progressValue = ref(0)
const volumeValue = ref(Math.round(lastVolume * 100))
const muted = ref(lastMuted)
const fullscreen = ref(false)
const bufferedSegs = ref<{ left: string; width: string }[]>([])

// ---- 播放 / 缓冲 ----

// canplay 在每次 seek/缓冲完成后都会重复触发，仅首次用于自动播放，避免覆盖用户的暂停操作
let autoStarted = false

function onCanPlay() {
  buffering.value = false
  if (autoStarted) return
  autoStarted = true
  // 自动播放被策略拦下时保持暂停态，控制栏仍可手动播放
  void videoEl.value?.play().catch(() => {})
}

function onPause() {
  playing.value = false
  pokeControls() // 暂停即常显控制栏
}

function togglePlay() {
  const el = videoEl.value
  if (!el) return
  if (el.paused) {
    void el.play().catch(() => {})
  } else {
    el.pause()
  }
}

function onError() {
  buffering.value = false
  emit('error', videoEl.value?.error?.code)
}

// ---- 进度与 seek ----

// 拖拽/点击进度条期间置 seeking，屏蔽 timeupdate 对滑块的回写抖动
const seeking = ref(false)
// change 事件在拖拽中连续触发，经短防抖收敛后再落 currentTime，避免在线流被 seek 请求打爆
let seekTimer: ReturnType<typeof setTimeout> | undefined

function syncDuration() {
  const d = videoEl.value?.duration
  duration.value = Number.isFinite(d) ? (d as number) : 0
}

function onTimeUpdate() {
  const el = videoEl.value
  if (!el) return
  currentTime.value = el.currentTime
  if (!seeking.value) progressValue.value = el.currentTime
  refreshBuffered()
}

function onSeekChange(v: number | number[]) {
  const t = Array.isArray(v) ? v[0] : v
  seeking.value = true
  currentTime.value = t
  if (seekTimer) clearTimeout(seekTimer)
  seekTimer = setTimeout(() => applySeek(t), 200)
}

function onSeekEnd(e: { value: number | number[] }) {
  const t = Array.isArray(e.value) ? e.value[0] : e.value
  if (seekTimer) clearTimeout(seekTimer)
  applySeek(t)
}

function applySeek(t: number) {
  const el = videoEl.value
  if (!el || !duration.value) return
  el.currentTime = Math.min(Math.max(t, 0), duration.value)
  // seeking 由 video 的 seeked 事件复位
}

function seekBy(sec: number) {
  const el = videoEl.value
  if (!el || !duration.value) return
  const t = Math.min(Math.max(el.currentTime + sec, 0), duration.value)
  el.currentTime = t
  progressValue.value = t
  currentTime.value = t
  pokeControls()
}

function refreshBuffered() {
  const el = videoEl.value
  if (!el || !duration.value) {
    bufferedSegs.value = []
    return
  }
  const segs: { left: string; width: string }[] = []
  for (let i = 0; i < el.buffered.length; i++) {
    const s = el.buffered.start(i) / duration.value
    const w = (el.buffered.end(i) - el.buffered.start(i)) / duration.value
    segs.push({ left: `${(s * 100).toFixed(2)}%`, width: `${(w * 100).toFixed(2)}%` })
  }
  bufferedSegs.value = segs
}

// ---- 音量 ----

function syncVolume() {
  const el = videoEl.value
  if (!el) return
  muted.value = el.muted
  volumeValue.value = Math.round(el.volume * 100)
  lastVolume = el.volume
  lastMuted = el.muted
}

function onVolumeChange(v: number | number[]) {
  const el = videoEl.value
  if (!el) return
  const vol = (Array.isArray(v) ? v[0] : v) / 100
  el.volume = Math.min(Math.max(vol, 0), 1)
  if (vol > 0) el.muted = false
}

function volumeBy(delta: number) {
  const el = videoEl.value
  if (!el) return
  el.volume = Math.min(Math.max(el.volume + delta, 0), 1)
  if (el.volume > 0) el.muted = false
  pokeControls()
}

function toggleMute() {
  const el = videoEl.value
  if (!el) return
  el.muted = !el.muted
}

// ---- 全屏 ----

function toggleFullscreen() {
  if (document.fullscreenElement) {
    void document.exitFullscreen().catch(() => {})
  } else {
    void rootEl.value?.requestFullscreen().catch(() => {})
  }
}

function onFullscreenChange() {
  fullscreen.value = document.fullscreenElement === rootEl.value
}

// ---- 控制栏自动隐藏（播放中鼠标静止 3s 隐藏，暂停时常显） ----

const controlsVisible = ref(true)
let hideTimer: ReturnType<typeof setTimeout> | undefined

function pokeControls() {
  controlsVisible.value = true
  if (hideTimer) clearTimeout(hideTimer)
  hideTimer = setTimeout(() => {
    if (playing.value) controlsVisible.value = false
  }, 3000)
}

function onMouseLeave() {
  if (playing.value) controlsVisible.value = false
}

// ---- 生命周期 ----

// 断开字节流：后端 /media/video 的 ctx 随之取消，不留后台音轨与无效拉取
function teardown() {
  const el = videoEl.value
  if (!el) return
  el.pause()
  el.removeAttribute('src')
  el.load()
}

onMounted(() => {
  document.addEventListener('fullscreenchange', onFullscreenChange)
  const el = videoEl.value
  if (el) {
    el.volume = lastVolume
    el.muted = lastMuted
  }
  pokeControls()
})

onBeforeUnmount(() => {
  document.removeEventListener('fullscreenchange', onFullscreenChange)
  if (hideTimer) clearTimeout(hideTimer)
  if (seekTimer) clearTimeout(seekTimer)
  teardown()
})

defineExpose({ togglePlay, seekBy, volumeBy, toggleMute, toggleFullscreen, teardown })
</script>

<style scoped>
.vp {
  /* lightbox 覆层内固定暗底，与 MediaPreview 同一主题豁免口径 */
  --vp-bg: #000;
  --vp-fg: #fff;
  position: relative;
  max-width: 92%;
  max-height: 92%;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--vp-bg);
  border-radius: 6px;
  overflow: hidden;
}

.vp:fullscreen {
  max-width: 100%;
  max-height: 100%;
  width: 100%;
  height: 100%;
  border-radius: 0;
}

.vp-video {
  max-width: 100%;
  max-height: 100%;
  display: block;
  outline: none;
}

.vp:fullscreen .vp-video {
  width: 100%;
  height: 100%;
}

.vp-loading {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 2rem;
  color: color-mix(in srgb, var(--vp-fg) 80%, transparent);
  pointer-events: none;
}

.vp-controls {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  padding: 20px 12px 6px;
  background: linear-gradient(to top, rgb(0 0 0 / 78%), rgb(0 0 0 / 0%));
  color: var(--vp-fg);
  transition: opacity 0.25s ease;
}

.vp-idle .vp-controls {
  opacity: 0;
  pointer-events: none;
}

.vp-idle {
  cursor: none;
}

.vp-progress {
  position: relative;
  padding: 4px 0;
}

.vp-buffered {
  position: absolute;
  left: 0;
  right: 0;
  top: 50%;
  height: 4px;
  transform: translateY(-50%);
  border-radius: 2px;
  overflow: hidden;
  pointer-events: none;
}

.vp-buffered-seg {
  position: absolute;
  top: 0;
  height: 100%;
  background: color-mix(in srgb, var(--vp-fg) 32%, transparent);
  border-radius: 2px;
}

.vp-slider {
  width: 100%;
}

/* Slider 轨道半透明化，露出下层 buffered 区段条（提升优先级覆盖主题背景） */
.vp-slider.p-slider {
  background: color-mix(in srgb, var(--vp-fg) 18%, transparent);
}

.vp-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 4px;
}

.vp-time {
  font-size: 12px;
  font-variant-numeric: tabular-nums;
  color: color-mix(in srgb, var(--vp-fg) 85%, transparent);
  white-space: nowrap;
}

.vp-spacer {
  flex: 1;
}

.vp-volume {
  width: 90px;
  flex-shrink: 0;
}

/* 窄窗口：播放器占满舞台，音量滑条收窄 */
@media (max-width: 900px) {
  .vp {
    max-width: 100%;
    max-height: 100%;
  }

  .vp-volume {
    width: 64px;
  }
}
</style>
