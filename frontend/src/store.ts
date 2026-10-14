// 应用状态与动作：一个响应式的 store，组件只负责渲染。
// 旧实现（static/app.js）里的行为在这里逐条对应，函数名尽量保持一致。
import { computed, reactive, watchEffect } from 'vue'
import * as api from './api'
import { ApiError, errorMessage } from './api'
import { lang, setLang, t, type I18nKey, type Lang } from './i18n'
import { closeStream, connectStream, streamReadyState, type StreamState } from './sse'
import type { Attachment, Peer, PeerGroup, Link, Message, MessageInput, MessagesFrame, Service, Snapshot, Summary } from './types'
import { formatDuration } from './utils/format'
import { formatTime, shortStamp, tickNow } from './utils/time'

const STREAM_SILENCE_MS = 30000
const TOPOLOGY_STORAGE_KEY = 'scope-status-topology'

export type TabId = 'network' | 'services' | 'links' | 'messages'
export type Theme = 'light' | 'dark'
type NoticeTone = 'ok' | 'bad'

export interface TabDef {
  id: TabId
  labelKey: I18nKey
}

export const TABS: TabDef[] = [
  { id: 'network', labelKey: 'tabs.network' },
  { id: 'services', labelKey: 'tabs.services' },
  { id: 'links', labelKey: 'tabs.links' },
  { id: 'messages', labelKey: 'tabs.messages' },
]

const ZERO_SUMMARY: Summary = { total: 0, online: 0, offline: 0 }

interface ComposeState {
  open: boolean
  mode: 'create' | 'edit'
  target: Message | null
  key: string | null
  kind: string
  text: string
  keyInput: string
  keyVisible: boolean
  draftFiles: Attachment[]
  uploading: number
  error: string
}

export const state = reactive({
  lang,
  theme: 'light' as Theme,
  tab: 'network' as TabId,
  menuOpen: false,
  topologyOpen: storedTopologyOpen(),
  snapshot: null as Snapshot | null,
  messages: [] as Message[],
  unlockedKeys: new Map<string, string>(),
  unlockedPayloads: new Map<string, Message>(),
  collapsed: new Set<string>(),
  expanded: new Set<string>(),
  stream: 'connecting' as StreamState,
  lastFrameAt: 0,
  streamInterval: 0,
  error: null as string | null,
  notice: null as { message: string; tone: NoticeTone } | null,
  refreshing: false,
  reloading: false,
  submitting: false,
  compose: {
    open: false,
    mode: 'create',
    target: null,
    key: null,
    kind: 'public',
    text: '',
    keyInput: '',
    keyVisible: false,
    draftFiles: [],
    uploading: 0,
    error: '',
  } as ComposeState,
})

// ---------------------------------------------------------------- 派生值

export const title = computed(() => state.snapshot?.title || t('app.title'))

export const activeSummary = computed<Summary>(() => {
  const payload = state.snapshot
  if (!payload) return ZERO_SUMMARY
  if (state.tab === 'services') return payload.servicesSummary || ZERO_SUMMARY
  if (state.tab === 'links') return payload.linksSummary || ZERO_SUMMARY
  return payload.summary || ZERO_SUMMARY
})

export const durationText = computed(() => formatDuration(state.snapshot?.durationMs))

export const messageSummary = computed(() => {
  const pending = state.messages.filter((message) => message.locked && !state.unlockedPayloads.has(message.id)).length
  return {
    total: state.messages.length,
    locked: pending,
    // 列表是按时间倒序的，第一条就是最近一条
    latest: state.messages.length ? shortStamp(state.messages[0].createdAt) : '–',
  }
})

export const subtitle = computed(() => {
  if (state.tab === 'messages') {
    const count = state.messages.length
    return count ? t('topbar.subtitle.messages', { count }) : t('topbar.subtitle.messagesEmpty')
  }
  const payload = state.snapshot
  if (!payload) return t('topbar.connecting')
  if (state.tab === 'services') {
    const count = (payload.services || []).length
    const groups = (payload.servicesGroups || []).length
    return count ? t('topbar.subtitle.services', { count, groups }) : t('topbar.subtitle.servicesEmpty')
  }
  if (state.tab === 'links') {
    const count = (payload.links || []).length
    return count ? t('topbar.subtitle.links', { count }) : t('topbar.subtitle.linksEmpty')
  }
  const groups = resolveGroups(payload)
  const shown = payload.summary?.total ?? groups.reduce((sum, group) => sum + group.peers.length, 0)
  return groups.length
    ? t('topbar.subtitle.network', { count: shown, groups: groups.length, method: (payload.method || 'icmp').toUpperCase() })
    : t('topbar.subtitle.networkEmpty')
})

const STREAM_TEXT: Record<StreamState, I18nKey> = {
  live: 'stream.live',
  waiting: 'stream.waiting',
  connecting: 'stream.connecting',
  off: 'stream.off',
}

const STREAM_CLASS: Record<StreamState, string> = {
  live: 'stream-live',
  waiting: 'stream-wait',
  connecting: 'stream-wait',
  off: 'stream-off',
}

export const streamText = computed(() => t(STREAM_TEXT[state.stream]))
export const streamClass = computed(() => `stream ${STREAM_CLASS[state.stream]}`)
export const streamTitle = computed(() => (state.streamInterval > 0 ? t('stream.interval', { seconds: state.streamInterval }) : ''))

export const updatedText = computed(() =>
  state.lastFrameAt ? t('topbar.updated', { time: formatTime(new Date(state.lastFrameAt)) }) : t('topbar.notUpdated'),
)

export const services = computed<Service[]>(() => state.snapshot?.services || [])
export const servicesGroups = computed(() => state.snapshot?.servicesGroups || [])
export const links = computed<Link[]>(() => state.snapshot?.links || [])

function onlineCount<T extends { online: boolean }>(entries: T[]): number {
  return entries.filter((entry) => entry.online).length
}

/** tab 角落的小计数：0/0 时不显示。 */
export const servicesTabCount = computed(() => (services.value.length ? `${onlineCount(services.value)}/${services.value.length}` : ''))
export const linksTabCount = computed(() => (links.value.length ? `${onlineCount(links.value)}/${links.value.length}` : ''))
export const linksGroupCount = computed(() =>
  links.value.length ? t('group.online', { online: onlineCount(links.value), total: links.value.length }) : '',
)

/** 优先用后端分好的 groups；没有就按 kind 在前端兜底分组。 */
export function resolveGroups(payload: Snapshot | null): PeerGroup[] {
  if (!payload) return []
  if (Array.isArray(payload.groups)) {
    return payload.groups.filter((group) => group.peers && group.peers.length)
  }
  const order = new Map((payload.categories || []).map((category, index) => [category.id, category.order ?? index]))
  const buckets = new Map<string, Peer[]>()
  for (const peer of payload.peers || []) {
    const id = peer.kind || 'peer'
    const bucket = buckets.get(id)
    if (bucket) bucket.push(peer)
    else buckets.set(id, [peer])
  }
  return [...buckets.entries()]
    .map(([id, peers]) => ({
      id,
      label: id,
      order: order.get(id) ?? 100,
      total: peers.length,
      online: peers.filter((peer) => peer.online).length,
      peers,
    }))
    .sort((a, b) => a.order - b.order || a.id.localeCompare(b.id))
}

export const groups = computed(() => resolveGroups(state.snapshot))

export const peerRowsShown = computed(() => groups.value.reduce((sum, group) => sum + group.peers.length, 0))

// ---------------------------------------------------------------- 状态提示

let noticeTimer: number | null = null

/** 页脚没有错误也没有提示时不占地方。 */
export const footerVisible = computed(() => Boolean(state.error) || Boolean(state.notice))

export function setError(message: string | null): void {
  state.error = message ? t('error.prefix', { message }) : null
}

export function showNotice(message: string, tone: NoticeTone = 'ok'): void {
  state.notice = { message, tone }
  if (noticeTimer !== null) clearTimeout(noticeTimer)
  noticeTimer = window.setTimeout(() => {
    state.notice = null
    noticeTimer = null
  }, 6000)
}

// ---------------------------------------------------------------- 主题 / 语言 / tab

export function applyTheme(theme: Theme): void {
  state.theme = theme
  document.documentElement.dataset.theme = theme
  try {
    localStorage.setItem('scope-status-theme', theme)
  } catch {
    /* 存不下也照常切 */
  }
}

export function toggleTheme(): void {
  applyTheme(state.theme === 'light' ? 'dark' : 'light')
}

export function toggleLang(): void {
  const next: Lang = state.lang === 'zh' ? 'en' : 'zh'
  setLang(next, true)
}

export function switchTab(tabId: string): void {
  state.tab = TABS.some((tab) => tab.id === tabId) ? (tabId as TabId) : 'network'
  if (location.hash.slice(1) !== state.tab) {
    history.replaceState(null, '', state.tab === 'network' ? location.pathname : `#${state.tab}`)
  }
}

/** 拓扑图默认展开；用户折过一次以后记住。 */
function storedTopologyOpen(): boolean {
  try {
    const saved = localStorage.getItem(TOPOLOGY_STORAGE_KEY)
    return saved === null ? true : saved === 'open'
  } catch {
    return true
  }
}

export function toggleTopology(): void {
  state.topologyOpen = !state.topologyOpen
  try {
    localStorage.setItem(TOPOLOGY_STORAGE_KEY, state.topologyOpen ? 'open' : 'closed')
  } catch {
    /* 存不下也照常切 */
  }
}

export function toggleMenu(): void {
  state.menuOpen = !state.menuOpen
}

export function closeMenu(): void {
  state.menuOpen = false
}

// ---------------------------------------------------------------- SSE

function applyPayload(payload: Snapshot): void {
  state.snapshot = payload
  state.stream = 'live'
}

export function connect(): void {
  connectStream({
    onSnapshot(payload) {
      state.lastFrameAt = Date.now()
      applyPayload(payload)
    },
    onHello(payload) {
      state.lastFrameAt = Date.now()
      state.stream = 'waiting'
      const interval = payload.streamInterval ?? 0
      // 推送节奏写在状态文字里，别去改副标题——那个位置是当前 tab 的概况
      if (interval > 0) state.streamInterval = interval
    },
    onMessages(frame) {
      applyMessages(frame)
    },
    onStateChange(next) {
      state.stream = next
    },
  })
}

/** SSE 长时间没有推送就自己重连一次。 */
export function checkStreamHealth(): void {
  if (streamReadyState() === EventSource.OPEN && state.lastFrameAt && Date.now() - state.lastFrameAt > STREAM_SILENCE_MS) {
    setError(t('stream.silent'))
    closeStream()
    connect()
  }
}

// ---------------------------------------------------------------- 消息

/** 服务端推来的消息列表（连上 SSE 先给一帧，之后每次有增删改再给）。 */
export function applyMessages(frame: MessagesFrame): void {
  state.messages = frame && Array.isArray(frame.messages) ? frame.messages : []

  const live = new Set(state.messages.map((message) => message.id))
  for (const id of [...state.unlockedKeys.keys()]) {
    if (!live.has(id)) {
      state.unlockedKeys.delete(id)
      state.unlockedPayloads.delete(id)
    }
  }
  // 密钥消息在服务端永远是「已锁定」，本地已解锁的先照旧渲染，顺手用记着的密钥刷新一遍
  for (const message of state.messages) {
    if (message.locked && state.unlockedKeys.has(message.id)) void refreshUnlocked(message.id)
  }
}

export async function refreshUnlocked(id: string): Promise<void> {
  const key = state.unlockedKeys.get(id)
  if (key === undefined) return
  try {
    const message = await api.unlockMessage(id, key)
    state.unlockedPayloads.set(id, message)
  } catch (error) {
    // 只有「密钥不对 / 消息没了」才丢掉本地这份；网络抖动就留着，别把用户正在看的内容弄没
    if (error instanceof ApiError && (error.status === 403 || error.status === 404)) {
      state.unlockedKeys.delete(id)
      state.unlockedPayloads.delete(id)
    }
  }
}

/** 解锁：成功返回 null，失败返回给用户看的文案。 */
export async function unlockMessage(id: string, key: string): Promise<string | null> {
  try {
    const message = await api.unlockMessage(id, key)
    state.unlockedKeys.set(id, key)
    state.unlockedPayloads.set(id, message)
    return null
  } catch (error) {
    if (error instanceof ApiError) {
      return error.body || t('messages.locked.failedHttp', { status: error.status })
    }
    return t('messages.locked.failed', { message: errorMessage(error) })
  }
}

export async function deleteMessage(id: string, key: string | null): Promise<void> {
  if (!window.confirm(t('messages.delete.confirm'))) return
  try {
    const result = await api.deleteMessage(id, key)
    state.unlockedKeys.delete(id)
    state.unlockedPayloads.delete(id)
    state.messages = state.messages.filter((message) => message.id !== id)
    showNotice(result.removedFiles?.length ? t('notice.deleteOkFiles', { count: result.removedFiles.length }) : t('notice.deleteOk'), 'ok')
  } catch (error) {
    showNotice(t('notice.deleteFailed', { message: errorMessage(error) }), 'bad')
  }
}

// ---------------------------------------------------------------- 发消息表单

export function openCompose(options: { mode?: 'create' | 'edit'; message?: Message; key?: string | null } = {}): void {
  const compose = state.compose
  const isEdit = options.mode === 'edit' && Boolean(options.message)
  compose.mode = isEdit ? 'edit' : 'create'
  compose.target = isEdit ? options.message! : null
  compose.key = options.key ?? null
  compose.draftFiles = isEdit ? [...(options.message!.attachments || [])] : []
  compose.kind = isEdit ? options.message!.kind : 'public'
  compose.text = isEdit ? options.message!.text || '' : ''
  compose.keyInput = isEdit ? compose.key || '' : ''
  compose.keyVisible = false
  compose.uploading = 0
  compose.error = ''
  compose.open = true
}

export function closeCompose(): void {
  const compose = state.compose
  compose.open = false
  compose.mode = 'create'
  compose.target = null
  compose.key = null
  compose.draftFiles = []
  compose.uploading = 0
  compose.text = ''
  compose.keyInput = ''
  compose.keyVisible = false
  compose.error = ''
  compose.kind = 'public'
}

export function removeDraftFile(index: number): void {
  state.compose.draftFiles.splice(index, 1)
}

export async function uploadFiles(files: File[]): Promise<void> {
  const compose = state.compose
  for (const file of files) {
    compose.uploading += 1
    try {
      const saved = await api.uploadFile(file)
      if (!compose.draftFiles.some((item) => item.sha256 === saved.sha256)) compose.draftFiles.push(saved)
      compose.error = ''
    } catch (error) {
      compose.error = t('messages.compose.uploadFailed', { name: file.name, message: errorMessage(error) })
    } finally {
      compose.uploading -= 1
    }
  }
}

export async function submitCompose(): Promise<void> {
  const compose = state.compose
  if (compose.uploading > 0) {
    compose.error = t('messages.compose.uploadingWait')
    return
  }
  const isEdit = compose.mode === 'edit' && Boolean(compose.target)
  const kind = isEdit ? compose.target!.kind || 'public' : compose.kind
  const body: MessageInput = {
    text: compose.text,
    attachments: compose.draftFiles.map(({ sha256, name }) => ({ sha256, name })),
  }
  let key: string | null = null
  if (isEdit) {
    key = compose.key
    if (kind === 'key') body.key = key
  } else {
    body.kind = kind
    if (kind === 'key') {
      key = compose.keyInput
      body.key = key
    }
  }

  state.submitting = true
  try {
    const saved = isEdit ? await api.updateMessage(compose.target!.id, body) : await api.createMessage(body)
    if (saved.kind === 'key' && key) {
      // 自己刚发的密钥消息，本地直接就能看，不用再解锁一次
      state.unlockedKeys.set(saved.id, key)
      state.unlockedPayloads.set(saved.id, saved)
    }
    closeCompose()
    showNotice(isEdit ? t('messages.compose.saved') : t('messages.compose.sent'), 'ok')
  } catch (error) {
    compose.error = t(isEdit ? 'messages.compose.saveFailed' : 'messages.compose.sendFailed', { message: errorMessage(error) })
  } finally {
    state.submitting = false
  }
}

// ---------------------------------------------------------------- 手动刷新 / 重载

/** 刷新按钮：强制探一次（GET /api/status?force=1）。 */
export async function refreshStatus(): Promise<void> {
  if (state.refreshing) return
  state.refreshing = true
  try {
    const payload = await api.fetchStatus(true)
    if (!payload) {
      showNotice(t('notice.noSnapshot'), 'bad')
      return
    }
    state.lastFrameAt = Date.now()
    applyPayload(payload)
    showNotice(t('notice.refreshed'), 'ok')
  } catch (error) {
    showNotice(t('notice.refreshFailed', { message: errorMessage(error) }), 'bad')
  } finally {
    state.refreshing = false
  }
}

/** 重载配置：重读 source.yaml；配置写坏时服务端回 500 纯文本，原文照显。 */
export async function reloadConfig(): Promise<void> {
  state.reloading = true
  try {
    const payload = await api.reload()
    state.lastFrameAt = Date.now()
    if (payload.snapshot) applyPayload(payload.snapshot)
    showNotice(t('notice.reloadOk', { message: payload.message ?? '', count: payload.reloadCount ?? 0 }), 'ok')
  } catch (error) {
    const body = error instanceof ApiError ? error.body || error.message : errorMessage(error)
    console.error('重载失败', body) // 完整报错（含行号）留在控制台
    showNotice(t('notice.reloadFailed', { message: body }), 'bad')
  } finally {
    state.reloading = false
  }
}

// ---------------------------------------------------------------- 启动

let inited = false

export function initApp(): void {
  if (inited) return
  inited = true

  setLang(state.lang)
  applyTheme(document.documentElement.dataset.theme === 'light' ? 'light' : 'dark')
  switchTab(location.hash.slice(1) || 'network')
  connect()
  watchEffect(() => {
    document.title = title.value
  })

  // 「刚刚 / N 分钟前」自己会变老，本地重算一次就行，不用问服务端
  window.setInterval(tickNow, 30000)

  document.addEventListener('click', (event) => {
    if (!state.menuOpen) return
    const target = event.target as HTMLElement | null
    if (!target || !target.closest('#actions')) closeMenu()
  })
  document.addEventListener('keydown', (event) => {
    if (event.key === 'Escape') closeMenu()
  })
  window.addEventListener('keydown', (event) => {
    if (event.metaKey || event.ctrlKey || event.altKey) return
    if (event.key === 'R') void reloadConfig()
  })
  window.addEventListener('beforeunload', () => {
    closeStream()
  })
}
