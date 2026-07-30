<template>
  <div class="page page-table tutorial-page">
    <div class="page-header">
      <h1>使用教程</h1>
      <p>按 基础 → 中级 → 高级 分级组织，左侧选择章节阅读</p>
    </div>

    <div class="tutorial-body">
      <!-- 左侧章节导航 -->
      <aside class="chapter-nav panel-card">
        <template v-for="level in levels" :key="level">
          <div class="level-label">
            <span class="level-dot" :class="level" />
            {{ LEVEL_LABELS[level] }}
          </div>
          <template v-for="ch in chaptersOf(level)" :key="ch.id">
            <div
              class="chapter-item"
              :class="{ active: ch.id === current.id }"
              role="button"
              @click="goChapter(ch.id)"
            >
              <div class="chapter-title">{{ ch.title }}</div>
              <div class="chapter-summary">{{ ch.summary }}</div>
            </div>
            <!-- 当前章节的小节目录：点击平滑跳转 -->
            <div v-if="ch.id === current.id && toc.length" class="chapter-toc">
              <div
                v-for="item in toc"
                :key="item.id"
                class="toc-item"
                :class="{ sub: item.level === 3 }"
                role="button"
                @click="scrollToSection(item.id)"
              >
                {{ item.text }}
              </div>
            </div>
          </template>
        </template>
      </aside>

      <!-- 右侧正文 -->
      <section ref="contentEl" class="chapter-content panel-card">
        <MarkdownView ref="mdView" :source="current.source" @toc="toc = $event" />
        <div class="chapter-footer">
          <Button
            v-if="prevChapter"
            :label="`上一章：${prevChapter.title}`"
            icon="pi pi-arrow-left"
            severity="secondary"
            outlined
            @click="goChapter(prevChapter.id)"
          />
          <span class="spacer" />
          <Button
            v-if="nextChapter"
            :label="`下一章：${nextChapter.title}`"
            icon="pi pi-arrow-right"
            icon-pos="right"
            @click="goChapter(nextChapter.id)"
          />
        </div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'

import MarkdownView from '../components/MarkdownView.vue'
import { chapters, findChapter, LEVEL_LABELS, type TutorialLevel } from '../content/tutorials'
import type { TocItem } from '../utils/markdown'

const route = useRoute()
const router = useRouter()

const levels: TutorialLevel[] = ['basic', 'intermediate', 'advanced']
const chaptersOf = (level: TutorialLevel) => chapters.filter((c) => c.level === level)

const current = computed(() => findChapter(route.params.chapter as string | undefined) ?? chapters[0])
const currentIndex = computed(() => chapters.findIndex((c) => c.id === current.value.id))
const prevChapter = computed(() => chapters[currentIndex.value - 1])
const nextChapter = computed(() => chapters[currentIndex.value + 1])

const toc = ref<TocItem[]>([])
const contentEl = ref<HTMLElement | null>(null)
const mdView = ref<InstanceType<typeof MarkdownView> | null>(null)

function goChapter(id: string) {
  router.push(`/tutorial/${id}`)
}

function scrollToSection(id: string) {
  mdView.value?.scrollTo(id)
}

// 切换章节后正文滚动复位
watch(
  () => current.value.id,
  () => {
    contentEl.value?.scrollTo({ top: 0 })
  },
)
</script>

<style scoped>
.tutorial-body {
  flex: 1;
  min-height: 0;
  display: flex;
  gap: 16px;
}

/* ---- 左侧章节导航 ---- */
.chapter-nav {
  width: 264px;
  flex-shrink: 0;
  overflow-y: auto;
  padding: 16px 12px;
}

.level-label {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  font-weight: 600;
  color: var(--p-text-muted-color);
  text-transform: uppercase;
  letter-spacing: 0.05em;
  padding: 8px 12px 4px;
}

.level-label:not(:first-child) {
  margin-top: 8px;
}

.level-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
}

.level-dot.basic {
  background: var(--p-green-500);
}

.level-dot.intermediate {
  background: var(--p-amber-500);
}

.level-dot.advanced {
  background: var(--p-red-500);
}

.chapter-item {
  padding: 8px 12px;
  border-radius: 8px;
  cursor: pointer;
  transition: background 0.15s ease;
}

.chapter-item:hover {
  background: var(--p-surface-100);
}

.app-dark .chapter-item:hover {
  background: var(--p-surface-800);
}

.chapter-item.active {
  background: var(--p-primary-50);
}

.app-dark .chapter-item.active {
  background: color-mix(in srgb, var(--p-primary-color) 20%, transparent);
}

.chapter-item.active .chapter-title {
  color: var(--p-primary-color);
}

.chapter-title {
  font-weight: 500;
  font-size: 13px;
}

.chapter-summary {
  font-size: 12px;
  color: var(--p-text-muted-color);
  margin-top: 2px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* ---- 当前章节小节目录 ---- */
.chapter-toc {
  margin: 4px 0 8px;
  padding-left: 16px;
  border-left: 2px solid var(--p-surface-200);
  margin-left: 16px;
}

.app-dark .chapter-toc {
  border-left-color: var(--p-surface-700);
}

.toc-item {
  font-size: 12px;
  color: var(--p-text-muted-color);
  padding: 3px 8px;
  border-radius: 6px;
  cursor: pointer;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.toc-item.sub {
  padding-left: 20px;
}

.toc-item:hover {
  color: var(--p-primary-color);
  background: var(--p-surface-100);
}

.app-dark .toc-item:hover {
  background: var(--p-surface-800);
}

/* ---- 右侧正文 ---- */
.chapter-content {
  flex: 1;
  min-width: 0;
  overflow-y: auto;
  padding: 24px 32px;
}

.chapter-footer {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 32px;
  padding-top: 16px;
  border-top: 1px solid var(--p-surface-200);
}

.app-dark .chapter-footer {
  border-top-color: var(--p-surface-700);
}

.chapter-footer .spacer {
  flex: 1;
}
</style>
