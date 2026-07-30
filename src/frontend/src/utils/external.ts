// 外部链接统一走系统浏览器；非 Wails 环境（vite 独立预览）回退 window.open
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'

export function openExternal(url: string): void {
  if (window.runtime) {
    BrowserOpenURL(url)
  } else {
    window.open(url, '_blank', 'noopener')
  }
}
