<template>
  <div class="search-box">
    <InputText
      :model-value="model"
      class="w-full"
      :placeholder="placeholder"
      :size="size"
      :invalid="invalid"
      @update:model-value="model = $event ?? ''"
      @keyup.enter="emit('enter')"
    />
    <i
      class="search-icon"
      :class="model ? 'pi pi-times clearable' : 'pi pi-search'"
      @click="model && (model = '')"
    />
  </div>
</template>

<script setup lang="ts">
// 带清除图标的搜索框（FE-15：抽取自 ChatsPage/LogsPage/MediaToolbar 三处重复模板）
// 样式复用 style.css 的 .search-box/.search-icon 全局规则
import InputText from 'primevue/inputtext'

const model = defineModel<string>({ required: true })

defineProps<{
  placeholder?: string
  size?: 'small' | 'large'
  invalid?: boolean
}>()

const emit = defineEmits<{ enter: [] }>()
</script>
