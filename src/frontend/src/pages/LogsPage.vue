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
        <div class="search-box grow">
          <InputText
            v-model="query"
            placeholder="搜索日志"
            class="w-full"
            size="small"
            :invalid="!!regexError"
          />
          <i
            class="search-icon"
            :class="query ? 'pi pi-times clearable' : 'pi pi-search'"
            @click="query && (query = '')"
          />
        </div>
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
      <div class="logs-body-wrap">
        <VirtualScroller
          v-if="rows.length"
          ref="scroller"
          :items="rows"
          :item-size="24"
          class="logs-body"
          @scroll="onScroll"
        >
          <template #item="{ item }">
            <div class="log-row">
              <span class="log-gutter" :style="{ minWidth: gutterWidth }">{{ item.num }}</span>
              <!-- eslint-disable-next-line vue/no-v-html -->
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
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useToast } from 'primevue/usetoast'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import ToggleButton from 'primevue/togglebutton'
import VirtualScroller from 'primevue/virtualscroller'

import { LogApi } from '../api'
import { useLogsStore } from '../stores/logs'
import type { LogFileInfo } from '../types'

const toast = useToast()
const logs = useLogsStore()

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
    historyLines.value = (await LogApi.readLogFile(name, 5000)) ?? []
  } catch (e) {
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

const regexError = ref('')

/** 编译搜索匹配器；正则非法时返回 null（不过滤）并记录错误。 */
const matcher = computed<RegExp | null>(() => {
  regexError.value = ''
  const q = query.value
  if (!q) return null
  const flags = caseSensitive.value ? 'g' : 'gi'
  try {
    return new RegExp(useRegex.value ? q : escapeRegExp(q), flags)
  } catch (e) {
    if (useRegex.value) {
      regexError.value = e instanceof Error ? e.message : String(e)
      return null
    }
    return null
  }
})

function escapeRegExp(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

// ---- 行模型：结构化实时条目与历史文本行统一为可渲染行 ----
interface Row {
  num: number
  level: string
  raw: string
  html: string
}

/** 默认格式模板产生的行：时间 [级别] - [模块] - [文件] - [函数] - [行号] - 消息 */
const lineRe =
  /^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}[.,]?\d*) \[(\w+)\] - \[([^\]]*)\] - \[([^\]]*)\] - \[(.*?)\] - \[(\d+)\] - (.*)$/

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

/** 无法结构化解析时从整行探测级别。 */
function detectLevel(line: string): string {
  const m = /\[(DEBUG|INFO|WARN|ERROR)\]/.exec(line)
  return m ? m[1] : ''
}

// ---- 语法高亮与搜索命中标记（渲染前 HTML 转义防注入） ----
function escapeHtml(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')
}

/** 对文本做搜索命中 <mark> 标记（分段转义，防注入）。 */
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

// ---- 行号沟槽宽度随最大位数自适应 ----
const gutterWidth = computed(() => {
  const max = rows.value.length ? rows.value[rows.value.length - 1].num : 0
  return `${Math.max(String(max).length, 3)}ch`
})

const emptyText = computed(() => {
  if (source.value !== 'live' && !historyLines.value.length) return '该日志文件为空'
  if (query.value || levels.value.length < levelOptions.length) return '没有匹配的日志'
  return '暂无日志'
})

// ---- 滚动：自动跳转与跟随 ----
const scroller = ref<InstanceType<typeof VirtualScroller> | null>(null)
const follow = ref(true)
let programmaticScroll = false

function scrollTo(index: number) {
  programmaticScroll = true
  scroller.value?.scrollToIndex(index, 'auto')
  setTimeout(() => (programmaticScroll = false), 100)
}

// 搜索条件变化后自动跳转到第一条命中
watch([query, useRegex, caseSensitive], async () => {
  if (!autoJump.value || !query.value) return
  await nextTick()
  if (rows.value.length) scrollTo(0)
})

// 实时源新日志到达时跟随滚动到底部
watch(
  () => rows.value.length,
  async (len) => {
    if (source.value !== 'live' || !follow.value || !len) return
    await nextTick()
    scrollTo(len - 1)
  },
)

/** 用户手动上滚暂停跟随，回到底部自动恢复。 */
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

function fmtSize(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / 1024 / 1024).toFixed(1)} MB`
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

/* 工具栏：一行粘性置顶 */
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

/* 日志主体：类终端深底 */
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
  font-size: 12px;
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
  height: 24px;
  line-height: 24px;
  white-space: nowrap;
}

.log-row:hover {
  background: color-mix(in srgb, var(--p-surface-0) 6%, transparent);
}

/* 行号沟槽：右对齐、不可选中、细分隔线 */
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
