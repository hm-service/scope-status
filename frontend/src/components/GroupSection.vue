<script setup lang="ts">
import { computed } from 'vue'
import { t, type I18nKey } from '../i18n'
import { state } from '../store'

const props = defineProps<{
  label: string
  online: number
  total: number
  columns: I18nKey[]
  listClass: string
  collapseKey: string
}>()

const collapsed = computed(() => state.collapsed.has(props.collapseKey))

/** 分组头部可点折叠，折叠状态在重渲染后保留。 */
function toggle(): void {
  if (collapsed.value) state.collapsed.delete(props.collapseKey)
  else state.collapsed.add(props.collapseKey)
}

function onKeydown(event: KeyboardEvent): void {
  if (event.key !== 'Enter' && event.key !== ' ') return
  event.preventDefault()
  toggle()
}
</script>

<template>
  <section class="group" :class="{ 'is-collapsed': collapsed }">
    <header
      class="group-head"
      role="button"
      tabindex="0"
      :title="t('group.toggle')"
      :aria-expanded="!collapsed"
      @click="toggle"
      @keydown="onKeydown"
    >
      <span class="group-chevron">▾</span>
      <span class="group-title">{{ label }}</span>
      <span class="group-count">{{ t('group.online', { online, total }) }}</span>
    </header>
    <div class="table" :hidden="collapsed">
      <div class="thead">
        <span v-for="column in columns" :key="column">{{ t(column) }}</span>
      </div>
      <ul class="rows" :class="listClass">
        <slot />
      </ul>
    </div>
  </section>
</template>
