// 会话令牌：认证页拿原始 AUTH_TOKEN 换来的那条，只存在 localStorage 里。
// 原始 token 只在登录时发一次，浏览器不长期留着它；服务端一重启，所有会话失效。
const SESSION_KEY = 'scope-status-session'
const AUTH_PATH = '/auth'

export function getSession(): string {
  try {
    return localStorage.getItem(SESSION_KEY) ?? ''
  } catch {
    return ''
  }
}

/** 写进 localStorage 才算成功：浏览器禁掉存储时返回 false，页面得说一声。 */
export function setSession(token: string): boolean {
  try {
    localStorage.setItem(SESSION_KEY, token)
    return localStorage.getItem(SESSION_KEY) === token
  } catch {
    return false
  }
}

export function clearSession(): void {
  try {
    localStorage.removeItem(SESSION_KEY)
  } catch {
    /* 清不掉就算了，重新登录会覆盖 */
  }
}

/** 每个 /api 请求都带这条会话；本地没有时不带，交给服务端回 401。 */
export function authHeader(): Record<string, string> {
  const token = getSession()
  return token ? { Authorization: `Bearer ${token}` } : {}
}

/** 进站入口的检查：本地没有会话就先去认证页。 */
export function requireSession(): boolean {
  if (getSession()) return true
  redirectToAuth()
  return false
}

export function redirectToAuth(): void {
  if (location.pathname !== AUTH_PATH) location.replace(AUTH_PATH)
}

/** 401 说明本地这条会话已经失效（多半是服务端重启过）：清掉再去认证页重登。 */
export function handleUnauthorized(): void {
  clearSession()
  redirectToAuth()
}
