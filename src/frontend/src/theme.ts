// 亮/暗主题管理：localStorage 持久化，system 跟随操作系统。
export type ThemeMode = 'light' | 'dark' | 'system'

const STORAGE_KEY = 'tdlui-theme'
const media = window.matchMedia('(prefers-color-scheme: dark)')

let current: ThemeMode = 'system'

function apply() {
  const dark = current === 'dark' || (current === 'system' && media.matches)
  document.documentElement.classList.toggle('app-dark', dark)
}

/** 应用启动时恢复主题偏好并监听系统变化。 */
export function initTheme() {
  const saved = localStorage.getItem(STORAGE_KEY) as ThemeMode | null
  if (saved === 'light' || saved === 'dark' || saved === 'system') {
    current = saved
  }
  media.addEventListener('change', () => {
    if (current === 'system') apply()
  })
  apply()
}

/** 切换主题模式并持久化。 */
export function setTheme(mode: ThemeMode) {
  current = mode
  localStorage.setItem(STORAGE_KEY, mode)
  apply()
}

export function getTheme(): ThemeMode {
  return current
}
