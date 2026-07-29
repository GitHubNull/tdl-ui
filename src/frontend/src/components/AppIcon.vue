<template>
  <span class="app-icon" v-html="svg" />
</template>

<script setup lang="ts">
import { computed } from 'vue'

// 品牌图标套组（img/icons，currentColor 描边，自动适配明暗主题）
import chats from '../assets/icons/nav-chats.svg?raw'
import download from '../assets/icons/nav-download.svg?raw'
import scripts from '../assets/icons/nav-scripts.svg?raw'
import logs from '../assets/icons/nav-logs.svg?raw'
import settings from '../assets/icons/nav-settings.svg?raw'
import account from '../assets/icons/nav-account.svg?raw'
import themeLight from '../assets/icons/nav-theme-light.svg?raw'
import themeDark from '../assets/icons/nav-theme-dark.svg?raw'
import loginCode from '../assets/icons/login-code.svg?raw'
import loginQrcode from '../assets/icons/login-qrcode.svg?raw'
import loginDesktop from '../assets/icons/login-desktop.svg?raw'

const icons: Record<string, string> = {
  chats,
  download,
  scripts,
  logs,
  settings,
  account,
  'theme-light': themeLight,
  'theme-dark': themeDark,
  'login-code': loginCode,
  'login-qrcode': loginQrcode,
  'login-desktop': loginDesktop,
}

const props = defineProps<{ name: string }>()

const svg = computed(() => icons[props.name] ?? '')
</script>

<style scoped>
.app-icon {
  display: inline-flex;
  width: 22px;
  height: 22px;
  flex-shrink: 0;
}

.app-icon :deep(svg) {
  width: 100%;
  height: 100%;
  display: block;
}
</style>
