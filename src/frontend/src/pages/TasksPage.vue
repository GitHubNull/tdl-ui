<template>
  <div class="page">
    <div class="page-header header-row">
      <div>
        <h1>下载</h1>
        <p>下载任务的实时进度与控制</p>
      </div>
      <div class="header-actions">
        <Button
          label="清除已完成记录"
          icon="pi pi-eraser"
          severity="secondary"
          outlined
          :disabled="!hasFinished"
          @click="onClearFinished"
        />
        <Button
          label="删除所有文件"
          icon="pi pi-trash"
          severity="danger"
          outlined
          :disabled="!tasks.tasks.length"
          @click="onDeleteAll"
        />
        <Button label="添加下载" icon="pi pi-plus" @click="showDialog = true" />
      </div>
    </div>

    <div v-if="!tasks.tasks.length" class="empty-state panel-card">
      <img class="empty-illustration" :src="emptyTasks" alt="" draggable="false" />
      <p>暂无下载任务</p>
      <p class="mt-8">点击右上角「添加下载」创建第一个任务</p>
    </div>

    <div v-for="t in tasks.tasks" :key="t.id" class="task-card">
      <div class="row">
        <Tag :value="statusLabel(t.status)" :severity="statusSeverity(t.status)" />
        <span class="title" :title="t.label || t.urls.join('\n')">{{ taskTitle(t) }}</span>
        <span class="meta">{{ t.finished }}/{{ t.total || '?' }} 个文件</span>

        <Button
          icon="pi pi-folder-open"
          severity="secondary"
          text
          rounded
          v-tooltip.top="'打开目录'"
          @click="onOpenDir(t.id)"
        />
        <Button
          v-if="t.status === 'running' || t.status === 'queued'"
          icon="pi pi-pause"
          severity="secondary"
          text
          rounded
          v-tooltip.top="'暂停'"
          @click="onPause(t.id)"
        />
        <Button
          v-if="t.status === 'paused'"
          icon="pi pi-play"
          severity="secondary"
          text
          rounded
          v-tooltip.top="'恢复（断点续传）'"
          @click="onResume(t.id)"
        />
        <Button
          v-if="t.status === 'failed' || t.status === 'canceled'"
          icon="pi pi-refresh"
          severity="warn"
          text
          rounded
          v-tooltip.top="'重试（断点续传）'"
          @click="onResume(t.id)"
        />
        <Button
          v-if="hasPendingFiles(t) && (t.status === 'done' || t.status === 'failed' || t.status === 'canceled')"
          icon="pi pi-forward"
          severity="info"
          text
          rounded
          v-tooltip.top="'继续下载未完成文件'"
          @click="onResumeWithPending(t.id)"
        />
        <Button
          v-if="t.status === 'running' || t.status === 'queued' || t.status === 'paused'"
          icon="pi pi-times"
          severity="danger"
          text
          rounded
          v-tooltip.top="'取消'"
          @click="onCancel(t.id)"
        />
        <Button
          v-if="isFinal(t.status)"
          icon="pi pi-trash"
          severity="secondary"
          text
          rounded
          v-tooltip.top="'移除记录'"
          @click="onRemove(t.id)"
        />
        <Button
          v-if="isFinal(t.status)"
          :icon="expanded[t.id] ? 'pi pi-chevron-up' : 'pi pi-list'"
          severity="secondary"
          text
          rounded
          v-tooltip.top="'文件列表'"
          @click="toggleFiles(t)"
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
        <span v-if="t.fileCount"><i class="pi pi-file" /> 已登记 {{ t.fileCount }} 个文件</span>
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

      <!-- 已登记文件列表（展开时懒加载） -->
      <div v-if="expanded[t.id]" class="files-panel mt-8">
        <div v-if="loadingFiles[t.id]" class="files-hint">加载中…</div>
        <template v-else>
          <div v-if="!fileLists[t.id]?.length" class="files-hint">无文件记录</div>
          <div v-for="f in fileLists[t.id]" :key="f.path" class="task-file-row">
            <Checkbox v-model="selectedPaths[t.id]" :value="f.path" />
            <span class="name" :title="f.path">{{ f.name }}</span>
            <span class="size">{{ fmtSize(f.size) }}</span>
            <Tag
              :value="fileStateLabel(f.state)"
              :severity="fileStateSeverity(f.state)"
              class="state-tag"
            />
            <div class="file-actions">
              <Button
                v-if="f.state === 'failed' || f.state === 'done'"
                icon="pi pi-refresh"
                severity="secondary"
                text
                rounded
                size="small"
                v-tooltip.top="'重新下载'"
                @click="onRedownloadFile(t.id, f.path)"
              />
              <Button
                icon="pi pi-folder-open"
                severity="secondary"
                text
                rounded
                size="small"
                v-tooltip.top="'打开所在目录'"
                @click="onRevealFile(f.path)"
              />
              <Button
                icon="pi pi-times"
                severity="secondary"
                text
                rounded
                size="small"
                v-tooltip.top="'删除记录'"
                @click="onDeleteFileRecord(t.id, f.path)"
              />
              <Button
                icon="pi pi-trash"
                severity="danger"
                text
                rounded
                size="small"
                v-tooltip.top="'删除文件'"
                @click="onDeleteFile(t.id, f.path)"
              />
            </div>
          </div>
          <div v-if="selectedPaths[t.id]?.length" class="files-actions">
            <Button
              :label="`删除选中文件（${selectedPaths[t.id].length}）`"
              icon="pi pi-trash"
              severity="danger"
              size="small"
              outlined
              @click="onDeleteSelected(t)"
            />
          </div>
        </template>
      </div>
    </div>

    <NewTaskDialog v-model:visible="showDialog" @created="onCreated" />
    <ConfirmDialog />
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import ConfirmDialog from 'primevue/confirmdialog'
import Message from 'primevue/message'
import ProgressBar from 'primevue/progressbar'
import Tag from 'primevue/tag'

import NewTaskDialog from '../components/NewTaskDialog.vue'
import emptyTasks from '../assets/illustrations/empty-tasks.svg'
import { Download } from '../api'
import { fmtSize } from '../utils/format'
import type { TaskFile, TaskView } from '../types'
import { useTasksStore } from '../stores/tasks'

const tasks = useTasksStore()
const toast = useToast()
const confirm = useConfirm()
const showDialog = ref(false)

// 任务文件列表的展开与多选状态
const expanded = reactive<Record<string, boolean>>({})
const loadingFiles = reactive<Record<string, boolean>>({})
const fileLists = reactive<Record<string, TaskFile[]>>({})
const selectedPaths = reactive<Record<string, string[]>>({})

// FE-25：任务被移除或重新运行（离开终态）时清理展开/文件列表状态，避免缓慢累积与陈旧列表
watch(
  () => tasks.tasks.map((t) => `${t.id}:${t.status}`),
  () => {
    const finalById = new Map(tasks.tasks.map((t) => [t.id, isFinal(t.status)]))
    for (const key of Object.keys(expanded)) {
      if (!finalById.get(key)) {
        delete expanded[key]
        delete loadingFiles[key]
        delete fileLists[key]
        delete selectedPaths[key]
      }
    }
  },
)

const hasFinished = computed(() => tasks.tasks.some((t) => t.status === 'done'))

async function toggleFiles(t: TaskView) {
  if (expanded[t.id]) {
    expanded[t.id] = false
    return
  }
  expanded[t.id] = true
  if (!selectedPaths[t.id]) selectedPaths[t.id] = []
  loadingFiles[t.id] = true
  try {
    fileLists[t.id] = (await Download.listTaskFiles(t.id)) ?? []
  } catch (e) {
    toast.add({ severity: 'error', summary: '加载文件列表失败', detail: String(e), life: 4000 })
    expanded[t.id] = false
  } finally {
    loadingFiles[t.id] = false
  }
}

async function onOpenDir(id: string) {
  try {
    await tasks.openDir(id)
  } catch (e) {
    toast.add({ severity: 'error', summary: '打开目录失败', detail: String(e), life: 4000 })
  }
}

async function onPause(id: string) {
  try {
    await tasks.pause(id)
  } catch (e) {
    toast.add({ severity: 'error', summary: '暂停失败', detail: String(e), life: 4000 })
  }
}

async function onResume(id: string) {
  try {
    await tasks.resume(id)
  } catch (e) {
    toast.add({ severity: 'error', summary: '恢复失败', detail: String(e), life: 4000 })
  }
}

async function onCancel(id: string) {
  try {
    await tasks.cancel(id)
  } catch (e) {
    toast.add({ severity: 'error', summary: '取消失败', detail: String(e), life: 4000 })
  }
}

async function onRemove(id: string) {
  try {
    await tasks.remove(id)
  } catch (e) {
    toast.add({ severity: 'error', summary: '移除记录失败', detail: String(e), life: 4000 })
  }
}

async function onClearFinished() {
  try {
    await tasks.clearFinished()
    toast.add({ severity: 'success', summary: '已清除完成记录', life: 3000 })
  } catch (e) {
    toast.add({ severity: 'error', summary: '清除失败', detail: String(e), life: 5000 })
  }
}

function onDeleteAll() {
  confirm.require({
    header: '删除所有文件',
    message: '将停止所有未完成任务，并删除全部已登记的下载文件与任务记录，且不可恢复。确定继续？',
    icon: 'pi pi-exclamation-triangle',
    acceptProps: { label: '删除', severity: 'danger' },
    rejectProps: { label: '取消', severity: 'secondary', outlined: true },
    accept: async () => {
      try {
        await tasks.deleteAllFiles()
        for (const k of Object.keys(expanded)) delete expanded[k]
        toast.add({ severity: 'success', summary: '已删除全部文件与记录', life: 3000 })
      } catch (e) {
        toast.add({ severity: 'error', summary: '删除失败', detail: String(e), life: 5000 })
      }
    },
  })
}

function onDeleteSelected(t: TaskView) {
  const paths = selectedPaths[t.id] ?? []
  if (!paths.length) return
  confirm.require({
    header: '删除选中文件',
    message: `将删除 ${paths.length} 个文件（记录同步移除），且不可恢复。确定继续？`,
    icon: 'pi pi-exclamation-triangle',
    acceptProps: { label: '删除', severity: 'danger' },
    rejectProps: { label: '取消', severity: 'secondary', outlined: true },
    accept: async () => {
      try {
        await tasks.deleteFiles(t.id, paths)
        selectedPaths[t.id] = []
        if (tasks.tasks.some((x) => x.id === t.id)) {
          fileLists[t.id] = (await Download.listTaskFiles(t.id)) ?? []
        } else {
          delete expanded[t.id] // 文件全部删除，记录已被后端移除
        }
        toast.add({ severity: 'success', summary: '已删除选中文件', life: 3000 })
      } catch (e) {
        toast.add({ severity: 'error', summary: '删除失败', detail: String(e), life: 5000 })
      }
    },
  })
}

// 检查任务是否有未完成的文件（用于显示"继续下载"按钮）
function hasPendingFiles(t: TaskView): boolean {
  // 如果任务有失败计数，说明有未完成的文件
  if (t.failed > 0) return true
  // 如果任务已完成但文件列表中有未完成或失败的文件，也显示继续下载按钮
  const files = fileLists[t.id]
  if (!files) return false
  return files.some((f) => f.state === 'downloading' || f.state === 'failed')
}

// 继续下载未完成文件
async function onResumeWithPending(id: string) {
  try {
    await tasks.resumeTaskWithPending(id)
    toast.add({ severity: 'success', summary: '已继续下载未完成文件', life: 3000 })
  } catch (e) {
    toast.add({ severity: 'error', summary: '继续下载失败', detail: String(e), life: 5000 })
  }
}

// 重新下载单个文件
async function onRedownloadFile(taskId: string, filePath: string) {
  try {
    await tasks.redownloadFile(taskId, filePath)
    toast.add({ severity: 'success', summary: '已加入重新下载队列', life: 3000 })
    // 刷新文件列表
    fileLists[taskId] = (await Download.listTaskFiles(taskId)) ?? []
  } catch (e) {
    toast.add({ severity: 'error', summary: '重新下载失败', detail: String(e), life: 5000 })
  }
}

// 打开文件所在目录并选中
async function onRevealFile(filePath: string) {
  try {
    await Download.revealFile(filePath)
  } catch (e) {
    toast.add({ severity: 'error', summary: '打开目录失败', detail: String(e), life: 4000 })
  }
}

// 删除文件记录（保留磁盘文件）
async function onDeleteFileRecord(taskId: string, filePath: string) {
  try {
    await tasks.deleteFileRecord(taskId, filePath)
    toast.add({ severity: 'success', summary: '已删除记录', life: 3000 })
    fileLists[taskId] = (await Download.listTaskFiles(taskId)) ?? []
  } catch (e) {
    toast.add({ severity: 'error', summary: '删除记录失败', detail: String(e), life: 5000 })
  }
}

// 删除文件（含磁盘文件）
function onDeleteFile(taskId: string, filePath: string) {
  confirm.require({
    header: '删除文件',
    message: '将删除该文件的磁盘副本与记录，且不可恢复。确定继续？',
    icon: 'pi pi-exclamation-triangle',
    acceptProps: { label: '删除', severity: 'danger' },
    rejectProps: { label: '取消', severity: 'secondary', outlined: true },
    accept: async () => {
      try {
        await tasks.deleteFiles(taskId, [filePath])
        toast.add({ severity: 'success', summary: '已删除文件', life: 3000 })
        fileLists[taskId] = (await Download.listTaskFiles(taskId)) ?? []
      } catch (e) {
        toast.add({ severity: 'error', summary: '删除失败', detail: String(e), life: 5000 })
      }
    },
  })
}

const fileStateText: Record<string, string> = {
  downloading: '未完成',
  done: '已完成',
  failed: '失败',
}

function fileStateLabel(s: string) {
  return fileStateText[s] ?? s
}

function fileStateSeverity(s: string) {
  switch (s) {
    case 'done':
      return 'success'
    case 'failed':
      return 'danger'
    default:
      return 'warn'
  }
}

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

.header-actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  justify-content: flex-end;
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

.files-panel {
  border-top: 1px solid var(--p-content-border-color);
  padding-top: 8px;
}

.files-hint {
  color: var(--p-text-muted-color);
  font-size: 13px;
  padding: 4px 0;
}

.task-file-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 4px 0;
  font-size: 13px;
}

.task-file-row .name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.task-file-row .size {
  color: var(--p-text-muted-color);
  font-size: 12px;
  white-space: nowrap;
}

.task-file-row .state-tag {
  font-size: 11px;
}

.files-actions {
  margin-top: 8px;
}

.file-actions {
  display: flex;
  gap: 2px;
  margin-left: auto;
}

.file-actions :deep(.p-button) {
  width: 28px;
  height: 28px;
}

.file-actions :deep(.p-button .p-button-icon) {
  font-size: 12px;
}
</style>
