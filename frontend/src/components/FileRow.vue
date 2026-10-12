<script setup lang="ts">
import { computed } from 'vue'
import { ICONS } from '../icons'
import { t } from '../i18n'
import type { Attachment } from '../types'
import { formatSize } from '../utils/format'

const props = defineProps<{ attachment: Attachment; messageKey?: string | null; draft?: boolean }>()
defineEmits<{ remove: [] }>()

const downloadHref = computed(() => {
  const url = props.attachment.url
  if (!url) return ''
  return props.messageKey ? `${url}&key=${encodeURIComponent(props.messageKey)}` : url
})
</script>

<template>
  <li class="file-row" :class="{ 'is-draft': draft }">
    <span class="icon file-icon" v-html="ICONS.file"></span>
    <span class="file-text">
      <span class="file-name" :title="attachment.name">{{ attachment.name }}</span>
      <span class="file-size">{{ formatSize(attachment.size) }}</span>
    </span>
    <a
      v-if="!draft && attachment.url"
      class="file-download"
      :href="downloadHref"
      :title="t('messages.download', { name: attachment.name })"
      :download="attachment.name"
      v-html="ICONS.download"
    ></a>
    <button
      v-if="draft"
      type="button"
      class="file-remove"
      :title="t('messages.removeFile')"
      :aria-label="t('messages.removeFileAria', { name: attachment.name })"
      v-html="ICONS.close"
      @click="$emit('remove')"
    ></button>
  </li>
</template>
