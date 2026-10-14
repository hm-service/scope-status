<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { verifyToken } from './api'
import { clearToken, getToken, setToken } from './auth'
import { t } from './i18n'

const token = ref('')
const error = ref('')
const busy = ref(false)
const inputRef = ref<HTMLInputElement | null>(null)

function focus(): void {
  inputRef.value?.focus()
}

async function submit(): Promise<void> {
  const value = token.value.trim()
  if (!value) {
    focus()
    return
  }
  busy.value = true
  const failure = await verifyToken(value)
  busy.value = false
  if (failure) {
    error.value = t('auth.failed', { message: failure })
    focus()
    return
  }
  setToken(value)
  location.replace('/')
}

// 本地上次存的 token 还有效就直接进站；无效就清掉，让用户看着报错重填
onMounted(async () => {
  const stored = getToken()
  if (!stored) {
    focus()
    return
  }
  busy.value = true
  const failure = await verifyToken(stored)
  busy.value = false
  if (!failure) {
    location.replace('/')
    return
  }
  clearToken()
  token.value = stored
  error.value = t('auth.failed', { message: failure })
  focus()
})
</script>

<template>
  <div class="auth-shell">
    <form class="auth-card" id="auth-form" @submit.prevent="submit">
      <h1 class="auth-title">{{ t('app.title') }}</h1>
      <p class="auth-subtitle">{{ t('auth.subtitle') }}</p>
      <div class="auth-field">
        <label class="field-label" for="auth-token">{{ t('auth.label') }}</label>
        <input
          id="auth-token"
          ref="inputRef"
          v-model="token"
          type="password"
          autocomplete="off"
          :placeholder="t('auth.placeholder')"
        />
      </div>
      <p class="auth-error" id="auth-error" :hidden="!error">{{ error }}</p>
      <button class="btn btn-primary btn-block" id="auth-submit" type="submit" :disabled="busy">
        {{ busy ? t('auth.checking') : t('auth.submit') }}
      </button>
      <p class="auth-hint">{{ t('auth.hint') }}</p>
    </form>
  </div>
</template>
