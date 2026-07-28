<template>
  <div class="page page-table">
    <div class="page-header detail-header">
      <Button
        icon="pi pi-arrow-left"
        text
        rounded
        v-tooltip.bottom="'返回对话列表'"
        aria-label="返回"
        @click="router.push('/chats')"
      />
      <div class="header-text">
        <h1>{{ title }}</h1>
        <p>对话内全部媒体文件，滚动加载更多</p>
      </div>
      <Tag :value="typeLabel(dialogType)" :severity="typeSeverity(dialogType)" />
    </div>

    <div class="table-card panel-card">
      <!-- 筛选栏 -->
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
      </div>

      <DataTable
        :value="items"
        data-key="messageId"
        lazy
        :total-records="totalRecords"
        scrollable
        scroll-height="flex"
        :virtual-scroller-options="vsOptions"
      >
        <Column field="name" header="名称">
          <template #body="{ data }">
            <span class="cell-name" :title="data.name">{{ data.name }}</span>
          </template>
        </Column>
        <Column field="caption" header="标题">
          <template #body="{ data }">
            <span class="cell-muted cell-ellipsis" :title="data.caption">{{ data.caption || '-' }}</span>
          </template>
        </Column>
        <Column field="kind" header="类型" style="width: 84px">
          <template #body="{ data }">
            <Tag :value="kindLabel(data.kind)" :severity="kindSeverity(data.kind)" />
          </template>
        </Column>
        <Column field="size" header="大小" style="width: 100px">
          <template #body="{ data }">
            <span class="cell-muted">{{ fmtSize(data.size) }}</span>
          </template>
        </Column>
        <Column field="name" header="格式" style="width: 90px">
          <template #body="{ data }">
            <span class="cell-muted">{{ extOf(data.name) }}</span>
          </template>
        </Column>
        <Column field="date" header="日期" style="width: 160px">
          <template #body="{ data }">
            <span class="cell-muted">{{ fmtTime(data.date) }}</span>
          </template>
        </Column>
        <Column field="messageId" header="消息 ID" style="width: 100px">
          <template #body="{ data }">
            <span class="cell-muted">{{ data.messageId }}</span>
          </template>
        </Column>
        <template #empty>
          <div class="empty-state small">
            <i class="pi pi-images" />
            <p>{{ loading ? '加载中…' : '没有匹配的媒体文件' }}</p>
          </div>
        </template>
      </DataTable>

      <div class="table-status">
        <span>已加载 {{ items.length }} 条</span>
        <span v-if="loading" class="status-loading"><i class="pi pi-spin pi-spinner" /> 加载中…</span>
        <span v-else-if="failed" class="status-error">上次加载失败，继续滚动可重试</span>
        <span v-else-if="!hasMore && items.length">没有更多了</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useToast } from 'primevue/usetoast'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import MultiSelect from 'primevue/multiselect'
import Tag from 'primevue/tag'

import { Chat } from '../api'
import type { MediaItem, MediaQuery } from '../types'
import { useChatsStore } from '../stores/chats'

const route = useRoute()
const router = useRouter()
const toast = useToast()
const chats = useChatsStore()

const dialogId = Number(route.params.id)
const dialogType = String(route.query.type ?? chats.byId(dialogId)?.type ?? '')
const title = String(route.query.title ?? chats.byId(dialogId)?.title ?? `对话 ${dialogId}`)

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

const totalRecords = computed(() => items.value.length + (hasMore.value ? PAGE_SIZE : 0))

const vsOptions = computed(() => ({
  itemSize: 48,
  lazy: true,
  delay: 100,
  showLoader: true,
  loading: loading.value,
  onLazyLoad,
}))

function onLazyLoad(e: { first: number; last: number }) {
  // 滚动接近已加载数据末尾时预取下一页
  if (e.last >= items.value.length - 10) {
    loadMore()
  }
}

async function loadMore() {
  if (loading.value || !hasMore.value || !dialogId) return
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
  }
}

function buildQuery(): MediaQuery {
  return {
    dialogId,
    dialogType,
    offsetId: offset.value,
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
  items.value = []
  offset.value = 0
  hasMore.value = true
  failed.value = false
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

function kindLabel(k: string) {
  switch (k) {
    case 'video':
      return '视频'
    case 'photo':
      return '图片'
    case 'audio':
      return '音频'
    default:
      return '文件'
  }
}

function kindSeverity(k: string) {
  switch (k) {
    case 'video':
      return 'info'
    case 'photo':
      return 'success'
    case 'audio':
      return 'warn'
    default:
      return 'secondary'
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

function fmtTime(unix: number) {
  if (!unix) return '-'
  const d = new Date(unix * 1000)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

onMounted(loadMore)
</script>

<style scoped>
.detail-header {
  display: flex;
  align-items: center;
  gap: 12px;
}

.header-text {
  flex: 1;
  min-width: 0;
}

.header-text h1 {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.search-input {
  width: 220px;
}

.w-140 {
  width: 140px;
}

.w-100 {
  width: 100px;
}

.sep {
  color: var(--p-text-muted-color);
}

.cell-name {
  font-weight: 500;
  display: inline-block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  vertical-align: bottom;
}

.cell-muted {
  color: var(--p-text-muted-color);
  font-size: 13px;
}

.cell-ellipsis {
  display: inline-block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  vertical-align: bottom;
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
}
</style>
