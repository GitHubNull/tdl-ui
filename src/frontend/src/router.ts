import { createRouter, createWebHashHistory } from 'vue-router'

// 首屏可能直达登录页，保留静态导入；其余页面懒加载拆 chunk（CODE-004）
import LoginPage from './pages/LoginPage.vue'

// Wails 嵌入环境使用 hash 路由避免刷新丢路径
export const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/', redirect: '/chats' },
    { path: '/chats', component: () => import('./pages/ChatsPage.vue'), meta: { title: '对话', icon: 'chats' } },
    { path: '/chats/:id', component: () => import('./pages/ChatsPage.vue'), meta: { hidden: true } },
    { path: '/login', component: LoginPage, meta: { title: '账号', icon: 'account', hidden: true } },
    { path: '/tasks', component: () => import('./pages/TasksPage.vue'), meta: { title: '下载', icon: 'download' } },
    { path: '/scripts', component: () => import('./pages/ScriptsPage.vue'), meta: { title: '脚本', icon: 'scripts' } },
    { path: '/logs', component: () => import('./pages/LogsPage.vue'), meta: { title: '日志', icon: 'logs' } },
    { path: '/settings', component: () => import('./pages/SettingsPage.vue'), meta: { title: '设置', icon: 'settings' } },
    { path: '/tutorial', component: () => import('./pages/TutorialPage.vue'), meta: { title: '教程', icon: 'tutorial' } },
    { path: '/tutorial/:chapter', component: () => import('./pages/TutorialPage.vue'), meta: { hidden: true } },
    { path: '/about', component: () => import('./pages/AboutPage.vue'), meta: { title: '关于', icon: 'about' } },
  ],
})
