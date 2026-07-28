import { defineStore } from 'pinia'
import { Chat } from '../api'
import type { DialogView } from '../types'

/** 对话列表（私聊 / 群组 / 频道），进入对话页时按需加载。 */
export const useChatsStore = defineStore('chats', {
  state: () => ({
    dialogs: [] as DialogView[],
    loading: false,
    loaded: false,
    error: '',
  }),
  actions: {
    /** 首次进入页面时加载（未登录等场景静默跳过，由页面提示）。 */
    async ensureLoaded() {
      if (this.loaded || this.loading) return
      await this.refresh()
    },
    /** 重新拉取对话列表；错误记录到 error 并重新抛出，由页面 toast。 */
    async refresh() {
      this.loading = true
      this.error = ''
      try {
        this.dialogs = (await Chat.listDialogs()) ?? []
        this.loaded = true
      } catch (e: any) {
        this.error = String(e?.message ?? e)
        throw e
      } finally {
        this.loading = false
      }
    },
    /** 按 ID 查找对话（详情页标题兜底）。 */
    byId(id: number): DialogView | undefined {
      return this.dialogs.find((d) => d.id === id)
    },
    /** 登出后清空缓存。 */
    reset() {
      this.dialogs = []
      this.loaded = false
      this.error = ''
    },
  },
})
