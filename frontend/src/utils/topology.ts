// 内网拓扑：从 peers + addresses 推出「中继基建 + 接入节点」两层结构，直接吐 SVG。
// 纯函数，颜色全走 CSS 变量（跟着主题变），文案走传进来的 labels（跟着语言变）。

export interface TopoAddress {
  address: string
  status: string
}

export interface TopoPeer {
  name: string
  kind?: string
  online: boolean
  addresses: TopoAddress[]
  /** kind = cloud 才有：core = WG 主干，wgPeers 是服务端按 core 推好的对端 */
  core?: boolean
  wgPeers?: string[]
}

export interface TopoLabels {
  hub: string
  hubSubtitle: (relays: number) => string
  attached: (nodes: number) => string
  relay: string
  core: string
  corePeers: (names: string) => string
  wg: string
}

interface Membership {
  prefix: string
  address: string
}

interface TopoNode {
  peer: TopoPeer
  kind: string
  endpoints: string[]
  memberships: Membership[]
}

interface Subnet {
  prefix: string
  label: string
  members: { node: TopoNode; address: string }[]
  hub: TopoNode
  hubAddress: string
}

// ------------------------------------------------------------------ 分析

const IPV4 = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/

function isIPv4(value: string): boolean {
  const match = IPV4.exec(value)
  if (!match) return false
  return match.slice(1).every((part) => Number(part) <= 255)
}

/** 10.x / 172.16-31.x / 192.168.x 之外的都算公网入口。 */
function isPrivateIPv4(value: string): boolean {
  if (!isIPv4(value)) return false
  const [a, b] = value.split('.').map(Number)
  if (a === 10) return true
  if (a === 172 && b >= 16 && b <= 31) return true
  if (a === 192 && b === 168) return true
  return false
}

function hostOf(address: string): string {
  const parts = address.split(':')
  return parts.length === 2 ? parts[0] : address
}

/** 类型直接来自配置（friend / server / cloud），不猜。 */
export function kindOf(peer: TopoPeer): string {
  return peer.kind || 'peer'
}

/** 中继基建里的机器：配置里 cloud_servers 那一块，解析完就是 kind = cloud 的条目。 */
function pickRelays(nodes: TopoNode[]): TopoNode[] {
  return nodes.filter((node) => node.kind === 'cloud')
}

/** 核心中继（WG 主干）：配置里写了 core: true 的那台。 */
function pickCore(relays: TopoNode[]): TopoNode | null {
  return relays.find((node) => node.peer.core === true) ?? null
}

/**
 * 私网地址按 /24 归子网；公网地址算节点的「入口」。
 * 每个子网的中心节点：主机号是 1 的 → 带公网入口的 → 第一个成员。
 */
function analyze(peers: TopoPeer[]): { nodes: TopoNode[]; subnets: Subnet[] } {
  const subnets = new Map<string, Subnet>()
  const nodes: TopoNode[] = []

  for (const peer of peers) {
    const node: TopoNode = { peer, kind: kindOf(peer), endpoints: [], memberships: [] }
    for (const entry of peer.addresses ?? []) {
      const host = hostOf(entry.address)
      if (isPrivateIPv4(host)) {
        const prefix = host.split('.').slice(0, 3).join('.')
        let subnet = subnets.get(prefix)
        if (!subnet) {
          subnet = { prefix, label: `${prefix}.0/24`, members: [], hub: node, hubAddress: '' }
          subnets.set(prefix, subnet)
        }
        subnet.members.push({ node, address: entry.address })
        node.memberships.push({ prefix, address: entry.address })
      } else {
        node.endpoints.push(entry.address)
      }
    }
    nodes.push(node)
  }

  for (const subnet of subnets.values()) {
    const gateway = subnet.members.find((entry) => entry.address.split('.').pop() === '1')
    const withEndpoint = subnet.members.find((entry) => entry.node.endpoints.length > 0)
    subnet.hub = (gateway ?? withEndpoint ?? subnet.members[0]).node
    subnet.hubAddress = subnet.members.find((entry) => entry.node === subnet.hub)?.address ?? ''
  }

  const list = [...subnets.values()].sort((a, b) => a.prefix.localeCompare(b.prefix, 'en', { numeric: true }))
  return { nodes, subnets: list }
}

// ------------------------------------------------------------------ 排版零件

const CJK = /[\u2e80-\u9fff\uff00-\uffef\u3000-\u303f]/

function textWidth(text: string, size: number, mono = false): number {
  let width = 0
  for (const char of text) {
    if (CJK.test(char)) width += size
    else if (mono) width += size * 0.6
    else width += size * 0.56
  }
  return width
}

const esc = (value: string) =>
  String(value).replace(/[&<>"']/g, (char) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[char] ?? char)

const C = {
  panel: 'var(--panel)',
  panel2: 'var(--panel-2)',
  line: 'var(--line)',
  lineStrong: 'var(--line-strong)',
  text: 'var(--text)',
  muted: 'var(--muted)',
  accent: 'var(--accent)',
  accentSoft: 'var(--accent-soft)',
  up: 'var(--up)',
  down: 'var(--down)',
  warn: 'var(--warn)',
  kind: { friend: 'var(--topo-friend)', server: 'var(--topo-server)', cloud: 'var(--topo-cloud)', peer: 'var(--muted)' } as Record<string, string>,
  net: ['var(--topo-net-1)', 'var(--topo-net-2)', 'var(--topo-net-3)'],
}

const line = (x1: number, y1: number, x2: number, y2: number, color: string, options: { dashed?: boolean; width?: number; opacity?: number } = {}) =>
  `<line x1="${x1}" y1="${y1}" x2="${x2}" y2="${y2}" stroke="${color}" stroke-width="${options.width ?? 1.8}" ${options.dashed ? 'stroke-dasharray="5 4" ' : ''}stroke-linecap="round" opacity="${options.opacity ?? 1}"/>`

/** 金色双线：看着像一根隧道。 */
function tunnel(x1: number, y1: number, x2: number, y2: number, width = 3.4): string {
  const d = `M ${x1} ${y1} L ${x2} ${y2}`
  return `<path d="${d}" stroke="${C.warn}" stroke-width="${width}" stroke-linecap="round" fill="none" opacity="0.55"/>` +
    `<path d="${d}" stroke="${C.warn}" stroke-width="${width / 3}" stroke-linecap="round" fill="none"/>`
}

function pill(x: number, y: number, text: string, fill: string, color: string, size = 10): { width: number; svg: string } {
  const width = textWidth(text, size) + size * 1.2
  return {
    width,
    svg: `<rect x="${x}" y="${y}" width="${width}" height="16" rx="8" fill="${fill}"/>` +
      `<text x="${x + width / 2}" y="${y + 11.5}" font-size="${size}" fill="${color}" text-anchor="middle">${esc(text)}</text>`,
  }
}

const dot = (cx: number, cy: number, color: string, r = 4) => `<circle cx="${cx}" cy="${cy}" r="${r}" fill="${color}"/>`

export function kindIcon(kind: string, cx: number, cy: number, color: string, size = 13): string {
  const s = size / 13
  if (kind === 'friend') {
    return `<circle cx="${cx}" cy="${cy - 3.4 * s}" r="${2.9 * s}" fill="${color}"/>` +
      `<path d="M ${cx - 5 * s} ${cy + 5.2 * s} a ${5 * s} ${4.4 * s} 0 0 1 ${10 * s} 0" fill="none" stroke="${color}" stroke-width="${1.9 * s}" stroke-linecap="round"/>`
  }
  if (kind === 'server') {
    return `<rect x="${cx - 5.2 * s}" y="${cy - 5.2 * s}" width="${10.4 * s}" height="${10.4 * s}" rx="${2 * s}" fill="none" stroke="${color}" stroke-width="${1.5 * s}"/>` +
      `<line x1="${cx - 2.6 * s}" y1="${cy - 1.8 * s}" x2="${cx + 2.6 * s}" y2="${cy - 1.8 * s}" stroke="${color}" stroke-width="${1.3 * s}" stroke-linecap="round"/>` +
      `<line x1="${cx - 2.6 * s}" y1="${cy + 1.6 * s}" x2="${cx + 1 * s}" y2="${cy + 1.6 * s}" stroke="${color}" stroke-width="${1.3 * s}" stroke-linecap="round"/>`
  }
  if (kind === 'cloud') {
    return `<circle cx="${cx - 2.9 * s}" cy="${cy - 0.6 * s}" r="${3.2 * s}" fill="${color}"/>` +
      `<circle cx="${cx + 1.4 * s}" cy="${cy - 2.4 * s}" r="${3.6 * s}" fill="${color}"/>` +
      `<circle cx="${cx + 4 * s}" cy="${cy + 0.4 * s}" r="${2.7 * s}" fill="${color}"/>` +
      `<rect x="${cx - 5.4 * s}" y="${cy + 0.4 * s}" width="${9.6 * s}" height="${3.4 * s}" rx="${1.7 * s}" fill="${color}"/>`
  }
  return `<circle cx="${cx}" cy="${cy}" r="${4 * s}" fill="none" stroke="${color}" stroke-width="${1.6 * s}"/>` +
    `<circle cx="${cx}" cy="${cy}" r="${1.4 * s}" fill="${color}"/>`
}

interface CardLine {
  text?: string
  color?: string
  size?: number
  mono?: boolean
  dot?: string
  gap?: number
  tag?: string
  tagFill?: string
  tagColor?: string
}

interface CardOptions {
  x: number
  y: number
  width: number
  height: number
  title: string
  kind: string
  hub?: boolean
  emphasis?: boolean
  online?: boolean
  badge?: string
  lines?: CardLine[]
}

interface CardBox {
  svg: string
  x: number
  y: number
  width: number
  height: number
  center: { x: number; y: number }
  top: { x: number; y: number }
  bottom: { x: number; y: number }
  left: { x: number; y: number }
  right: { x: number; y: number }
}

function card(options: CardOptions): CardBox {
  const { x, y, width, height, title, kind } = options
  const lines = options.lines ?? []
  const kindColor = C.kind[kind] ?? C.kind.peer
  const body: string[] = []
  body.push(`<rect x="${x + 7.5}" y="${y + 8}" width="4" height="${height - 16}" rx="2" fill="${kindColor}"/>`)
  const titleY = lines.length ? y + 27 : y + height / 2 + 5
  body.push(kindIcon(kind, x + 26, titleY - 5, kindColor, 14))
  body.push(`<text x="${x + 43}" y="${titleY}" font-size="15" font-weight="${options.hub ? 650 : 550}" fill="${C.text}">${esc(title)}</text>`)
  if (options.badge) {
    const badgeX = x + 43 + textWidth(title, 15) + 8
    const tag = pill(badgeX, titleY - 13, options.badge, C.accentSoft, C.accent, 11)
    if (tag.width <= width - (badgeX - x) - 16) body.push(tag.svg)
  }
  body.push(dot(x + width - 15, titleY - 5, options.online === false ? C.down : C.up, 4.4))

  let cursor = titleY + 21
  for (const item of lines) {
    let textX = x + 17
    if (item.dot) {
      body.push(dot(x + 20, cursor - 4, item.dot, 3.2))
      textX = x + 30
    }
    const cls = item.mono === false ? '' : ' class="mono"'
    body.push(`<text x="${textX}" y="${cursor}" font-size="${item.size ?? 12}"${cls} fill="${item.color ?? C.muted}">${esc(item.text ?? '')}</text>`)
    if (item.tag) {
      const tagWidth = textWidth(item.tag, 11) + 13
      body.push(`<rect x="${x + width - 12 - tagWidth}" y="${cursor - 10.5}" width="${tagWidth}" height="16" rx="8" fill="${item.tagFill ?? C.accentSoft}"/>`)
      body.push(`<text x="${x + width - 12 - tagWidth / 2}" y="${cursor + 1}" font-size="11" fill="${item.tagColor ?? C.accent}" text-anchor="middle">${esc(item.tag)}</text>`)
    }
    cursor += item.gap ?? 16
  }

  const emphasized = Boolean(options.emphasis || options.hub)
  const strokeWidth = options.emphasis ? 2.4 : options.hub ? 1.6 : 1
  return {
    svg: `<rect x="${x}" y="${y}" width="${width}" height="${height}" rx="10" fill="${C.panel}" stroke="${emphasized ? C.accent : C.line}" stroke-width="${strokeWidth}"/>${body.join('')}`,
    x, y, width, height,
    center: { x: x + width / 2, y: y + height / 2 },
    top: { x: x + width / 2, y },
    bottom: { x: x + width / 2, y: y + height },
    left: { x, y: y + height / 2 },
    right: { x: x + width, y: y + height / 2 },
  }
}

const diamond = (cx: number, cy: number, r: number, radiusStroke: string) =>
  `<path d="M ${cx} ${cy - r} L ${cx + r} ${cy} L ${cx} ${cy + r} L ${cx - r} ${cy} Z" fill="${C.panel}" stroke="${radiusStroke}" stroke-width="1.2"/>`

function doubleArrow(x: number, topY: number, bottomY: number, color: string, dashed: boolean, width = 2): string {
  const head = 7
  return line(x, topY + head + 2, x, bottomY - head - 2, color, { dashed, width }) +
    `<path d="M ${x} ${topY} L ${x - 5.4} ${topY + head + 2} L ${x + 5.4} ${topY + head + 2} Z" fill="${color}"/>` +
    `<path d="M ${x} ${bottomY} L ${x - 5.4} ${bottomY - head - 2} L ${x + 5.4} ${bottomY - head - 2} Z" fill="${color}"/>`
}

// ------------------------------------------------------------------ 画

const WIDTH = 1240

/** 折叠标题上的小计数：跟画图用同一套规则算，别在组件里另写一份。 */
export function topologyCounts(peers: TopoPeer[]): { relays: number; peers: number } {
  if (!peers.length) return { relays: 0, peers: 0 }
  const { nodes } = analyze(peers)
  return { relays: pickRelays(nodes).length, peers: nodes.length }
}

/** peers → 一张 SVG；没有能画的条目就返回空串。 */
export function renderTopology(peers: TopoPeer[], labels: TopoLabels): string {
  if (!peers.length) return ''
  const { nodes, subnets } = analyze(peers)
  if (!nodes.length) return ''

  const relays = pickRelays(nodes)
  const relaySet = new Set(relays)
  const members = nodes.filter((node) => !relaySet.has(node))

  // WG 链路：服务端已经按 core 推好（主干 ↔ 其余），这里只做无向去重
  const byName = new Map(nodes.map((node) => [node.peer.name, node]))
  const links: { a: TopoNode; b: TopoNode }[] = []
  const seen = new Set<string>()
  for (const node of relays) {
    for (const name of node.peer.wgPeers ?? []) {
      const other = byName.get(name)
      if (!other || other === node || !relaySet.has(other)) continue
      const key = [node.peer.name, other.peer.name].sort().join('\u0000')
      if (seen.has(key)) continue
      seen.add(key)
      links.push({ a: node, b: other })
    }
  }

  const degree = new Map(relays.map((node) => [node, 0]))
  for (const link of links) {
    degree.set(link.a, (degree.get(link.a) ?? 0) + 1)
    degree.set(link.b, (degree.get(link.b) ?? 0) + 1)
  }
  const core = pickCore(relays)

  // 中继排布：链路最多的摆中间，其它交替两边
  const ordered = [...relays].sort((a, b) => (degree.get(b) ?? 0) - (degree.get(a) ?? 0))
  const laid: TopoNode[] = []
  ordered.forEach((node, index) => (index % 2 === 0 ? laid.push(node) : laid.unshift(node)))

  // 服务器类的接入节点摆基建上方，其它摆下方；两排各自铺满宽度
  const upper = members.filter((node) => node.kind === 'server')
  const lower = members.filter((node) => node.kind !== 'server')
  const rowHeight = (node: TopoNode) => 52 + node.memberships.length * 16
  const rowMax = (list: TopoNode[]) => list.reduce((max, node) => Math.max(max, rowHeight(node)), 96)

  const boxX = 56
  const boxW = WIDTH - boxX * 2
  const boxH = 186
  const upperY = 20
  const boxY = upper.length ? upperY + rowMax(upper) + 74 : 20
  const boxBottom = boxY + boxH
  const lowerY = boxBottom + 112
  const height = (lower.length ? lowerY + rowMax(lower) : boxBottom) + 26

  const parts: string[] = []
  parts.push(`<svg xmlns="http://www.w3.org/2000/svg" width="${WIDTH}" height="${height}" viewBox="0 0 ${WIDTH} ${height}" role="img" aria-label="${esc(labels.hub)}">
<defs>
  <style>
    text { font-family: 'Inter','PingFang SC','Noto Sans CJK SC','Microsoft YaHei',system-ui,sans-serif; }
    .mono { font-family: 'SFMono-Regular',Menlo,Consolas,'DejaVu Sans Mono',monospace; }
  </style>
</defs>
<rect width="${WIDTH}" height="${height}" fill="transparent"/>`)

  // 基建容器
  parts.push(`<rect x="${boxX}" y="${boxY}" width="${boxW}" height="${boxH}" rx="16" fill="${C.panel2}" stroke="${C.lineStrong}" stroke-width="1.4"/>`)
  parts.push(`<text x="${boxX + 22}" y="${boxY + 32}" font-size="16.5" font-weight="650" fill="${C.text}">${esc(labels.hub)}</text>`)
  parts.push(`<text x="${boxX + 22 + textWidth(labels.hub, 16.5) + 10}" y="${boxY + 32}" font-size="12.5" fill="${C.muted}">${esc(labels.hubSubtitle(relays.length))}</text>`)
  parts.push(`<text x="${boxX + boxW - 22}" y="${boxY + 32}" font-size="12.5" fill="${C.muted}" text-anchor="end">${esc(labels.attached(members.length))}</text>`)

  // 中继服务器
  const step = Math.min(360, (boxW - 280) / Math.max(1, laid.length - 1))
  const centerY = boxY + 114
  const boxes = laid.map((node, index) => {
    const cx = boxX + boxW / 2 + (index - (laid.length - 1) / 2) * step
    const subnet = subnets.find((entry) => entry.hub === node) ??
      subnets.find((entry) => node.memberships.some((membership) => membership.prefix === entry.prefix))
    const subnetColor = C.net[Math.max(0, subnets.indexOf(subnet as Subnet)) % C.net.length]
    const up = node.memberships.every((entry) => peerStatus(node.peer, entry.address) === 'up')
    const isCore = node === core && links.length > 0
    const width = isCore ? 292 : 244
    const cardHeight = isCore ? 114 : 94
    const lines: CardLine[] = [
      { text: `${subnet ? subnet.label : ''}  ${subnet ? subnet.hubAddress : node.memberships.map((entry) => entry.address).join(' ')}`, color: subnetColor, size: 11.5 },
      { text: node.endpoints[0] ?? '—', color: C.warn, size: 11.5, gap: isCore ? 17 : 0 },
    ]
    if (isCore) {
      const names = links.filter((link) => link.a === node || link.b === node).map((link) => (link.a === node ? link.b : link.a).peer.name)
      lines.push({ text: labels.corePeers(names.join('、')), color: C.warn, size: 11, gap: 0 })
    }
    return {
      node, cx, isCore,
      box: card({
        x: cx - width / 2, y: centerY - cardHeight / 2, width, height: cardHeight, hub: true, emphasis: isCore,
        online: up, kind: node.kind, title: node.peer.name, badge: isCore ? labels.core : labels.relay, lines,
      }),
    }
  })

  for (const entry of boxes) {
    if (!entry.isCore) continue
    parts.push(`<rect x="${entry.box.x - 13}" y="${entry.box.y - 13}" width="${entry.box.width + 26}" height="${entry.box.height + 26}" rx="20" fill="${C.accentSoft}" stroke="${C.accent}" stroke-width="1" stroke-dasharray="3 4" opacity="0.9"/>`)
  }

  for (const link of links) {
    const from = boxes.find((entry) => entry.node === link.a)
    const to = boxes.find((entry) => entry.node === link.b)
    if (!from || !to) continue
    const [left, right] = from.cx < to.cx ? [from, to] : [to, from]
    const adjacent = laid.indexOf(right.node) - laid.indexOf(left.node) === 1
    const strong = left.isCore || right.isCore
    if (adjacent) {
      const x1 = left.box.right.x + 4
      const x2 = right.box.left.x - 4
      const y = left.box.center.y
      parts.push(tunnel(x1, y, x2, y, strong ? 4.6 : 3.4))
      parts.push(pill((x1 + x2) / 2 - 13, y - 20, labels.wg, C.panel, C.warn).svg)
    } else {
      const x1 = left.box.center.x
      const x2 = right.box.center.x
      const peak = boxY + 22
      const path = `M ${x1} ${left.box.top.y} Q ${(x1 + x2) / 2} ${peak} ${x2} ${right.box.top.y}`
      parts.push(`<path d="${path}" fill="none" stroke="${C.warn}" stroke-width="3.4" stroke-linecap="round" opacity="0.55"/>`)
      parts.push(`<path d="${path}" fill="none" stroke="${C.warn}" stroke-width="1.2" stroke-linecap="round"/>`)
      parts.push(pill((x1 + x2) / 2 - 13, peak - 2, labels.wg, C.panel, C.warn).svg)
    }
  }
  for (const entry of boxes) parts.push(entry.box.svg)

  // 接入节点：服务器在上、其它在下，都用双箭头接进基建
  const layRow = (list: TopoNode[], top: number) =>
    list.map((node, index) => {
      const rowStep = (WIDTH - 200) / Math.max(1, list.length - 1)
      const cx = list.length === 1 ? WIDTH / 2 : 100 + index * rowStep
      const state = reachState(node)
      const color = state === 'down' ? C.down : state === 'partial' ? C.warn : C.up
      const lines: CardLine[] = node.memberships.map((entry) => {
        const subnetIndex = subnets.findIndex((subnet) => subnet.prefix === entry.prefix)
        const up = peerStatus(node.peer, entry.address) === 'up'
        return { text: entry.address, color: up ? C.net[subnetIndex % C.net.length] : C.down, dot: up ? C.up : C.down, gap: 16 }
      })
      const needed = Math.max(...lines.map((entry) => 32 + textWidth(entry.text ?? '', 12, true)), 0)
      const width = Math.max(176, 43 + textWidth(node.peer.name, 15) + 28, needed + 20)
      return {
        node, cx, color, dashed: state === 'down',
        box: card({
          x: cx - width / 2, y: top, width, height: rowHeight(node),
          online: state !== 'down', kind: node.kind, title: node.peer.name, lines,
        }),
      }
    })

  const upperBoxes = layRow(upper, upperY)
  const lowerBoxes = layRow(lower, lowerY)
  for (const entry of upperBoxes) {
    parts.push(doubleArrow(entry.cx, boxY - 5, entry.box.bottom.y + 5, entry.color, entry.dashed))
    parts.push(diamond(entry.cx, boxY, 5, entry.color))
  }
  for (const entry of lowerBoxes) {
    parts.push(doubleArrow(entry.cx, boxBottom + 5, entry.box.top.y - 5, entry.color, entry.dashed))
    parts.push(diamond(entry.cx, boxBottom, 5, entry.color))
  }
  for (const entry of [...upperBoxes, ...lowerBoxes]) parts.push(entry.box.svg)

  parts.push('</svg>')
  return parts.join('\n')
}

/** 某个地址在探测结果里的状态（缺记录就当成还没探到）。 */
function peerStatus(peer: TopoPeer, address: string): string {
  return peer.addresses.find((entry) => entry.address === address)?.status ?? 'down'
}

function reachState(node: TopoNode): 'up' | 'partial' | 'down' {
  if (!node.memberships.length) return node.peer.online ? 'up' : 'down'
  const ups = node.memberships.filter((entry) => peerStatus(node.peer, entry.address) === 'up').length
  if (ups === 0) return 'down'
  return ups < node.memberships.length ? 'partial' : 'up'
}
