<template>
  <Dialog
    :visible="visible"
    modal
    header="导出日志"
    :style="{ width: '480px' }"
    @update:visible="(v: boolean) => emit('update:visible', v)"
  >
    <div class="form-field">
      <label>范围</label>
      <SelectButton v-model="range" :options="rangeOptions" option-label="label" option-value="value" :allow-empty="false" />
    </div>

    <div class="form-field">
      <label>格式</label>
      <SelectButton v-model="format" :options="formatOptions" option-label="label" option-value="value" :allow-empty="false" />
    </div>

    <div class="form-field">
      <label for="el-dir">保存目录</label>
      <DirSelect id="el-dir" v-model="dir" kind="logExport" />
    </div>

    <div class="form-field">
      <label for="el-name">文件名</label>
      <InputText id="el-name" v-model="name" class="mono" />
      <span class="hint">扩展名自动拼接：{{ ext }}</span>
    </div>

    <template #footer>
      <Button label="取消" severity="secondary" text @click="close" />
      <Button label="导出" icon="pi pi-file-export" :loading="exporting" @click="doExport" />
    </template>
  </Dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useToast } from 'primevue/usetoast'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import SelectButton from 'primevue/selectbutton'

import DirSelect from './DirSelect.vue'
import { LogApi } from '../api'
import type { LogEntry } from '../types'

const props = defineProps<{
  visible: boolean
  filteredRows: LogEntry[]
  allLines: string[]
}>()

const emit = defineEmits<{
  'update:visible': [boolean]
}>()

const toast = useToast()

const range = ref<'filtered' | 'all'>('filtered')
const format = ref<'.log' | '.txt' | '.csv'>('.log')
const dir = ref('')
const name = ref(`tdl-ui-log-${fmtNow()}`)
const exporting = ref(false)

const rangeOptions = [
  { label: '当前过滤结果', value: 'filtered' },
  { label: '全部', value: 'all' },
]

const formatOptions = [
  { label: '.log', value: '.log' },
  { label: '.txt', value: '.txt' },
  { label: '.csv', value: '.csv' },
]

const ext = computed(() => format.value)

const fullName = computed(() => {
  const n = name.value.trim()
  if (!n) return ''
  // 若用户手填了扩展名则替换，否则追加
  const base = n.replace(/\.(log|txt|csv)$/i, '')
  return base + ext.value
})

watch(() => props.visible, (v) => {
  if (v) {
    name.value = `tdl-ui-log-${fmtNow()}`
  }
})

function close() {
  emit('update:visible', false)
}

function fmtNow(): string {
  const d = new Date()
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}${pad(d.getMonth() + 1)}${pad(d.getDate())}-${pad(d.getHours())}${pad(d.getMinutes())}${pad(d.getSeconds())}`
}

function buildContent(): string {
  if (format.value === '.csv') {
    const lines: string[] = ['time,level,module,source,message']
    const src = range.value === 'filtered' ? props.filteredRows : parseAllLines()
    for (const row of src) {
      const r = row as any
      const time = r.time ?? ''
      const level = r.level ?? ''
      const module = r.module ?? ''
      const source = r.source ?? `${r.file ?? ''} · ${r.func ?? ''}:${r.line ?? ''}`
      const msg = r.msg ?? r.raw ?? ''
      lines.push(`${escapeCSV(time)},${escapeCSV(level)},${escapeCSV(module)},${escapeCSV(source)},${escapeCSV(msg)}`)
    }
    return lines.join('\n')
  }
  // .log / .txt：逐行原文
  if (range.value === 'filtered') {
    return props.filteredRows.map((r) => r.text).join('\n')
  }
  return props.allLines.join('\n')
}

function parseAllLines(): LogEntry[] {
  // 把历史文本行包装成最小 LogEntry
  return props.allLines.map((line, i) => ({
    seq: i + 1,
    time: '',
    level: '',
    module: '',
    file: '',
    func: '',
    line: 0,
    msg: line,
    text: line,
    raw: line,
  } as LogEntry))
}

function escapeCSV(s: string): string {
  const needQuote = /[",\n]/.test(s)
  const escaped = s.replace(/"/g, '""')
  return needQuote ? `"${escaped}"` : escaped
}

async function doExport() {
  if (!dir.value || !fullName.value) {
    toast.add({ severity: 'warn', summary: '请填写目录和文件名', life: 3000 })
    return
  }
  exporting.value = true
  try {
    const content = buildContent()
    const path = await LogApi.exportLogs(dir.value, fullName.value, content)
    toast.add({ severity: 'success', summary: '日志已导出', detail: path, life: 4000 })
    close()
  } catch (e: any) {
    toast.add({ severity: 'error', summary: '导出失败', detail: String(e), life: 5000 })
  } finally {
    exporting.value = false
  }
}
</script>
