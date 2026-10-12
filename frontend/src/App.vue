<script setup lang="ts">
import LinksPanel from './components/LinksPanel.vue'
import MessagesPanel from './components/MessagesPanel.vue'
import NetworkPanel from './components/NetworkPanel.vue'
import ServicesPanel from './components/ServicesPanel.vue'
import SummaryBars from './components/SummaryBars.vue'
import TabsBar from './components/TabsBar.vue'
import TopBar from './components/TopBar.vue'
import { footerVisible, state } from './store'
</script>

<template>
  <div class="shell">
    <TopBar />
    <SummaryBars />
    <TabsBar />
    <main>
      <NetworkPanel />
      <LinksPanel />
      <ServicesPanel />
      <MessagesPanel />
    </main>
    <footer class="footer" id="footer" :hidden="!footerVisible">
      <span id="error" class="error" :hidden="!state.error">{{ state.error }}</span>
      <span id="notice" class="notice" :class="state.notice?.tone" :hidden="!state.notice">{{ state.notice?.message }}</span>
    </footer>
  </div>
</template>
