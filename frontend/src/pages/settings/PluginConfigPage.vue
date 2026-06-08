<template>
  <div class="page" v-if="pluginConfigName">
    <div class="top-bar">
      <n-button text @click="settings.navigate('plugins')"><template #icon><n-icon><ArrowBackOutline /></n-icon></template>返回</n-button>
      <span class="title">{{ pluginConfigName }} 配置</span>
    </div>

    <n-spin v-if="loading" style="display:flex;justify-content:center;padding:60px;" />

    <div v-else-if="config" class="config-area">
      <!-- Boolean toggles -->
      <n-card v-if="boolEntries.length" :bordered="false" size="small" style="margin-bottom:20px;">
        <div v-for="[key, value] in boolEntries" :key="key" class="bool-row">
          <span class="bool-label">{{ humanLabel(key) }}</span>
          <n-switch :value="!!value" @update:value="onFieldUpdate(key, $event)" />
        </div>
      </n-card>

      <!-- Group sections -->
      <n-card v-for="[key, val] in groupEntries" :key="key" :bordered="false" size="small" style="margin-bottom:20px;" :title="humanLabel(key)">
        <n-form label-placement="left" label-width="140" :show-feedback="false">
          <template v-for="[k, v] in fieldEntries(val as Record<string, unknown>)" :key="k">
            <n-form-item :label="humanLabel(String(k))">
              <n-switch v-if="typeof v === 'boolean'" :value="!!v" @update:value="updateNested(String(key), String(k), $event)" />
              <n-slider
                v-else-if="typeof v === 'number' && v >= 0 && v <= 1"
                :value="v as number" :min="0" :max="1" :step="0.05"
                @update:value="updateNested(String(key), String(k), $event)"
                style="max-width:260px;"
              />
              <n-input-number
                v-else-if="typeof v === 'number'"
                :value="v as number" :min="0"
                @update:value="updateNested(String(key), String(k), $event)"
                size="small"
              />
              <n-input
                v-else-if="typeof v === 'string'"
                :value="v as string"
                :type="isPasswordKey(String(k)) ? 'password' : 'text'"
                show-password-on="click"
                @update:value="updateNested(String(key), String(k), $event)"
              />
            </n-form-item>
          </template>
        </n-form>
      </n-card>
    </div>

    <n-space justify="end" v-if="!loading">
      <n-button type="primary" @click="handleSave" :loading="saving">保存配置</n-button>
    </n-space>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { NCard, NForm, NFormItem, NInput, NInputNumber, NSlider, NSwitch, NButton, NSpace, NSpin, NIcon, useMessage } from 'naive-ui'
import { ArrowBackOutline } from '@vicons/ionicons5'
import { useSettingsStore } from '../../stores/settingsStore'

type ConfigValue = boolean | number | string | Record<string, unknown>
interface ConfigObject { [key: string]: ConfigValue }

const API_BASE = 'http://127.0.0.1:19840'
const settings = useSettingsStore()
const pluginConfigName = settings.pluginConfigName
const message = useMessage()

const config = ref<ConfigObject | null>(null)
const loading = ref(true)
const saving = ref(false)

const entries = computed(() => config.value ? Object.entries(config.value) : [])
const boolEntries = computed(() => entries.value.filter(([, v]) => typeof v === 'boolean'))
const groupEntries = computed(() => entries.value.filter(([, v]) => typeof v === 'object' && v !== null))

function fieldEntries(obj: Record<string, unknown>) { return Object.entries(obj) }

function onFieldUpdate(key: string, value: unknown) {
  if (!config.value) return
  config.value = { ...config.value, [key]: value as ConfigValue }
}

function updateNested(groupKey: string, fieldKey: string, value: unknown) {
  if (!config.value) return
  const group = config.value[groupKey] as Record<string, unknown> | undefined
  config.value = { ...config.value, [groupKey]: { ...group, [fieldKey]: value } as ConfigValue }
}

function humanLabel(key: string): string {
  const map: Record<string, string> = {
    enabled: '启用', l0_threshold: 'L0 压缩阈值', merge_enabled: '合并开关',
    merge_similarity: '合并相似度阈值', qa_max_per_day: '每日提问上限',
    qa_max_pending: '待处理上限', history_days: '历史保留天数',
    diary_days: '日记保留天数', decay_threshold: '衰减阈值',
    interval_days: '更新间隔 (天)', interval_sec: '循环间隔 (秒)',
    interval_min: '间隔 (分钟)', screen_cool_down_sec: '截图冷却 (秒)',
    ocr_enabled: '本地 OCR', cloud_enabled: '云端分析',
    app_id: 'App ID', app_secret: 'App Secret', cooldown_min: '冷却 (分钟)',
    lunch_hour: '午餐整点', dinner_hour: '晚餐整点',
    continuous_work_min: '连续工作阈值 (分钟)', break_min: '建议休息 (分钟)',
    encourage: '鼓励', escalation: '升级提醒', max_daily: '每日上限',
    emotion_weight: '情绪权重', context_weight: '上下文权重',
    loneliness_threshold: '孤独触发阈值', worry_threshold: '担忧触发阈值',
    curiosity_threshold: '好奇触发阈值', dedup_window_min: '去重窗口 (分钟)',
    emotion_cloud_enabled: '云端情绪评估',
  }
  return map[key] ?? key
}

function isPasswordKey(key: string): boolean { return /secret|key|password/i.test(key) }

onMounted(async () => {
  if (!pluginConfigName) { settings.navigate('plugins'); return }
  try {
    const res = await fetch(`${API_BASE}/api/plugins/${pluginConfigName}`)
    if (!res.ok) throw new Error(`HTTP ${res.status}`)
    config.value = await res.json()
  } catch (e: any) { message.error('加载失败: ' + (e?.message || e)) }
  finally { loading.value = false }
})

async function handleSave() {
  if (!pluginConfigName || !config.value) return
  saving.value = true
  try {
    const res = await fetch(`${API_BASE}/api/plugins/${pluginConfigName}`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(config.value),
    })
    if (!res.ok) throw new Error(`HTTP ${res.status}`)
    message.success('配置已保存')
  } catch (e: any) { message.error('保存失败: ' + (e?.message || e)) }
  finally { saving.value = false }
}
</script>

<style scoped>
.page { max-width: 680px; height: 100%; overflow-y: auto; }
.top-bar { display: flex; align-items: center; gap: 8px; margin-bottom: 24px; }
.title { font-size: 20px; font-weight: 700; color: #1a1a2e; }
.bool-row { display: flex; align-items: center; justify-content: space-between; padding: 6px 0; }
.bool-label { font-size: 13px; color: #4b5563; }
.config-area :deep(.n-card__content) { padding-top: 4px; }
</style>
