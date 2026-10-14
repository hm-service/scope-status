// SSE：一条 /api/events 长连接，服务端主动推 snapshot / messages / hello。
// 用 fetch + ReadableStream 而不是 EventSource：EventSource 不能带 Authorization 头，
// token 只能塞进 URL 里，那会漏进日志与历史记录。
import { authHeader, handleUnauthorized } from './auth'
import type { MessagesFrame, Snapshot } from './types'

export type StreamState = 'live' | 'waiting' | 'connecting' | 'off'

export interface StreamHandlers {
  onSnapshot(snapshot: Snapshot): void
  onMessages(frame: MessagesFrame): void
  onHello(payload: { streamInterval?: number }): void
  onStateChange(state: StreamState): void
}

/** 断线后多久重连一次；服务端每 15 秒发一个注释帧保活，够区分「静默」与「已死」。 */
const RECONNECT_MS = 3000

let controller: AbortController | null = null
let running = false

export function connectStream(handlers: StreamHandlers): void {
  if (running) return
  running = true
  void readLoop(handlers)
}

export function closeStream(): void {
  running = false
  controller?.abort()
  controller = null
}

/** 连接还在（没被主动关掉）时为真；静默检测拿它当开关。 */
export function streamConnected(): boolean {
  return running && controller !== null
}

async function readLoop(handlers: StreamHandlers): Promise<void> {
  while (running) {
    handlers.onStateChange('connecting')
    const abort = new AbortController()
    controller = abort
    try {
      const response = await fetch('/api/events', {
        headers: { ...authHeader(), Accept: 'text/event-stream' },
        cache: 'no-store',
        signal: abort.signal,
      })
      if (response.status === 401) {
        running = false
        controller = null
        handleUnauthorized()
        return
      }
      if (!response.ok || !response.body) throw new Error(`HTTP ${response.status}`)
      await readFrames(response.body, handlers)
    } catch {
      // 断线、被 abort、网络抖动都走同一条重连路径
    }
    controller = null
    if (!running) return
    handlers.onStateChange('off')
    await sleep(RECONNECT_MS)
  }
}

/** 服务端只写 "\n"，所以空行分帧、行内 `field: value` 直接切。 */
async function readFrames(body: ReadableStream<Uint8Array>, handlers: StreamHandlers): Promise<void> {
  const reader = body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  for (;;) {
    const { value, done } = await reader.read()
    if (done) return
    buffer += decoder.decode(value, { stream: true })
    let end = buffer.indexOf('\n\n')
    while (end >= 0) {
      dispatch(buffer.slice(0, end), handlers)
      buffer = buffer.slice(end + 2)
      end = buffer.indexOf('\n\n')
    }
  }
}

function dispatch(block: string, handlers: StreamHandlers): void {
  let name = 'message'
  const data: string[] = []
  for (const line of block.split('\n')) {
    if (line.startsWith(':')) continue // 注释帧（保活）不往上抛
    const colon = line.indexOf(':')
    const field = colon < 0 ? line : line.slice(0, colon)
    let value = colon < 0 ? '' : line.slice(colon + 1)
    if (value.startsWith(' ')) value = value.slice(1)
    if (field === 'event') name = value
    else if (field === 'data') data.push(value)
  }
  if (!data.length) return
  let payload: unknown
  try {
    payload = JSON.parse(data.join('\n'))
  } catch {
    return
  }
  switch (name) {
    case 'snapshot':
      handlers.onSnapshot(payload as Snapshot)
      break
    case 'messages':
      handlers.onMessages(payload as MessagesFrame)
      break
    case 'hello':
      handlers.onHello(payload as { streamInterval?: number })
      break
  }
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => window.setTimeout(resolve, ms))
}
