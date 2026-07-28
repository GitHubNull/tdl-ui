// Wails 绑定与事件的类型安全封装。
// 运行于 Wails 窗口时 window.go / window.runtime 由运行时注入；
// 纯浏览器环境（vite dev 独立预览）下调用会抛错，由页面层兜底提示。
import type {
  DesktopAccount,
  DialogView,
  LogEntry,
  LogFileInfo,
  LogSettings,
  LoginStatus,
  MediaPage,
  MediaQuery,
  ScriptMeta,
  Settings,
  TaskFile,
  TaskOptions,
  TaskView,
  AppendOptions,
  TestRunResult,
  ValidateResult,
} from './types'

/* eslint-disable @typescript-eslint/no-explicit-any */
declare global {
  interface Window {
    go?: any
    runtime?: any
  }
}

function svc(name: string): any {
  const s = window.go?.services?.[name]
  if (!s) {
    throw new Error('Wails 绑定不可用：请通过 wails dev / 构建后的应用访问')
  }
  return s
}

// ---- AuthService ----
export const Auth = {
  status: (): Promise<LoginStatus> => svc('AuthService').Status(),
  startCodeLogin: (phone: string): Promise<void> => svc('AuthService').StartCodeLogin(phone),
  startQRLogin: (): Promise<void> => svc('AuthService').StartQRLogin(),
  submitCode: (code: string): Promise<void> => svc('AuthService').SubmitCode(code),
  submitPassword: (pwd: string): Promise<void> => svc('AuthService').SubmitPassword(pwd),
  cancelLogin: (): Promise<void> => svc('AuthService').CancelLogin(),
  logout: (): Promise<void> => svc('AuthService').Logout(),
  detectDesktopPath: (): Promise<string> => svc('AuthService').DetectDesktopPath(),
  listDesktopAccounts: (path: string, passcode: string): Promise<DesktopAccount[]> =>
    svc('AuthService').ListDesktopAccounts(path, passcode),
  importDesktopSession: (path: string, passcode: string, userId: string): Promise<void> =>
    svc('AuthService').ImportDesktopSession(path, passcode, userId),
}

// ---- DownloadService ----
export const Download = {
  createTask: (opts: TaskOptions): Promise<string> => svc('DownloadService').CreateTask(opts),
  appendTaskItems: (id: string, opts: AppendOptions): Promise<void> =>
    svc('DownloadService').AppendTaskItems(id, opts),
  listTasks: (): Promise<TaskView[]> => svc('DownloadService').ListTasks(),
  pauseTask: (id: string): Promise<void> => svc('DownloadService').PauseTask(id),
  resumeTask: (id: string): Promise<void> => svc('DownloadService').ResumeTask(id),
  cancelTask: (id: string): Promise<void> => svc('DownloadService').CancelTask(id),
  removeTask: (id: string): Promise<void> => svc('DownloadService').RemoveTask(id),
  listTaskFiles: (id: string): Promise<TaskFile[]> => svc('DownloadService').ListTaskFiles(id),
  clearFinishedTasks: (): Promise<void> => svc('DownloadService').ClearFinishedTasks(),
  deleteTaskFiles: (id: string, paths: string[]): Promise<void> =>
    svc('DownloadService').DeleteTaskFiles(id, paths),
  deleteAllFiles: (): Promise<void> => svc('DownloadService').DeleteAllFiles(),
  openTaskDir: (id: string): Promise<void> => svc('DownloadService').OpenTaskDir(id),
  selectDirectory: (): Promise<string> => svc('DownloadService').SelectDirectory(),
}

// ---- ScriptService ----
export const Script = {
  list: (): Promise<ScriptMeta[]> => svc('ScriptService').List(),
  read: (name: string): Promise<string> => svc('ScriptService').Read(name),
  save: (name: string, src: string): Promise<void> => svc('ScriptService').Save(name, src),
  remove: (name: string): Promise<void> => svc('ScriptService').Delete(name),
  validate: (src: string): Promise<ValidateResult> => svc('ScriptService').Validate(src),
  testRun: (src: string): Promise<TestRunResult> => svc('ScriptService').TestRun(src),
  starterTemplate: (): Promise<string> => svc('ScriptService').StarterTemplate(),
}

// ---- SettingsService ----
export const SettingsApi = {
  get: (): Promise<Settings> => svc('SettingsService').Get(),
  save: (s: Settings): Promise<void> => svc('SettingsService').Save(s),
  dataDir: (): Promise<string> => svc('SettingsService').DataDir(),
  clearCache: (): Promise<void> => svc('SettingsService').ClearCache(),
}

// ---- ChatService ----
export const Chat = {
  listDialogs: (): Promise<DialogView[]> => svc('ChatService').ListDialogs(),
  listMedia: (q: MediaQuery): Promise<MediaPage> => svc('ChatService').ListMedia(q),
}

// ---- LogService ----
export const LogApi = {
  getRecent: (): Promise<LogEntry[]> => svc('LogService').GetRecent(),
  listLogFiles: (): Promise<LogFileInfo[]> => svc('LogService').ListLogFiles(),
  readLogFile: (name: string, maxLines: number): Promise<string[]> =>
    svc('LogService').ReadLogFile(name, maxLines),
  importYAMLConfig: (): Promise<LogSettings> => svc('LogService').ImportYAMLConfig(),
  exportYAMLConfig: (): Promise<void> => svc('LogService').ExportYAMLConfig(),
  openLogDir: (): Promise<void> => svc('LogService').OpenLogDir(),
}

// ---- 媒体图 HTTP 通道（后端 assetserver Handler，浏览器接管并发与缓存） ----
export function thumbURL(dialogId: number, dialogType: string, messageId: number): string {
  return `/media/thumb?d=${dialogId}&m=${messageId}&t=${encodeURIComponent(dialogType)}`
}

export function previewURL(dialogId: number, dialogType: string, messageId: number): string {
  return `/media/preview?d=${dialogId}&m=${messageId}&t=${encodeURIComponent(dialogType)}`
}

// ---- 事件契约（与 internal/events/events.go 一致） ----
export const EVENT_LOGIN = 'login:update'
export const EVENT_TASK = 'task:update'
export const EVENT_TASK_FILE = 'task:file'
export const EVENT_SCRIPT_LOG = 'script:log'
export const EVENT_LOG = 'log:batch'

/** 订阅 Wails 事件，返回取消函数；非 Wails 环境下为 no-op。 */
export function on<T = any>(event: string, cb: (payload: T) => void): () => void {
  const rt = window.runtime
  if (!rt?.EventsOn) return () => {}
  return rt.EventsOn(event, cb)
}
