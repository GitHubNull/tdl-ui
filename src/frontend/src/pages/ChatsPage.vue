<template>
  <div class="page page-table chats-split">
    <Message v-if="!auth.loggedIn" severity="warn" class="mb-16">
      <span>尚未登录账号，请先登录后再查看对话。</span>
      <Button label="去登录" size="small" class="ml-8" @click="router.push('/login')" />
    </Message>

    <Message v-else-if="chats.error" severity="error" class="mb-16">
      <span>加载对话失败：{{ chats.error }}</span>
      <Button label="重试" size="small" class="ml-8" :loading="chats.loading" @click="refresh" />
    </Message>

    <div class="split-body">
      <!-- 左面板：对话列表 -->
      <aside class="dialog-panel panel-card">
        <div class="dialog-toolbar">
          <div class="search-box grow">
            <InputText v-model="search" placeholder="搜索对话" class="w-full" />
            <i
              class="search-icon"
              :class="search ? 'pi pi-times clearable' : 'pi pi-search'"
              @click="search && (search = '')"
            />
          </div>
          <Button
            icon="pi pi-refresh"
            size="small"
            severity="secondary"
            text
            rounded
            :loading="chats.loading"
            v-tooltip.top="'刷新对话列表'"
            @click="refresh"
          />
        </div>
        <div class="dialog-toolbar filters">
          <SelectButton
            v-model="typeFilter"
            :options="typeFilterOptions"
            option-label="label"
            option-value="value"
            :allow-empty="false"
            size="small"
          />
          <span class="spacer" />
          <Button
            label=".*"
            size="small"
            :severity="useRegex ? 'primary' : 'secondary'"
            :outlined="!useRegex"
            v-tooltip.top="'正则表达式'"
            @click="useRegex = !useRegex"
          />
          <Button
            :icon="sortBy === 'recent' ? 'pi pi-sort-amount-down' : 'pi pi-sort-alpha-down'"
            size="small"
            severity="secondary"
            text
            rounded
            v-tooltip.top="sortBy === 'recent' ? '按最近消息排序（点击切换按标题）' : '按标题排序（点击切换按最近消息）'"
            @click="sortBy = sortBy === 'recent' ? 'title' : 'recent'"
          />
        </div>
        <span v-if="regexInvalid" class="regex-error">正则表达式无效</span>

        <VirtualScroller :items="filteredDialogs" :item-size="60" class="dialog-list">
          <template #item="{ item }">
            <div
              class="dialog-row"
              :class="{ active: item.id === selectedId }"
              role="button"
              @click="selectDialog(item)"
            >
              <div class="dialog-avatar" :class="`t-${item.type}`">
                <i :class="typeIcon(item.type)" />
              </div>
              <div class="dialog-main">
                <div class="dialog-line">
                  <span class="dialog-title" :title="item.title">{{ item.title }}</span>
                  <span class="dialog-time">{{ fmtShortTime(item.lastMessageAt) }}</span>
                </div>
                <div class="dialog-line">
                  <span class="dialog-sub">{{ item.username ? '@' + item.username : typeLabel(item.type) }}</span>
                  <Badge v-if="item.unreadCount" :value="item.unreadCount" size="small" />
                </div>
              </div>
            </div>
          </template>
        </VirtualScroller>
        <div v-if="!filteredDialogs.length" class="empty-state small">
          <template v-if="chats.loading">
            <i class="pi pi-spin pi-spinner" />
            <p>正在加载对话…</p>
          </template>
          <template v-else>
            <i class="pi pi-comments" />
            <p>{{ chats.loaded ? '没有匹配的对话' : '暂无对话' }}</p>
          </template>
        </div>
        <div class="dialog-count">{{ filteredDialogs.length }} / {{ chats.dialogs.length }} 个对话</div>
      </aside>

      <!-- 右面板：媒体内容 -->
      <section class="media-panel panel-card">
        <div v-if="!selectedId" class="empty-state media-empty">
          <i class="pi pi-images" />
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

          <div class="table-toolbar">
            <div class="search-box">
              <InputText v-model="queryText" placeholder="搜索文件名或标题" class="search-input" @keyup.enter="applyFilters" />
              <i
                class="search-icon"
                :class="queryText ? 'pi pi-times clearable' : 'pi pi-search'"
                @click="queryText && (queryText = '')"
              />
            </div>
            <MultiSelect
              v-model="kinds"
              :options="kindOptions"
              option-label="label"
              option-value="value"
              placeholder="类型"
              :max-selected-labels="2"
              selected-items-label="{0} 类"
              class="w-140"
            />
            <InputText v-model="extsText" placeholder="扩展名: mp4, jpg" class="w-140" @keyup.enter="applyFilters" />
            <InputNumber v-model="minMB" placeholder="最小 MB" :min="0" class="w-100" @keyup.enter="applyFilters" />
            <span class="sep">-</span>
            <InputNumber v-model="maxMB" placeholder="最大 MB" :min="0" class="w-100" @keyup.enter="applyFilters" />
            <Button label="查询" icon="pi pi-filter" size="small" @click="applyFilters" />
            <Button label="重置" icon="pi pi-filter-slash" size="small" severity="secondary" outlined @click="resetFilters" />
            <span class="spacer-flex" />
            <SelectButton
              v-model="layout"
              :options="layoutOptions"
              option-value="value"
              :allow-empty="false"
              size="small"
            >
              <template #option="{ option }">
                <i :class="option.icon" v-tooltip.top="option.label" />
              </template>
            </SelectButton>
            <DatePicker
              v-if="layout === 'timeline'"
              v-model="jumpMonth"
              view="month"
              date-format="yy-mm"
              placeholder="跳转月份"
              show-icon
              class="w-140"
            />
          </div>

          <div class="media-body">
            <div ref="scrollEl" class="media-scroll">
              <section
                v-for="g in displayGroups"
                :key="g.key"
                class="month-section"
                :ref="(el) => registerMonth(g.key, el)"
              >
                <h3 v-if="layout === 'timeline'" class="month-title">{{ g.label }}</h3>
                <div class="media-grid" :class="{ waterfall: layout !== 'grid' }">
                  <div
                    v-for="item in g.items"
                    :key="item.messageId"
                    class="media-card"
                    :class="{ selected: selectedMsgs.has(item.messageId) }"
                    :style="cardStyle(item)"
                    @click="toggleSelect(item)"
                  >
                    <div class="thumb" @click.stop="openPreview(item)">
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
                        <template v-if="fileStateOf(item)!.state === 'downloading'">{{ fileStateOf(item)!.pct }}%</template>
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
    <MediaPreview
      v-model:visible="previewVisible"
      v-model:index="previewIndex"
      :items="items"
      :dialog-type="selectedType"
      :has-more="hasMore"
      @download="downloadOne"
      @load-more="loadMore"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useToast } from 'primevue/usetoast'
import Badge from 'primevue/badge'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import DatePicker from 'primevue/datepicker'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import MultiSelect from 'primevue/multiselect'
import SelectButton from 'primevue/selectbutton'
import Tag from 'primevue/tag'
import VirtualScroller from 'primevue/virtualscroller'

import MediaPreview from '../components/MediaPreview.vue'
import NewTaskDialog from '../components/NewTaskDialog.vue'
import { Chat, EVENT_TASK_FILE, on, thumbURL } from '../api'
import type { DialogView, FileEvent, MediaItem, MediaQuery } from '../types'
import { useAuthStore } from '../stores/auth'
import { useChatsStore } from '../stores/chats'

const auth = useAuthStore()
const chats = useChatsStore()
const route = useRoute()
const router = useRouter()
const toast = useToast()

// ---- 左面板：对话列表 ----

const search = ref('')
const useRegex = ref(false)
const caseSensitive = ref(false)
const typeFilter = ref<'all' | 'private' | 'group' | 'channel'>('all')
const sortBy = ref<'recent' | 'title'>('recent')

const typeFilterOptions = [
  { label: '全部', value: 'all' },
  { label: '私聊', value: 'private' },
  { label: '群组', value: 'group' },
  { label: '频道', value: 'channel' },
]

// 编译正则：undefined 表示非法（无副作用，供模板提示）
const regex = computed(() => {
  if (!useRegex.value || !search.value.trim()) return null
  try {
    return new RegExp(search.value.trim(), caseSensitive.value ? '' : 'i')
  } catch {
    return undefined
  }
})
const regexInvalid = computed(() => useRegex.value && search.value.trim() !== '' && regex.value === undefined)

const filteredDialogs = computed(() => {
  let list = chats.dialogs
  if (typeFilter.value !== 'all') {
    list = list.filter((d) => d.type === typeFilter.value)
  }

  const q = search.value.trim()
  if (q) {
    if (useRegex.value) {
      const re = regex.value
      if (re) list = list.filter((d) => re.test(d.title) || re.test(d.username ?? ''))
    } else {
      const needle = q.toLowerCase()
      list = list.filter(
        (d) => d.title.toLowerCase().includes(needle) || (d.username ?? '').toLowerCase().includes(needle),
      )
    }
  }

  const sorted = [...list]
  if (sortBy.value === 'recent') {
    sorted.sort((a, b) => (b.lastMessageAt ?? 0) - (a.lastMessageAt ?? 0))
  } else {
    sorted.sort((a, b) => a.title.localeCompare(b.title, 'zh'))
  }
  return sorted
})

async function refresh() {
  try {
    await chats.refresh()
  } catch (e: any) {
    toast.add({ severity: 'error', summary: '加载对话失败', detail: String(e?.message ?? e), life: 5000 })
  }
}

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

watch(selectedId, () => {
  clearSelection()
  resetMedia()
  loadMore()
})

// ---- 右面板：媒体分页查询 ----

const PAGE_SIZE = 50
// 连续空页（过滤后无命中）自动续拉上限，避免筛选苛刻时假死
const MAX_EMPTY_PAGES = 3

const items = ref<MediaItem[]>([])
const offset = ref(0)
const hasMore = ref(true)
const loading = ref(false)
const failed = ref(false)

// 筛选条件（应用后生效，与输入解耦）
const queryText = ref('')
const kinds = ref<string[]>([])
const extsText = ref('')
const minMB = ref<number | null>(null)
const maxMB = ref<number | null>(null)

interface AppliedFilters {
  query: string
  kinds: string[]
  exts: string[]
  minSize: number
  maxSize: number
}

const applied = ref<AppliedFilters>({ query: '', kinds: [], exts: [], minSize: 0, maxSize: 0 })

const kindOptions = [
  { label: '视频', value: 'video' },
  { label: '图片', value: 'photo' },
  { label: '音频', value: 'audio' },
  { label: '文件', value: 'file' },
]

function resetMedia() {
  items.value = []
  offset.value = 0
  hasMore.value = true
  failed.value = false
  jumpOffsetDate.value = 0
}

async function loadMore() {
  if (loading.value || !hasMore.value || !selectedId.value) return
  loading.value = true
  failed.value = false

  try {
    let emptyPages = 0
    for (;;) {
      const page = await Chat.listMedia(buildQuery())
      items.value.push(...(page.items ?? []))
      offset.value = page.nextOffset
      hasMore.value = page.nextOffset !== 0

      if (!hasMore.value || (page.items?.length ?? 0) > 0 || ++emptyPages >= MAX_EMPTY_PAGES) {
        break
      }
    }
  } catch (e: any) {
    failed.value = true
    toast.add({ severity: 'error', summary: '加载媒体失败', detail: String(e?.message ?? e), life: 5000 })
  } finally {
    loading.value = false
    // 哨兵仍在视口内（如首屏未铺满）则继续拉取
    if (sentinelVisible && hasMore.value && !failed.value) {
      setTimeout(() => loadMore(), 0)
    }
  }
}

function buildQuery(): MediaQuery {
  return {
    dialogId: selectedId.value!,
    dialogType: selectedType.value,
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

function applyFilters() {
  applied.value = {
    query: queryText.value.trim(),
    kinds: [...kinds.value],
    exts: parseExts(extsText.value),
    minSize: toBytes(minMB.value),
    maxSize: toBytes(maxMB.value),
  }
  resetMedia()
  loadMore()
}

function resetFilters() {
  queryText.value = ''
  kinds.value = []
  extsText.value = ''
  minMB.value = null
  maxMB.value = null
  applyFilters()
}

function parseExts(text: string): string[] {
  return text
    .split(/[,，\s]+/)
    .map((s) => s.trim().replace(/^\./, '').toLowerCase())
    .filter(Boolean)
}

function toBytes(mb: number | null): number {
  return mb && mb > 0 ? Math.round(mb * 1024 * 1024) : 0
}

// ---- 无限滚动哨兵 ----

const sentinel = ref<HTMLElement | null>(null)
let sentinelVisible = false
let sentinelObserver: IntersectionObserver | null = null

watch(sentinel, (el, old) => {
  if (old && sentinelObserver) sentinelObserver.unobserve(old)
  if (el && sentinelObserver) sentinelObserver.observe(el)
})

// ---- 布局切换与月份分组 ----

type MediaLayout = 'waterfall' | 'grid' | 'timeline'

const storedLayout = localStorage.getItem('chats.mediaLayout')
const layout = ref<MediaLayout>(storedLayout === 'waterfall' || storedLayout === 'timeline' ? storedLayout : 'grid')
watch(layout, (v) => localStorage.setItem('chats.mediaLayout', v))

const layoutOptions = [
  { label: '瀑布流', value: 'waterfall', icon: 'pi pi-th-large' },
  { label: '网格', value: 'grid', icon: 'pi pi-table' },
  { label: '时间流', value: 'timeline', icon: 'pi pi-calendar' },
]

// 月份跳转：以目标月的下月 1 日 0 点为 OffsetDate，后端解析为消息 ID 后走既有分页
const jumpMonth = ref<Date | null>(null)
const jumpOffsetDate = ref(0)

watch(jumpMonth, (d) => {
  if (!d) return
  resetMedia()
  jumpOffsetDate.value = Math.floor(new Date(d.getFullYear(), d.getMonth() + 1, 1).getTime() / 1000)
  loadMore()
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
const gridWidth = ref(0)
let scrollRO: ResizeObserver | null = null

watch(scrollEl, (el) => {
  scrollRO?.disconnect()
  scrollRO = null
  if (el) {
    scrollRO = new ResizeObserver(() => {
      gridWidth.value = el.clientWidth - 32
    })
    scrollRO.observe(el)
    gridWidth.value = el.clientWidth - 32
  }
})

// 与 .media-grid 的 minmax(150px, 1fr) + 12px gap 对应的实际列宽
const colWidth = computed(() => {
  const w = gridWidth.value
  if (w <= 0) return 150
  const n = Math.max(1, Math.floor((w + 12) / 162))
  return (w - (n - 1) * 12) / n
})

function cardStyle(it: MediaItem) {
  if (layout.value === 'grid') return undefined
  const w = it.width ?? 0
  const h = it.height ?? 0
  const ratio = w > 0 && h > 0 ? Math.min(2.2, Math.max(0.5, h / w)) : 1
  // 行步长 = auto-rows 8px + gap 12px；52px 为卡片信息区高度
  const span = Math.ceil((colWidth.value * ratio + 52 + 12) / 20)
  return { gridRowEnd: `span ${span}` }
}

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
  openDownload([it.messageId])
}

function downloadSelected() {
  openDownload([...selectedMsgs.value])
}

function onTaskCreated() {
  clearSelection()
  toast.add({ severity: 'success', summary: '下载任务已创建', detail: '可到「下载」页查看进度', life: 3000 })
}

// ---- 下载状态回填（task:file 事件 → 卡片角标） ----

const fileStates = reactive(new Map<string, { state: string; pct: number }>())
let offTaskFile: (() => void) | null = null

function fileStateOf(it: MediaItem) {
  return fileStates.get(itemKey(it))
}

// ---- 展示辅助 ----

function typeLabel(t: string) {
  switch (t) {
    case 'private':
      return '私聊'
    case 'group':
      return '群组'
    case 'channel':
      return '频道'
    default:
      return t || '未知'
  }
}

function typeSeverity(t: string) {
  switch (t) {
    case 'private':
      return 'success'
    case 'group':
      return 'info'
    case 'channel':
      return 'warn'
    default:
      return 'secondary'
  }
}

function typeIcon(t: string) {
  switch (t) {
    case 'private':
      return 'pi pi-user'
    case 'group':
      return 'pi pi-users'
    case 'channel':
      return 'pi pi-megaphone'
    default:
      return 'pi pi-comment'
  }
}

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

function extOf(name: string) {
  const i = name.lastIndexOf('.')
  return i >= 0 ? name.slice(i + 1).toLowerCase() : '-'
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

const pad = (n: number) => String(n).padStart(2, '0')

function fmtDate(unix: number) {
  if (!unix) return '-'
  const d = new Date(unix * 1000)
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

function fmtShortTime(unix?: number) {
  if (!unix) return ''
  const d = new Date(unix * 1000)
  const now = new Date()
  if (d.toDateString() === now.toDateString()) {
    return `${pad(d.getHours())}:${pad(d.getMinutes())}`
  }
  if (d.getFullYear() === now.getFullYear()) {
    return `${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
  }
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

// ---- 生命周期 ----

onMounted(async () => {
  sentinelObserver = new IntersectionObserver(
    (entries) => {
      sentinelVisible = entries[0]?.isIntersecting ?? false
      if (sentinelVisible) loadMore()
    },
    { rootMargin: '600px' },
  )
  if (sentinel.value) sentinelObserver.observe(sentinel.value)

  offTaskFile = on<FileEvent>(EVENT_TASK_FILE, (ev) => {
    if (!ev.dialogId || !ev.messageId) return
    const pct = ev.total > 0 ? Math.round((ev.downloaded / ev.total) * 100) : 0
    fileStates.set(`${ev.dialogId}:${ev.messageId}`, { state: ev.state, pct })
  })

  if (auth.loggedIn) await refresh()

  // 深链 /chats/:id 预选对话
  const pid = Number(route.params.id)
  if (pid) selectedId.value = pid
})

onBeforeUnmount(() => {
  sentinelObserver?.disconnect()
  scrollRO?.disconnect()
  offTaskFile?.()
})

// 登录状态变化：登录后自动加载，登出后清空
watch(
  () => auth.loggedIn,
  (v) => {
    if (v) {
      refresh()
    } else {
      chats.reset()
      selectedId.value = null
      resetMedia()
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

.w-full {
  width: 100%;
}

.split-body {
  flex: 1;
  min-height: 0;
  display: flex;
  gap: 16px;
}

/* ---- 左面板 ---- */
.dialog-panel {
  width: 320px;
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  padding: 0;
  overflow: hidden;
}

.dialog-toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 12px 0;
}

.dialog-toolbar.filters {
  padding-bottom: 12px;
  border-bottom: 1px solid var(--p-surface-200);
}

.app-dark .dialog-toolbar.filters {
  border-bottom-color: var(--p-surface-700);
}

.dialog-toolbar .grow {
  flex: 1;
}

.dialog-toolbar .spacer {
  flex: 1;
}

.regex-error {
  color: var(--p-red-500);
  font-size: 12px;
  padding: 4px 12px;
}

.dialog-list {
  flex: 1;
  min-height: 0;
}

.dialog-row {
  display: flex;
  align-items: center;
  gap: 10px;
  height: 60px;
  padding: 0 12px;
  cursor: pointer;
  transition: background 0.15s ease;
}

.dialog-row:hover {
  background: var(--p-surface-100);
}

.app-dark .dialog-row:hover {
  background: var(--p-surface-800);
}

.dialog-row.active {
  background: var(--p-primary-50);
}

.app-dark .dialog-row.active {
  background: color-mix(in srgb, var(--p-primary-color) 20%, transparent);
}

.dialog-avatar {
  width: 40px;
  height: 40px;
  border-radius: 50%;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
}

.dialog-avatar.t-private {
  background: var(--p-green-400);
}

.dialog-avatar.t-group {
  background: var(--p-blue-400);
}

.dialog-avatar.t-channel {
  background: var(--p-orange-400);
}

.dialog-main {
  flex: 1;
  min-width: 0;
}

.dialog-line {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.dialog-title {
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.dialog-time {
  color: var(--p-text-muted-color);
  font-size: 11px;
  white-space: nowrap;
}

.dialog-sub {
  color: var(--p-text-muted-color);
  font-size: 12px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.dialog-count {
  padding: 8px 12px;
  border-top: 1px solid var(--p-surface-200);
  color: var(--p-text-muted-color);
  font-size: 12px;
}

.app-dark .dialog-count {
  border-top-color: var(--p-surface-700);
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

.search-input {
  width: 200px;
}

.w-140 {
  width: 140px;
}

.w-100 {
  width: 100px;
}

.sep {
  color: var(--p-text-muted-color);
  flex-shrink: 0;
}

.spacer-flex {
  flex: 1;
  min-width: 0;
}

/* 包装器组件（InputNumber/DatePicker）内部 input 跟随包装器宽度，防止溢出遮挡相邻组件 */
.table-toolbar :deep(.p-inputnumber-input),
.table-toolbar :deep(.p-datepicker-input) {
  width: 100%;
  min-width: 0;
}

/* 固定宽度输入组件不被挤压 */
.table-toolbar .w-100,
.table-toolbar .w-140,
.table-toolbar .search-input {
  flex-shrink: 0;
}

/* 按钮不被挤压、文字不折行 */
.table-toolbar :deep(.p-button) {
  flex-shrink: 0;
  white-space: nowrap;
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
  grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
  gap: 12px;
  padding: 16px;
}

/* 瀑布流：小行步 + 按纵横比跨行，DOM 顺序保持时间倒序 */
.media-grid.waterfall {
  grid-auto-rows: 8px;
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
  box-shadow: 0 2px 8px rgb(0 0 0 / 8%);
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
  color: rgb(255 255 255 / 90%);
  text-shadow: 0 1px 4px rgb(0 0 0 / 50%);
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
  color: #fff;
  background: rgb(0 0 0 / 55%);
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
