<template>
  <div class="form-row">
    <Select
      :input-id="inputId"
      :model-value="modelValue"
      editable
      :options="options"
      :placeholder="placeholder"
      class="grow"
      @update:model-value="(v: string) => emit('update:modelValue', v)"
    />
    <Button icon="pi pi-folder-open" severity="secondary" outlined v-tooltip.top="'浏览…'" @click="browse" />
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import Button from 'primevue/button'
import Select from 'primevue/select'

import { Download, SettingsApi } from '../api'

const props = defineProps<{
  modelValue: string
  kind: string
  placeholder?: string
  inputId?: string
}>()

const emit = defineEmits<{
  'update:modelValue': [string]
}>()

const options = ref<string[]>([])

async function loadOptions() {
  try {
    const dirs = await SettingsApi.recentDirs(props.kind)
    options.value = dirs ?? []
    // 若当前值为空且存在历史，默认选中最近一次
    if (!props.modelValue && options.value.length) {
      emit('update:modelValue', options.value[0])
    }
  } catch {
    /* 非 Wails 环境静默 */
  }
}

onMounted(loadOptions)

async function browse() {
  try {
    const picked = await Download.selectDirectory(props.kind, props.modelValue)
    if (picked) {
      emit('update:modelValue', picked)
      await loadOptions()
    }
  } catch {
    /* 非 Wails 环境静默 */
  }
}

// 由调用方在确认/保存成功后调用，把手工输入的目录也进历史
async function commit() {
  if (!props.modelValue) return
  try {
    await SettingsApi.addRecentDir(props.kind, props.modelValue)
    await loadOptions()
  } catch {
    /* 静默 */
  }
}

defineExpose({ commit })
</script>
