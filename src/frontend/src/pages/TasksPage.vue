<template>
  <div class="page">
    <div class="page-header header-row">
      <div>
        <h1>下载</h1>
        <p>下载任务的实时进度与控制</p>
      </div>
      <Button label="添加下载" icon="pi pi-plus" @click="showDialog = true" />
    </div>

    <div v-if="!tasks.tasks.length" class="empty-state panel-card">
      <i class="pi pi-inbox" />
      <p>暂无下载任务</p>
      <p class="mt-8">点击右上角「添加下载」创建第一个任务</p>
    </div>

    <div v-for="t in tasks.tasks" :key="t.id" class="task-card">
      <div class="row">
        <Tag :value="statusLabel(t.status)" :severity="statusSeverity(t.status)" />
        <span class="title" :title="t.label || t.urls.join('\n')">{{ taskTitle(t) }}</span>
        <span class="meta">{{ t.finished }}/{{ t.total || '?' }} 个文件</span>

        <Button
          v-if="t.status === 'running' || t.status === 'queued'"
          icon="pi pi-pause"
          severity="secondary"
          text
          rounded
          v-tooltip.top="'暂停'"
          @click="tasks.pause(t.id)"
        />
        <Button
          v-if="t.status === 'paused'"
          icon="pi pi-play"
          severity="secondary"
          text
          rounded
          v-tooltip.top="'恢复（断点续传）'"
          @click="tasks.resume(t.id)"
        />
        <Button
          v-if="t.status === 'running' || t.status === 'queued' || t.status === 'paused'"
          icon="pi pi-times"
          severity="danger"
          text
          rounded
          v-tooltip.top="'取消'"
          @click="tasks.cancel(t.id)"
        />
        <Button
          v-if="isFinal(t.status)"
          icon="pi pi-trash"
          severity="secondary"
          text
          rounded
          v-tooltip.top="'移除记录'"
          @click="tasks.remove(t.id)"
        />
      </div>

      <ProgressBar
        class="mt-8"
        :value="taskPercent(t)"
        :mode="t.status === 'running' && !t.total ? 'indeterminate' : 'determinate'"
        :show-value="false"
        style="height: 6px"
      />

      <div class="task-meta mt-8">
        <span><i class="pi pi-folder" /> {{ t.dir }}</span>
        <span v-if="t.scriptName"><i class="pi pi-code" /> {{ t.scriptName }}</span>
        <span v-if="t.failed"><i class="pi pi-exclamation-triangle" /> 失败 {{ t.failed }}</span>
        <span>{{ t.createdAt }}</span>
      </div>

      <Message v-if="t.error" severity="error" class="mt-8" size="small">{{ t.error }}</Message>

      <!-- 进行中的文件 -->
      <div v-for="f in tasks.filesOf(t.id)" :key="f.fileId" class="file-row">
        <i :class="f.state === 'failed' ? 'pi pi-times-circle failed' : 'pi pi-arrow-circle-down'" />
        <span class="name" :title="f.name">{{ f.name }}</span>
        <span class="size">{{ fmtSize(f.downloaded) }} / {{ fmtSize(f.total) }}</span>
        <ProgressBar
          :value="f.total > 0 ? Math.round((f.downloaded / f.total) * 100) : 0"
          :show-value="false"
          style="width: 120px; height: 4px"
        />
      </div>
    </div>

    <NewTaskDialog v-model:visible="showDialog" @created="onCreated" />
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useToast } from 'primevue/usetoast'
import Button from 'primevue/button'
import Message from 'primevue/message'
import ProgressBar from 'primevue/progressbar'
import Tag from 'primevue/tag'

import NewTaskDialog from '../components/NewTaskDialog.vue'
import type { TaskView } from '../types'
import { useTasksStore } from '../stores/tasks'

const tasks = useTasksStore()
const toast = useToast()
const showDialog = ref(false)

async function onCreated() {
  await tasks.refresh()
  toast.add({ severity: 'success', summary: '任务已创建', life: 3000 })
}

const statusText: Record<string, string> = {
  queued: '排队中',
  running: '下载中',
  paused: '已暂停',
  done: '已完成',
  failed: '失败',
  canceled: '已取消',
}

function statusLabel(s: string) {
  return statusText[s] ?? s
}

function statusSeverity(s: string) {
  switch (s) {
    case 'running':
      return 'info'
    case 'done':
      return 'success'
    case 'failed':
      return 'danger'
    case 'paused':
      return 'warn'
    case 'canceled':
      return 'secondary'
    default:
      return 'secondary'
  }
}

function isFinal(s: string) {
  return s === 'done' || s === 'failed' || s === 'canceled' || s === 'paused'
}

function taskTitle(t: TaskView) {
  if (t.label) return t.label
  const first = t.urls[0] ?? ''
  return t.urls.length > 1 ? `${first} 等 ${t.urls.length} 条链接` : first
}

function taskPercent(t: TaskView) {
  if (t.status === 'done') return 100
  if (!t.total) return 0
  return Math.round(((t.finished + t.failed) / t.total) * 100)
}

function fmtSize(n: number) {
  if (!n || n < 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  let v = n
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(v >= 100 || i === 0 ? 0 : 1)} ${units[i]}`
}
</script>

<style scoped>
.mt-8 {
  margin-top: 8px;
}

.header-row {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
}

.meta {
  color: var(--p-text-muted-color);
  font-size: 13px;
  white-space: nowrap;
}

.task-meta {
  display: flex;
  gap: 16px;
  flex-wrap: wrap;
  color: var(--p-text-muted-color);
  font-size: 12px;
}

.task-meta .pi {
  font-size: 12px;
  margin-right: 4px;
}

.file-row .size {
  color: var(--p-text-muted-color);
  font-size: 12px;
  white-space: nowrap;
}

.file-row .failed {
  color: var(--p-red-500);
}
</style>
