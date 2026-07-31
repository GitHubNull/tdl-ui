import { defineConfig, type Plugin } from 'vite'
import vue from '@vitejs/plugin-vue'

import pkg from './package.json'

// wails dev 下阻止 SPA fallback 吞掉 /media/*，返回 404 让 Wails 回落到后端缩略图 Handler
function media404(): Plugin {
  return {
    name: 'media-404',
    configureServer(server) {
      server.middlewares.use((req, res, next) => {
        if (req.url?.startsWith('/media/')) {
          res.statusCode = 404
          res.end()
          return
        }
        next()
      })
    },
  }
}

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [vue(), media404()],
  define: {
    // 版本号单一事实来源：package.json（与 wails.json 同步维护）
    __APP_VERSION__: JSON.stringify(pkg.version),
  },
  build: {
    // CODE-004：PrimeVue 系依赖拆为 vendor chunk，路由页面懒加载后各 chunk 应低于默认 500kB 阈值
    rollupOptions: {
      output: {
        manualChunks(id) {
          // 主题引擎全局共享，固定为独立 vendor chunk；
          // primevue 组件为按需导入，交由路由懒加载边界自然分割
          if (id.includes('node_modules/@primeuix')) {
            return 'primeuix'
          }
        },
      },
    },
  },
})
