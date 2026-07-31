<template>
  <Dialog
    :visible="visible"
    modal
    header="内置脚本模板"
    :style="{ width: '640px' }"
    @update:visible="(v: boolean) => emit('update:visible', v)"
    @show="load"
  >
    <p class="dialog-hint">选择一个模板灌入编辑器，可直接编辑后保存为自己的脚本。</p>

    <div v-if="!templates.length" class="empty-state small">
      <i class="pi pi-copy" />
      <p>没有可用的内置模板</p>
    </div>

    <div
      v-for="t in templates"
      :key="t.id"
      class="tpl-card"
      :class="{ active: t.id === selectedId }"
      @click="selectedId = t.id"
    >
      <div class="tpl-head">
        <span class="tpl-name">{{ t.name }}</span>
        <Tag :value="t.category" severity="secondary" />
        <span class="grow" />
        <span class="tpl-id mono">{{ t.id }}.go</span>
      </div>
      <p class="tpl-desc">{{ t.description }}</p>
      <p v-if="t.notes" class="tpl-notes"><i class="pi pi-info-circle" /> {{ t.notes }}</p>
    </div>

    <template #footer>
      <Button label="取消" severity="secondary" text @click="close" />
      <Button label="载入编辑器" icon="pi pi-file-import" :disabled="!selected" @click="pick" />
    </template>
  </Dialog>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import Tag from 'primevue/tag'

import { Script } from '../api'
import type { ScriptTemplate } from '../types'

defineProps<{ visible: boolean }>()
const emit = defineEmits<{
  'update:visible': [boolean]
  pick: [ScriptTemplate]
}>()

const templates = ref<ScriptTemplate[]>([])
const selectedId = ref('')

const selected = computed(() => templates.value.find((t) => t.id === selectedId.value))

async function load() {
  try {
    templates.value = (await Script.templates()) ?? []
  } catch {
    templates.value = []
  }
  if (!selected.value) selectedId.value = templates.value[0]?.id ?? ''
}

function close() {
  emit('update:visible', false)
}

function pick() {
  const t = selected.value
  if (!t) return
  emit('pick', t)
  close()
}
</script>

<style scoped>
.dialog-hint {
  margin: 0 0 12px;
  font-size: 12px;
  color: var(--p-text-muted-color);
}

.tpl-card {
  border: 1px solid var(--p-content-border-color);
  border-radius: 8px;
  padding: 10px 12px;
  margin-bottom: 10px;
  cursor: pointer;
  transition: border-color 0.15s ease, background 0.15s ease;
}

.tpl-card:hover {
  background: var(--p-surface-100);
}

.app-dark .tpl-card:hover {
  background: var(--p-surface-800);
}

.tpl-card.active {
  border-color: var(--p-primary-color);
  background: var(--p-primary-50);
}

.app-dark .tpl-card.active {
  background: color-mix(in srgb, var(--p-primary-color) 18%, transparent);
}

.tpl-head {
  display: flex;
  align-items: center;
  gap: 8px;
}

.tpl-head .grow {
  flex: 1;
}

.tpl-name {
  font-size: 13px;
  font-weight: 600;
}

.tpl-id {
  font-size: 11px;
  color: var(--p-text-muted-color);
}

.tpl-desc {
  margin: 6px 0 0;
  font-size: 12px;
  line-height: 1.6;
}

.tpl-notes {
  margin: 6px 0 0;
  font-size: 11px;
  color: var(--p-text-muted-color);
  line-height: 1.6;
}
</style>
