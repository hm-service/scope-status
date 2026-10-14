import vue from '@vitejs/plugin-vue'
import { defineConfig } from 'vite'

// 后端默认在 8080（PORT 可改）；开发时前端跑 Vite，把 /api、探针这些转给后端，
// 这样前端改一行就热更新，不用重编译进二进制。
const BACKEND = process.env.BACKEND_URL || 'http://127.0.0.1:8080'

// https://vite.dev/config/
export default defineConfig({
  plugins: [vue()],
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
