import { defineStore } from 'pinia'
import { SettingsApi } from '../api'
import { getTheme, setTheme, type ThemeMode } from '../theme'
import type { Settings } from '../types'
import { config } from '../../wailsjs/go/models'

const defaults = (): Settings =>
  config.Settings.createFrom({
    proxy: '',
    downloadDir: '',
    template: '',
    threads: 4,
    limit: 2,
    poolSize: 8,
    theme: 'system',
    cacheDir: '',
    tempDir: '',
    loggedInUserId: 0,
    loggedInUsername: '',
    ui: {
      logFontSize: 14,
      scrollbarSize: 10,
      maxLogLines: 128,
    },
    log: {
      targets: 'both',
      level: 'info',
      dir: '',
      format: '',
      maxSizeMb: 10,
      maxAgeDays: 7,
      maxBackups: 10,
    },
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
        const remote = await SettingsApi.get()
        // 旧版本 settings.json 可能缺少 log 字段，用默认值兜底防表单绑定 undefined
        this.settings = config.Settings.createFrom({
          ...defaults(),
          ...remote,
          ui: { ...defaults().ui, ...(remote.ui ?? {}) },
          log: { ...defaults().log, ...(remote.log ?? {}) },
        })
        this.dataDir = await SettingsApi.dataDir()
      } catch {
        /* 非 Wails 环境忽略 */
      }
      // 主题以 localStorage 为准（启动即生效），与后端设置保持同步
      this.settings.theme = getTheme()
    },
    async save() {
      await SettingsApi.save(config.Settings.createFrom(this.settings))
    },
    applyTheme(mode: ThemeMode) {
      this.settings.theme = mode
      setTheme(mode)
    },
  },
})
