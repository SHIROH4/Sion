import { defineStore } from 'pinia'
import { ref } from 'vue'

export const useSettingsStore = defineStore('settings', () => {
  const currentPage = ref('dashboard')
  const pluginConfigName = ref<string | null>(null)

  function navigate(page: string) {
    currentPage.value = page
    pluginConfigName.value = null
  }
  function openPluginConfig(name: string) {
    currentPage.value = 'plugin-config'
    pluginConfigName.value = name
  }

  return { currentPage, pluginConfigName, navigate, openPluginConfig }
})
