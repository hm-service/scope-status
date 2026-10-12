<script setup lang="ts">
import { t, type I18nKey } from '../i18n'
import { friendRowsShown, groups, state } from '../store'
import FriendRow from './FriendRow.vue'
import GroupSection from './GroupSection.vue'

const friendColumns: I18nKey[] = ['columns.device', 'columns.status', 'columns.minLatency', 'columns.addressLatency']
</script>

<template>
  <section class="panel" id="panel-network" :hidden="state.tab !== 'network'">
    <div class="groups" id="groups">
      <GroupSection
        v-for="group in groups"
        :key="group.id"
        :label="group.label || group.id"
        :online="group.online"
        :total="group.total"
        :columns="friendColumns"
        list-class="friend-list"
        :collapse-key="`group:${group.id}`"
      >
        <FriendRow v-for="(friend, index) in group.friends" :key="`${friend.name}:${index}`" :friend="friend" />
      </GroupSection>
    </div>
    <p class="empty" id="empty" :hidden="friendRowsShown > 0">{{ t('empty.friends') }}</p>
  </section>
</template>
