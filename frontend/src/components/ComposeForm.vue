<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { ICONS } from '../icons'
import { t } from '../i18n'
import { closeCompose, removeDraftFile, state, submitCompose, uploadFiles } from '../store'
import FileRow from './FileRow.vue'

const compose = state.compose
const isEdit = computed(() => compose.mode === 'edit')

const submitLabel = computed(() => {
  if (compose.uploading > 0) return t('messages.compose.uploading')
  return isEdit.value ? t('messages.compose.save') : t('messages.compose.send')
})
const submitDisabled = computed(() => compose.uploading > 0 || state.submitting)

const textRef = ref<HTMLTextAreaElement | null>(null)
const fileInput = ref<HTMLInputElement | null>(null)

watch(
  () => compose.open,
  async (open) => {
    if (!open) return
    await nextTick()
    textRef.value?.focus()
  },
)

function pickFiles(): void {
  fileInput.value?.click()
}

function onFiles(event: Event): void {
  const input = event.target as HTMLInputElement
  const files = [...(input.files || [])]
  input.value = ''
  void uploadFiles(files)
}
</script>

<template>
  <form class="msg-compose" id="msg-compose" @submit.prevent="submitCompose">
    <button class="link-back" id="msg-back" type="button" @click="closeCompose">{{ t('messages.compose.back') }}</button>
    <h2 class="msg-title" id="msg-compose-title">{{ isEdit ? t('messages.compose.editTitle') : t('messages.compose.title') }}</h2>

    <div class="field">
      <label class="field-label" for="msg-text">{{ t('messages.compose.body') }}</label>
      <textarea
        id="msg-text"
        ref="textRef"
        v-model="compose.text"
        rows="6"
        :placeholder="t('messages.compose.bodyPlaceholder')"
      ></textarea>
    </div>

    <div class="field">
      <div class="field-row">
        <span class="field-label">{{ t('messages.compose.attachments') }}</span>
        <button class="link-action" id="msg-attach" type="button" @click="pickFiles">
          <span class="icon" aria-hidden="true" v-html="ICONS.paperclip"></span>{{ t('messages.compose.addFile') }}
        </button>
      </div>
      <input id="msg-file-input" ref="fileInput" type="file" multiple hidden @change="onFiles" />
      <p class="msg-hint" id="msg-no-files" :hidden="compose.draftFiles.length > 0">{{ t('messages.compose.noFiles') }}</p>
      <ul class="file-list" id="msg-draft-files">
        <FileRow
          v-for="(file, index) in compose.draftFiles"
          :key="`${file.sha256}:${index}`"
          draft
          :attachment="file"
          @remove="removeDraftFile(index)"
        />
      </ul>
    </div>

    <p class="msg-hint msg-hint-bad" id="msg-compose-error" :hidden="!compose.error">{{ compose.error }}</p>
    <button class="btn btn-primary btn-block" id="msg-submit" type="submit" :disabled="submitDisabled">{{ submitLabel }}</button>
    <button class="btn btn-block" id="msg-cancel" type="button" :hidden="!isEdit" @click="closeCompose">
      {{ t('messages.compose.cancelEdit') }}
    </button>
  </form>
</template>
