<script setup lang="ts">
import { computed } from 'vue'
import { t } from '../i18n'
import { fullStamp, relativeTime } from '../utils/time'

const props = defineProps<{ createdAt: number; updatedAt?: number | null }>()

const created = computed(() => fullStamp(props.createdAt))
const updated = computed(() => (props.updatedAt ? fullStamp(props.updatedAt) : ''))
const title = computed(() =>
  props.updatedAt
    ? t('messages.createdEditedAt', { created: created.value, updated: updated.value })
    : t('messages.createdAt', { time: created.value }),
)
</script>

<template>
  <span class="msg-time" :title="title">
    <span class="msg-ago">{{ relativeTime(createdAt) }}</span>
    <span class="msg-sep" aria-hidden="true">·</span>
    <span class="msg-abs">{{ created }}</span>
    <template v-if="updatedAt">
      <span class="msg-sep" aria-hidden="true">·</span>
      <span class="msg-edited" :title="t('messages.editedAt', { time: updated })">{{ t('messages.edited') }}</span>
    </template>
  </span>
</template>
