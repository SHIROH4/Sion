import { create } from 'zustand'

export interface SettingsStore {
  currentPage: string
  pluginConfigName: string | null
  navigate: (page: string) => void
  openPluginConfig: (name: string) => void
}

export const useSettingsStore = create<SettingsStore>((set) => ({
  currentPage: 'dashboard',
  pluginConfigName: null,
  navigate: (page) => set({ currentPage: page, pluginConfigName: null }),
  openPluginConfig: (name) => set({ currentPage: 'plugin-config', pluginConfigName: name }),
}))
