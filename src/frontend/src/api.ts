// Wails 绑定与事件的类型安全封装。
// 运行于 Wails 窗口时 window.go / window.runtime 由运行时注入；
// 纯浏览器环境（vite dev 独立预览）下调用会抛错，由页面层兜底提示。
import type {
  DesktopAccount,
  DialogView,
  LoginStatus,
  MediaPage,
  MediaQuery,
  ScriptMeta,
  Settings,
  TaskOptions,
  TaskView,
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
  listTasks: (): Promise<TaskView[]> => svc('DownloadService').ListTasks(),
  pauseTask: (id: string): Promise<void> => svc('DownloadService').PauseTask(id),
  resumeTask: (id: string): Promise<void> => svc('DownloadService').ResumeTask(id),
  cancelTask: (id: string): Promise<void> => svc('DownloadService').CancelTask(id),
  removeTask: (id: string): Promise<void> => svc('DownloadService').RemoveTask(id),
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
}

// ---- ChatService ----
export const Chat = {
  listDialogs: (): Promise<DialogView[]> => svc('ChatService').ListDialogs(),
  listMedia: (q: MediaQuery): Promise<MediaPage> => svc('ChatService').ListMedia(q),
}

// ---- 事件契约（与 internal/events/events.go 一致） ----
export const EVENT_LOGIN = 'login:update'
export const EVENT_TASK = 'task:update'
export const EVENT_TASK_FILE = 'task:file'
export const EVENT_SCRIPT_LOG = 'script:log'

/** 订阅 Wails 事件，返回取消函数；非 Wails 环境下为 no-op。 */
export function on<T = any>(event: string, cb: (payload: T) => void): () => void {
  const rt = window.runtime
  if (!rt?.EventsOn) return () => {}
  return rt.EventsOn(event, cb)
}
