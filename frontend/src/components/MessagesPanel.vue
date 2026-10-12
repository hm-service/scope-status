<script setup lang="ts">
import { ICONS } from '../icons'
import { t } from '../i18n'
import { openCompose, state } from '../store'
import type { Message } from '../types'
import ComposeForm from './ComposeForm.vue'
import LockedMessageCard from './LockedMessageCard.vue'
import MessageCard from './MessageCard.vue'

/** 已解锁的密钥消息用本地拿到的完整内容渲染。 */
function payloadFor(entry: Message): Message {
  return entry.locked ? state.unlockedPayloads.get(entry.id) ?? entry : entry
}
</script>

<template>
  <section class="panel" id="panel-messages" :hidden="state.tab !== 'messages'" :class="{ 'is-composing': state.compose.open }">
    <div class="msg-layout">
      <div class="msg-list-view" id="msg-list-view">
        <header class="msg-head">
          <h2 class="msg-title">{{ t('messages.title') }}</h2>
          <button class="btn btn-primary" id="msg-new" type="button" @click="openCompose()">
            <span class="icon" id="msg-new-icon" aria-hidden="true" v-html="ICONS.plus"></span>{{ t('messages.new') }}
          </button>
        </header>
        <div class="msg-list" id="msg-list">
          <template v-for="entry in state.messages" :key="entry.id">
            <MessageCard v-if="!entry.locked || state.unlockedPayloads.has(entry.id)" :message="payloadFor(entry)" />
            <LockedMessageCard v-else :entry="entry" />
          </template>
        </div>
        <p class="empty" id="msg-empty" :hidden="state.messages.length > 0">{{ t('empty.messages') }}</p>
      </div>
      <ComposeForm v-if="state.compose.open" />
    </div>
  </section>
</template>
