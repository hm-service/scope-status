<script setup lang="ts">
import { t } from '../i18n'
import { activeSummary, durationText, messageSummary, state } from '../store'
</script>

<template>
  <section class="summary" id="summary" :hidden="state.tab === 'messages'" aria-live="polite">
    <span class="sum-item"><span class="sum-label">{{ t('summary.total') }}</span><b id="sum-total">{{ activeSummary.total }}</b></span>
    <span class="sum-item"><span class="sum-label">{{ t('summary.online') }}</span><b id="sum-online" class="is-up">{{ activeSummary.online }}</b></span>
    <span class="sum-item"><span class="sum-label">{{ t('summary.offline') }}</span><b id="sum-offline" class="is-down">{{ activeSummary.offline }}</b></span>
    <span class="sum-time">{{ t('summary.duration') }} <b id="sum-duration">{{ durationText }}</b></span>
  </section>

  <section class="summary" id="summary-messages" :hidden="state.tab !== 'messages'" aria-live="polite">
    <span class="sum-item"><span class="sum-label">{{ t('summary.messages.total') }}</span><b id="sum-messages-total">{{ messageSummary.total }}</b></span>
    <span class="sum-item"><span class="sum-label">{{ t('summary.messages.locked') }}</span><b id="sum-messages-locked">{{ messageSummary.locked }}</b></span>
    <span class="sum-item"><span class="sum-label">{{ t('summary.messages.latest') }}</span><b id="sum-messages-latest" class="sum-stamp">{{ messageSummary.latest }}</b></span>
  </section>
</template>
