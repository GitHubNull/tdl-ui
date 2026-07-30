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
    chunkSizeWarningLimit: 1024,
  },
})
