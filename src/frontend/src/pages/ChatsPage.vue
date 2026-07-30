<template>
  <div class="page page-table chats-split">
    <Message v-if="!auth.loggedIn" severity="warn" class="mb-16">
      <span>尚未登录账号，请先登录后再查看对话。</span>
      <Button label="去登录" size="small" class="ml-8" @click="router.push('/login')" />
    </Message>

    <div class="split-body">
      <!-- 左面板：对话列表 -->
      <DialogListPanel :selected-id="selectedId" @select="selectDialog" />

      <!-- 右面板：媒体内容 -->
      <section class="media-panel panel-card">
        <div v-if="!selectedId" class="empty-state media-empty">
          <img class="empty-illustration" :src="emptyChats" alt="" draggable="false" />
          <p>从左侧选择一个对话，浏览并下载其中的媒体文件</p>
        </div>

        <template v-else>
          <div class="media-header">
            <div class="media-title">
              <h2 :title="selectedTitle">{{ selectedTitle }}</h2>
              <Tag :value="typeLabel(selectedType)" :severity="typeSeverity(selectedType)" />
            </div>
            <div class="media-select-bar">
              <template v-if="selectedMsgs.size">
                <span class="count-hint">已选 {{ selectedMsgs.size }} 项</span>
                <Button label="清除" size="small" severity="secondary" text @click="clearSelection" />
              </template>
              <Button
                v-else
                label="全选本页"
                size="small"
                severity="secondary"
                text
                :disabled="!items.length"
                @click="selectAllLoaded"
              />
              <Button
                label="下载选中"
                icon="pi pi-download"
                size="small"
                :disabled="!selectedMsgs.size"
                :badge="selectedMsgs.size ? String(selectedMsgs.size) : undefined"
                @click="downloadSelected"
              />
            </div>
          </div>

          <MediaToolbar
            v-model:layout="layout"
            v-model:jump-month="jumpMonth"
            :applied="applied"
            @apply="applyFilters"
          />

          <div class="media-body">
            <div ref="scrollEl" class="media-scroll">
              <section
                v-for="g in displayGroups"
                :key="g.key"
                class="month-section"
                :ref="(el) => registerMonth(g.key, el)"
              >
                <h3 v-if="layout === 'timeline'" class="month-title">{{ g.label }}</h3>
                <div class="media-grid" :class="{ waterfall: layout !== 'grid' }" :style="gridVars">
                  <div
                    v-for="item in g.items"
                    :key="item.messageId"
                    class="media-card"
                    :class="{ selected: selectedMsgs.has(item.messageId) }"
                    :style="cardStyle(item)"
                    @click="toggleSelect(item)"
                    @dblclick="onCardDblClick(item)"
                  >
                    <div class="thumb" @click.stop="onThumbClick(item)" @dblclick.stop>
                      <img
                        v-if="hasThumb(item)"
                        :src="thumbURL(item.dialogId, selectedType, item.messageId)"
                        :style="item.thumb ? { backgroundImage: `url(${item.thumb})` } : undefined"
                        loading="lazy"
                        alt=""
                        draggable="false"
                        @error="thumbFailed.add(itemKey(item))"
                      />
                      <i v-else :class="kindIcon(item.kind)" class="thumb-icon" />
                      <i v-if="item.kind === 'video' && hasThumb(item)" class="pi pi-play-circle play-badge" />

                      <Checkbox
                        class="card-check"
                        :model-value="selectedMsgs.has(item.messageId)"
                        binary
                        @click.stop
                        @update:model-value="() => toggleSelect(item)"
                      />
                      <Button
                        class="card-dl"
                        icon="pi pi-download"
                        size="small"
                        rounded
                        v-tooltip.top="'下载此文件'"
                        @click.stop="downloadOne(item)"
                      />

                      <span v-if="fileStateOf(item)" class="dl-state" :class="fileStateOf(item)!.state">
                        <i v-if="fileStateOf(item)!.state === 'downloading'" class="pi pi-spin pi-spinner" />
                        <i v-else-if="fileStateOf(item)!.state === 'done'" class="pi pi-check" />
                        <i v-else class="pi pi-times" />
                        <template v-if="fileStateOf(item)!.state === 'downloading'">下载中 {{ fileStateOf(item)!.pct }}%</template>
                      </span>
                      <span
                        v-else-if="downloadedMap.has(item.messageId)"
                        class="dl-state done"
                        v-tooltip.top="'已下载，双击打开'"
                      >
                        <i class="pi pi-check" /> 已下载
                      </span>
                    </div>
                    <div class="card-info">
                      <span class="card-name" :title="item.name">{{ item.name }}</span>
                      <span class="card-meta">{{ fmtSize(item.size) }} · {{ extOf(item.name) }} · {{ fmtDate(item.date) }}</span>
                    </div>
                  </div>
                </div>
              </section>

              <div v-if="!items.length && !loading" class="empty-state small">
                <i class="pi pi-images" />
                <p>没有匹配的媒体文件</p>
              </div>

              <div ref="sentinel" class="scroll-sentinel" />

              <div class="table-status">
                <span>已加载 {{ items.length }} 条</span>
                <span v-if="loading" class="status-loading"><i class="pi pi-spin pi-spinner" /> 加载中…</span>
                <span v-else-if="failed" class="status-error">
                  上次加载失败
                  <Button label="重试" size="small" text @click="loadMore" />
                </span>
                <span v-else-if="!hasMore && items.length">没有更多了</span>
              </div>
            </div>

            <!-- 月份索引条（已加载月份锚点） -->
            <nav v-if="layout === 'timeline' && monthGroups.length" class="month-rail">
              <button
                v-for="g in monthGroups"
                :key="g.key"
                class="month-rail-item"
                :title="g.label"
                @click="scrollToMonth(g.key)"
              >
                {{ g.key }}
              </button>
            </nav>
          </div>
        </template>
      </section>
    </div>

    <NewTaskDialog v-model:visible="dlVisible" :selection="dlSelection" @created="onTaskCreated" />
    <ConfirmDialog />
    <MediaPreview
      v-model:visible="previewVisible"
      v-model:index="previewIndex"
      :items="items"
      :dialog-type="selectedType"
      :has-more="hasMore"
      :downloaded-ids="downloadedIds"
      @download="downloadOne"
      @open="openDownloaded"
      @load-more="loadMore"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import emptyChats from '../assets/illustrations/empty-chats.svg'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import ConfirmDialog from 'primevue/confirmdialog'
import Message from 'primevue/message'
import Tag from 'primevue/tag'

import DialogListPanel from '../components/DialogListPanel.vue'
import MediaPreview from '../components/MediaPreview.vue'
import MediaToolbar from '../components/MediaToolbar.vue'
import NewTaskDialog from '../components/NewTaskDialog.vue'
import { EVENT_TASK_FILE, on, thumbURL, Download } from '../api'
import type { DialogView, FileEvent, MediaItem } from '../types'
import { useMediaPager, type AppliedFilters } from '../composables/useMediaPager'
import { useWaterfall, gridVars, type MediaLayout } from '../composables/useWaterfall'
import { extOf, fmtDate, fmtSize, kindIcon, pad, typeLabel, typeSeverity } from '../utils/format'
import { useAuthStore } from '../stores/auth'
import { useChatsStore } from '../stores/chats'
import { useTasksStore } from '../stores/tasks'

const auth = useAuthStore()
const chats = useChatsStore()
const tasksStore = useTasksStore()
const route = useRoute()
const router = useRouter()
const toast = useToast()
const confirm = useConfirm()

// ---- 对话选中 ----

const selectedId = ref<number | null>(null)
const selectedDialog = computed(() => (selectedId.value ? chats.byId(selectedId.value) : undefined))
// 深链兜底：store 未命中时回退路由 query 里的 type/title
const selectedType = computed(() => selectedDialog.value?.type ?? String(route.query.type ?? ''))
const selectedTitle = computed(
  () => selectedDialog.value?.title ?? String(route.query.title ?? `对话 ${selectedId.value}`),
)

function selectDialog(d: DialogView) {
  if (selectedId.value === d.id) return
  selectedId.value = d.id
  router.replace({ path: `/chats/${d.id}`, query: { type: d.type, title: d.title } })
}

watch(selectedId, (id) => {
  clearSelection()
  jumpMonth.value = null
  thumbFailed.clear()
  refreshDownloaded(id)
  restoreFileStates()
  const restored = switchTo(id)
  if (!restored) loadMore()
})

// ---- 右面板：媒体分页（useMediaPager 内置 epoch 防乱序 + LRU 缓存） ----

const {
  items,
  hasMore,
  loading,
  failed,
  applied,
  switchTo,
  saveSnapshot,
  applyFilters: pagerApplyFilters,
  jumpToDate,
  loadMore,
} = useMediaPager({
  dialogType: () => selectedType.value,
  onError: (e: any) =>
    toast.add({ severity: 'error', summary: '加载媒体失败', detail: String(e?.message ?? e), life: 5000 }),
  autoContinue: () => sentinelVisible,
})

function applyFilters(f: AppliedFilters) {
  // FE-20：重新查询从最新开始，同步清空月份跳转显示值（也使重选同一月可再次触发）
  jumpMonth.value = null
  pagerApplyFilters(f)
}

// ---- 无限滚动哨兵 ----

/** 哨兵提前触发距离：距可视区 600px 即开始预加载下一页 */
const SENTINEL_ROOT_MARGIN = '600px'

const sentinel = ref<HTMLElement | null>(null)
let sentinelVisible = false
let sentinelObserver: IntersectionObserver | null = null

watch(sentinel, (el, old) => {
  if (old && sentinelObserver) sentinelObserver.unobserve(old)
  if (el && sentinelObserver) sentinelObserver.observe(el)
})

// ---- 布局切换与月份分组 ----

const storedLayout = localStorage.getItem('chats.mediaLayout')
const layout = ref<MediaLayout>(storedLayout === 'waterfall' || storedLayout === 'timeline' ? storedLayout : 'grid')
watch(layout, (v) => localStorage.setItem('chats.mediaLayout', v))

// 月份跳转：以目标月的下月 1 日 0 点为 OffsetDate，后端解析为消息 ID 后走既有分页
const jumpMonth = ref<Date | null>(null)

watch(jumpMonth, (d) => {
  if (!d) return
  jumpToDate(Math.floor(new Date(d.getFullYear(), d.getMonth() + 1, 1).getTime() / 1000))
})

interface MonthGroup {
  key: string
  label: string
  items: MediaItem[]
}

// 按月份分组（Map 合并，防止跨页乱序产生重复 key）
const monthGroups = computed<MonthGroup[]>(() => {
  const map = new Map<string, MonthGroup>()
  const out: MonthGroup[] = []
  for (const it of items.value) {
    const d = new Date(it.date * 1000)
    const key = `${d.getFullYear()}-${pad(d.getMonth() + 1)}`
    let g = map.get(key)
    if (!g) {
      g = { key, label: `${d.getFullYear()} 年 ${d.getMonth() + 1} 月`, items: [] }
      map.set(key, g)
      out.push(g)
    }
    g.items.push(it)
  }
  return out
})

const displayGroups = computed<MonthGroup[]>(() =>
  layout.value === 'timeline' ? monthGroups.value : [{ key: 'all', label: '', items: items.value }],
)

const monthEls = new Map<string, HTMLElement>()

function registerMonth(key: string, el: unknown) {
  if (el) monthEls.set(key, el as HTMLElement)
  else monthEls.delete(key)
}

function scrollToMonth(key: string) {
  monthEls.get(key)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

// ---- 瀑布流：按纵横比计算 grid 跨行 ----

const scrollEl = ref<HTMLElement | null>(null)
const { cardStyle } = useWaterfall(scrollEl, layout)

// ---- HTTP 缩略图（磁盘缓存 + 浏览器缓存接管并发） ----

const thumbFailed = reactive(new Set<string>())

function itemKey(it: MediaItem) {
  return `${it.dialogId}:${it.messageId}`
}

function hasThumb(it: MediaItem) {
  if (thumbFailed.has(itemKey(it))) return false
  return it.kind === 'photo' || it.kind === 'video' || !!it.thumb
}

// ---- Lightbox 预览 ----

const previewVisible = ref(false)
const previewIndex = ref(0)

function openPreview(it: MediaItem) {
  const i = items.value.findIndex((x) => x.messageId === it.messageId)
  if (i < 0) return
  previewIndex.value = i
  previewVisible.value = true
}

// ---- 多选与下载 ----

const selectedMsgs = ref(new Set<number>())
const dlVisible = ref(false)
const dlSelection = ref<{ dialogId: number; dialogType: string; title: string; messageIds: number[] } | null>(null)

function toggleSelect(it: MediaItem) {
  const next = new Set(selectedMsgs.value)
  if (next.has(it.messageId)) {
    next.delete(it.messageId)
  } else {
    next.add(it.messageId)
  }
  selectedMsgs.value = next
}

function selectAllLoaded() {
  selectedMsgs.value = new Set(items.value.map((it) => it.messageId))
}

function clearSelection() {
  selectedMsgs.value = new Set()
}

function openDownload(messageIds: number[]) {
  if (!selectedId.value || !messageIds.length) return
  dlSelection.value = {
    dialogId: selectedId.value,
    dialogType: selectedType.value,
    title: selectedTitle.value,
    messageIds,
  }
  dlVisible.value = true
}

function downloadOne(it: MediaItem) {
  if (downloadedMap.has(it.messageId)) {
    confirm.require({
      header: '重新下载',
      message: '该文件已下载，是否重新下载？',
      icon: 'pi pi-exclamation-triangle',
      acceptProps: { label: '重新下载' },
      rejectProps: { label: '取消', severity: 'secondary', outlined: true },
      accept: () => openDownload([it.messageId]),
    })
    return
  }
  openDownload([it.messageId])
}

function downloadSelected() {
  const ids = [...selectedMsgs.value]
  const dupCount = ids.filter((id) => downloadedMap.has(id)).length
  if (dupCount > 0) {
    confirm.require({
      header: '重新下载',
      message: `选中文件中有 ${dupCount} 个已下载，是否重新下载？`,
      icon: 'pi pi-exclamation-triangle',
      acceptProps: { label: '重新下载' },
      rejectProps: { label: '取消', severity: 'secondary', outlined: true },
      accept: () => openDownload(ids),
    })
    return
  }
  openDownload(ids)
}

function onTaskCreated() {
  clearSelection()
  toast.add({ severity: 'success', summary: '下载任务已创建', detail: '可到「下载」页查看进度', life: 3000 })
}

// ---- 已下载标记与双击打开 ----

/** 当前对话已下载消息：messageId → 本地文件路径 */
const downloadedMap = reactive(new Map<number, string>())
const downloadedIds = computed(() => new Set(downloadedMap.keys()))

/** 切对话时重拉已下载列表（失败静默，不阻塞媒体加载） */
async function refreshDownloaded(id: number | null) {
  downloadedMap.clear()
  if (!id || !auth.loggedIn) return
  try {
    const list = await Download.listDownloadedMessages(id)
    if (selectedId.value !== id) return // 返回时已切走，丢弃防串对话
    for (const f of list ?? []) downloadedMap.set(f.messageId, f.path)
  } catch {
    /* 非 Wails 环境或查询失败静默 */
  }
}

/** 双击已下载卡片：调系统默认程序打开文件 */
async function openDownloaded(it: MediaItem) {
  if (!downloadedMap.has(it.messageId)) return
  try {
    await Download.openDownloadedFile(it.dialogId, it.messageId)
  } catch (e: any) {
    // 磁盘文件已被删：同步摘掉标记，避免持续误导
    downloadedMap.delete(it.messageId)
    toast.add({ severity: 'warn', summary: '打开文件失败', detail: String(e?.message ?? e), life: 5000 })
  }
}

function onCardDblClick(it: MediaItem) {
  // 卡片单击是 toggleSelect，双击两次 toggle 净效果为零，仅需处理打开
  openDownloaded(it)
}

// 缩略图区域：单击开预览会遮住第二击，对已下载项用延时区分单击/双击
const DBLCLICK_DELAY_MS = 250
let thumbClickTimer: ReturnType<typeof setTimeout> | null = null
let thumbClickMsgId = 0

function onThumbClick(it: MediaItem) {
  if (!downloadedMap.has(it.messageId)) {
    openPreview(it)
    return
  }
  if (thumbClickTimer && thumbClickMsgId === it.messageId) {
    clearTimeout(thumbClickTimer)
    thumbClickTimer = null
    openDownloaded(it)
    return
  }
  if (thumbClickTimer) clearTimeout(thumbClickTimer)
  thumbClickMsgId = it.messageId
  thumbClickTimer = setTimeout(() => {
    thumbClickTimer = null
    openPreview(it)
  }, DBLCLICK_DELAY_MS)
}

// ---- 下载状态回填（task:file 事件 → 卡片角标） ----

/** 完成角标展示时长，到期后从集合移除 */
const DONE_BADGE_TTL_MS = 5000

const fileStates = reactive(new Map<string, { state: string; pct: number }>())
let offTaskFile: (() => void) | null = null

function fileStateOf(it: MediaItem) {
  return fileStates.get(itemKey(it))
}

/** 从 tasks store 回填当前对话进行中的下载角标（路由切走再回来后恢复） */
function restoreFileStates() {
  fileStates.clear()
  if (!selectedId.value) return
  for (const byFile of Object.values(tasksStore.files)) {
    for (const ev of Object.values(byFile)) {
      if (ev.dialogId !== selectedId.value || ev.state !== 'downloading') continue
      const pct = ev.total > 0 ? Math.round((ev.downloaded / ev.total) * 100) : 0
      fileStates.set(`${ev.dialogId}:${ev.messageId}`, { state: ev.state, pct })
    }
  }
}

// ---- 生命周期 ----

onMounted(() => {
  sentinelObserver = new IntersectionObserver(
    (entries) => {
      sentinelVisible = entries[0]?.isIntersecting ?? false
      if (sentinelVisible) loadMore()
    },
    { rootMargin: SENTINEL_ROOT_MARGIN },
  )
  if (sentinel.value) sentinelObserver.observe(sentinel.value)

  offTaskFile = on<FileEvent>(EVENT_TASK_FILE, (ev) => {
    if (!ev.dialogId || !ev.messageId) return
    const key = `${ev.dialogId}:${ev.messageId}`
    const pct = ev.total > 0 ? Math.round((ev.downloaded / ev.total) * 100) : 0
    fileStates.set(key, { state: ev.state, pct })
    if (ev.state === 'done') {
      // 完成即刻登记"已下载"，角标短暂展示后自然退化为常驻 chip
      if (ev.dialogId === selectedId.value && ev.path) {
        downloadedMap.set(ev.messageId, ev.path)
      }
      // 完成角标短暂展示后移除，避免长会话下集合无限增长
      setTimeout(() => {
        if (fileStates.get(key)?.state === 'done') fileStates.delete(key)
      }, DONE_BADGE_TTL_MS)
    }
  })

  // 深链 /chats/:id 预选对话
  const pid = Number(route.params.id)
  if (pid) selectedId.value = pid
})

// 深链与历史前进后退：/chats/:id 变化时同步选中
//（selectDialog 的 router.replace 因 id 相同不会回环触发）
watch(
  () => route.params.id,
  (v) => {
    const pid = Number(v)
    if (pid && pid !== selectedId.value) selectedId.value = pid
  },
)

onBeforeUnmount(() => {
  saveSnapshot() // 离开页面保留当前对话浏览进度（LRU 缓存）
  sentinelObserver?.disconnect()
  offTaskFile?.()
  if (thumbClickTimer) clearTimeout(thumbClickTimer)
})

// 登出后清空选中与媒体（对话列表由 DialogListPanel 负责）
watch(
  () => auth.loggedIn,
  (v) => {
    if (!v) {
      selectedId.value = null
    }
  },
)
</script>

<style scoped>
.mb-16 {
  margin-bottom: 16px;
}

.ml-8 {
  margin-left: 8px;
}

.split-body {
  flex: 1;
  min-height: 0;
  display: flex;
  gap: 16px;
}

/* ---- 右面板 ---- */
.media-panel {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  padding: 0;
  overflow: hidden;
}

.media-empty {
  margin: auto;
}

.media-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 12px 16px;
  border-bottom: 1px solid var(--p-surface-200);
}

.app-dark .media-header {
  border-bottom-color: var(--p-surface-700);
}

.media-title {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.media-title h2 {
  margin: 0;
  font-size: 1.1rem;
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.media-select-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}

.count-hint {
  color: var(--p-text-muted-color);
  font-size: 12px;
}

.media-body {
  flex: 1;
  min-height: 0;
  display: flex;
  overflow: hidden;
}

.media-scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
}

.media-grid {
  display: grid;
  /* 几何参数由 useWaterfall.gridVars 注入，与 JS 跨行计算同源（FE-24） */
  grid-template-columns: repeat(auto-fill, minmax(var(--grid-min-col), 1fr));
  gap: var(--grid-gap);
  padding: 16px;
}

/* 瀑布流：小行步 + 按纵横比跨行，DOM 顺序保持时间倒序 */
.media-grid.waterfall {
  grid-auto-rows: var(--grid-row);
}

.waterfall .media-card {
  display: flex;
  flex-direction: column;
}

.waterfall .thumb {
  flex: 1;
  aspect-ratio: auto;
}

/* 时间流：月份标题吸顶 */
.month-title {
  position: sticky;
  top: 0;
  z-index: 2;
  margin: 0;
  padding: 8px 16px;
  font-size: 14px;
  font-weight: 600;
  background: var(--p-surface-0);
}

.app-dark .month-title {
  background: var(--p-surface-900);
}

/* 右侧月份索引条 */
.month-rail {
  width: 72px;
  flex-shrink: 0;
  overflow-y: auto;
  border-left: 1px solid var(--p-surface-200);
  display: flex;
  flex-direction: column;
  padding: 8px 4px;
  gap: 2px;
}

.app-dark .month-rail {
  border-left-color: var(--p-surface-700);
}

.month-rail-item {
  border: none;
  background: transparent;
  color: var(--p-text-muted-color);
  font-size: 12px;
  padding: 4px 6px;
  border-radius: 6px;
  cursor: pointer;
  white-space: nowrap;
  transition: background 0.15s ease, color 0.15s ease;
}

.month-rail-item:hover {
  background: var(--p-surface-100);
  color: var(--p-text-color);
}

.app-dark .month-rail-item:hover {
  background: var(--p-surface-800);
}

/* ---- 媒体卡片 ---- */
.media-card {
  /* 遮罩/徽标叠加在任意缩略图上，需固定黑白配色，豁免主题 token，集中为组件级变量 */
  --mc-overlay-fg: rgb(255 255 255 / 90%);
  --mc-overlay-solid-fg: #fff;
  --mc-overlay-bg: rgb(0 0 0 / 55%);
  --mc-overlay-shadow: rgb(0 0 0 / 50%);
  --mc-hover-shadow: rgb(0 0 0 / 8%);
  border: 1px solid var(--p-surface-200);
  border-radius: 10px;
  overflow: hidden;
  cursor: pointer;
  background: var(--p-surface-0);
  transition: border-color 0.15s ease, box-shadow 0.15s ease;
}

.app-dark .media-card {
  border-color: var(--p-surface-700);
  background: var(--p-surface-900);
}

.media-card:hover {
  border-color: var(--p-primary-color);
  box-shadow: 0 2px 8px var(--mc-hover-shadow);
}

.media-card.selected {
  border-color: var(--p-primary-color);
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--p-primary-color) 40%, transparent);
}

.thumb {
  position: relative;
  aspect-ratio: 1;
  background: var(--p-surface-100);
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
}

.app-dark .thumb {
  background: var(--p-surface-800);
}

.thumb img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  /* 内嵌模糊图作背景占位，HTTP 清晰图加载完成后自然覆盖 */
  background-size: cover;
  background-position: center;
}

.thumb-icon {
  font-size: 2rem;
  color: var(--p-surface-400);
}

.play-badge {
  position: absolute;
  left: 50%;
  top: 50%;
  transform: translate(-50%, -50%);
  font-size: 2rem;
  color: var(--mc-overlay-fg);
  text-shadow: 0 1px 4px var(--mc-overlay-shadow);
  pointer-events: none;
}

.card-check {
  position: absolute;
  top: 6px;
  left: 6px;
  opacity: 0;
  transition: opacity 0.15s ease;
}

.media-card:hover .card-check,
.media-card.selected .card-check {
  opacity: 1;
}

.card-dl {
  position: absolute;
  top: 4px;
  right: 4px;
  width: 30px;
  height: 30px;
  opacity: 0;
  transition: opacity 0.15s ease;
}

.media-card:hover .card-dl {
  opacity: 1;
}

.dl-state {
  position: absolute;
  right: 6px;
  bottom: 6px;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px 8px;
  border-radius: 10px;
  font-size: 11px;
  color: var(--mc-overlay-solid-fg);
  background: var(--mc-overlay-bg);
}

.dl-state.done {
  background: var(--p-green-500);
}

.dl-state.failed {
  background: var(--p-red-500);
}

.card-info {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 8px 10px;
}

.card-name {
  font-size: 13px;
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.card-meta {
  color: var(--p-text-muted-color);
  font-size: 11px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.scroll-sentinel {
  height: 1px;
}

.empty-state.small {
  padding: 48px 24px;
}

.empty-state.small .pi {
  font-size: 2rem;
  margin-bottom: 12px;
}

.status-loading {
  color: var(--p-primary-color);
}

.status-error {
  color: var(--p-red-500);
  display: inline-flex;
  align-items: center;
  gap: 4px;
}
</style>
