<template>
  <div class="page page-wide">
    <div class="page-header">
      <h1>脚本</h1>
      <p>用 Go 语法编写过滤 / 命名脚本与任务生命周期钩子（Yaegi 解释执行）</p>
    </div>

    <div class="scripts-layout">
      <!-- 左侧脚本列表 -->
      <div class="panel-card list-pane">
        <div class="list-head">
          <span class="list-title">脚本列表</span>
          <Button icon="pi pi-plus" size="small" text rounded v-tooltip.top="'新建脚本'" @click="newScript" />
        </div>
        <div v-if="!scripts.scripts.length" class="empty-state small">
          <i class="pi pi-code" />
          <p>还没有脚本</p>
        </div>
        <div
          v-for="s in scripts.scripts"
          :key="s.name"
          class="script-item"
          :class="{ active: s.name === currentName }"
          @click="open(s.name)"
        >
          <i class="pi pi-file" />
          <div class="grow">
            <div class="name">{{ s.name }}</div>
            <div class="meta">{{ s.updatedAt }}</div>
          </div>
        </div>
      </div>

      <!-- 右侧编辑器 -->
      <div class="panel-card editor-pane">
        <div class="form-field">
          <label for="script-name">脚本名</label>
          <InputText id="script-name" v-model="currentName" placeholder="my-filter" :disabled="!editing" />
        </div>

        <div class="form-field">
          <label for="script-src">源码（package main，契约函数均可选）</label>
          <Textarea id="script-src" v-model="source" class="code-editor" spellcheck="false" />
        </div>

        <div class="editor-actions">
          <Button label="校验" icon="pi pi-check-circle" severity="secondary" outlined :disabled="!source" @click="validate" />
          <Button label="试运行" icon="pi pi-play-circle" severity="secondary" outlined :disabled="!source" @click="testRun" />
          <span class="grow" />
          <Button
            v-if="currentName && !editing"
            label="删除"
            icon="pi pi-trash"
            severity="danger"
            text
            @click="remove"
          />
          <Button label="保存" icon="pi pi-save" :disabled="!currentName || !source" @click="save" />
        </div>

        <Message v-if="result" :severity="result.ok ? 'success' : 'error'" class="mt-16">
          <div style="white-space: pre-wrap">{{ result.text }}</div>
        </Message>

        <div v-if="scripts.logs.length" class="mt-16">
          <div class="list-head">
            <span class="list-title">脚本日志</span>
            <Button icon="pi pi-eraser" size="small" text rounded v-tooltip.top="'清空日志'" @click="scripts.clearLogs()" />
          </div>
          <div class="log-console">{{ scripts.logs.join('\n') }}</div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useToast } from 'primevue/usetoast'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Textarea from 'primevue/textarea'

import { Script } from '../api'
import { useScriptsStore } from '../stores/scripts'

const scripts = useScriptsStore()
const toast = useToast()

const currentName = ref('')
const source = ref('')
// editing=true 表示新建（脚本名可改）
const editing = ref(false)
const result = ref<{ ok: boolean; text: string } | null>(null)

async function newScript() {
  currentName.value = ''
  editing.value = true
  result.value = null
  try {
    source.value = await Script.starterTemplate()
  } catch {
    source.value = 'package main\n'
  }
}

async function open(name: string) {
  try {
    source.value = await Script.read(name)
    currentName.value = name
    editing.value = false
    result.value = null
  } catch (e: any) {
    toast.add({ severity: 'error', summary: '读取失败', detail: String(e), life: 4000 })
  }
}

async function save() {
  try {
    await Script.save(currentName.value, source.value)
    editing.value = false
    await scripts.refresh()
    toast.add({ severity: 'success', summary: '已保存', detail: currentName.value, life: 2500 })
  } catch (e: any) {
    toast.add({ severity: 'error', summary: '保存失败', detail: String(e), life: 5000 })
  }
}

async function remove() {
  try {
    await Script.remove(currentName.value)
    currentName.value = ''
    source.value = ''
    result.value = null
    await scripts.refresh()
    toast.add({ severity: 'info', summary: '已删除', life: 2500 })
  } catch (e: any) {
    toast.add({ severity: 'error', summary: '删除失败', detail: String(e), life: 5000 })
  }
}

async function validate() {
  const r = await Script.validate(source.value)
  result.value = r.ok
    ? { ok: true, text: `校验通过，检测到契约函数：${r.funcs.length ? r.funcs.join(', ') : '（无）'}` }
    : { ok: false, text: r.error ?? '校验失败' }
}

async function testRun() {
  const r = await Script.testRun(source.value)
  if (!r.ok) {
    result.value = { ok: false, text: r.error ?? '试运行失败' }
    return
  }
  const lines = [`示例文件：${r.sampleFile}`]
  if (r.filterKeep !== undefined && r.filterKeep !== null) {
    lines.push(`Filter → ${r.filterKeep ? '保留（下载）' : '跳过'}`)
  }
  if (r.renameTo !== undefined) {
    lines.push(`Rename → ${r.renameTo === '' ? '（空串，使用默认模板）' : r.renameTo}`)
  }
  result.value = { ok: true, text: lines.join('\n') }
}
</script>

<style scoped>
.page-wide {
  max-width: 1080px;
}

.scripts-layout {
  display: flex;
  gap: 16px;
  align-items: flex-start;
}

.list-pane {
  width: 240px;
  flex-shrink: 0;
  padding: 16px;
}

.editor-pane {
  flex: 1;
  min-width: 0;
}

.list-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
}

.list-title {
  font-weight: 600;
  font-size: 13px;
}

.script-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px;
  border-radius: 8px;
  cursor: pointer;
  transition: background 0.15s ease;
}

.script-item:hover {
  background: var(--p-surface-100);
}

.app-dark .script-item:hover {
  background: var(--p-surface-800);
}

.script-item.active {
  background: var(--p-primary-50);
  color: var(--p-primary-color);
}

.app-dark .script-item.active {
  background: color-mix(in srgb, var(--p-primary-color) 20%, transparent);
}

.script-item .grow {
  flex: 1;
  min-width: 0;
}

.script-item .name {
  font-size: 13px;
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.script-item .meta {
  font-size: 11px;
  color: var(--p-text-muted-color);
}

.empty-state.small {
  padding: 24px 8px;
}

.empty-state.small .pi {
  font-size: 1.5rem;
  margin-bottom: 8px;
}

.editor-actions {
  display: flex;
  gap: 8px;
  align-items: center;
}

.editor-actions .grow {
  flex: 1;
}

.mt-16 {
  margin-top: 16px;
}
</style>
