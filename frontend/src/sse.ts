// SSE：一条 /api/events 长连接，服务端主动推 snapshot / messages / hello。
// 断线由 EventSource 自己重连，这里只把连接状态报给界面。
import type { MessagesFrame, Snapshot } from './types'

export type StreamState = 'live' | 'waiting' | 'connecting' | 'off'

export interface StreamHandlers {
  onSnapshot(snapshot: Snapshot): void
  onMessages(frame: MessagesFrame): void
  onHello(payload: { streamInterval?: number }): void
  onStateChange(state: StreamState): void
}

let source: EventSource | null = null

function parse<T>(event: Event): T {
  return JSON.parse((event as MessageEvent<string>).data) as T
}

export function connectStream(handlers: StreamHandlers): void {
  if (source) return
  handlers.onStateChange('connecting')
  const stream = new EventSource('/api/events')
  source = stream

  stream.addEventListener('snapshot', (event) => handlers.onSnapshot(parse<Snapshot>(event)))
  // 服务端连上但第一轮还没探完时发 hello，告诉前端推送节奏
  stream.addEventListener('hello', (event) => handlers.onHello(parse<{ streamInterval?: number }>(event)))
  // 消息有增删改时服务端单独推一帧，不用等下一轮探测
  stream.addEventListener('messages', (event) => handlers.onMessages(parse<MessagesFrame>(event)))
  stream.addEventListener('open', () => handlers.onStateChange('connecting'))
  stream.addEventListener('error', () => handlers.onStateChange('off'))
}

export function closeStream(): void {
  if (!source) return
  source.close()
  source = null
}

export function streamReadyState(): number {
  return source ? source.readyState : EventSource.CLOSED
}
