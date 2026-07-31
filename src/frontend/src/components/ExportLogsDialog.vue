<template>
  <Dialog
    :visible="visible"
    modal
    header="导出日志"
    :style="{ width: '520px' }"
    @update:visible="(v: boolean) => emit('update:visible', v)"
    @show="onShow"
  >
    <div class="form-field">
      <label>导出范围</label>
      <SelectButton
        v-model="scope"
        :options="scopeOptions"
        option-label="label"
        option-value="value"
        :allow-empty="false"
      />
      <span class="hint">共 {{ records.length }} 行</span>
    </div>

    <div class="form-field">
      <label>文件格式</label>
      <SelectButton
        v-model="ext"
        :options="extOptions"
        option-label="label"
        option-value="value"
        :allow-empty="false"
      />
      <span class="hint">{{ ext === '.csv' ? '按 time,level,module,source,message 五列输出' : '逐行导出原始日志文本' }}</span>
    </div>

    <div class="form-field">
      <label for="ex-dir">导出目录</label>
      <DirSelect ref="dirSelect" v-model="dir" kind="logExport" input-id="ex-dir" />
    </div>

    <div class="form-field">
      <label for="ex-name">文件名</label>
      <div class="form-row name-row">
        <InputText id="ex-name" v-model="name" class="grow" spellcheck="false" />
        <span class="ext-tag mono">{{ ext }}</span>
      </div>
      <span class="hint">扩展名随格式自动追加，无需手动输入</span>
    </div>

    <template #footer>
      <Button label="取消" severity="secondary" text @click="close" />
      <Button
        label="导出"
        icon="pi pi-file-export"
        :disabled="!dir.trim() || !name.trim() || !records.length"
        :loading="exporting"
        @click="doExport"
      />
    </template>
  </Dialog>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useToast } from 'primevue/usetoast'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import SelectButton from 'primevue/selectbutton'

import DirSelect from './DirSelect.vue'
import { LogApi } from '../api'
import { buildContent, type LogRecord } from '../utils/logexport'

const props = defineProps<{
  visible: boolean
  /** 当前过滤结果 */
  filtered: LogRecord[]
  /** 当前数据源全部行 */
  all: LogRecord[]
}>()
const emit = defineEmits<{ 'update:visible': [boolean] }>()

const toast = useToast()

const scope = ref<'filtered' | 'all'>('filtered')
const ext = ref('.log')
const dir = ref('')
const name = ref('')
const exporting = ref(false)
const dirSelect = ref<InstanceType<typeof DirSelect> | null>(null)

const scopeOptions = [
  { label: '当前过滤结果', value: 'filtered' },
  { label: '当前数据源全部', value: 'all' },
]
const extOptions = [
  { label: '.log', value: '.log' },
  { label: '.txt', value: '.txt' },
  { label: '.csv', value: '.csv' },
]

const records = computed(() => (scope.value === 'filtered' ? props.filtered : props.all))

/** tdl-ui-log-20240501-153000 */
function defaultName(): string {
  const d = new Date()
  const p = (n: number) => String(n).padStart(2, '0')
  return `tdl-ui-log-${d.getFullYear()}${p(d.getMonth() + 1)}${p(d.getDate())}-${p(d.getHours())}${p(d.getMinutes())}${p(d.getSeconds())}`
}

function onShow() {
  name.value = defaultName()
}

function close() {
  emit('update:visible', false)
}

async function doExport() {
  exporting.value = true
  try {
    const content = buildContent(records.value, ext.value)
    const path = await LogApi.exportLogs(dir.value.trim(), name.value.trim() + ext.value, content)
    toast.add({ severity: 'success', summary: '日志已导出', detail: path, life: 5000 })
    await dirSelect.value?.commit()
    close()
  } catch (e: any) {
    toast.add({ severity: 'error', summary: '导出失败', detail: String(e), life: 6000 })
  } finally {
    exporting.value = false
  }
}
</script>

<style scoped>
.name-row {
  display: flex;
  gap: 8px;
  align-items: center;
}

.name-row > .grow {
  flex: 1;
  min-width: 0;
}

.ext-tag {
  flex: none;
  color: var(--p-text-muted-color);
  font-size: 13px;
}
</style>
