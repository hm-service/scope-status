<script setup lang="ts">
import { t } from '../i18n'
import { linksTabCount, servicesTabCount, state, switchTab, TABS } from '../store'
</script>

<template>
  <nav class="tabs" role="tablist">
    <button
      v-for="tab in TABS"
      :key="tab.id"
      type="button"
      class="tab"
      :class="{ 'is-active': tab.id === state.tab }"
      :data-tab="tab.id"
      role="tab"
      :aria-selected="tab.id === state.tab"
      @click="switchTab(tab.id)"
    >
      {{ tab.id === 'messages' && state.unread > 0 ? t('tabs.messagesUnread', { count: state.unread }) : t(tab.labelKey) }}
      <span v-if="tab.id === 'services'" class="tab-count" id="tab-services-count">{{ servicesTabCount }}</span>
      <span v-else-if="tab.id === 'links'" class="tab-count" id="tab-links-count">{{ linksTabCount }}</span>
    </button>
  </nav>
</template>
