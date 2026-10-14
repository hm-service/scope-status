<script setup lang="ts">
import { ICONS } from '../icons'
import { t } from '../i18n'
import type { Attachment } from '../types'
import { formatSize } from '../utils/format'

defineProps<{ attachment: Attachment; draft?: boolean }>()
defineEmits<{ remove: [] }>()
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
      :href="attachment.url"
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
