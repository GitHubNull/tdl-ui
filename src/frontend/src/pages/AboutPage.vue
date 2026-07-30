<template>
  <div class="page about-page">
    <div class="page-header">
      <h1>关于</h1>
      <p>版本信息、项目介绍与反馈渠道</p>
    </div>

    <!-- 英雄区：应用标识 + 简介 + 快捷操作，整体居中 -->
    <div class="panel-card hero-card">
      <img class="app-logo" :src="appIcon" alt="tdl UI" draggable="false" />
      <div class="app-name">tdl UI</div>
      <div class="app-version">v{{ version }}</div>
      <p class="intro">
        tdl UI 是命令行工具 tdl 的桌面 GUI 化改造：复用其经过验证的下载引擎与会话体系，
        提供对话浏览、媒体筛选、批量下载、断点续传等能力，并内置 Yaegi Go
        脚本引擎实现灵活的下载过滤、重命名与任务自动化。
      </p>
      <div class="link-row">
        <Button
          label="GitHub 仓库"
          icon="pi pi-github"
          severity="secondary"
          outlined
          @click="openExternal(REPO_URL)"
        />
        <Button
          label="问题反馈"
          icon="pi pi-comment"
          severity="secondary"
          outlined
          @click="openExternal(`${REPO_URL}/issues`)"
        />
        <Button
          label="上游 tdl 项目"
          icon="pi pi-external-link"
          severity="secondary"
          outlined
          @click="openExternal('https://github.com/GitHubNull/tdl')"
        />
        <Button label="查看使用教程" icon="pi pi-book" @click="router.push('/tutorial')" />
      </div>
    </div>

    <!-- 技术栈 -->
    <div class="panel-card">
      <h2 class="section-title">技术栈</h2>
      <div class="tech-grid">
        <div v-for="t in techStack" :key="t.name" class="tech-item">
          <span class="tech-name">{{ t.name }}</span>
          <span class="tech-desc">{{ t.desc }}</span>
        </div>
      </div>
    </div>

    <!-- 作者与许可证 -->
    <div class="panel-card">
      <h2 class="section-title">作者与许可证</h2>
      <div class="meta-grid">
        <div class="meta-item">
          <span class="meta-label">作者</span>
          <span class="meta-value">tdl-ui contributors</span>
        </div>
        <div class="meta-item">
          <span class="meta-label">许可证</span>
          <span class="meta-value">AGPL-3.0</span>
        </div>
        <div class="meta-item">
          <span class="meta-label">版权</span>
          <span class="meta-value">Copyright (c) 2026 tdl-ui contributors</span>
        </div>
        <div class="meta-item">
          <span class="meta-label">上游代码</span>
          <span class="meta-value">复用上游 tdl 代码（Git 子模块引用），依其协议发布</span>
        </div>
      </div>
      <p class="hint disclaimer">
        本工具仅供学习与个人备份用途，请遵守 Telegram 服务条款与所在地法律法规。
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useRouter } from 'vue-router'
import Button from 'primevue/button'

import appIcon from '../assets/app-icon.svg'
import { openExternal } from '../utils/external'

const router = useRouter()

const REPO_URL = 'https://github.com/GitHubNull/tdl-ui'
const version = __APP_VERSION__

const techStack = [
  { name: 'Go 1.25', desc: '后端服务与下载引擎' },
  { name: 'Wails v2.11', desc: '跨平台桌面框架' },
  { name: 'Vue 3.5 + TypeScript', desc: '前端框架' },
  { name: 'PrimeVue 4.3（Aura）', desc: 'UI 组件库与主题' },
  { name: 'Yaegi v0.16', desc: 'Go 脚本沙箱引擎' },
  { name: 'tdl 上游引擎', desc: 'Telegram 下载与会话体系' },
  { name: 'modernc SQLite', desc: '任务与文件状态存储' },
]
</script>

<style scoped>
/* 居中内容列：宽窗口下不再整体贴左 */
.about-page {
  max-width: 860px;
  margin: 0 auto;
}

.about-page .panel-card + .panel-card {
  margin-top: 16px;
}

/* ---- 英雄区 ---- */
.hero-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  text-align: center;
  padding: 40px 32px 32px;
}

.app-logo {
  width: 80px;
  height: 80px;
  user-select: none;
  -webkit-user-drag: none;
}

.app-name {
  font-size: 1.6rem;
  font-weight: 600;
  margin-top: 14px;
}

.app-version {
  margin-top: 6px;
  font-size: 12px;
  color: var(--p-primary-600);
  background: var(--p-primary-50);
  border: 1px solid var(--p-primary-200);
  border-radius: 999px;
  padding: 2px 12px;
}

.app-dark .app-version {
  color: var(--p-primary-300);
  background: color-mix(in srgb, var(--p-primary-900) 40%, transparent);
  border-color: var(--p-primary-800);
}

.intro {
  margin: 18px 0 0;
  max-width: 620px;
  line-height: 1.75;
  color: var(--p-text-muted-color);
}

.link-row {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 10px;
  margin-top: 24px;
}

/* ---- 分区卡片 ---- */
.section-title {
  font-size: 1rem;
  font-weight: 600;
  margin: 0 0 16px;
  padding-bottom: 10px;
  border-bottom: 1px solid var(--p-surface-200);
}

.app-dark .section-title {
  border-bottom-color: var(--p-surface-700);
}

.tech-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
  gap: 10px;
}

.tech-item {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 12px 14px;
  border: 1px solid var(--p-surface-200);
  border-radius: 8px;
  background: var(--p-surface-50);
}

.app-dark .tech-item {
  border-color: var(--p-surface-700);
  background: var(--p-surface-800);
}

.tech-name {
  font-weight: 500;
  font-size: 13px;
}

.tech-desc {
  font-size: 12px;
  color: var(--p-text-muted-color);
}

/* ---- 作者与许可证 ---- */
.meta-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 8px 24px;
}

.meta-item {
  display: flex;
  align-items: baseline;
  gap: 12px;
  padding: 6px 0;
}

.meta-label {
  flex-shrink: 0;
  width: 64px;
  font-size: 13px;
  color: var(--p-text-muted-color);
}

.meta-value {
  font-size: 13px;
  color: var(--p-text-color);
}

.disclaimer {
  margin: 16px 0 0;
  padding-top: 14px;
  border-top: 1px dashed var(--p-surface-200);
}

.app-dark .disclaimer {
  border-top-color: var(--p-surface-700);
}
</style>
