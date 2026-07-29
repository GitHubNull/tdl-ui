<template>
  <div class="app-shell">
    <aside class="app-sidebar">
      <img class="brand" :src="appIcon" alt="tdl UI" draggable="false" />
      <router-link
        v-for="item in navItems"
        :key="item.path"
        :to="item.path"
        custom
        v-slot="{ navigate, isActive }"
      >
        <div
          class="nav-item"
          :class="{ active: isActive }"
          v-tooltip.right="item.title"
          role="button"
          :aria-label="item.title"
          @click="navigate"
        >
          <AppIcon :name="item.icon" />
        </div>
      </router-link>
      <div class="nav-spacer" />
      <div
        class="nav-item"
        :class="{ active: isLoginRoute }"
        v-tooltip.right="accountTip"
        role="button"
        aria-label="账号"
        @click="goLogin"
      >
        <AppIcon name="account" />
      </div>
      <div
        class="nav-item"
        v-tooltip.right="isDark ? '切换到亮色' : '切换到暗色'"
        role="button"
        aria-label="切换主题"
        @click="toggleTheme"
      >
        <AppIcon :name="isDark ? 'theme-light' : 'theme-dark'" />
      </div>
    </aside>

    <main class="app-content">
      <router-view />
    </main>

    <Toast position="bottom-right" />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import Toast from 'primevue/toast'

import AppIcon from './components/AppIcon.vue'
import appIcon from './assets/app-icon.svg'
import { router } from './router'
import { getTheme, setTheme } from './theme'
import { useAuthStore } from './stores/auth'
import { useTasksStore } from './stores/tasks'
import { useScriptsStore } from './stores/scripts'
import { useSettingsStore } from './stores/settings'
import { useLogsStore } from './stores/logs'

const navItems = computed(() =>
  router.options.routes
    .filter((r) => r.meta && !r.meta.hidden)
    .map((r) => ({ path: r.path, title: r.meta!.title as string, icon: r.meta!.icon as string })),
)

const auth = useAuthStore()
const route = useRoute()
const isLoginRoute = computed(() => route.path === '/login')
const accountTip = computed(() =>
  auth.loggedIn ? `已登录：${auth.username || auth.userId}` : '账号',
)

function goLogin() {
  router.push('/login')
}

const dark = ref(document.documentElement.classList.contains('app-dark'))
const isDark = computed(() => dark.value)

function toggleTheme() {
  const next = getTheme() === 'dark' || dark.value ? 'light' : 'dark'
  setTheme(next)
  dark.value = next === 'dark'
}

onMounted(async () => {
  // 全局事件订阅与初始数据加载
  await Promise.all([
    useAuthStore().init(),
    useTasksStore().init(),
    useScriptsStore().init(),
    useSettingsStore().init(),
    useLogsStore().init(),
  ])
  dark.value = document.documentElement.classList.contains('app-dark')
})
</script>

<style scoped>
</style>
