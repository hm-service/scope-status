<script setup lang="ts">
import { computed } from 'vue'
import { ICONS } from '../icons'
import { t } from '../i18n'
import { deleteMessage, openCompose, state } from '../store'
import type { Message } from '../types'
import FileRow from './FileRow.vue'
import TimeStamp from './TimeStamp.vue'

const props = defineProps<{ message: Message }>()

const isKey = computed(() => props.message.kind === 'key')
const unlockKey = computed(() => (isKey.value ? state.unlockedKeys.get(props.message.id) ?? null : null))
const attachments = computed(() => props.message.attachments || [])
</script>

<template>
  <article class="msg-card" :class="{ 'is-key': isKey }">
    <header class="msg-card-head">
      <span class="msg-kind">
        <span class="icon msg-kind-icon" aria-hidden="true" v-html="isKey ? ICONS.lock : ICONS.globe"></span>
        <span class="msg-kind-text">{{ isKey ? t('messages.kind.key') : t('messages.kind.public') }}</span>
      </span>
      <TimeStamp :created-at="message.createdAt" :updated-at="message.updatedAt ?? null" />
    </header>
    <p v-if="message.text" class="msg-text">{{ message.text }}</p>
    <ul v-if="attachments.length" class="file-list msg-files">
      <FileRow
        v-for="(attachment, index) in attachments"
        :key="`${attachment.sha256}:${index}`"
        :attachment="attachment"
        :message-key="unlockKey"
      />
    </ul>
    <footer class="msg-foot">
      <span class="msg-perm">{{ isKey ? t('messages.perm.key') : t('messages.perm.public') }}</span>
      <span class="msg-actions">
        <button class="link-action" type="button" @click="openCompose({ mode: 'edit', message, key: unlockKey })">
          <span class="icon" aria-hidden="true" v-html="ICONS.pencil"></span>{{ t('messages.action.edit') }}
        </button>
        <button class="link-action is-danger" type="button" @click="deleteMessage(message.id, unlockKey)">
          <span class="icon" aria-hidden="true" v-html="ICONS.trash"></span>{{ t('messages.action.delete') }}
        </button>
      </span>
    </footer>
  </article>
</template>
