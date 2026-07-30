<template>
  <div ref="root" class="markdown-body" v-html="rendered.html" @click="onClick" />
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'

import { renderMarkdown, type TocItem } from '../utils/markdown'
import { openExternal } from '../utils/external'

const props = defineProps<{ source: string }>()
const emit = defineEmits<{ toc: [items: TocItem[]] }>()

const root = ref<HTMLElement | null>(null)
const rendered = computed(() => renderMarkdown(props.source))

watch(
  () => rendered.value.toc,
  (toc) => emit('toc', toc),
  { immediate: true },
)

/** 平滑滚动到章节内锚点，供页面目录调用。 */
function scrollTo(id: string) {
  const el = root.value?.querySelector<HTMLElement>(`#${CSS.escape(id)}`)
  el?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

// 事件委托：代码块一键复制、外链走系统浏览器、页内锚点平滑滚动
function onClick(e: MouseEvent) {
  const target = e.target as HTMLElement

  const copyBtn = target.closest<HTMLButtonElement>('.code-copy')
  if (copyBtn) {
    copyCode(copyBtn)
    return
  }

  const link = target.closest('a')
  if (!link) return
  e.preventDefault()
  const href = link.getAttribute('href') ?? ''
  if (href.startsWith('#')) {
    scrollTo(decodeURIComponent(href.slice(1)))
  } else if (/^https?:\/\//i.test(href)) {
    openExternal(href)
  }
}

// 复制代码块内容（行号列在 code 之外，textContent 不含行号）
async function copyCode(btn: HTMLButtonElement) {
  const code = btn.closest('.code-block')?.querySelector('code')?.textContent ?? ''
  try {
    await navigator.clipboard.writeText(code)
  } catch {
    // 剪贴板 API 不可用时回退到临时 textarea
    const ta = document.createElement('textarea')
    ta.value = code
    document.body.appendChild(ta)
    ta.select()
    document.execCommand('copy')
    ta.remove()
  }
  btn.textContent = '已复制 ✓'
  btn.classList.add('copied')
  setTimeout(() => {
    btn.textContent = '复制'
    btn.classList.remove('copied')
  }, 1500)
}

defineExpose({ scrollTo })
</script>

<!-- v-html 内容无法使用 scoped，样式统一挂在 .markdown-body 下 -->
<style>
.markdown-body {
  line-height: 1.75;
  color: var(--p-text-color);
  word-break: break-word;
}

.markdown-body h1 {
  font-size: 1.5rem;
  font-weight: 600;
  margin: 0 0 16px;
}

.markdown-body h2 {
  font-size: 1.2rem;
  font-weight: 600;
  margin: 32px 0 12px;
  padding-bottom: 8px;
  border-bottom: 1px solid var(--p-surface-200);
}

.app-dark .markdown-body h2 {
  border-bottom-color: var(--p-surface-700);
}

.markdown-body h3 {
  font-size: 1.05rem;
  font-weight: 600;
  margin: 24px 0 8px;
}

.markdown-body p {
  margin: 8px 0;
}

.markdown-body ul,
.markdown-body ol {
  margin: 8px 0;
  padding-left: 24px;
}

.markdown-body li {
  margin: 4px 0;
}

.markdown-body a {
  color: var(--p-primary-color);
  text-decoration: none;
}

.markdown-body a:hover {
  text-decoration: underline;
}

.markdown-body blockquote {
  margin: 12px 0;
  padding: 8px 16px;
  border-left: 3px solid var(--p-primary-color);
  background: var(--p-surface-100);
  border-radius: 0 8px 8px 0;
  color: var(--p-text-muted-color);
}

.app-dark .markdown-body blockquote {
  background: var(--p-surface-800);
}

.markdown-body blockquote p {
  margin: 4px 0;
}

.markdown-body table {
  border-collapse: collapse;
  margin: 12px 0;
  width: 100%;
  font-size: 13px;
}

.markdown-body th,
.markdown-body td {
  border: 1px solid var(--p-surface-200);
  padding: 8px 12px;
  text-align: left;
}

.app-dark .markdown-body th,
.app-dark .markdown-body td {
  border-color: var(--p-surface-700);
}

.markdown-body th {
  background: var(--p-surface-100);
  font-weight: 600;
}

.app-dark .markdown-body th {
  background: var(--p-surface-800);
}

.markdown-body code {
  font-family: 'Cascadia Code', Consolas, 'Courier New', monospace;
  font-size: 0.85em;
  background: var(--p-surface-100);
  border-radius: 4px;
  padding: 2px 6px;
}

/* {{icon:xxx}} 占位符渲染出的应用内置图标，随文字颜色适配明暗主题 */
.markdown-body .md-icon {
  display: inline-flex;
  width: 18px;
  height: 18px;
  vertical-align: -4px;
  color: var(--p-text-color);
}

.markdown-body .md-icon svg {
  width: 100%;
  height: 100%;
  display: block;
}

.app-dark .markdown-body code {
  background: var(--p-surface-800);
}

/* ---- 围栏代码块：工具栏 + 行号 + 代码 ---- */
.markdown-body .code-block {
  margin: 12px 0;
  border: 1px solid var(--p-surface-200);
  border-radius: 8px;
  background: var(--p-surface-100);
  overflow: hidden;
}

.app-dark .markdown-body .code-block {
  background: var(--p-surface-950);
  border-color: var(--p-surface-700);
}

.markdown-body .code-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 6px 12px;
  border-bottom: 1px solid var(--p-surface-200);
}

.app-dark .markdown-body .code-toolbar {
  border-bottom-color: var(--p-surface-700);
}

.markdown-body .code-lang {
  font-family: 'Cascadia Code', Consolas, 'Courier New', monospace;
  font-size: 12px;
  color: var(--p-text-muted-color);
}

.markdown-body .code-copy {
  font-size: 12px;
  padding: 2px 10px;
  border: 1px solid var(--p-surface-300);
  border-radius: 6px;
  background: var(--p-surface-0);
  color: var(--p-text-muted-color);
  cursor: pointer;
  transition:
    color 0.15s,
    border-color 0.15s;
}

.markdown-body .code-copy:hover {
  color: var(--p-primary-color);
  border-color: var(--p-primary-color);
}

.markdown-body .code-copy.copied {
  color: var(--p-green-600);
  border-color: var(--p-green-400);
}

.app-dark .markdown-body .code-copy {
  background: var(--p-surface-800);
  border-color: var(--p-surface-600);
  color: var(--p-text-muted-color);
}

.app-dark .markdown-body .code-copy:hover {
  color: var(--p-primary-color);
  border-color: var(--p-primary-color);
}

.app-dark .markdown-body .code-copy.copied {
  color: var(--p-green-400);
  border-color: var(--p-green-600);
}

.markdown-body .code-block pre {
  display: flex;
  margin: 0;
  padding: 14px 16px;
  border: none;
  border-radius: 0;
  background: none;
  overflow-x: auto;
}

/* 行号列：随代码横向滚动时固定在左侧，选中/复制不会带上 */
.markdown-body .line-gutter {
  position: sticky;
  left: 0;
  display: flex;
  flex-direction: column;
  flex-shrink: 0;
  padding-right: 12px;
  margin-right: 12px;
  border-right: 1px solid var(--p-surface-300);
  background: var(--p-surface-100);
  font-family: 'Cascadia Code', Consolas, 'Courier New', monospace;
  font-size: 13px;
  line-height: 1.6;
  text-align: right;
  color: var(--p-text-muted-color);
  user-select: none;
}

.app-dark .markdown-body .line-gutter {
  border-right-color: var(--p-surface-700);
  background: var(--p-surface-950);
}

.markdown-body pre code,
.app-dark .markdown-body pre code {
  background: none;
  padding: 0;
  font-size: 13px;
  line-height: 1.6;
}

.markdown-body hr {
  border: none;
  border-top: 1px solid var(--p-surface-200);
  margin: 24px 0;
}

.app-dark .markdown-body hr {
  border-top-color: var(--p-surface-700);
}

/* ---- highlight.js token 配色（亮色） ---- */
.markdown-body .hljs-keyword,
.markdown-body .hljs-literal {
  color: #af00db;
}

.markdown-body .hljs-string {
  color: #a31515;
}

.markdown-body .hljs-comment {
  color: #008000;
  font-style: italic;
}

.markdown-body .hljs-number {
  color: #098658;
}

.markdown-body .hljs-title,
.markdown-body .hljs-title.function_ {
  color: #795e26;
}

.markdown-body .hljs-type,
.markdown-body .hljs-built_in {
  color: #267f99;
}

.markdown-body .hljs-attr,
.markdown-body .hljs-attribute,
.markdown-body .hljs-variable {
  color: #001080;
}

/* ---- highlight.js token 配色（暗色） ---- */
.app-dark .markdown-body .hljs-keyword,
.app-dark .markdown-body .hljs-literal {
  color: #c586c0;
}

.app-dark .markdown-body .hljs-string {
  color: #ce9178;
}

.app-dark .markdown-body .hljs-comment {
  color: #6a9955;
}

.app-dark .markdown-body .hljs-number {
  color: #b5cea8;
}

.app-dark .markdown-body .hljs-title,
.app-dark .markdown-body .hljs-title.function_ {
  color: #dcdcaa;
}

.app-dark .markdown-body .hljs-type,
.app-dark .markdown-body .hljs-built_in {
  color: #4ec9b0;
}

.app-dark .markdown-body .hljs-attr,
.app-dark .markdown-body .hljs-attribute,
.app-dark .markdown-body .hljs-variable {
  color: #9cdcfe;
}
</style>
