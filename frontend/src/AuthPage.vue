<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { errorMessage, login } from './api'
import { getSession, setSession } from './auth'
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
  try {
    const session = await login(value)
    if (!session) {
      error.value = t('auth.noSession')
      return
    }
    if (!setSession(session)) {
      error.value = t('auth.noStorage')
      return
    }
    // 原始 token 不留在浏览器里：换成会话之后立刻清掉输入框
    token.value = ''
    location.replace('/')
  } catch (failure) {
    error.value = t('auth.failed', { message: errorMessage(failure) })
    focus()
  } finally {
    busy.value = false
  }
}

// 手上已经有会话就直接回主站；会话失效时主站会 401 并把我们送回来，那时再显示表单
onMounted(() => {
  if (getSession()) {
    location.replace('/')
    return
  }
  focus()
})
</script>

<template>
  <div class="auth-shell">
    <form class="auth-card" id="auth-form" @submit.prevent="submit">
      <h1 class="auth-title">{{ t('app.title') }}</h1>
      <div class="auth-field">
        <input
          id="auth-token"
          ref="inputRef"
          v-model="token"
          type="password"
          autocomplete="off"
          :aria-label="t('auth.placeholder')"
          :placeholder="t('auth.placeholder')"
        />
      </div>
      <p class="auth-error" id="auth-error" :hidden="!error">{{ error }}</p>
      <button class="btn btn-primary btn-block" id="auth-submit" type="submit" :disabled="busy">
        {{ busy ? t('auth.checking') : t('auth.submit') }}
      </button>
    </form>
  </div>
</template>
