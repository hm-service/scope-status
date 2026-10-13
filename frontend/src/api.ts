// fetch 封装：每次请求都带 Accept-Language；错误体统一是 {"detail": "…"}。
import { lang } from './i18n'
import type { Attachment, DeleteResult, Message, MessageInput, MessagesFrame, ReloadResult, Snapshot } from './types'

export class ApiError extends Error {
  readonly status: number
  readonly body: string

  constructor(status: number, body: string) {
    super(body || `HTTP ${status}`)
    this.name = 'ApiError'
    this.status = status
    this.body = body
  }
}

function baseHeaders(): Record<string, string> {
  return { 'Accept-Language': lang.value }
}

function jsonHeaders(): Record<string, string> {
  return { ...baseHeaders(), 'Content-Type': 'application/json' }
}

/** 非 2xx：读统一错误体里的 detail；正文不是 JSON（比如反代自己吐的 HTML）就退回 HTTP 状态码。 */
async function errorFrom(response: Response): Promise<ApiError> {
  let detail = ''
  try {
    const body = (await response.text()).trim()
    const parsed: unknown = body === '' ? undefined : JSON.parse(body)
    if (typeof parsed === 'object' && parsed !== null && typeof (parsed as { detail?: unknown }).detail === 'string') {
      detail = (parsed as { detail: string }).detail
    }
  } catch {
    detail = ''
  }
  return new ApiError(response.status, detail)
}

/** 当前共享快照；还没有快照时服务端回 204，这里给 null。 */
export async function fetchStatus(force = false): Promise<Snapshot | null> {
  const response = await fetch(force ? '/api/status?force=1' : '/api/status', {
    headers: baseHeaders(),
    cache: 'no-store',
  })
  if (response.status === 204) return null
  if (!response.ok) throw await errorFrom(response)
  return (await response.json()) as Snapshot
}

/** 重载配置：成功时服务端顺带探一次并给出新快照。 */
export async function reload(): Promise<ReloadResult> {
  const response = await fetch('/api/reload', { method: 'POST', headers: baseHeaders(), cache: 'no-store' })
  if (!response.ok) throw await errorFrom(response)
  return (await response.json()) as ReloadResult
}

export async function listMessages(): Promise<MessagesFrame> {
  const response = await fetch('/api/messages', { headers: baseHeaders(), cache: 'no-store' })
  if (!response.ok) throw await errorFrom(response)
  return (await response.json()) as MessagesFrame
}

export async function createMessage(body: MessageInput): Promise<Message> {
  const response = await fetch('/api/messages', {
    method: 'POST',
    headers: jsonHeaders(),
    body: JSON.stringify(body),
    cache: 'no-store',
  })
  if (!response.ok) throw await errorFrom(response)
  return (await response.json()) as Message
}

export async function updateMessage(id: string, body: MessageInput): Promise<Message> {
  const response = await fetch(`/api/messages/${id}`, {
    method: 'PATCH',
    headers: jsonHeaders(),
    body: JSON.stringify(body),
    cache: 'no-store',
  })
  if (!response.ok) throw await errorFrom(response)
  return (await response.json()) as Message
}

/** 用密钥换完整内容（页面上的「输入密钥」和自动刷新都走这里）。 */
export async function unlockMessage(id: string, key: string): Promise<Message> {
  const response = await fetch(`/api/messages/${id}/unlock`, {
    method: 'POST',
    headers: jsonHeaders(),
    body: JSON.stringify({ key }),
    cache: 'no-store',
  })
  if (!response.ok) throw await errorFrom(response)
  return (await response.json()) as Message
}

export async function deleteMessage(id: string, key: string | null): Promise<DeleteResult> {
  const response = await fetch(`/api/messages/${id}/delete`, {
    method: 'POST',
    headers: jsonHeaders(),
    body: JSON.stringify(key ? { key } : {}),
    cache: 'no-store',
  })
  if (!response.ok) throw await errorFrom(response)
  return (await response.json()) as DeleteResult
}

/** 附件上传：裸 body 直接当文件流发，不拼 multipart；文件名走 ?name=。 */
export async function uploadFile(file: File): Promise<Attachment> {
  const response = await fetch(`/api/messages/files?name=${encodeURIComponent(file.name)}`, {
    method: 'PUT',
    headers: { ...baseHeaders(), 'Content-Type': 'application/octet-stream' },
    body: file,
    cache: 'no-store',
  })
  if (!response.ok) throw await errorFrom(response)
  return (await response.json()) as Attachment
}

export function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}
