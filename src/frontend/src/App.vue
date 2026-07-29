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
      <div class="nav-item theme-switch" v-tooltip.right="themeTip" aria-label="切换主题">
        <ToggleSwitch :model-value="isDark" @update:model-value="toggleTheme">
          <template #handle>
            <AppIcon class="handle-icon" :name="isDark ? 'theme-dark' : 'theme-light'" />
          </template>
        </ToggleSwitch>
      </div>
    </aside>

    <main class="app-content">
      <router-view />
    </main>

    <Toast position="bottom-right" />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import Toast from 'primevue/toast'
import ToggleSwitch from 'primevue/toggleswitch'

import AppIcon from './components/AppIcon.vue'
import appIcon from './assets/app-icon.svg'
import { router } from './router'
import { isDark, themeMode } from './theme'
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

const settings = useSettingsStore()
const themeTip = computed(() => {
  const target = isDark.value ? '切换到亮色' : '切换到暗色'
  return themeMode.value === 'system' ? `${target}（手动切换将脱离跟随系统）` : target
})

function toggleTheme() {
  // 快捷开关只在亮/暗间切换并写入设置，跟随系统请在设置页选择
  settings.applyTheme(isDark.value ? 'light' : 'dark')
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
})
</script>

<style scoped>
.theme-switch {
  cursor: default;
}
.theme-switch :deep(.p-toggleswitch) {
  transform: scale(0.85);
}
.theme-switch :deep(.handle-icon) {
  width: 12px;
  height: 12px;
}
</style>
