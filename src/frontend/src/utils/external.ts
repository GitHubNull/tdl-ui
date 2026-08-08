// 外部链接统一走系统浏览器；非 Wails 环境（vite 独立预览）回退 window.open
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'

export function openExternal(url: string): void {
  // 安全校验：仅允许 http/https 协议，防止 javascript: 等伪协议注入
  if (!/^https?:\/\//i.test(url)) {
    console.warn('Blocked non-http(s) URL:', url)
    return
  }
  if (window.runtime) {
    BrowserOpenURL(url)
  } else {
    // noopener + noreferrer 防止新页面通过 window.opener 访问源页面
    window.open(url, '_blank', 'noopener,noreferrer')
  }
}
