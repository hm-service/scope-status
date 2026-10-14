<script setup lang="ts">
// 拓扑图：从当前快照的 peers 现算布局吐 SVG，外面套一层画布（滚轮/按钮缩放、拖动平移、双击复位）。
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { t } from '../i18n'
import { state, toggleTopology } from '../store'
import { kindIcon, renderTopology, topologyCounts, type TopoLabels, type TopoPeer } from '../utils/topology'

const labels = computed<TopoLabels>(() => ({
  hub: t('topology.hub'),
  hubSubtitle: (relays: number) => t('topology.hubSubtitle', { count: relays }),
  attached: (nodes: number) => t('topology.attached', { count: nodes }),
  relay: t('topology.relay'),
  core: t('topology.core'),
  corePeers: (names: string) => t('topology.corePeers', { peers: names }),
  wg: t('topology.wg'),
}))

const svg = computed(() => renderTopology((state.snapshot?.peers ?? []) as TopoPeer[], labels.value))
const counts = computed(() => topologyCounts((state.snapshot?.peers ?? []) as TopoPeer[]))

/** 图例里那三个小图标：跟 SVG 里用的是同一套画法。 */
const legendIcons: Record<string, string> = {
  friend: kindIcon('friend', 8, 8, 'var(--topo-friend)', 14),
  server: kindIcon('server', 8, 8, 'var(--topo-server)', 14),
  cloud: kindIcon('cloud', 8, 8, 'var(--topo-cloud)', 14),
}

// ---------------------------------------------------------------- 画布

const canvas = ref<HTMLElement | null>(null)
const size = ref({ w: 0, h: 0 })
const natural = ref({ w: 1240, h: 620 })
const scale = ref(1)
const offset = ref({ x: 0, y: 0 })
const dragging = ref(false)
const fitScale = ref(1)
const viewportH = ref(typeof window === 'undefined' ? 800 : window.innerHeight)

const MIN_FACTOR = 0.5 // 最小能缩到「适应宽度」的 50%
const MAX_FACTOR = 4 // 最大放到自然尺寸的 4 倍
const MIN_READABLE = 0.55 // 窄屏别为了塞下而把字缩没：宁可让它横向拖

let pointerStart = { x: 0, y: 0 }
const clamp = (value: number, min: number, max: number) => Math.min(Math.max(value, min), max)

/** 别让图被拖出画布：比画布小就居中/贴顶，比画布大就限制在边界内。 */
function clampOffset(): void {
  const { w, h } = size.value
  const scaledW = natural.value.w * scale.value
  const scaledH = natural.value.h * scale.value
  offset.value = {
    x: scaledW <= w ? (w - scaledW) / 2 : clamp(offset.value.x, w - scaledW, 0),
    y: scaledH <= h ? 0 : clamp(offset.value.y, h - scaledH, 0),
  }
}

/** 缩放比例相对「适应宽度」显示不直观，这里给个绝对值百分数。 */

/** 回到「适应宽度」的初始状态（宽了就居中，窄了就贴左让人拖）。 */
function resetView(): void {
  scale.value = fitScale.value
  offset.value = { x: (size.value.w - natural.value.w * scale.value) / 2, y: 0 }
  clampOffset()
}

/** 以画布里的某个点为锚点缩放。 */
function zoomAt(next: number, px: number, py: number): void {
  const target = clamp(next, fitScale.value * MIN_FACTOR, MAX_FACTOR)
  if (target === scale.value) return
  const ratio = target / scale.value
  offset.value = { x: px - (px - offset.value.x) * ratio, y: py - (py - offset.value.y) * ratio }
  scale.value = target
  clampOffset()
}

function zoomBy(factor: number): void {
  zoomAt(scale.value * factor, size.value.w / 2, size.value.h / 2)
}

function measure(): void {
  const element = canvas.value
  if (!element) return
  const rect = element.getBoundingClientRect()
  if (rect.width < 10) return // 切到别的 tab 时宽度是 0，别把状态算坏
  const match = /viewBox="0 0 ([\d.]+) ([\d.]+)"/.exec(svg.value)
  if (match) natural.value = { w: Number(match[1]), h: Number(match[2]) }
  size.value = { w: rect.width, h: rect.height }
  viewportH.value = window.innerHeight
  const previousFit = fitScale.value
  fitScale.value = Math.max(rect.width / natural.value.w, MIN_READABLE)
  // 之前一直停在「适应宽度」上，尺寸变化后继续保持适应；否则只做边界修正
  if (!previousFit || Math.abs(scale.value - previousFit) < 0.0001) resetView()
  else clampOffset()
}

function onPointerDown(event: PointerEvent): void {
  if (event.button !== 0) return
  dragging.value = true
  pointerStart = { x: event.clientX - offset.value.x, y: event.clientY - offset.value.y }
  ;(event.currentTarget as HTMLElement).setPointerCapture(event.pointerId)
}

function onPointerMove(event: PointerEvent): void {
  if (!dragging.value) return
  offset.value = { x: event.clientX - pointerStart.x, y: event.clientY - pointerStart.y }
  clampOffset()
}

function onPointerUp(event: PointerEvent): void {
  if (!dragging.value) return
  dragging.value = false
  const element = event.currentTarget as HTMLElement
  if (element.hasPointerCapture(event.pointerId)) element.releasePointerCapture(event.pointerId)
}

/** 滚轮缩放；已经缩到最小还继续往下滚，就放行让页面正常滚。 */
function onWheel(event: WheelEvent): void {
  const next = scale.value * Math.exp(-event.deltaY * 0.0015)
  if (next <= fitScale.value * MIN_FACTOR && event.deltaY > 0) return
  event.preventDefault()
  const rect = canvas.value?.getBoundingClientRect()
  if (!rect) return
  zoomAt(next, event.clientX - rect.left, event.clientY - rect.top)
}

/**
 * 缩放改「布局尺寸」而不是 CSS scale：SVG 会按目标尺寸重新光栅化，放到 400% 也是矢量清晰的；
 * CSS scale + 合成层会拉伸旧位图，那就糊了。平移只做整数 translate，避免半像素发虚。
 */
const stageStyle = computed(() => ({
  width: `${natural.value.w * scale.value}px`,
  height: `${natural.value.h * scale.value}px`,
  transform: `translate(${Math.round(offset.value.x)}px, ${Math.round(offset.value.y)}px)`,
}))

/** 画布高度按「适应宽度时的图高」给：不缩放就刚好装下，放大了再拖。 */
const canvasStyle = computed(() => ({
  height: `${Math.round(clamp(natural.value.h * fitScale.value + 2, 240, Math.min(640, viewportH.value * 0.7)))}px`,
}))

const zoomText = computed(() => `${Math.round(scale.value * 100)}%`)

let observer: ResizeObserver | null = null
let attached: HTMLElement | null = null

function attach(element: HTMLElement): void {
  attached = element
  element.addEventListener('wheel', onWheel, { passive: false })
  observer = new ResizeObserver(() => measure())
  observer.observe(element)
  measure()
  resetView()
}

function detach(): void {
  if (attached) attached.removeEventListener('wheel', onWheel)
  observer?.disconnect()
  observer = null
  attached = null
  fitScale.value = 0 // 下次出现时重新算「适应宽度」
  scale.value = 1
}

// 画布是 v-if 出来的（首帧快照还没到），挂载时机跟着 ref 走
watch(canvas, (element) => (element ? attach(element) : detach()), { flush: 'post' })
onBeforeUnmount(detach)
</script>

<template>
  <section v-if="svg" class="topo" id="topology" :class="{ 'is-collapsed': !state.topologyOpen }">
    <header
      class="topo-head"
      role="button"
      tabindex="0"
      :title="t('topology.toggle')"
      :aria-expanded="state.topologyOpen"
      @click="toggleTopology"
      @keydown.enter.prevent="toggleTopology"
      @keydown.space.prevent="toggleTopology"
    >
      <span class="group-chevron">▾</span>
      <span class="topo-title">{{ t('topology.title') }}</span>
      <span class="topo-count">{{ t('topology.count', { relays: counts.relays, peers: counts.peers }) }}</span>
      <span v-if="state.topologyOpen" class="topo-tools" @click.stop>
        <button type="button" class="topo-tool" :title="t('topology.zoomOut')" :aria-label="t('topology.zoomOut')" @click="zoomBy(1 / 1.25)">−</button>
        <span class="topo-zoom" :title="t('topology.zoomHint')">{{ zoomText }}</span>
        <button type="button" class="topo-tool" :title="t('topology.zoomIn')" :aria-label="t('topology.zoomIn')" @click="zoomBy(1.25)">+</button>
        <button type="button" class="topo-tool topo-tool-text" @click="resetView">{{ t('topology.reset') }}</button>
      </span>
    </header>

    <div v-show="state.topologyOpen" class="topo-body">
      <div class="topo-legend">
        <span class="topo-legend-item"><svg width="16" height="16" viewBox="0 0 16 16" aria-hidden="true" v-html="legendIcons.friend" />{{ t('topology.legend.friend') }}</span>
        <span class="topo-legend-item"><svg width="16" height="16" viewBox="0 0 16 16" aria-hidden="true" v-html="legendIcons.server" />{{ t('topology.legend.server') }}</span>
        <span class="topo-legend-item"><svg width="16" height="16" viewBox="0 0 16 16" aria-hidden="true" v-html="legendIcons.cloud" />{{ t('topology.legend.cloud') }}</span>
        <span class="topo-legend-item"><i class="topo-sample topo-sample-wg" />{{ t('topology.legend.wg') }}</span>
        <span class="topo-legend-item"><i class="topo-sample topo-sample-core" />{{ t('topology.legend.core') }}</span>
        <span class="topo-legend-item"><i class="topo-sample topo-sample-up" />{{ t('topology.legend.up') }}</span>
        <span class="topo-legend-item"><i class="topo-sample topo-sample-partial" />{{ t('topology.legend.partial') }}</span>
        <span class="topo-legend-item"><i class="topo-sample topo-sample-down" />{{ t('topology.legend.down') }}</span>
      </div>

      <div
        ref="canvas"
        class="topo-canvas"
        :class="{ 'is-dragging': dragging }"
        :style="canvasStyle"
        :title="t('topology.canvasHint')"
        @pointerdown="onPointerDown"
        @pointermove="onPointerMove"
        @pointerup="onPointerUp"
        @pointercancel="onPointerUp"
        @dblclick="resetView"
      >
        <div class="topo-stage" :style="stageStyle" v-html="svg" />
      </div>

      <p class="topo-note">{{ t('topology.note') }}</p>
    </div>
  </section>
</template>
