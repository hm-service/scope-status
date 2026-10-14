import { createApp } from 'vue'
import './style.css'
import AuthPage from './AuthPage.vue'
import { lang, setLang } from './i18n'

// 认证页只做一件事：拿 token 换进站资格，界面文案与主题跟主站保持一致
setLang(lang.value)
createApp(AuthPage).mount('#auth')
