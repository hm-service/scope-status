<script setup lang="ts">
import { computed } from 'vue'
import { ICONS } from '../icons'
import { t } from '../i18n'
import { state } from '../store'
import type { Peer } from '../types'
import { latencyText } from '../utils/format'
import PropertyList from './PropertyList.vue'

const props = defineProps<{ peer: Peer }>()

const properties = computed(() => props.peer.properties || {})
const propCount = computed(() => Object.keys(properties.value).length)
const hasProps = computed(() => propCount.value > 0)
const expandKey = computed(() => `peer:${props.peer.kind || ''}:${props.peer.name}`)
const isOpen = computed(() => hasProps.value && state.expanded.has(expandKey.value))

const offlineDetail = computed(() => {
  const bad = (props.peer.addresses || []).find((entry) => entry.status !== 'up')
  return bad ? bad.detail : null
})
const statusTitle = computed(() =>
  props.peer.online ? t('status.onlineTitle') : offlineDetail.value || t('status.offlineTitle'),
)

function toggle(): void {
  if (state.expanded.has(expandKey.value)) state.expanded.delete(expandKey.value)
  else state.expanded.add(expandKey.value)
}

/** 属性面板内部、以及复制按钮之类自己的操作，都不要顺便开合。 */
function onClick(event: MouseEvent): void {
  if (!hasProps.value) return
  const target = event.target as HTMLElement | null
  if (target && target.closest('.props, button, a, input, select, textarea, code')) return
  toggle()
}

function onKeydown(event: KeyboardEvent): void {
  if (!hasProps.value) return
  if (event.key !== 'Enter' && event.key !== ' ') return
  event.preventDefault()
  toggle()
}
</script>

<template>
  <li
    class="row peer"
    :class="[peer.online ? 'is-up' : 'is-down', { 'has-props': hasProps, 'is-open': isOpen }]"
    :role="hasProps ? 'button' : undefined"
    :tabindex="hasProps ? 0 : undefined"
    :title="hasProps ? t('props.toggle') : undefined"
    :aria-expanded="hasProps ? isOpen : undefined"
    @click="onClick"
    @keydown="onKeydown"
  >
    <div class="row-main">
      <span class="cell device">
        <span class="chevron-slot">
          <span class="chevron" v-html="hasProps ? ICONS.chevron : ''"></span>
          <span class="chevron-count">{{ hasProps ? propCount : '' }}</span>
        </span>
        <span class="device-text">
          <span class="row-name peer-name">{{ peer.name }}</span>
          <span class="row-note peer-note">{{ peer.note || '' }}</span>
        </span>
      </span>
      <span class="cell status-cell">
        <span class="status" :title="statusTitle">
          <i class="dot"></i>
          <span class="status-text">{{ peer.online ? t('status.online') : t('status.offline') }}</span>
          <span class="status-count">{{ t('status.addresses', { count: peer.addresses.length }) }}</span>
        </span>
      </span>
      <span class="cell latency latency-cell peer-latency">
        <span class="latency-value">{{ latencyText(peer.latencyMs) || '—' }}</span>
        <span class="latency-label">{{ t('row.minLatency') }}</span>
      </span>
      <span class="cell addr-cell addresses">
        <span
          v-for="(entry, index) in peer.addresses"
          :key="index"
          class="addr-row"
          :class="`is-${entry.status}`"
          :title="entry.detail || entry.status"
        >
          <span class="addr-ip">{{ entry.address }}</span>
          <span class="addr-lat">{{ entry.status === 'up' ? latencyText(entry.latencyMs) : '—' }}</span>
        </span>
      </span>
    </div>
    <PropertyList :properties="properties" :open="isOpen" />
  </li>
</template>
