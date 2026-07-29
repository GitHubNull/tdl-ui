// 亮/暗主题管理：单一响应式状态源（FE-08），localStorage 持久化，system 跟随操作系统。
import { computed, ref } from 'vue'

export type ThemeMode = 'light' | 'dark' | 'system'

const STORAGE_KEY = 'tdlui-theme'
const media = window.matchMedia('(prefers-color-scheme: dark)')

/** 当前主题模式（全局唯一状态源，App/设置页均从此派生） */
export const themeMode = ref<ThemeMode>('system')
const systemDark = ref(media.matches)

/** 当前是否实际处于暗色（含 system 跟随结果） */
export const isDark = computed(
  () => themeMode.value === 'dark' || (themeMode.value === 'system' && systemDark.value),
)

function apply() {
  document.documentElement.classList.toggle('app-dark', isDark.value)
}

/** 应用启动时恢复主题偏好并监听系统变化。 */
export function initTheme() {
  const saved = localStorage.getItem(STORAGE_KEY) as ThemeMode | null
  if (saved === 'light' || saved === 'dark' || saved === 'system') {
    themeMode.value = saved
  }
  media.addEventListener('change', (e) => {
    systemDark.value = e.matches
    apply()
  })
  apply()
}

/** 切换主题模式并持久化。 */
export function setTheme(mode: ThemeMode) {
  themeMode.value = mode
  localStorage.setItem(STORAGE_KEY, mode)
  apply()
}

export function getTheme(): ThemeMode {
  return themeMode.value
}
