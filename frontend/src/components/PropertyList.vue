<script setup lang="ts">
import CopyButton from './CopyButton.vue'
import { t } from '../i18n'
import type { PropertyMap } from '../types'

withDefaults(defineProps<{ properties?: PropertyMap; open: boolean }>(), {
  properties: () => ({}),
})
</script>

<template>
  <div class="props" :hidden="!open">
    <div v-for="(value, name) in properties" :key="name" class="prop">
      <span class="prop-name">{{ name }}</span>
      <code class="prop-value">{{ value }}</code>
      <CopyButton class="btn-copy-sm" :text="String(value)" :title="t('props.copyTitle', { name })" />
    </div>
  </div>
</template>
