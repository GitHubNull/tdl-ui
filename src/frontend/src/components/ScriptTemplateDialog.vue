<template>
  <Dialog
    :visible="visible"
    modal
    header="从模板创建"
    :style="{ width: '520px' }"
    @update:visible="(v: boolean) => emit('update:visible', v)"
  >
    <div v-if="!templates.length" class="empty-state small">
      <i class="pi pi-copy" />
      <p>暂无可用模板</p>
    </div>
    <div v-for="t in templates" :key="t.id" class="tpl-card" @click="select(t)">
      <div class="tpl-head">
        <span class="tpl-name">{{ t.name }}</span>
        <Tag :value="t.category" severity="secondary" />
      </div>
      <p class="tpl-desc">{{ t.description }}</p>
      <p class="tpl-notes"><i class="pi pi-info-circle" /> {{ t.notes }}</p>
    </div>
  </Dialog>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import Dialog from 'primevue/dialog'
import Tag from 'primevue/tag'

import { Script } from '../api'
import type { ScriptTemplate } from '../types'

const emit = defineEmits<{
  'update:visible': [boolean]
  select: [ScriptTemplate]
}>()

defineProps<{ visible: boolean }>()

const templates = ref<ScriptTemplate[]>([])

onMounted(async () => {
  try {
    templates.value = (await Script.templates()) ?? []
  } catch {
    /* 非 Wails 环境静默 */
  }
})

function select(t: ScriptTemplate) {
  emit('select', t)
  emit('update:visible', false)
}
</script>

<style scoped>
.tpl-card {
  padding: 12px 16px;
  border-radius: 8px;
  border: 1px solid var(--p-surface-200);
  cursor: pointer;
  transition: background 0.15s ease, border-color 0.15s ease;
  margin-bottom: 8px;
}

.app-dark .tpl-card {
  border-color: var(--p-surface-700);
}

.tpl-card:hover {
  background: var(--p-surface-100);
  border-color: var(--p-primary-color);
}

.app-dark .tpl-card:hover {
  background: var(--p-surface-800);
}

.tpl-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 6px;
}

.tpl-name {
  font-weight: 600;
  font-size: 14px;
}

.tpl-desc {
  font-size: 13px;
  color: var(--p-text-muted-color);
  margin: 0 0 6px;
  line-height: 1.4;
}

.tpl-notes {
  font-size: 12px;
  color: var(--p-orange-400);
  margin: 0;
}

.tpl-notes i {
  margin-right: 4px;
}

.empty-state.small {
  padding: 32px;
  text-align: center;
  color: var(--p-text-muted-color);
}

.empty-state.small i {
  font-size: 1.8rem;
  display: block;
  margin-bottom: 8px;
}
</style>
