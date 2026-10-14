<script setup lang="ts">
import { computed } from 'vue'
import { ICONS } from '../icons'
import { t } from '../i18n'
import { deleteMessage, openCompose } from '../store'
import type { Message } from '../types'
import FileRow from './FileRow.vue'
import TimeStamp from './TimeStamp.vue'

const props = defineProps<{ message: Message }>()

const attachments = computed(() => props.message.attachments || [])
</script>

<template>
  <article class="msg-card">
    <header class="msg-card-head">
      <TimeStamp :created-at="message.createdAt" :updated-at="message.updatedAt ?? null" />
    </header>
    <p v-if="message.text" class="msg-text">{{ message.text }}</p>
    <ul v-if="attachments.length" class="file-list msg-files">
      <FileRow
        v-for="(attachment, index) in attachments"
        :key="`${attachment.sha256}:${index}`"
        :attachment="attachment"
      />
    </ul>
    <footer class="msg-foot">
      <span class="msg-actions">
        <button class="link-action" type="button" @click="openCompose({ mode: 'edit', message })">
          <span class="icon" aria-hidden="true" v-html="ICONS.pencil"></span>{{ t('messages.action.edit') }}
        </button>
        <button class="link-action is-danger" type="button" @click="deleteMessage(message.id)">
          <span class="icon" aria-hidden="true" v-html="ICONS.trash"></span>{{ t('messages.action.delete') }}
        </button>
      </span>
    </footer>
  </article>
</template>
