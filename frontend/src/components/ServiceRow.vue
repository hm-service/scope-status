<script setup lang="ts">
import { computed } from 'vue'
import { ICONS } from '../icons'
import { t } from '../i18n'
import { state } from '../store'
import type { Service } from '../types'
import { latencyText } from '../utils/format'
import CopyButton from './CopyButton.vue'
import PropertyList from './PropertyList.vue'

const props = defineProps<{ service: Service }>()

const properties = computed(() => props.service.properties || {})
const propCount = computed(() => Object.keys(properties.value).length)
const hasProps = computed(() => propCount.value > 0)
const expandKey = computed(() => `service:${props.service.name}`)
const isOpen = computed(() => hasProps.value && state.expanded.has(expandKey.value))
const statusTitle = computed(() =>
  props.service.online ? t('status.onlineTitle') : props.service.detail || t('status.offlineTitle'),
)

function toggle(): void {
  if (state.expanded.has(expandKey.value)) state.expanded.delete(expandKey.value)
  else state.expanded.add(expandKey.value)
}

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
    class="row service"
    :class="[service.online ? 'is-up' : 'is-down', { 'has-props': hasProps, 'is-open': isOpen }]"
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
          <span class="row-name">{{ service.name }}</span>
          <span class="row-note service-endpoint">{{ service.protocol.toUpperCase() }} {{ service.address }}</span>
        </span>
      </span>
      <span class="cell status-cell">
        <span class="status" :title="statusTitle">
          <i class="dot"></i>
          <span class="status-text">{{ service.online ? t('status.online') : t('status.offline') }}</span>
        </span>
      </span>
      <span class="cell latency latency-cell">
        <span class="latency-value">{{ latencyText(service.latencyMs) || '—' }}</span>
        <span class="latency-label">{{ t('row.minLatency') }}</span>
      </span>
      <span class="cell cmd-cell">
        <template v-if="service.connectCmd">
          <code class="connect-cmd" :title="service.detail || ''">{{ service.connectCmd }}</code>
          <CopyButton :text="service.connectCmd" :title="t('services.copyCmd')" />
        </template>
      </span>
    </div>
    <PropertyList :properties="properties" :open="isOpen" />
  </li>
</template>
