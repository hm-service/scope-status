import { createApp } from 'vue'
import './style.css'
import App from './App.vue'
import * as api from './api'
import { initApp, state } from './store'

declare global {
  interface Window {
    __app?: {
      state: typeof state
      api: typeof api
    }
  }
}

initApp()
createApp(App).mount('#app')

// 调试入口：window.__app = { state, api }
window.__app = { state, api }
