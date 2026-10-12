<script setup lang="ts">
import { t } from '../i18n'
import { links, linksGroupCount, state } from '../store'
import LinkRow from './LinkRow.vue'
</script>

<template>
  <section class="panel" id="panel-links" :hidden="state.tab !== 'links'">
    <section class="group">
      <header class="group-head">
        <span class="group-chevron">▾</span>
        <span class="group-title">{{ t('tabs.links') }}</span>
        <span class="group-count" id="links-count">{{ linksGroupCount }}</span>
      </header>
      <div class="table">
        <div class="thead">
          <span>{{ t('columns.name') }}</span>
          <span>{{ t('columns.status') }}</span>
          <span>{{ t('columns.latency') }}</span>
          <span>{{ t('columns.address') }}</span>
        </div>
        <ul class="rows" id="links">
          <LinkRow v-for="(link, index) in links" :key="`${link.name}:${index}`" :link="link" />
        </ul>
      </div>
      <p class="empty" id="links-empty" :hidden="links.length > 0">{{ t('empty.links') }}</p>
    </section>
  </section>
</template>
