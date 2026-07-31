<template>
  <div class="page">
    <ConfirmDialog />
    <div class="page-header">
      <h1>脚本</h1>
      <p>用 Go 语法编写过滤 / 命名脚本与任务生命周期钩子（Yaegi 解释执行）</p>
    </div>

    <div class="scripts-layout">
      <!-- 左侧脚本列表 -->
      <div class="panel-card list-pane">
        <div class="list-head">
          <span class="list-title">脚本列表</span>
          <div class="list-actions">
            <Button icon="pi pi-copy" size="small" text rounded v-tooltip.top="'从模板创建'" @click="tplVisible = true" />
            <Button icon="pi pi-plus" size="small" text rounded v-tooltip.top="'新建脚本'" @click="newScript" />
          </div>
        </div>
        <div v-if="!scripts.scripts.length" class="empty-state small">
          <i class="pi pi-code" />
          <p>还没有脚本，可点击「模板」从内置示例开始</p>
        </div>
        <div
          v-for="s in scripts.scripts"
          :key="s.name"
          class="script-item"
          :class="{ active: s.name === currentName }"
          @click="open(s.name)"
        >
          <Checkbox
            :model-value="s.enabled"
            binary
            @click.stop
            @update:model-value="(v: boolean) => toggleEnabled(s.name, v)"
          />
          <i class="pi pi-file" />
          <div class="grow">
            <div class="name">{{ s.name }}</div>
            <div class="meta">{{ s.updatedAt }}</div>
            <div v-if="s.description" class="desc" :title="s.description">{{ s.description }}</div>
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

        <div class="editor-hint">
          <i class="pi pi-info-circle" />
          未启用的脚本不会出现在「添加下载」弹窗的脚本下拉中
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

    <ScriptTemplateDialog v-model:visible="tplVisible" @select="onTemplateSelect" />
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import ConfirmDialog from 'primevue/confirmdialog'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Textarea from 'primevue/textarea'

import ScriptTemplateDialog from '../components/ScriptTemplateDialog.vue'
import { Script } from '../api'
import { useScriptsStore } from '../stores/scripts'
import type { ScriptTemplate } from '../types'

const scripts = useScriptsStore()
const toast = useToast()
const confirm = useConfirm()

const currentName = ref('')
const source = ref('')
const editing = ref(false)
const result = ref<{ ok: boolean; text: string } | null>(null)
const tplVisible = ref(false)

// FE-12：追踪未保存修改
const savedSource = ref('')
const dirty = computed(() => source.value !== savedSource.value)

function confirmDiscard(onAccept: () => void) {
  if (!dirty.value) {
    onAccept()
    return
  }
  confirm.require({
    message: '当前编辑器有未保存的修改，继续将丢失这些改动。',
    header: '未保存的修改',
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: '放弃修改',
    rejectLabel: '取消',
    acceptProps: { severity: 'danger' },
    accept: onAccept,
  })
}

function newScript() {
  confirmDiscard(async () => {
    currentName.value = ''
    editing.value = true
    result.value = null
    try {
      source.value = await Script.starterTemplate()
    } catch {
      source.value = 'package main\n'
    }
    savedSource.value = source.value
  })
}

function open(name: string) {
  confirmDiscard(async () => {
    try {
      source.value = await Script.read(name)
      savedSource.value = source.value
      currentName.value = name
      editing.value = false
      result.value = null
    } catch (e: any) {
      toast.add({ severity: 'error', summary: '读取失败', detail: String(e), life: 4000 })
    }
  })
}

async function save() {
  try {
    await Script.save(currentName.value, source.value)
    savedSource.value = source.value
    editing.value = false
    await scripts.refresh()
    toast.add({ severity: 'success', summary: '已保存', detail: currentName.value, life: 2500 })
  } catch (e: any) {
    toast.add({ severity: 'error', summary: '保存失败', detail: String(e), life: 5000 })
  }
}

function remove() {
  confirm.require({
    message: `确定删除脚本「${currentName.value}」？此操作不可恢复。`,
    header: '删除脚本',
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: '删除',
    rejectLabel: '取消',
    acceptProps: { severity: 'danger' },
    accept: async () => {
      try {
        await Script.remove(currentName.value)
        currentName.value = ''
        source.value = ''
        savedSource.value = ''
        result.value = null
        await scripts.refresh()
        toast.add({ severity: 'info', summary: '已删除', life: 2500 })
      } catch (e: any) {
        toast.add({ severity: 'error', summary: '删除失败', detail: String(e), life: 5000 })
      }
    },
  })
}

async function toggleEnabled(name: string, v: boolean) {
  try {
    await scripts.setEnabled(name, v)
  } catch (e: any) {
    toast.add({ severity: 'error', summary: '设置失败', detail: String(e), life: 4000 })
    await scripts.refresh()
  }
}

function onTemplateSelect(tpl: ScriptTemplate) {
  confirmDiscard(() => {
    source.value = tpl.source
    savedSource.value = tpl.source
    currentName.value = tpl.id
    editing.value = true
    result.value = null
  })
}

async function validate() {
  try {
    const r = await Script.validate(source.value)
    result.value = r.ok
      ? { ok: true, text: `校验通过，检测到契约函数：${r.funcs.length ? r.funcs.join(', ') : '（无）'}` }
      : { ok: false, text: r.error ?? '校验失败' }
  } catch (e) {
    result.value = { ok: false, text: `校验调用失败：${String(e)}` }
  }
}

async function testRun() {
  let r
  try {
    r = await Script.testRun(source.value)
  } catch (e) {
    result.value = { ok: false, text: `试运行调用失败：${String(e)}` }
    return
  }
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
.scripts-layout {
  display: flex;
  gap: 16px;
  align-items: flex-start;
}

.list-pane {
  width: 260px;
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

.list-actions {
  display: flex;
  gap: 2px;
}

.list-title {
  font-weight: 600;
  font-size: 13px;
}

.script-item {
  display: flex;
  align-items: flex-start;
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

.script-item .desc {
  font-size: 11px;
  color: var(--p-text-muted-color);
  overflow: hidden;
  text-overflow: ellipsis;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  line-height: 1.3;
  margin-top: 2px;
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

.editor-hint {
  font-size: 12px;
  color: var(--p-text-muted-color);
  margin: -8px 0 12px;
}

.editor-hint i {
  margin-right: 4px;
}

.mt-16 {
  margin-top: 16px;
}

.log-console {
  background: var(--p-surface-950);
  color: var(--p-surface-200);
  font-family: Consolas, 'Courier New', monospace;
  font-size: 12px;
  padding: 12px;
  border-radius: 8px;
  white-space: pre-wrap;
  max-height: 200px;
  overflow-y: auto;
}
</style>
