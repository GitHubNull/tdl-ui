import { defineStore } from 'pinia'
import { EVENT_SCRIPT_LOG, Script, on } from '../api'
import type { ScriptMeta } from '../types'

// FE-17：保存事件取消函数，HMR 重建模块时注销旧回调，避免事件双触发
const unsubs: Array<() => void> = []
import.meta.hot?.dispose(() => {
  unsubs.splice(0).forEach((off) => off())
})

/** 用户脚本列表与脚本日志。 */
export const useScriptsStore = defineStore('scripts', {
  state: () => ({
    scripts: [] as ScriptMeta[],
    logs: [] as string[],
    inited: false,
  }),
  getters: {
    enabled: (state) => state.scripts.filter((s) => s.enabled),
  },
  actions: {
    async init() {
      if (this.inited) return
      this.inited = true

      unsubs.push(
        on<string>(EVENT_SCRIPT_LOG, (msg) => {
          this.logs.push(msg)
          if (this.logs.length > 200) this.logs.splice(0, this.logs.length - 200)
        }),
      )
      await this.refresh()
    },
    async refresh() {
      try {
        this.scripts = (await Script.list()) ?? []
      } catch {
        /* 非 Wails 环境忽略 */
      }
    },
    async setEnabled(name: string, v: boolean) {
      try {
        await Script.setEnabled(name, v)
        await this.refresh()
      } catch (e: any) {
        throw e
      }
    },
    clearLogs() {
      this.logs = []
    },
  },
})
