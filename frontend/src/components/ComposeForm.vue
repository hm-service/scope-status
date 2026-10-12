<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { ICONS } from '../icons'
import { t } from '../i18n'
import { closeCompose, removeDraftFile, state, submitCompose, uploadFiles } from '../store'
import FileRow from './FileRow.vue'

const compose = state.compose
const isEdit = computed(() => compose.mode === 'edit')
/** 选「密钥消息」才露出密钥输入框；编辑时类型不可改。 */
const keyFieldVisible = computed(() => compose.kind === 'key' && !isEdit.value)

const submitLabel = computed(() => {
  if (compose.uploading > 0) return t('messages.compose.uploading')
  if (isEdit.value) return t('messages.compose.save')
  return compose.kind === 'key' ? t('messages.compose.sendKey') : t('messages.compose.sendPublic')
})
const submitDisabled = computed(() => compose.uploading > 0 || state.submitting)

const textRef = ref<HTMLTextAreaElement | null>(null)
const keyRef = ref<HTMLInputElement | null>(null)
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

function toggleKeyVisible(): void {
  compose.keyVisible = !compose.keyVisible
  keyRef.value?.focus()
}
</script>

<template>
  <form class="msg-compose" id="msg-compose" @submit.prevent="submitCompose">
    <button class="link-back" id="msg-back" type="button" @click="closeCompose">{{ t('messages.compose.back') }}</button>
    <h2 class="msg-title" id="msg-compose-title">{{ isEdit ? t('messages.compose.editTitle') : t('messages.compose.title') }}</h2>

    <div class="field" id="msg-kind-field" :hidden="isEdit">
      <span class="field-label">{{ t('messages.compose.visibility') }}</span>
      <label class="radio-card">
        <input type="radio" name="msg-kind" value="public" v-model="compose.kind" />
        <span class="radio-text">
          <span class="radio-title">{{ t('messages.compose.publicTitle') }}</span>
          <span class="radio-note">{{ t('messages.compose.publicNote') }}</span>
        </span>
      </label>
      <label class="radio-card">
        <input type="radio" name="msg-kind" value="key" v-model="compose.kind" />
        <span class="radio-text">
          <span class="radio-title">{{ t('messages.compose.keyTitle') }}</span>
          <span class="radio-note">{{ t('messages.compose.keyNote') }}</span>
        </span>
      </label>
    </div>

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

    <div class="field" id="msg-key-field" :hidden="!keyFieldVisible">
      <label class="field-label field-label-strong" for="msg-key">{{ t('messages.compose.keyLabel') }}</label>
      <div class="input-with-action">
        <input
          id="msg-key"
          ref="keyRef"
          v-model="compose.keyInput"
          :type="compose.keyVisible ? 'text' : 'password'"
          :placeholder="t('messages.compose.keyPlaceholder')"
          autocomplete="new-password"
        />
        <button
          class="input-action"
          id="msg-key-eye"
          type="button"
          :aria-label="t('messages.compose.keyToggle')"
          :title="t('messages.compose.keyToggle')"
          @click="toggleKeyVisible"
        >
          <span class="icon" aria-hidden="true" v-html="compose.keyVisible ? ICONS.eyeOff : ICONS.eye"></span>
        </button>
      </div>
      <p class="msg-hint">{{ t('messages.compose.keyHint') }}</p>
    </div>

    <p class="msg-hint msg-hint-bad" id="msg-compose-error" :hidden="!compose.error">{{ compose.error }}</p>
    <button class="btn btn-primary btn-block" id="msg-submit" type="submit" :disabled="submitDisabled">{{ submitLabel }}</button>
    <button class="btn btn-block" id="msg-cancel" type="button" :hidden="!isEdit" @click="closeCompose">
      {{ t('messages.compose.cancelEdit') }}
    </button>
  </form>
</template>
