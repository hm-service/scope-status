import { fileURLToPath, URL } from 'node:url'
import vue from '@vitejs/plugin-vue'
import { defineConfig, type Plugin } from 'vite'

// 后端默认在 8080（PORT 可改）；开发时前端跑 Vite，把 /api、探针这些转给后端，
// 这样前端改一行就热更新，不用重编译进二进制。
const BACKEND = process.env.BACKEND_URL || 'http://127.0.0.1:8080'

// 产物里认证页是 auth.html，线上由后端把 /auth 映射过去；
// 开发时 Vite 的 SPA 回退会把 /auth 喂成 index.html，所以这里先改写成 auth.html。
function authPage(): Plugin {
  return {
    name: 'scope-status-auth-page',
    configureServer(server) {
      server.middlewares.use((request, _response, next) => {
        if (request.url === '/auth' || request.url?.startsWith('/auth?')) {
          request.url = '/auth.html' + (request.url.includes('?') ? '?' + request.url.split('?')[1] : '')
        }
        next()
      })
    },
  }
}

// https://vite.dev/config/
export default defineConfig({
  plugins: [vue(), authPage()],
  build: {
    rollupOptions: {
      // 两个入口：主站 index.html + 认证页 auth.html
      input: {
        index: fileURLToPath(new URL('./index.html', import.meta.url)),
        auth: fileURLToPath(new URL('./auth.html', import.meta.url)),
      },
    },
  },
  server: {
    port: 5173,
    strictPort: true,
    proxy: {
      '/api': { target: BACKEND, changeOrigin: true },
      '/livez': { target: BACKEND, changeOrigin: true },
      '/readyz': { target: BACKEND, changeOrigin: true },
    },
  },
})
