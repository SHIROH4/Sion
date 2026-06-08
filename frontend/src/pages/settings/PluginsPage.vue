<template>
  <div class="page-root">
    <div class="page-header">
      <h2 class="page-title">插件中心</h2>
      <n-button size="small" @click="loadPlugins" secondary>刷新</n-button>
    </div>

    <n-spin v-if="loading" style="display:flex;justify-content:center;padding:60px;" />
    <n-text v-else-if="error" type="error">{{ error }}</n-text>

    <n-grid v-else :cols="2" :x-gap="16" :y-gap="16">
      <n-gi v-for="p in plugins" :key="p.name">
        <n-card :bordered="false" size="small" class="plugin-card">
          <template #header>
            <div class="p-hd">
              <span class="p-dot" :class="{ on: p.running }" />
              <span class="p-name">{{ p.name }}</span>
              <n-tag size="tiny" :bordered="false" round>v{{ p.version }}</n-tag>
              <span class="p-status">{{ p.running ? '运行中' : '已停止' }}</span>
            </div>
          </template>
          <p class="p-desc">{{ p.description }}</p>
          <p v-if="p.requires?.length" class="p-requires">依赖: {{ p.requires.join(', ') }}</p>
          <n-space style="margin-top:12px;">
            <n-button size="small" @click="settings.openPluginConfig(p.name)" secondary>配置</n-button>
            <n-button
              v-if="p.name !== 'chat'"
              size="small"
              :type="p.running ? 'error' : 'primary'"
              @click="handleToggle(p)"
              :loading="toggling === p.name"
              secondary
            >{{ p.running ? '停止' : '启动' }}</n-button>
          </n-space>
        </n-card>
      </n-gi>
    </n-grid>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { NCard, NGrid, NGi, NTag, NButton, NSpace, NSpin, NText } from 'naive-ui'
import { getPlugins, togglePlugin, PluginInfo } from '../../stores/api'
import { useSettingsStore } from '../../stores/settingsStore'

const settings = useSettingsStore()
const plugins = ref<PluginInfo[]>([])
const loading = ref(true)
const error = ref<string | null>(null)
const toggling = ref<string | null>(null)

async function loadPlugins() {
  try { plugins.value = await getPlugins(); error.value = null }
  catch (e: any) { error.value = e?.message || '加载失败' }
  finally { loading.value = false }
}
onMounted(loadPlugins)

async function handleToggle(p: PluginInfo) {
  toggling.value = p.name
  try { await togglePlugin(p.name); await loadPlugins() }
  catch (e: any) { console.warn('Toggle failed:', e) }
  finally { toggling.value = null }
}
</script>

<style scoped>
.page-root { height: 100%; overflow-y: auto; }
.page-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 24px; }
.page-title { font-size: 22px; font-weight: 700; margin: 0; color: #1a1a2e; }
.plugin-card :deep(.n-card__content) { padding-top: 4px; }
.p-hd { display: flex; align-items: center; gap: 8px; width: 100%; }
.p-dot { width: 8px; height: 8px; border-radius: 50%; background: #d1d5db; flex-shrink: 0; }
.p-dot.on { background: #10b981; }
.p-name { font-size: 14px; font-weight: 600; flex: 1; }
.p-status { font-size: 12px; color: #9ca3af; }
.p-desc { font-size: 13px; color: #4b5563; line-height: 1.5; margin: 0; }
.p-requires { font-size: 11px; color: #9ca3af; margin: 4px 0 0; }
</style>
