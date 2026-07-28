import { defineStore } from 'pinia'
import { EVENT_SCRIPT_LOG, Script, on } from '../api'
import type { ScriptMeta } from '../types'

/** 用户脚本列表与脚本日志。 */
export const useScriptsStore = defineStore('scripts', {
  state: () => ({
    scripts: [] as ScriptMeta[],
    logs: [] as string[],
    inited: false,
  }),
  actions: {
    async init() {
      if (this.inited) return
      this.inited = true

      on<string>(EVENT_SCRIPT_LOG, (msg) => {
        this.logs.push(msg)
        if (this.logs.length > 200) this.logs.splice(0, this.logs.length - 200)
      })
      await this.refresh()
    },
    async refresh() {
      try {
        this.scripts = (await Script.list()) ?? []
      } catch {
        /* 非 Wails 环境忽略 */
      }
    },
    clearLogs() {
      this.logs = []
    },
  },
})
