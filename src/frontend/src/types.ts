// 与后端 Go 结构体一一对应的类型契约（JSON tag 命名）。

/** internal/config.Settings */
export interface Settings {
  proxy: string
  downloadDir: string
  template: string
  threads: number
  limit: number
  poolSize: number
  theme: 'light' | 'dark' | 'system'
  loggedInUserId: number
  loggedInUsername: string
}

/** services.LoginStatus */
export interface LoginStatus {
  loggedIn: boolean
  userId: number
  username: string
}

/** events.User */
export interface LoginUser {
  id: number
  username: string
  name: string
}

/** events.LoginUpdate（login:update 事件负载） */
export interface LoginUpdate {
  stage: 'qr' | 'need_code' | 'need_password' | 'success' | 'error' | 'logout'
  qrUrl?: string
  user?: LoginUser
  error?: string
}

/** engine.Selection（按对话 + 消息 ID 直接选集下载） */
export interface Selection {
  dialogId: number
  dialogType: string
  messageIds: number[]
}

/** engine.TaskOptions */
export interface TaskOptions {
  urls: string[]
  selections?: Selection[]
  label?: string
  dir: string
  scriptName: string
  template: string
  rewriteExt: boolean
  skipSame: boolean
  group: boolean
  restart: boolean
}

/** engine.TaskView（task:update 事件负载） */
export interface TaskView {
  id: string
  urls: string[]
  label?: string
  dir: string
  scriptName: string
  status: 'queued' | 'running' | 'paused' | 'done' | 'failed' | 'canceled'
  error?: string
  total: number
  finished: number
  failed: number
  createdAt: string
}

/** engine.FileEvent（task:file 事件负载） */
export interface FileEvent {
  taskId: string
  fileId: number
  dialogId: number
  messageId: number
  name: string
  total: number
  downloaded: number
  state: 'downloading' | 'done' | 'failed'
  error?: string
}

/** script.Meta */
export interface ScriptMeta {
  name: string
  size: number
  updatedAt: string
}

/** services.ValidateResult */
export interface ValidateResult {
  ok: boolean
  error?: string
  funcs: string[]
}

/** services.TestRunResult */
export interface TestRunResult {
  ok: boolean
  error?: string
  sampleFile: string
  filterKeep?: boolean
  renameTo?: string
}

/** services.DesktopAccount */
export interface DesktopAccount {
  userId: string
}

/** services.Dialog */
export interface DialogView {
  id: number
  type: 'private' | 'group' | 'channel'
  title: string
  username?: string
  unreadCount: number
  lastMessageAt?: number // unix 秒
}

/** services.MediaItem */
export interface MediaItem {
  dialogId: number
  messageId: number
  name: string
  caption: string
  size: number
  mime: string
  kind: 'video' | 'photo' | 'audio' | 'file'
  date: number // unix 秒
  thumb?: string // 内嵌模糊占位图（data URI，可空）
}

/** services.MediaQuery */
export interface MediaQuery {
  dialogId: number
  dialogType: string
  offsetId: number
  limit: number
  query: string
  kinds: string[]
  exts: string[]
  minSize: number
  maxSize: number
}

/** services.MediaPage */
export interface MediaPage {
  items: MediaItem[]
  nextOffset: number // 0=没有更多
}
