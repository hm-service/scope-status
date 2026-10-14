// 首次绘制前把主题定好，避免闪一下；?theme=light|dark 可以强制指定。
// 单独一个同源文件而不是内联：这样 CSP 的 script-src 才能收成 'self'。
const forced = new URLSearchParams(location.search).get('theme')
document.documentElement.dataset.theme = forced || localStorage.getItem('scope-status-theme') || 'light'
