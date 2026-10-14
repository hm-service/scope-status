// 后端接口的数据形状，字段名以 DESIGN.md 为准。

export type PropertyValue = string | number | boolean | null
export type PropertyMap = Record<string, PropertyValue>

export interface Summary {
  total: number
  online: number
  offline: number
}

export interface AddressStatus {
  address: string
  status: string
  latencyMs: number | null
  detail: string | null
  method?: string
}

export interface Peer {
  name: string
  note?: string | null
  /** 条目类型：friend | server | cloud（cloud 来自配置里的 cloud_servers 块） */
  kind: string
  online: boolean
  latencyMs: number | null
  addresses: AddressStatus[]
  properties?: PropertyMap
  /** 只有 kind = cloud 才有：core = WG 主干，private / public 分开给 */
  core?: boolean
  private?: string[]
  public?: string[]
  /** WG 链路对端（服务端按 core 推好，两边都给了名字） */
  wgPeers?: string[]
}

export interface Service {
  name: string
  host?: string
  port?: number
  address: string
  protocol: string
  connectCmd?: string | null
  category?: string
  properties?: PropertyMap
  online: boolean
  latencyMs: number | null
  detail?: string | null
  method?: string
  status?: string
}

export interface Link {
  name: string
  url: string
  probeUrl?: string
  expectStatus?: number
  properties?: PropertyMap
  online: boolean
  latencyMs: number | null
  detail?: string | null
  status?: string
  faviconUrl?: string | null
}

export interface PeerGroup {
  id: string
  label?: string
  order?: number
  total: number
  online: number
  peers: Peer[]
}

export interface ServiceGroup {
  id: string
  label?: string
  order?: number
  total: number
  online: number
  services?: Service[]
}

export interface CategoryInfo {
  id: string
  label?: string
  order?: number
}

export interface Snapshot {
  title?: string
  method?: string
  generatedAt?: string
  durationMs?: number | null
  reloadable?: boolean
  reloadCount?: number
  streamInterval?: number
  reused?: boolean
  summary?: Summary
  groups?: PeerGroup[]
  peers?: Peer[]
  categories?: CategoryInfo[]
  services?: Service[]
  servicesGroups?: ServiceGroup[]
  servicesSummary?: Summary
  links?: Link[]
  linksSummary?: Summary
}

export interface Attachment {
  sha256: string
  name: string
  size: number
  url?: string
}

export interface Message {
  id: string
  kind: string
  createdAt: number
  locked?: boolean
  text?: string | null
  updatedAt?: number | null
  attachments?: Attachment[]
}

export interface MessagesFrame {
  messages: Message[]
  summary?: {
    total: number
    locked: number
  }
}

export interface MessageInput {
  kind?: string
  text?: string
  key?: string | null
  attachments?: { sha256: string; name: string }[]
}

export interface DeleteResult {
  removedFiles?: string[]
}

export interface ReloadResult {
  message?: string
  reloadCount?: number
  snapshot?: Snapshot
}
