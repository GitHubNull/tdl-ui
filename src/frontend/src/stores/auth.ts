import { defineStore } from 'pinia'
import { Auth, EVENT_LOGIN, on } from '../api'
import type { LoginUpdate, LoginUser } from '../types'

// FE-17：保存事件取消函数，HMR 重建模块时注销旧回调，避免事件双触发
const unsubs: Array<() => void> = []
import.meta.hot?.dispose(() => {
  unsubs.splice(0).forEach((off) => off())
})

// FE-19：pending 态超时兜底，后端既不 reject 也不发事件时避免永久转圈
const PENDING_TIMEOUT_MS = 30_000
let pendingTimer: ReturnType<typeof setTimeout> | undefined

/** 登录状态与登录流程事件。 */
export const useAuthStore = defineStore('auth', {
  state: () => ({
    loggedIn: false,
    userId: 0,
    username: '',
    name: '',
    // LOGIC-003：kv 中存在会话但展示态未登录时，登录页提示"检测到本地会话"
    sessionPresent: false,
    // 登录流程阶段：idle / pending / qr / need_code / need_password / error
    stage: 'idle' as string,
    qrUrl: '',
    error: '',
    inited: false,
  }),
  actions: {
    async init() {
      if (this.inited) return
      this.inited = true

      unsubs.push(on<LoginUpdate>(EVENT_LOGIN, (u) => this.handleUpdate(u)))
      try {
        const st = await Auth.status()
        this.loggedIn = st.loggedIn
        this.userId = st.userId
        this.username = st.username
        this.sessionPresent = st.sessionPresent
      } catch {
        /* 非 Wails 环境忽略 */
      }
    },
    handleUpdate(u: LoginUpdate) {
      // 后端已响应，解除 pending 超时兜底
      clearTimeout(pendingTimer)
      switch (u.stage) {
        case 'qr':
          this.stage = 'qr'
          this.qrUrl = u.qrUrl ?? ''
          break
        case 'need_code':
        case 'need_password':
          this.stage = u.stage
          break
        case 'success':
          this.stage = 'idle'
          this.error = ''
          this.qrUrl = ''
          this.sessionPresent = true
          this.applyUser(u.user)
          break
        case 'error':
          this.stage = 'error'
          this.error = u.error ?? '未知错误'
          this.qrUrl = ''
          break
        case 'logout':
          this.loggedIn = false
          this.userId = 0
          this.username = ''
          this.name = ''
          this.sessionPresent = false
          this.stage = 'idle'
          break
      }
    },
    applyUser(user?: LoginUser) {
      this.loggedIn = true
      if (user) {
        this.userId = user.id
        this.username = user.username
        this.name = user.name
      }
    },
    async startCodeLogin(phone: string) {
      this.beginPending()
      try {
        await Auth.startCodeLogin(phone)
      } catch (e) {
        // FE-18：出错复位收敛到 action 内，组件不再直接改 stage
        this.resetPending()
        throw e
      }
    },
    async startQRLogin() {
      this.beginPending()
      try {
        await Auth.startQRLogin()
      } catch (e) {
        this.resetPending()
        throw e
      }
    },
    beginPending() {
      this.error = ''
      this.stage = 'pending'
      clearTimeout(pendingTimer)
      pendingTimer = setTimeout(() => {
        if (this.stage === 'pending') {
          this.stage = 'error'
          this.error = '登录请求长时间无响应，请重试'
        }
      }, PENDING_TIMEOUT_MS)
    },
    resetPending() {
      clearTimeout(pendingTimer)
      this.stage = 'idle'
    },
    async cancel() {
      await Auth.cancelLogin()
      clearTimeout(pendingTimer)
      this.stage = 'idle'
      this.qrUrl = ''
    },
    async logout() {
      await Auth.logout()
    },
  },
})
