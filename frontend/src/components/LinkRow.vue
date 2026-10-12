<script setup lang="ts">
import { computed, ref } from 'vue'
import { ICONS } from '../icons'
import { t } from '../i18n'
import { state } from '../store'
import type { Link } from '../types'
import { latencyText } from '../utils/format'
import PropertyList from './PropertyList.vue'

const props = defineProps<{ link: Link }>()

const properties = computed(() => props.link.properties || {})
const propCount = computed(() => Object.keys(properties.value).length)
const hasProps = computed(() => propCount.value > 0)
const expandKey = computed(() => `link:${props.link.name}`)
const isOpen = computed(() => hasProps.value && state.expanded.has(expandKey.value))

// 图标拿不到就继续显示默认图标，不留破图
const iconLoaded = ref(false)
const iconFailed = ref(false)
const showIcon = computed(() => Boolean(props.link.faviconUrl) && !iconFailed.value)

const statusTitle = computed(() =>
  props.link.online ? t('status.onlineTitle') : props.link.detail || t('status.offlineTitle'),
)

/** 名字本身就是入口：点它打开网站；探测地址放在 title 里，不占一行。 */
const nameTitle = computed(() => {
  if (props.link.probeUrl === props.link.url) return props.link.url
  const probe = t('links.probe', { probeUrl: props.link.probeUrl ?? '' })
  return `${props.link.url}\n${probe}${props.link.detail ? ` (${props.link.detail})` : ''}`
})
const openTitle = computed(() =>
  props.link.detail ? `${props.link.url}${t('links.openDetail', { detail: props.link.detail })}` : props.link.url,
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
    class="row link"
    :class="[link.online ? 'is-up' : 'is-down', { 'has-props': hasProps, 'is-open': isOpen }]"
    :role="hasProps ? 'button' : undefined"
    :tabindex="hasProps ? 0 : undefined"
    :title="hasProps ? t('props.toggle') : undefined"
    :aria-expanded="hasProps ? isOpen : undefined"
    @click="onClick"
    @keydown="onKeydown"
  >
    <div class="row-main">
      <span class="cell device">
        <span class="link-icon" aria-hidden="true">
          <span class="link-icon-fallback" :hidden="iconLoaded">🔗</span>
          <img v-if="showIcon" alt="" :src="link.faviconUrl ?? ''" @load="iconLoaded = true" @error="iconFailed = true" />
        </span>
        <span class="chevron-slot">
          <span class="chevron" v-html="hasProps ? ICONS.chevron : ''"></span>
          <span class="chevron-count">{{ hasProps ? propCount : '' }}</span>
        </span>
        <span class="device-text">
          <a class="row-name link-name" target="_blank" rel="noopener" :href="link.url" :title="nameTitle">{{ link.name }}</a>
        </span>
      </span>
      <span class="cell status-cell">
        <span class="status" :title="statusTitle">
          <i class="dot"></i>
          <span class="status-text">{{ link.online ? t('status.online') : t('status.offline') }}</span>
        </span>
      </span>
      <span class="cell latency latency-cell">
        <span class="latency-value">{{ latencyText(link.latencyMs) || '—' }}</span>
      </span>
      <span class="cell link-cell">
        <a class="link-open" target="_blank" rel="noopener" :href="link.url" :title="openTitle">{{ t('links.open') }}</a>
      </span>
    </div>
    <PropertyList :properties="properties" :open="isOpen" />
  </li>
</template>
