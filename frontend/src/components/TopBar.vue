<script setup lang="ts">
import { computed } from 'vue'
import { THEME_ICONS } from '../icons'
import { t } from '../i18n'
import {
  closeMenu,
  refreshStatus,
  reloadConfig,
  state,
  streamClass,
  streamText,
  streamTitle,
  subtitle,
  title,
  toggleLang,
  toggleMenu,
  toggleTheme,
  updatedText,
} from '../store'

const themeTitle = computed(() => (state.theme === 'light' ? t('actions.toDark') : t('actions.toLight')))
</script>

<template>
  <header class="topbar">
    <div class="brand">
      <h1 id="title">{{ title }}</h1>
      <p class="meta">
        <span id="subtitle">{{ subtitle }}</span>
        <span class="sep">·</span>
        <span class="meta-right">
          <span id="updated">{{ updatedText }}</span>
          <span class="sep">·</span>
          <span id="stream" :class="streamClass" :title="streamTitle || undefined">{{ streamText }}</span>
        </span>
      </p>
    </div>
    <div class="actions" id="actions">
      <button
        id="menu"
        type="button"
        class="btn btn-icon"
        :title="t('actions.more')"
        :aria-label="t('actions.moreLabel')"
        aria-haspopup="true"
        :aria-expanded="state.menuOpen"
        @click.stop="toggleMenu"
      >⋯</button>
      <button id="refresh" type="button" class="btn" :title="t('actions.refreshTitle')" :disabled="state.refreshing" @click="refreshStatus">
        {{ state.refreshing ? t('actions.refreshing') : t('actions.refresh') }}
      </button>
      <button id="reload" type="button" class="btn" :title="t('actions.reloadTitle')" :disabled="state.reloading" @click="reloadConfig">
        {{ state.reloading ? t('actions.reloading') : t('actions.reload') }}
      </button>
      <button id="lang" type="button" class="btn" :title="t('actions.langTitle')" @click="toggleLang">{{ t('actions.lang') }}</button>
      <button id="theme" type="button" class="btn btn-icon" :title="themeTitle" v-html="THEME_ICONS[state.theme]" @click="toggleTheme"></button>
      <div class="menu" id="menu-panel" :hidden="!state.menuOpen">
        <button type="button" class="menu-item" @click="closeMenu(); refreshStatus()">{{ t('actions.refresh') }}</button>
        <button type="button" class="menu-item" @click="closeMenu(); reloadConfig()">{{ t('actions.reload') }}</button>
        <button type="button" class="menu-item" @click="closeMenu(); toggleLang()">{{ t('actions.langTitle') }}</button>
        <button type="button" class="menu-item" @click="closeMenu(); toggleTheme()">{{ t('actions.theme') }}</button>
      </div>
    </div>
  </header>
</template>
