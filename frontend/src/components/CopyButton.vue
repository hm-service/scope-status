<script setup lang="ts">
import { ref } from 'vue'
import { errorMessage } from '../api'
import { t } from '../i18n'
import { showNotice } from '../store'

const props = defineProps<{ text: string; title: string }>()
const copied = ref(false)

/** 复制按钮：成功后短暂变成「已复制」，失败给出提示（http 下没有 clipboard 也要说一声）。 */
async function copy(): Promise<void> {
  try {
    await navigator.clipboard.writeText(props.text)
    copied.value = true
    window.setTimeout(() => {
      copied.value = false
    }, 1500)
  } catch (error) {
    copied.value = false
    showNotice(t('notice.copyFailed', { message: errorMessage(error) }), 'bad')
  }
}
</script>

<template>
  <button type="button" class="btn btn-copy" :title="title" @click="copy">{{ copied ? t('actions.copied') : t('actions.copy') }}</button>
</template>
