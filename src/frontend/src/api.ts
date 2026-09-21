// 与后端的绑定通道：直接复用 wailsjs 生成绑定（单一事实来源，ARC-06/FE-03）。
// 生成函数内部经 window['go'] 动态调用；纯浏览器环境（vite dev 独立预览）下
// 调用会抛错，由页面层 try/catch 兜底提示。
import * as AuthService from '../wailsjs/go/services/AuthService'
import * as ChatService from '../wailsjs/go/services/ChatService'
import * as DownloadService from '../wailsjs/go/services/DownloadService'
import * as LogService from '../wailsjs/go/services/LogService'
import * as ScriptService from '../wailsjs/go/services/ScriptService'
import * as SettingsService from '../wailsjs/go/services/SettingsService'
import { EventsOn } from '../wailsjs/runtime/runtime'

declare global {
  interface Window {
    runtime?: unknown
  }
}

// ---- AuthService ----
export const Auth = {
  status: AuthService.Status,
  startCodeLogin: AuthService.StartCodeLogin,
  startQRLogin: AuthService.StartQRLogin,
  submitCode: AuthService.SubmitCode,
  submitPassword: AuthService.SubmitPassword,
  cancelLogin: AuthService.CancelLogin,
  logout: AuthService.Logout,
  detectDesktopPath: AuthService.DetectDesktopPath,
  listDesktopAccounts: AuthService.ListDesktopAccounts,
  importDesktopSession: AuthService.ImportDesktopSession,
}

// ---- DownloadService ----
export const Download = {
  createTask: DownloadService.CreateTask,
  listTasks: DownloadService.ListTasks,
  pauseTask: DownloadService.PauseTask,
  resumeTask: DownloadService.ResumeTask,
  cancelTask: DownloadService.CancelTask,
  removeTask: DownloadService.RemoveTask,
  listTaskFiles: DownloadService.ListTaskFiles,
  clearFinishedTasks: DownloadService.ClearFinishedTasks,
  deleteTaskFiles: DownloadService.DeleteTaskFiles,
  deleteAllFiles: DownloadService.DeleteAllFiles,
  openTaskDir: DownloadService.OpenTaskDir,
  selectDirectory: DownloadService.SelectDirectory,
  listDownloadedMessages: DownloadService.ListDownloadedMessages,
  openDownloadedFile: DownloadService.OpenDownloadedFile,
  redownloadFile: DownloadService.RedownloadFile,
  deleteFileRecord: DownloadService.DeleteFileRecord,
  revealFile: DownloadService.RevealFileInDir,
  resumeTaskWithPending: DownloadService.ResumeTaskWithPending,
  getTotalSpeed: DownloadService.GetTotalSpeed,
}

// ---- ScriptService ----
export const Script = {
  list: ScriptService.List,
  read: ScriptService.Read,
  save: ScriptService.Save,
  remove: ScriptService.Delete,
  setEnabled: ScriptService.SetEnabled,
  validate: ScriptService.Validate,
  testRun: ScriptService.TestRun,
  templates: ScriptService.Templates,
  starterTemplate: ScriptService.StarterTemplate,
}

// ---- SettingsService ----
export const SettingsApi = {
  get: SettingsService.Get,
  save: SettingsService.Save,
  dataDir: SettingsService.DataDir,
  clearCache: SettingsService.ClearCache,
  recentDirs: SettingsService.RecentDirs,
  addRecentDir: SettingsService.AddRecentDir,
}

// ---- ChatService ----
export const Chat = {
  listDialogs: ChatService.ListDialogs,
  listMedia: ChatService.ListMedia,
  listMediaBatch: ChatService.ListMediaBatch,
  stopVideoPrefetch: ChatService.StopVideoPrefetch,
  // 性能优化相关 API
  preloadThumbs: ChatService.PreloadThumbs,
  preloadVisibleThumbs: ChatService.PreloadVisibleThumbs,
  getThumbPreloadStats: ChatService.GetThumbPreloadStats,
  warmupCache: ChatService.WarmupCache,
  getCacheStats: ChatService.GetCacheStats,
  getCacheHitRate: ChatService.GetCacheHitRate,
  setPreloadEnabled: ChatService.SetPreloadEnabled,
  isPreloadEnabled: ChatService.IsPreloadEnabled,
  reportPerformance: ChatService.ReportPerformance,
}

// ---- LogService ----
export const LogApi = {
  getRecent: LogService.GetRecent,
  listLogFiles: LogService.ListLogFiles,
  readLogFile: LogService.ReadLogFile,
  importYAMLConfig: LogService.ImportYAMLConfig,
  exportYAMLConfig: LogService.ExportYAMLConfig,
  exportLogs: LogService.ExportLogs,
  openLogDir: LogService.OpenLogDir,
}

// ---- 媒体图 HTTP 通道（后端 assetserver Handler，浏览器接管并发与缓存） ----
export function thumbURL(dialogId: number, dialogType: string, messageId: number): string {
  return `/media/thumb?d=${dialogId}&m=${messageId}&t=${encodeURIComponent(dialogType)}`
}

export function previewURL(dialogId: number, dialogType: string, messageId: number): string {
  return `/media/preview?d=${dialogId}&m=${messageId}&t=${encodeURIComponent(dialogType)}`
}

/** 在线视频分段流（后端按 Range 从 Telegram 拉取，未下载时使用）。 */
export function videoStreamURL(dialogId: number, dialogType: string, messageId: number): string {
  return `/media/video?d=${dialogId}&m=${messageId}&t=${encodeURIComponent(dialogType)}`
}

/** 已下载文件字节流（http.ServeContent，原生 Range/seek，零 API 消耗）。 */
export function localMediaURL(dialogId: number, messageId: number): string {
  return `/media/local?d=${dialogId}&m=${messageId}`
}

// ---- 事件契约（与 internal/events/events.go 一致） ----
export const EVENT_LOGIN = 'login:update'
export const EVENT_TASK = 'task:update'
export const EVENT_TASK_FILE = 'task:file'
export const EVENT_SCRIPT_LOG = 'script:log'
export const EVENT_LOG = 'log:batch'

/** 订阅 Wails 事件，返回取消函数；非 Wails 环境下为 no-op。 */
export function on<T = unknown>(event: string, cb: (payload: T) => void): () => void {
  // wailsjs/runtime 直接访问 window.runtime，非 Wails 环境需先行守卫
  if (!window.runtime) return () => {}
  return EventsOn(event, (payload: T) => cb(payload))
}
