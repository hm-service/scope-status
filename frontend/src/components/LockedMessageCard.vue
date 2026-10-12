<script setup lang="ts">
import { nextTick, ref } from 'vue'
import { ICONS } from '../icons'
import { t } from '../i18n'
import { unlockMessage } from '../store'
import type { Message } from '../types'
import TimeStamp from './TimeStamp.vue'

const props = defineProps<{ entry: Message }>()

const asked = ref(false)
const keyInput = ref('')
const error = ref('')
const inputRef = ref<HTMLInputElement | null>(null)

async function ask(): Promise<void> {
  asked.value = true
  await nextTick()
  inputRef.value?.focus()
}

async function submit(): Promise<void> {
  if (!keyInput.value) {
    inputRef.value?.focus()
    return
  }
  const failure = await unlockMessage(props.entry.id, keyInput.value)
  if (failure) {
    error.value = failure
    inputRef.value?.select()
  }
}
</script>

<template>
  <article class="msg-card is-key is-locked">
    <header class="msg-card-head">
      <span class="msg-kind">
        <span class="icon msg-kind-icon" aria-hidden="true" v-html="ICONS.lock"></span>
        <span class="msg-kind-text">{{ t('messages.kind.key') }}</span>
      </span>
      <TimeStamp :created-at="entry.createdAt" :updated-at="null" />
    </header>
    <p class="msg-locked-title">{{ t('messages.locked.title') }}</p>
    <p class="msg-locked-note">{{ t('messages.locked.note') }}</p>
    <button class="btn btn-soft btn-block" type="button" :hidden="asked" @click="ask">{{ t('messages.locked.ask') }}</button>
    <form class="unlock-row" :hidden="!asked" @submit.prevent="submit">
      <input
        ref="inputRef"
        v-model="keyInput"
        type="password"
        class="unlock-input"
        :placeholder="t('messages.locked.placeholder')"
        autocomplete="off"
        :aria-label="t('messages.locked.aria')"
      />
      <button class="btn btn-primary" type="submit">{{ t('messages.locked.submit') }}</button>
    </form>
    <p class="msg-hint msg-hint-bad unlock-error" :hidden="!error">{{ error }}</p>
  </article>
</template>
