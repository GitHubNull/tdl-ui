<template>
  <div class="page page-table logs-page">
    <div class="page-header">
      <h1>日志</h1>
      <p>应用运行日志实时浏览与历史日志文件查看</p>
    </div>

    <div class="panel-card logs-card">
      <!-- 顶部工具栏 -->
      <div class="logs-toolbar">
        <Select
          v-model="source"
          :options="sourceOptions"
          option-label="label"
          option-value="value"
          class="source-select"
          size="small"
          @before-show="loadFiles"
        />
        <SearchBox
          v-model="query"
          placeholder="搜索日志"
          class="grow"
          size="small"
          :invalid="!!regexError"
        />
        <ToggleButton v-model="useRegex" on-label=".*" off-label=".*" size="small" v-tooltip.top="'正则表达式'" />
        <ToggleButton v-model="caseSensitive" on-label="Aa" off-label="Aa" size="small" v-tooltip.top="'区分大小写'" />
        <ToggleButton v-model="autoJump" on-label="跳转" off-label="跳转" size="small" v-tooltip.top="'自动跳转到匹配日志'" />
        <SelectButton
          v-model="levels"
          :options="levelOptions"
          multiple
          size="small"
          :allow-empty="true"
          class="level-filter"
        />
        <span class="grow" />
        <!-- 字号控制 -->
        <div class="font-controls" v-tooltip.top="'Ctrl + 滚轮调整字号'">
          <Button icon="pi pi-minus" size="small" severity="secondary" text rounded @click="adjustFont(-1)" />
          <Button icon="pi pi-font" size="small" severity="secondary" text rounded @click="resetFont" />
          <Button icon="pi pi-plus" size="small" severity="secondary" text rounded @click="adjustFont(1)" />
        </div>
        <ToggleButton
          v-model="follow"
          on-label="跟随"
          off-label="跟随"
          size="small"
          :disabled="source !== 'live'"
          v-tooltip.top="'新日志到达自动滚动到底部'"
        />
        <Button
          icon="pi pi-eraser"
          size="small"
          severity="secondary"
          text
          rounded
          :disabled="source !== 'live'"
          v-tooltip.top="'清空当前显示'"
          @click="logs.clear()"
        />
        <Button
          icon="pi pi-file-export"
          size="small"
          severity="secondary"
          text
          rounded
          v-tooltip.top="'导出日志'"
          @click="exportVisible = true"
        />
        <Button
          icon="pi pi-folder-open"
          size="small"
          severity="secondary"
          text
          rounded
          v-tooltip.top="'打开日志目录'"
          @click="openDir"
        />
      </div>
      <div v-if="regexError" class="regex-error">正则表达式无效：{{ regexError }}（已退化为不过滤）</div>

      <!-- 日志主体 -->
      <div
        class="logs-body-wrap"
        :style="bodyWrapStyle"
        @wheel="onWheel"
      >
        <VirtualScroller
          v-if="rows.length"
          ref="scroller"
          :items="rows"
          :item-size="rowHeight"
          class="logs-body"
          @scroll="onScroll"
        >
          <template #item="{ item }">
            <div class="log-row" :style="rowStyle">
              <span class="log-gutter" :style="{ minWidth: gutterWidth }">{{ item.num }}</span>
              <span class="log-line" v-html="item.html" />
            </div>
          </template>
        </VirtualScroller>
        <div v-else class="empty-state small logs-empty">
          <i class="pi pi-list" />
          <p>{{ emptyText }}</p>
        </div>
      </div>
    </div>

    <ExportLogsDialog
      v-model:visible="exportVisible"
      :filtered-rows="filteredLogEntries"
      :all-lines="source === 'live' ? logs.entries.map((e) => e.text) : historyLines"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useToast } from 'primevue/usetoast'
import Button from 'primevue/button'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import ToggleButton from 'primevue/togglebutton'
import VirtualScroller from 'primevue/virtualscroller'

import SearchBox from '../components/SearchBox.vue'
import ExportLogsDialog from '../components/ExportLogsDialog.vue'
import { LogApi } from '../api'
import { fmtSize } from '../utils/format'
import { useLogsStore } from '../stores/logs'
import { useSettingsStore } from '../stores/settings'
import type { LogFileInfo, LogEntry } from '../types'

const toast = useToast()
const logs = useLogsStore()
const settings = useSettingsStore()

// ---- 字号与滚动条 ----
const fontSize = computed(() => settings.settings.ui?.logFontSize || 14)
const scrollbarSize = computed(() => settings.settings.ui?.scrollbarSize || 10)
const rowHeight = computed(() => Math.round(fontSize.value * 1.8))

const bodyWrapStyle = computed(() => ({
  '--log-font-size': `${fontSize.value}px`,
  '--log-row-height': `${rowHeight.value}px`,
  '--logs-scrollbar-size': `${scrollbarSize.value}px`,
} as Record<string, string>))

const rowStyle = computed(() => ({
  height: `${rowHeight.value}px`,
  lineHeight: `${rowHeight.value}px`,
  fontSize: `${fontSize.value}px`,
}))

let fontSaveTimer: ReturnType<typeof setTimeout> | undefined

function adjustFont(delta: number) {
  const next = Math.max(10, Math.min(28, fontSize.value + delta))
  if (!settings.settings.ui) settings.settings.ui = { logFontSize: 14, scrollbarSize: 10 }
  settings.settings.ui.logFontSize = next
  // 防抖持久化
  clearTimeout(fontSaveTimer)
  fontSaveTimer = setTimeout(() => {
    settings.save().catch(() => {})
  }, 600)
}

function resetFont() {
  if (!settings.settings.ui) settings.settings.ui = { logFontSize: 14, scrollbarSize: 10 }
  settings.settings.ui.logFontSize = 14
  clearTimeout(fontSaveTimer)
  fontSaveTimer = setTimeout(() => {
    settings.save().catch(() => {})
  }, 600)
}

function onWheel(ev: WheelEvent) {
  if (!ev.ctrlKey) return
  ev.preventDefault()
  adjustFont(ev.deltaY > 0 ? -1 : 1)
}

onBeforeUnmount(() => clearTimeout(fontSaveTimer))

// ---- 导出弹窗 ----
const exportVisible = ref(false)

// ---- 数据源：实时 / 历史文件 ----
const source = ref<'live' | string>('live')
const files = ref<LogFileInfo[]>([])
const historyLines = ref<string[]>([])

const sourceOptions = computed(() => [
  { label: '实时日志', value: 'live' },
  ...files.value.map((f) => ({ label: `${f.name}（${fmtSize(f.size)}）`, value: f.name })),
])

async function loadFiles() {
  try {
    files.value = (await LogApi.listLogFiles()) ?? []
  } catch {
    /* 非 Wails 环境忽略 */
  }
}
loadFiles()

watch(source, async (name) => {
  if (name === 'live') {
    historyLines.value = []
    return
  }
  try {
    const lines = (await LogApi.readLogFile(name, 5000)) ?? []
    if (source.value !== name) return
    historyLines.value = lines
  } catch (e) {
    if (source.value !== name) return
    toast.add({ severity: 'error', summary: '读取日志文件失败', detail: String(e), life: 4000 })
    historyLines.value = []
  }
})

// ---- 搜索与过滤 ----
const query = ref('')
const useRegex = ref(false)
const caseSensitive = ref(false)
const autoJump = ref(true)
const levelOptions = ['DEBUG', 'INFO', 'WARN', 'ERROR']
const levels = ref<string[]>([...levelOptions])

const QUERY_DEBOUNCE_MS = 200
const debouncedQuery = ref('')
let queryTimer: ReturnType<typeof setTimeout> | undefined
watch(query, (q) => {
  clearTimeout(queryTimer)
  queryTimer = setTimeout(() => {
    debouncedQuery.value = q
  }, QUERY_DEBOUNCE_MS)
})
onBeforeUnmount(() => clearTimeout(queryTimer))

const compiled = computed<{ re: RegExp | null; error: string }>(() => {
  const q = debouncedQuery.value
  if (!q) return { re: null, error: '' }
  const flags = caseSensitive.value ? 'g' : 'gi'
  try {
    return { re: new RegExp(useRegex.value ? q : escapeRegExp(q), flags), error: '' }
  } catch (e) {
    return { re: null, error: useRegex.value ? (e instanceof Error ? e.message : String(e)) : '' }
  }
})

const matcher = computed<RegExp | null>(() => compiled.value.re)
const regexError = computed(() => compiled.value.error)

function escapeRegExp(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

// ---- 行模型 ----
interface Row {
  num: number
  level: string
  raw: string
  html: string
}

const lineRe =
  /^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}[.,]?\d*) \[(\w+)\] - \[([^\]]*)\] - \[([^\]]*)\] - \[(.*?)\] - \[(\d+)\] - (.*)$/

const filteredLogEntries = computed<LogEntry[]>(() => {
  const m = matcher.value
  const lvSet = new Set(levels.value)
  if (source.value === 'live') {
    return logs.entries.filter((e) => lvSet.has(e.level) && (!m || test(m, e.text)))
  }
  const out: LogEntry[] = []
  for (let i = 0; i < historyLines.value.length; i++) {
    const line = historyLines.value[i]
    const parsed = lineRe.exec(line)
    const level = parsed ? parsed[2].toUpperCase() : detectLevel(line)
    if (level && !lvSet.has(level)) continue
    if (m && !test(m, line)) continue
    out.push({
      seq: i + 1, time: parsed?.[1] ?? '', level: level || '',
      module: parsed?.[3] ?? '', file: parsed?.[4] ?? '',
      func: parsed?.[5] ?? '', line: parsed ? parseInt(parsed[6]) : 0,
      msg: parsed?.[7] ?? line, text: line,
    } as LogEntry)
  }
  return out
})

const rows = computed<Row[]>(() => {
  const m = matcher.value
  const lvSet = new Set(levels.value)
  const out: Row[] = []

  if (source.value === 'live') {
    for (const e of logs.entries) {
      if (!lvSet.has(e.level)) continue
      if (m && !test(m, e.text)) continue
      out.push({
        num: e.seq,
        level: e.level,
        raw: e.text,
        html: renderParts(m, e.time, e.level, e.module, `${e.file} · ${e.func}:${e.line}`, e.msg),
      })
    }
    return out
  }

  for (let i = 0; i < historyLines.value.length; i++) {
    const line = historyLines.value[i]
    const parsed = lineRe.exec(line)
    const level = parsed ? parsed[2].toUpperCase() : detectLevel(line)
    if (level && !lvSet.has(level)) continue
    if (m && !test(m, line)) continue
    out.push({
      num: i + 1,
      level,
      raw: line,
      html: parsed
        ? renderParts(m, parsed[1], parsed[2], parsed[3], `${parsed[4]} · ${parsed[5]}:${parsed[6]}`, parsed[7])
        : `<span class="lg-msg">${mark(m, line)}</span>`,
    })
  }
  return out
})

function test(re: RegExp, s: string): boolean {
  re.lastIndex = 0
  return re.test(s)
}

function detectLevel(line: string): string {
  const m = /\[(DEBUG|INFO|WARN|ERROR)\]/.exec(line)
  return m ? m[1] : ''
}

function escapeHtml(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')
}

function mark(re: RegExp | null, s: string): string {
  if (!re) return escapeHtml(s)
  re.lastIndex = 0
  let out = ''
  let last = 0
  let m: RegExpExecArray | null
  while ((m = re.exec(s)) !== null) {
    if (m[0] === '') {
      re.lastIndex++
      continue
    }
    out += escapeHtml(s.slice(last, m.index)) + '<mark>' + escapeHtml(m[0]) + '</mark>'
    last = m.index + m[0].length
  }
  return out + escapeHtml(s.slice(last))
}

function renderParts(
  re: RegExp | null,
  time: string,
  level: string,
  module: string,
  src: string,
  msg: string,
): string {
  const lv = level.toUpperCase()
  return (
    `<span class="lg-time">${mark(re, time)}</span>` +
    `<span class="lg-level lv-${lv.toLowerCase()}">${mark(re, lv)}</span>` +
    `<span class="lg-module">${mark(re, module)}</span>` +
    `<span class="lg-src">${mark(re, src)}</span>` +
    `<span class="lg-msg">${mark(re, msg)}</span>`
  )
}

const gutterWidth = computed(() => {
  const max = rows.value.length ? rows.value[rows.value.length - 1].num : 0
  return `${Math.max(String(max).length, 3)}ch`
})

const emptyText = computed(() => {
  if (source.value !== 'live' && !historyLines.value.length) return '该日志文件为空'
  if (query.value || levels.value.length < levelOptions.length) return '没有匹配的日志'
  return '暂无日志'
})

// ---- 滚动 ----
const scroller = ref<InstanceType<typeof VirtualScroller> | null>(null)
const follow = ref(true)
let programmaticScroll = false

function scrollTo(index: number) {
  programmaticScroll = true
  scroller.value?.scrollToIndex(index, 'auto')
  setTimeout(() => (programmaticScroll = false), 100)
}

watch([query, useRegex, caseSensitive], async () => {
  if (!autoJump.value || !query.value) return
  await nextTick()
  if (rows.value.length) scrollTo(0)
})

watch(
  () => rows.value.length,
  async (len) => {
    if (source.value !== 'live' || !follow.value || !len) return
    await nextTick()
    scrollTo(len - 1)
  },
)

function onScroll(ev: Event) {
  if (programmaticScroll) return
  const el = ev.target as HTMLElement
  if (!el) return
  const atBottom = el.scrollTop + el.clientHeight >= el.scrollHeight - 40
  if (source.value === 'live') follow.value = atBottom
}

// ---- 其他操作 ----
async function openDir() {
  try {
    await LogApi.openLogDir()
  } catch (e) {
    toast.add({ severity: 'error', summary: '打开日志目录失败', detail: String(e), life: 4000 })
  }
}
</script>

<style scoped>
.logs-page {
  display: flex;
  flex-direction: column;
}

.logs-card {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  padding: 12px 16px 16px;
}

/* 工具栏 */
.logs-toolbar {
  position: sticky;
  top: 0;
  z-index: 2;
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  padding-bottom: 12px;
}

.font-controls {
  display: flex;
  align-items: center;
  gap: 2px;
}

.source-select {
  min-width: 160px;
  max-width: 280px;
}

.logs-toolbar .search-box {
  min-width: 180px;
  max-width: 340px;
}

.level-filter :deep(.p-togglebutton) {
  padding-inline: 10px;
}

.regex-error {
  color: var(--p-red-500);
  font-size: 12px;
  margin: -6px 0 8px;
}

/* 日志主体 */
.logs-body-wrap {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  background: var(--p-surface-950);
  border-radius: 8px;
  overflow: hidden;
}

.logs-body {
  flex: 1;
  min-height: 0;
  width: 100%;
  font-family: Consolas, 'Courier New', monospace;
  font-size: var(--log-font-size, 14px);
}

.logs-body :deep(.p-virtualscroller)::-webkit-scrollbar {
  width: var(--logs-scrollbar-size, 10px);
  height: var(--logs-scrollbar-size, 10px);
}
.logs-body :deep(.p-virtualscroller)::-webkit-scrollbar-thumb {
  background: var(--p-surface-600);
  border-radius: calc(var(--logs-scrollbar-size, 10px) / 2);
}
.logs-body :deep(.p-virtualscroller)::-webkit-scrollbar-track {
  background: transparent;
}

.logs-empty {
  margin: auto;
  color: var(--p-surface-400);
}

.logs-empty i {
  font-size: 1.6rem;
  display: block;
  margin-bottom: 8px;
  text-align: center;
}

.log-row {
  display: flex;
  align-items: flex-start;
  height: var(--log-row-height, 25px);
  line-height: var(--log-row-height, 25px);
  font-size: var(--log-font-size, 14px);
  white-space: nowrap;
}

.log-row:hover {
  background: color-mix(in srgb, var(--p-surface-0) 6%, transparent);
}

/* 行号沟槽 */
.log-gutter {
  flex: none;
  text-align: right;
  color: var(--p-surface-500);
  padding: 0 8px 0 12px;
  border-right: 1px solid var(--p-surface-800);
  user-select: none;
}

.log-line {
  padding: 0 12px 0 10px;
  color: var(--p-surface-200);
  overflow: hidden;
  text-overflow: ellipsis;
}

/* 语法高亮 */
.log-line :deep(.lg-time) {
  color: var(--p-surface-500);
  margin-right: 8px;
}

.log-line :deep(.lg-level) {
  display: inline-block;
  min-width: 46px;
  text-align: center;
  border-radius: 4px;
  margin-right: 8px;
  padding: 0 4px;
  line-height: 18px;
  font-weight: 600;
}

.log-line :deep(.lv-debug) {
  color: var(--p-surface-400);
  background: color-mix(in srgb, var(--p-surface-400) 15%, transparent);
}

.log-line :deep(.lv-info) {
  color: var(--p-blue-400);
  background: color-mix(in srgb, var(--p-blue-400) 15%, transparent);
}

.log-line :deep(.lv-warn) {
  color: var(--p-orange-400);
  background: color-mix(in srgb, var(--p-orange-400) 15%, transparent);
}

.log-line :deep(.lv-error) {
  color: var(--p-red-400);
  background: color-mix(in srgb, var(--p-red-400) 15%, transparent);
}

.log-line :deep(.lg-module) {
  color: var(--p-primary-400);
  margin-right: 8px;
}

.log-line :deep(.lg-src) {
  color: var(--p-surface-500);
  margin-right: 8px;
}

.log-line :deep(.lg-msg) {
  color: var(--p-surface-200);
}

.log-line :deep(mark) {
  background: var(--p-yellow-500);
  color: var(--p-surface-950);
  border-radius: 2px;
  padding: 0 1px;
}
</style>
