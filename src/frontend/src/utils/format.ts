// 展示格式化纯函数（FE-11/FE-15：从页面组件抽出，跨组件复用）

export const pad = (n: number) => String(n).padStart(2, '0')

/** 字节数 → 人类可读大小 */
export function fmtSize(n: number): string {
  if (!n || n < 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  let v = n
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(v >= 100 || i === 0 ? 0 : 1)} ${units[i]}`
}

/** 字节/秒 → 人类可读速度 */
export function fmtSpeed(bytesPerSec: number): string {
  if (!bytesPerSec || bytesPerSec <= 0) return '0 B/s'
  return `${fmtSize(bytesPerSec)}/s`
}

/** unix 秒 → YYYY-MM-DD */
export function fmtDate(unix: number): string {
  if (!unix) return '-'
  const d = new Date(unix * 1000)
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

/** unix 秒 → 今天显示时分 / 今年显示月-日 / 其余显示全日期 */
export function fmtShortTime(unix?: number): string {
  if (!unix) return ''
  const d = new Date(unix * 1000)
  const now = new Date()
  if (d.toDateString() === now.toDateString()) {
    return `${pad(d.getHours())}:${pad(d.getMinutes())}`
  }
  if (d.getFullYear() === now.getFullYear()) {
    return `${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
  }
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

/** 秒数 → mm:ss 或 h:mm:ss（播放器时长显示） */
export function fmtDuration(sec: number): string {
  if (!Number.isFinite(sec) || sec < 0) return '0:00'
  const s = Math.floor(sec % 60)
  const m = Math.floor((sec / 60) % 60)
  const h = Math.floor(sec / 3600)
  return h > 0 ? `${h}:${pad(m)}:${pad(s)}` : `${m}:${pad(s)}`
}

/** 文件名 → 小写扩展名（无扩展名返回 '-'） */
export function extOf(name: string): string {
  const i = name.lastIndexOf('.')
  return i >= 0 ? name.slice(i + 1).toLowerCase() : '-'
}

/** 对话类型 → 中文标签 */
export function typeLabel(t: string): string {
  switch (t) {
    case 'private':
      return '私聊'
    case 'group':
      return '群组'
    case 'channel':
      return '频道'
    default:
      return t || '未知'
  }
}

/** 对话类型 → Tag severity */
export function typeSeverity(t: string): 'success' | 'info' | 'warn' | 'secondary' {
  switch (t) {
    case 'private':
      return 'success'
    case 'group':
      return 'info'
    case 'channel':
      return 'warn'
    default:
      return 'secondary'
  }
}

/** 对话类型 → 图标 class */
export function typeIcon(t: string): string {
  switch (t) {
    case 'private':
      return 'pi pi-user'
    case 'group':
      return 'pi pi-users'
    case 'channel':
      return 'pi pi-megaphone'
    default:
      return 'pi pi-comment'
  }
}

/** 媒体类别 → 图标 class */
export function kindIcon(k: string): string {
  switch (k) {
    case 'video':
      return 'pi pi-video'
    case 'photo':
      return 'pi pi-image'
    case 'audio':
      return 'pi pi-volume-up'
    default:
      return 'pi pi-file'
  }
}

// ---- 视频可播性判定（WebView2 原生 <video> 支持范围） ----

/** 可在应用内直接播放的视频扩展名（mkv/avi/wmv/flv/rmvb/ts 等不在其中，走下载后观看） */
export const PLAYABLE_VIDEO_EXTS = ['mp4', 'm4v', 'mov', 'webm', 'ogv', 'ogg']

/** 可在应用内直接播放的视频 MIME */
export const PLAYABLE_VIDEO_MIMES = ['video/mp4', 'video/webm', 'video/ogg', 'video/quicktime']

/** 该视频能否用原生播放器播放：MIME 优先，缺失时回退扩展名（避免对不支持格式白发请求） */
export function isPlayableVideo(mime: string, name: string): boolean {
  const m = (mime || '').trim().toLowerCase()
  if (m) return PLAYABLE_VIDEO_MIMES.includes(m)
  return PLAYABLE_VIDEO_EXTS.includes(extOf(name || ''))
}

/** HTMLMediaElement.error.code → 友好文案 */
export function mediaErrorText(code?: number): string {
  switch (code) {
    case 1:
      return '播放已被中止'
    case 2:
      return '网络中断，视频加载失败'
    case 3:
      return '视频解码失败，编码格式不受支持'
    case 4:
      return '该格式无法在应用内播放'
    default:
      return '视频加载失败，请重试或下载后观看'
  }
}
