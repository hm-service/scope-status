<script setup lang="ts">
import { ref } from 'vue'
import { errorMessage } from '../api'
import { t } from '../i18n'
import { showNotice } from '../store'

const props = defineProps<{ text: string; title: string }>()
const copied = ref(false)

/**
 * 内网多半是明文 http：那种页面上没有 navigator.clipboard（只在安全上下文存在），
 * 所以退回「临时 textarea + 选中 + execCommand」。两条都失败才提示。
 */
function fallbackCopy(text: string): boolean {
  const area = document.createElement('textarea')
  area.value = text
  area.setAttribute('readonly', '')
  area.style.position = 'fixed'
  area.style.opacity = '0'
  document.body.appendChild(area)
  area.select()
  try {
    return document.execCommand('copy')
  } catch {
    return false
  } finally {
    document.body.removeChild(area)
  }
}

/** 复制按钮：成功后短暂变成「已复制」，失败给出提示。 */
async function copy(): Promise<void> {
  let failure = ''
  try {
    if (navigator.clipboard) {
      await navigator.clipboard.writeText(props.text)
    } else if (!fallbackCopy(props.text)) {
      throw new Error('execCommand copy failed')
    }
  } catch (error) {
    if (fallbackCopy(props.text)) {
      failure = ''
    } else {
      failure = errorMessage(error)
    }
  }
  if (failure) {
    copied.value = false
    showNotice(t('notice.copyFailed', { message: failure }), 'bad')
    return
  }
  copied.value = true
  window.setTimeout(() => {
    copied.value = false
  }, 1500)
}
</script>

<template>
  <button type="button" class="btn btn-copy" :title="title" @click="copy">{{ copied ? t('actions.copied') : t('actions.copy') }}</button>
</template>
