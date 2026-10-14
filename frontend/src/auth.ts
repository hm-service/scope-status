// Bearer token：进站先检查，没有就跳 /auth。
// 只存在 localStorage 里，服务端不认 cookie、也不认别的认证方式。
const TOKEN_KEY = 'scope-status-token'
const AUTH_PATH = '/auth'

export function getToken(): string {
  try {
    return localStorage.getItem(TOKEN_KEY) ?? ''
  } catch {
    return ''
  }
}

export function setToken(token: string): void {
  try {
    localStorage.setItem(TOKEN_KEY, token)
  } catch {
    /* 存不下也照常进站，刷新后重新填一次 */
  }
}

export function clearToken(): void {
  try {
    localStorage.removeItem(TOKEN_KEY)
  } catch {
    /* 清不掉就算了，重填会覆盖 */
  }
}

/** 每个 /api 请求都带这个头；本地没 token 时不带，交给服务端回 401。 */
export function authHeader(): Record<string, string> {
  const token = getToken()
  return token ? { Authorization: `Bearer ${token}` } : {}
}

/** 进站入口的检查：本地没有 token 就先去认证页。 */
export function requireToken(): boolean {
  if (getToken()) return true
  redirectToAuth()
  return false
}

export function redirectToAuth(): void {
  if (location.pathname !== AUTH_PATH) location.replace(AUTH_PATH)
}

/** 401 说明本地这份 token 已经无效：清掉再去认证页重填。 */
export function handleUnauthorized(): void {
  clearToken()
  redirectToAuth()
}
