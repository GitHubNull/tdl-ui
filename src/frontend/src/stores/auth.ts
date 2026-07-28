import { defineStore } from 'pinia'
import { Auth, EVENT_LOGIN, on } from '../api'
import type { LoginUpdate, LoginUser } from '../types'

/** 登录状态与登录流程事件。 */
export const useAuthStore = defineStore('auth', {
  state: () => ({
    loggedIn: false,
    userId: 0,
    username: '',
    name: '',
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

      on<LoginUpdate>(EVENT_LOGIN, (u) => this.handleUpdate(u))
      try {
        const st = await Auth.status()
        this.loggedIn = st.loggedIn
        this.userId = st.userId
        this.username = st.username
      } catch {
        /* 非 Wails 环境忽略 */
      }
    },
    handleUpdate(u: LoginUpdate) {
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
      this.error = ''
      this.stage = 'pending'
      await Auth.startCodeLogin(phone)
    },
    async startQRLogin() {
      this.error = ''
      this.stage = 'pending'
      await Auth.startQRLogin()
    },
    async cancel() {
      await Auth.cancelLogin()
      this.stage = 'idle'
      this.qrUrl = ''
    },
    async logout() {
      await Auth.logout()
    },
  },
})
