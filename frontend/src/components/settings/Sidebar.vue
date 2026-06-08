<template>
  <n-layout-sider
    bordered
    :width="240"
    :native-scrollbar="false"
    class="sider"
    collapse-mode="width"
  >
    <!-- Brand -->
    <div class="brand">
      <div class="brand-icon">
        <n-icon size="28" color="#fff"><PawOutline /></n-icon>
      </div>
      <div class="brand-text">
        <div class="brand-name">诗音</div>
        <div class="brand-ver">Desktop Pet v0.5</div>
      </div>
    </div>

    <!-- Menu -->
    <n-menu
      :value="settings.currentPage"
      :options="menuOptions"
      @update:value="settings.navigate($event)"
    />

    <!-- Model Selector -->
    <div class="model-area" v-if="models.length > 1">
      <div class="model-label">Live2D 模型</div>
      <n-select
        v-model:value="selectedModel"
        :options="modelOptions"
        size="small"
        @update:value="onModelChange"
      />
    </div>

    <!-- Open Pet button -->
    <div class="sider-footer">
      <n-button type="primary" block @click="handleOpenPet" secondary>
        <template #icon><n-icon><PlayOutline /></n-icon></template>
        打开宠物
      </n-button>
    </div>
  </n-layout-sider>
</template>

<script setup lang="ts">
import { ref, onMounted, h, computed } from 'vue'
import { NLayoutSider, NMenu, NSelect, NButton, NIcon } from 'naive-ui'
import {
  SpeedometerOutline, ChatbubblesOutline, BookOutline, GlobeOutline,
  HammerOutline, SettingsOutline, AppsOutline, PulseOutline,
  PawOutline, PlayOutline, BulbOutline, FileTrayFullOutline,
} from '@vicons/ionicons5'
import { useSettingsStore } from '../../stores/settingsStore'
import { usePetStore } from '../../stores/petStore'
import { getModels, ModelInfo } from '../../stores/api'
import type { MenuOption } from 'naive-ui'

const settings = useSettingsStore()
const pet = usePetStore()

const icon = (icon: any) => () => h(NIcon, null, { default: () => h(icon) })

const menuOptions: MenuOption[] = [
  { label: '仪表盘', key: 'dashboard', icon: icon(SpeedometerOutline) },
  { label: '执行状态', key: 'observability', icon: icon(PulseOutline) },
  { label: '对话面板', key: 'chat', icon: icon(ChatbubblesOutline) },
  { label: '日记', key: 'diary', icon: icon(BookOutline) },
  { label: '记忆图谱', key: 'memory', icon: icon(GlobeOutline) },
  { label: '策略实验室', key: 'strategy-lab', icon: icon(BulbOutline) },
  { label: 'API 配置', key: 'api', icon: icon(SettingsOutline) },
  { label: '插件', key: 'plugins', icon: icon(AppsOutline) },
]

const models = ref<ModelInfo[]>([])
const selectedModel = ref(pet.modelPath)

const modelOptions = computed(() =>
  models.value.map(m => ({ label: m.name, value: m.path }))
)

onMounted(async () => {
  try { models.value = await getModels() } catch {}
})

function onModelChange(path: string) {
  pet.setModelPath(path)
  setTimeout(() => window.location.reload(), 200)
}

function handleOpenPet() {
  try { (window as any).go?.main?.SettingsApp?.OpenPet() } catch {}
}
</script>

<style scoped>
.sider {
  height: 100vh;
  display: flex;
  flex-direction: column;
}
.brand {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 24px 20px 20px;
  border-bottom: 1px solid rgba(0,0,0,0.06);
}
.brand-icon {
  width: 40px; height: 40px;
  border-radius: 10px;
  background: linear-gradient(135deg, #4f6ef7, #6b85f9);
  display: flex; align-items: center; justify-content: center;
}
.brand-name {
  font-size: 18px; font-weight: 700;
  color: #1a1a2e; line-height: 1.2;
}
.brand-ver {
  font-size: 11px; color: #9ca3af;
}
.model-area {
  padding: 12px 20px;
  border-top: 1px solid rgba(0,0,0,0.06);
}
.model-label {
  font-size: 11px; color: #9ca3af;
  margin-bottom: 6px; text-transform: uppercase; letter-spacing: 0.5px;
}
.sider-footer {
  padding: 12px 16px 20px;
  border-top: 1px solid rgba(0,0,0,0.06);
  margin-top: auto;
}
</style>
