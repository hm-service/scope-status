import { createApp } from 'vue'
import './style.css'
import App from './App.vue'
import * as api from './api'
import { requireToken } from './auth'
import { initApp, state } from './store'

declare global {
  interface Window {
    __app?: {
      state: typeof state
      api: typeof api
    }
  }
}

// 进站先检查 token：本地没有就去 /auth，界面本身不带任何数据
if (requireToken()) {
  initApp()
  createApp(App).mount('#app')

  // 调试入口：window.__app = { state, api }
  window.__app = { state, api }
}
