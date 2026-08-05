<template>
  <div class="table-toolbar">
    <SearchBox v-model="queryText" name="mediaQuery" placeholder="搜索文件名或标题" class="search-input" @enter="apply" />
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
    <InputText v-model="extsText" name="exts" placeholder="扩展名: mp4, jpg" class="w-140" @keyup.enter="apply" />
    <InputNumber v-model="minMB" name="minSize" placeholder="最小 MB" :min="0" class="w-100" @keyup.enter="apply" />
    <span class="sep">-</span>
    <InputNumber v-model="maxMB" name="maxSize" placeholder="最大 MB" :min="0" class="w-100" @keyup.enter="apply" />
    <Button label="查询" icon="pi pi-filter" size="small" @click="apply" />
    <Button label="重置" icon="pi pi-filter-slash" size="small" severity="secondary" outlined @click="reset" />
    <span class="spacer-flex" />
    <SelectButton
      v-model="layout"
      :options="layoutOptions"
      option-value="value"
      option-label="label"
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
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import Button from 'primevue/button'
import DatePicker from 'primevue/datepicker'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import MultiSelect from 'primevue/multiselect'
import SelectButton from 'primevue/selectbutton'

import SearchBox from './SearchBox.vue'
import type { AppliedFilters } from '../composables/useMediaPager'
import type { MediaLayout } from '../composables/useWaterfall'

const props = defineProps<{
  /** 已生效筛选（缓存恢复时用于回填输入框） */
  applied: AppliedFilters
}>()
const emit = defineEmits<{
  apply: [AppliedFilters]
}>()

const layout = defineModel<MediaLayout>('layout', { required: true })
const jumpMonth = defineModel<Date | null>('jumpMonth', { required: true })

// 输入态（应用后才生效，与 applied 解耦）
const queryText = ref('')
const kinds = ref<string[]>([])
const extsText = ref('')
const minMB = ref<number | null>(null)
const maxMB = ref<number | null>(null)

const kindOptions = [
  { label: '视频', value: 'video' },
  { label: '图片', value: 'photo' },
  { label: '音频', value: 'audio' },
  { label: '文件', value: 'file' },
]

const layoutOptions = [
  { label: '瀑布流', value: 'waterfall', icon: 'pi pi-th-large' },
  { label: '网格', value: 'grid', icon: 'pi pi-table' },
  { label: '时间流', value: 'timeline', icon: 'pi pi-calendar' },
]

// applied 外部变化（切对话重置/缓存恢复）时回填输入框
watch(
  () => props.applied,
  (f) => {
    queryText.value = f.query
    kinds.value = [...f.kinds]
    extsText.value = f.exts.join(', ')
    minMB.value = f.minSize > 0 ? f.minSize / (1024 * 1024) : null
    maxMB.value = f.maxSize > 0 ? f.maxSize / (1024 * 1024) : null
  },
)

function apply() {
  emit('apply', {
    query: queryText.value.trim(),
    kinds: [...kinds.value],
    exts: parseExts(extsText.value),
    minSize: toBytes(minMB.value),
    maxSize: toBytes(maxMB.value),
  })
}

function reset() {
  queryText.value = ''
  kinds.value = []
  extsText.value = ''
  minMB.value = null
  maxMB.value = null
  apply()
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
</script>

<style scoped>
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
</style>
