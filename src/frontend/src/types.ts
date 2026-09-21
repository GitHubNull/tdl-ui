// 类型单一事实来源为 wailsjs/go/models.ts（wails 生成，ARC-06/FE-03）；
// 本文件仅做命名别名 + 补充 models.ts 没有的事件负载类型。
import type { config, engine, logging, script, services } from '../wailsjs/go/models'

// ---- 生成类型别名（保持既有前端命名，不重复维护字段） ----
export type Settings = config.Settings
export type LogSettings = logging.LogSettings
export type LogEntry = logging.LogEntry
export type LogFileInfo = services.LogFileInfo
export type LoginStatus = services.LoginStatus
export type Selection = engine.Selection
export type TaskOptions = engine.TaskOptions
export type TaskView = engine.TaskView
export type TaskFile = engine.TaskFile
export type ScriptMeta = script.Meta
export type ScriptTemplate = script.Template
export type ValidateResult = services.ValidateResult
export type TestRunResult = services.TestRunResult
export type DesktopAccount = services.DesktopAccount
export type DialogView = services.Dialog
export type MediaItem = services.MediaItem
export type MediaQuery = services.MediaQuery
export type MediaPage = services.MediaPage

// ---- 事件负载（Wails 事件不生成绑定，与 internal/events/events.go 手工对齐） ----

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
  /** 仅 done 时携带：最终文件路径 */
  path?: string
  /** 当前下载速度，字节/秒；仅 downloading 时有效 */
  speed?: number
}

/** services.DownloadedFile（对话内已下载消息，"已下载"标记数据源） */
export interface DownloadedFile {
  messageId: number
  path: string
  size: number
}
