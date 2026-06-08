<template>
  <NConfigProvider :theme-overrides="themeOverrides" :locale="zhCN" :date-locale="dateZhCN">
    <NMessageProvider>
      <SettingsLayout>
        <component :is="currentComponent" />
      </SettingsLayout>
    </NMessageProvider>
  </NConfigProvider>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { NConfigProvider, NMessageProvider, zhCN, dateZhCN, type GlobalThemeOverrides } from 'naive-ui'
import { useSettingsStore } from './stores/settingsStore'
import SettingsLayout from './components/settings/SettingsLayout.vue'
import DashboardPage from './pages/settings/DashboardPage.vue'
import ObservabilityPage from './pages/settings/ObservabilityPage.vue'
import ChatPage from './pages/settings/ChatPage.vue'
import DiaryPage from './pages/settings/DiaryPage.vue'
import MemoryIdentityPage from './pages/settings/MemoryIdentityPage.vue'
import StrategyLabPage from './pages/settings/StrategyLabPage.vue'
import ApiConfigPage from './pages/settings/ApiConfigPage.vue'
import PluginsPage from './pages/settings/PluginsPage.vue'
import PluginConfigPage from './pages/settings/PluginConfigPage.vue'

const pages: Record<string, any> = {
  dashboard: DashboardPage,
  observability: ObservabilityPage,
  chat: ChatPage,
  diary: DiaryPage,
  memory: MemoryIdentityPage,
  'strategy-lab': StrategyLabPage,
  api: ApiConfigPage,
  plugins: PluginsPage,
  'plugin-config': PluginConfigPage,
}

const settings = useSettingsStore()
const currentComponent = computed(() => pages[settings.currentPage] || DashboardPage)

const themeOverrides: GlobalThemeOverrides = {
  common: {
    primaryColor: '#4f6ef7',
    primaryColorHover: '#6b85f9',
    primaryColorPressed: '#3b54d4',
    borderRadius: '8px',
    fontSizeSmall: '12px',
    fontSizeMedium: '14px',
    fontSizeLarge: '16px',
  },
  Layout: {
    siderColor: '#e8f4fd',
    headerColor: '#fff',
    footerColor: '#f5f7fa',
  },
  Menu: {
    itemColor: 'transparent',
    itemTextColor: '#4b5563',
    itemColorHover: 'rgba(79,110,247,0.06)',
    itemTextColorHover: '#1a1a2e',
    itemColorActive: 'rgba(79,110,247,0.1)',
    itemTextColorActive: '#4f6ef7',
    itemIconColorActive: '#4f6ef7',
    arrowColor: '#9ca3af',
    itemIconColor: '#9ca3af',
    itemIconColorHover: '#4b5563',
  },
  Card: {
    borderRadius: '12px',
    paddingSmall: '12px',
    paddingMedium: '16px',
    paddingLarge: '24px',
  },
}
</script>
