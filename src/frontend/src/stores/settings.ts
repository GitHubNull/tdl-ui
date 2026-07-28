import { defineStore } from 'pinia'
import { SettingsApi } from '../api'
import { getTheme, setTheme, type ThemeMode } from '../theme'
import type { Settings } from '../types'

const defaults = (): Settings => ({
  proxy: '',
  downloadDir: '',
  template: '',
  threads: 4,
  limit: 2,
  poolSize: 8,
  theme: 'system',
  loggedInUserId: 0,
  loggedInUsername: '',
})

/** 应用设置。 */
export const useSettingsStore = defineStore('settings', {
  state: () => ({
    settings: defaults(),
    dataDir: '',
    inited: false,
  }),
  actions: {
    async init() {
      if (this.inited) return
      this.inited = true
      try {
        this.settings = await SettingsApi.get()
        this.dataDir = await SettingsApi.dataDir()
      } catch {
        /* 非 Wails 环境忽略 */
      }
      // 主题以 localStorage 为准（启动即生效），与后端设置保持同步
      this.settings.theme = getTheme()
    },
    async save() {
      await SettingsApi.save({ ...this.settings })
    },
    applyTheme(mode: ThemeMode) {
      this.settings.theme = mode
      setTheme(mode)
    },
  },
})
