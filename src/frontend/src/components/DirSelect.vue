<template>
  <div class="form-row dir-select">
    <Select
      :id="inputId"
      v-model="text"
      editable
      :options="options"
      :placeholder="placeholder"
      class="grow"
    />
    <Button
      icon="pi pi-folder-open"
      severity="secondary"
      outlined
      v-tooltip.top="'浏览…'"
      @click="browse"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import Select from 'primevue/select'

import { Download, SettingsApi } from '../api'

const props = withDefaults(
  defineProps<{
    modelValue: string
    /** 目录用途，决定历史分组（download / logExport / cache / temp / logDir） */
    kind: string
    placeholder?: string
    inputId?: string
    /** 历史为空时是否自动填入最近一次目录（挂载时生效） */
    autoFill?: boolean
  }>(),
  { placeholder: '选择或输入目录', autoFill: true },
)
const emit = defineEmits<{ 'update:modelValue': [string] }>()

const options = ref<string[]>([])

const text = computed({
  get: () => props.modelValue,
  set: (v: string | null) => emit('update:modelValue', v ?? ''),
})

onMounted(async () => {
  await reload()
  // 默认选中最近一次使用的目录
  if (props.autoFill && !props.modelValue && options.value.length) {
    emit('update:modelValue', options.value[0])
  }
})

/** 刷新历史下拉；非 Wails 环境静默降级为空列表。 */
async function reload() {
  try {
    options.value = (await SettingsApi.recentDirs(props.kind)) ?? []
  } catch {
    options.value = []
  }
}

async function browse() {
  try {
    const picked = await Download.selectDirectory(props.kind, props.modelValue)
    if (picked) {
      emit('update:modelValue', picked)
      await reload()
    }
  } catch {
    // 非 Wails 环境或用户取消：保持当前值
  }
}

/** 由调用方在「确认 / 保存」成功后调用，让手工输入的目录也进入历史。 */
async function commit() {
  const v = props.modelValue.trim()
  if (!v) return
  try {
    options.value = (await SettingsApi.addRecentDir(props.kind, v)) ?? options.value
  } catch {
    // 历史入库失败不影响主流程
  }
}

defineExpose({ commit, reload })
</script>

<style scoped>
.dir-select {
  display: flex;
  gap: 8px;
  align-items: center;
}

.dir-select > .grow {
  flex: 1;
  min-width: 0;
}

/* 可编辑 Select 内部 input 跟随容器宽度 */
.dir-select :deep(.p-select-label) {
  width: 100%;
}
</style>
