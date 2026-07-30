// Markdown 渲染工具：markdown-it + highlight.js（按需注册语言，离线可用）
import MarkdownIt from 'markdown-it'
import hljs from 'highlight.js/lib/core'
import go from 'highlight.js/lib/languages/go'
import bash from 'highlight.js/lib/languages/bash'
import yaml from 'highlight.js/lib/languages/yaml'
import json from 'highlight.js/lib/languages/json'
import plaintext from 'highlight.js/lib/languages/plaintext'

// 应用内置图标（与左侧导航栏使用同一套 SVG，教程中经 {{icon:xxx}} 占位符引用）
import navChats from '../assets/icons/nav-chats.svg?raw'
import navDownload from '../assets/icons/nav-download.svg?raw'
import navScripts from '../assets/icons/nav-scripts.svg?raw'
import navLogs from '../assets/icons/nav-logs.svg?raw'
import navSettings from '../assets/icons/nav-settings.svg?raw'
import navTutorial from '../assets/icons/nav-tutorial.svg?raw'
import navAbout from '../assets/icons/nav-about.svg?raw'
import navAccount from '../assets/icons/nav-account.svg?raw'
import navThemeLight from '../assets/icons/nav-theme-light.svg?raw'
import navThemeDark from '../assets/icons/nav-theme-dark.svg?raw'

hljs.registerLanguage('go', go)
hljs.registerLanguage('bash', bash)
hljs.registerLanguage('yaml', yaml)
hljs.registerLanguage('json', json)
hljs.registerLanguage('plaintext', plaintext)

/** 章节内标题（h2/h3）目录项 */
export interface TocItem {
  level: 2 | 3
  text: string
  id: string
}

export interface RenderResult {
  html: string
  toc: TocItem[]
}

const md: MarkdownIt = new MarkdownIt({
  html: false, // 禁止内嵌 HTML，防注入
  linkify: true,
})

// 自定义围栏代码块渲染：工具栏（语言标签 + 一键复制），Go 脚本示例附带行号列
md.renderer.rules.fence = (tokens, idx) => {
  const token = tokens[idx]
  const lang = token.info.trim().split(/\s+/)[0] ?? ''
  const language = lang && hljs.getLanguage(lang) ? lang : 'plaintext'
  const highlighted = hljs.highlight(token.content, { language, ignoreIllegals: true }).value

  // 行号列：仅脚本示例（go）需要，行号在 code 之外，复制/选中不会带上
  const withLines = language === 'go'
  let gutter = ''
  if (withLines) {
    const count = token.content.replace(/\n$/, '').split('\n').length
    const nums = Array.from({ length: count }, (_, i) => `<span>${i + 1}</span>`).join('')
    const gutterHtml = `<span class="line-gutter" aria-hidden="true">${nums}</span>`
    gutter = gutterHtml
  }

  return (
    `<div class="code-block${withLines ? ' has-lines' : ''}">` +
    `<div class="code-toolbar"><span class="code-lang">${language}</span>` +
    `<button type="button" class="code-copy">复制</button></div>` +
    `<pre>${gutter}<code class="hljs">${highlighted}</code></pre>` +
    `</div>`
  )
}

// 标题转锚点 id：保留中日韩等 Unicode 字母与数字，空白转连字符
function slugify(text: string): string {
  const slug = text
    .trim()
    .toLowerCase()
    .replace(/\s+/g, '-')
    .replace(/[^\p{L}\p{N}\-_]/gu, '')
  return slug || 'section'
}

// 图标白名单：仅内置 SVG 可被占位符替换，用户内容无法注入任意 HTML
const INLINE_ICONS: Record<string, string> = {
  chats: navChats,
  download: navDownload,
  scripts: navScripts,
  logs: navLogs,
  settings: navSettings,
  tutorial: navTutorial,
  about: navAbout,
  account: navAccount,
  'theme-light': navThemeLight,
  'theme-dark': navThemeDark,
}

// 将渲染结果中的 {{icon:xxx}} 占位符替换为内联 SVG（未命中白名单则原样保留）
function replaceIconPlaceholders(html: string): string {
  return html.replace(/\{\{icon:([a-z-]+)\}\}/g, (raw, name: string) => {
    const svg = INLINE_ICONS[name]
    return svg ? `<span class="md-icon" aria-hidden="true">${svg}</span>` : raw
  })
}

/** 渲染 Markdown 为 HTML，同时为 h2/h3 注入锚点 id 并提取目录。 */
export function renderMarkdown(source: string): RenderResult {
  const tokens = md.parse(source, {})
  const toc: TocItem[] = []
  const used = new Map<string, number>()

  for (let i = 0; i < tokens.length; i++) {
    const token = tokens[i]
    if (token.type !== 'heading_open' || (token.tag !== 'h2' && token.tag !== 'h3')) continue
    const inline = tokens[i + 1]
    const text =
      inline?.children
        ?.filter((c) => c.type === 'text' || c.type === 'code_inline')
        .map((c) => c.content)
        .join('') ?? ''
    let id = slugify(text)
    const count = used.get(id) ?? 0
    used.set(id, count + 1)
    if (count > 0) id = `${id}-${count}`
    token.attrSet('id', id)
    toc.push({ level: token.tag === 'h2' ? 2 : 3, text, id })
  }

  return { html: replaceIconPlaceholders(md.renderer.render(tokens, md.options, {})), toc }
}
