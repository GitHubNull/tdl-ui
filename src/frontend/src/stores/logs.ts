import { defineStore } from 'pinia'
import { EVENT_LOG, LogApi, on } from '../api'
import type { LogEntry } from '../types'

/** 实时日志缓冲上限，与后端环形缓冲一致。 */
const MAX_ENTRIES = 5000

// FE-17：保存事件取消函数，HMR 重建模块时注销旧回调，避免事件双触发
const unsubs: Array<() => void> = []
import.meta.hot?.dispose(() => {
  unsubs.splice(0).forEach((off) => off())
})

/** 应用日志：初始快照 + log:batch 实时批量追加。 */
export const useLogsStore = defineStore('logs', {
  state: () => ({
    entries: [] as LogEntry[],
    inited: false,
  }),
  actions: {
    async init() {
      if (this.inited) return
      this.inited = true

      unsubs.push(
        on<LogEntry[]>(EVENT_LOG, (batch) => {
          if (!batch?.length) return
          this.entries.push(...batch)
          if (this.entries.length > MAX_ENTRIES) {
            this.entries.splice(0, this.entries.length - MAX_ENTRIES)
          }
        }),
      )
      try {
        // 初始快照与事件订阅之间可能有微小重叠，按 seq 去重合并
        const recent = (await LogApi.getRecent()) ?? []
        const seen = new Set(this.entries.map((e) => e.seq))
        const merged = recent.filter((e) => !seen.has(e.seq))
        this.entries.unshift(...merged)
      } catch {
        /* 非 Wails 环境忽略 */
      }
    },
    clear() {
      this.entries = []
    },
  },
})
