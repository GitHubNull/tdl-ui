import { defineStore } from 'pinia'
import { Download, EVENT_TASK, EVENT_TASK_FILE, on } from '../api'
import type { FileEvent, TaskView } from '../types'

// FE-17：保存事件取消函数，HMR 重建模块时注销旧回调，避免事件双触发
const unsubs: Array<() => void> = []
import.meta.hot?.dispose(() => {
  unsubs.splice(0).forEach((off) => off())
})

/** 下载任务列表与实时进度。 */
export const useTasksStore = defineStore('tasks', {
  state: () => ({
    tasks: [] as TaskView[],
    // taskId -> fileId -> FileEvent（仅保留进行中/失败文件）
    files: {} as Record<string, Record<number, FileEvent>>,
    inited: false,
  }),
  getters: {
    activeCount: (s) => s.tasks.filter((t) => t.status === 'running' || t.status === 'queued').length,
  },
  actions: {
    async init() {
      if (this.inited) return
      this.inited = true

      unsubs.push(on<TaskView>(EVENT_TASK, (t) => this.upsert(t)))
      unsubs.push(on<FileEvent>(EVENT_TASK_FILE, (f) => this.onFile(f)))
      await this.refresh()
    },
    async refresh() {
      try {
        this.tasks = (await Download.listTasks()) ?? []
      } catch {
        /* 非 Wails 环境忽略 */
      }
    },
    upsert(t: TaskView) {
      const i = this.tasks.findIndex((x) => x.id === t.id)
      if (i >= 0) {
        this.tasks[i] = t
      } else {
        this.tasks.unshift(t)
      }
      // 任务进入终态后清理文件进度
      if (t.status === 'done' || t.status === 'failed' || t.status === 'canceled' || t.status === 'paused') {
        delete this.files[t.id]
      }
    },
    onFile(f: FileEvent) {
      if (f.state === 'done') {
        const m = this.files[f.taskId]
        if (m) delete m[f.fileId]
        return
      }
      if (!this.files[f.taskId]) this.files[f.taskId] = {}
      this.files[f.taskId][f.fileId] = f
    },
    filesOf(taskId: string): FileEvent[] {
      return Object.values(this.files[taskId] ?? {})
    },
    async pause(id: string) {
      await Download.pauseTask(id)
    },
    async resume(id: string) {
      await Download.resumeTask(id)
    },
    async cancel(id: string) {
      await Download.cancelTask(id)
    },
    async remove(id: string) {
      await Download.removeTask(id)
      this.tasks = this.tasks.filter((t) => t.id !== id)
      delete this.files[id]
    },
    async clearFinished() {
      await Download.clearFinishedTasks()
      await this.refresh()
    },
    async deleteAllFiles() {
      await Download.deleteAllFiles()
      this.files = {}
      await this.refresh()
    },
    async deleteFiles(id: string, paths: string[]) {
      await Download.deleteTaskFiles(id, paths)
      await this.refresh()
    },
    async openDir(id: string) {
      await Download.openTaskDir(id)
    },
  },
})
