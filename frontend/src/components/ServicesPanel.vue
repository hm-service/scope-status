<script setup lang="ts">
import { t, type I18nKey } from '../i18n'
import { services, servicesGroups, state } from '../store'
import GroupSection from './GroupSection.vue'
import ServiceRow from './ServiceRow.vue'

const serviceColumns: I18nKey[] = ['columns.name', 'columns.status', 'columns.latency', 'columns.command']
</script>

<template>
  <section class="panel" id="panel-services" :hidden="state.tab !== 'services'">
    <div class="groups" id="service-groups">
      <GroupSection
        v-for="group in servicesGroups"
        :key="group.id"
        :label="group.label || group.id"
        :online="group.online"
        :total="group.total"
        :columns="serviceColumns"
        list-class="service-list"
        :collapse-key="`service-group:${group.id}`"
      >
        <ServiceRow v-for="(service, index) in group.services || []" :key="`${service.name}:${index}`" :service="service" />
      </GroupSection>
    </div>
    <p class="empty" id="services-empty" :hidden="services.length > 0">{{ t('empty.services') }}</p>
  </section>
</template>
