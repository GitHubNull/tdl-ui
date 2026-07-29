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
