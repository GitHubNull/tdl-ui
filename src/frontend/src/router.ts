import { createRouter, createWebHashHistory } from 'vue-router'

import LoginPage from './pages/LoginPage.vue'
import ChatsPage from './pages/ChatsPage.vue'
import TasksPage from './pages/TasksPage.vue'
import ScriptsPage from './pages/ScriptsPage.vue'
import LogsPage from './pages/LogsPage.vue'
import SettingsPage from './pages/SettingsPage.vue'

// Wails 嵌入环境使用 hash 路由避免刷新丢路径
export const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/', redirect: '/chats' },
    { path: '/chats', component: ChatsPage, meta: { title: '对话', icon: 'chats' } },
    { path: '/chats/:id', component: ChatsPage, meta: { hidden: true } },
    { path: '/login', component: LoginPage, meta: { title: '账号', icon: 'account', hidden: true } },
    { path: '/tasks', component: TasksPage, meta: { title: '下载', icon: 'download' } },
    { path: '/scripts', component: ScriptsPage, meta: { title: '脚本', icon: 'scripts' } },
    { path: '/logs', component: LogsPage, meta: { title: '日志', icon: 'logs' } },
    { path: '/settings', component: SettingsPage, meta: { title: '设置', icon: 'settings' } },
  ],
})
