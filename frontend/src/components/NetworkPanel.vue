<script setup lang="ts">
import { t, type I18nKey } from '../i18n'
import { peerRowsShown, groups, state } from '../store'
import PeerRow from './PeerRow.vue'
import TopologyGraph from './TopologyGraph.vue'
import GroupSection from './GroupSection.vue'

const peerColumns: I18nKey[] = ['columns.device', 'columns.status', 'columns.minLatency', 'columns.addressLatency']
</script>

<template>
  <section class="panel" id="panel-network" :hidden="state.tab !== 'network'">
    <TopologyGraph />
    <div class="groups" id="groups">
      <GroupSection
        v-for="group in groups"
        :key="group.id"
        :label="group.label || group.id"
        :online="group.online"
        :total="group.total"
        :columns="peerColumns"
        list-class="peer-list"
        :collapse-key="`group:${group.id}`"
      >
        <PeerRow v-for="(peer, index) in group.peers" :key="`${peer.name}:${index}`" :peer="peer" />
      </GroupSection>
    </div>
    <p class="empty" id="empty" :hidden="peerRowsShown > 0">{{ t('empty.peers') }}</p>
  </section>
</template>
