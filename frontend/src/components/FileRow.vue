<script setup lang="ts">
import { ref } from 'vue'
import { downloadAttachment, errorMessage } from '../api'
import { ICONS } from '../icons'
import { t } from '../i18n'
import { showNotice } from '../store'
import type { Attachment } from '../types'
import { formatSize } from '../utils/format'

const props = defineProps<{ attachment: Attachment; draft?: boolean }>()
defineEmits<{ remove: [] }>()

const downloading = ref(false)

/** 导航请求带不上 Authorization 头，所以下载走 fetch（见 api.downloadAttachment）。 */
async function download(): Promise<void> {
  const url = props.attachment.url
  if (!url || downloading.value) return
  downloading.value = true
  try {
    await downloadAttachment(url, props.attachment.name)
  } catch (error) {
    showNotice(t('messages.downloadFailed', { name: props.attachment.name, message: errorMessage(error) }), 'bad')
  } finally {
    downloading.value = false
  }
}
</script>

<template>
  <li class="file-row" :class="{ 'is-draft': draft }">
    <span class="icon file-icon" v-html="ICONS.file"></span>
    <span class="file-text">
      <span class="file-name" :title="attachment.name">{{ attachment.name }}</span>
      <span class="file-size">{{ formatSize(attachment.size) }}</span>
    </span>
    <button
      v-if="!draft && attachment.url"
      type="button"
      class="file-download"
      :title="t('messages.download', { name: attachment.name })"
      :aria-label="t('messages.download', { name: attachment.name })"
      :disabled="downloading"
      v-html="ICONS.download"
      @click="download"
    ></button>
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
