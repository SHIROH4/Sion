<template>
  <div class="page-root">
    <div class="page-header">
      <div>
        <h2 class="page-title">策略实验室</h2>
        <p class="subtitle">猫娘从互动中自主学习的行为策略，活跃策略影响 LLM 兜底决策</p>
      </div>
    </div>

    <div v-if="strategies.length === 0" class="empty-text">还没有学到任何策略。多互动几天，猫娘会从经验中总结策略。</div>

    <template v-if="active.length">
      <div class="section-label">活跃策略 · {{ active.length }}</div>
      <div class="strategy-list">
        <n-card v-for="s in active" :key="s.id" size="small" :bordered="false" class="strategy-card">
          <div class="s-top">
            <span class="s-situation">{{ s.situation }}</span>
            <n-tag :type="s.confidence > 0.7 ? 'success' : s.confidence > 0.4 ? 'warning' : 'error'" size="tiny" round :bordered="false">{{ (s.confidence * 100).toFixed(0) }}%</n-tag>
            <n-tag size="tiny" :bordered="false" round>{{ sourceLabel(s.source) }}</n-tag>
          </div>
          <n-grid :cols="2" :x-gap="12" style="margin-top: 8px;">
            <n-gi>
              <div class="s-label g">好策略</div>
              <div class="s-body">{{ s.good_strategy }}</div>
            </n-gi>
            <n-gi v-if="s.bad_strategy">
              <div class="s-label b">坏策略</div>
              <div class="s-body">{{ s.bad_strategy }}</div>
            </n-gi>
          </n-grid>
          <div v-if="s.reason" class="s-reason">{{ s.reason }}</div>
        </n-card>
      </div>
    </template>

    <template v-if="inactive.length">
      <div class="section-label dimmed">已停用 · {{ inactive.length }}</div>
      <div class="strategy-list dimmed">
        <n-card v-for="s in inactive" :key="s.id" size="small" :bordered="false" class="strategy-card">
          <div class="s-top">
            <span class="s-situation">{{ s.situation }}</span>
            <n-tag :type="s.confidence > 0.7 ? 'success' : s.confidence > 0.4 ? 'warning' : 'error'" size="tiny" round :bordered="false">{{ (s.confidence * 100).toFixed(0) }}%</n-tag>
          </div>
          <div style="margin-top:8px;">
            <div class="s-label g">好策略</div>
            <div class="s-body">{{ s.good_strategy }}</div>
          </div>
        </n-card>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { NCard, NTag, NGrid, NGi } from 'naive-ui'
import { getStrategies, StrategyViewModel } from '../../stores/api'

const strategies = ref<StrategyViewModel[]>([])
onMounted(() => { getStrategies().then(s => strategies.value = s).catch(() => {}) })
const active = computed(() => strategies.value.filter(s => s.active))
const inactive = computed(() => strategies.value.filter(s => !s.active))
function sourceLabel(s: string) { const m: Record<string, string> = { daily_reflection: '日反思', immediate_feedback: '即时反馈' }; return m[s] || s }
</script>

<style scoped>
.page-root { height: 100%; overflow-y: auto; }
.page-header { margin-bottom: 20px; }
.page-title { font-size: 22px; font-weight: 700; margin: 0; color: #1a1a2e; }
.subtitle { font-size: 13px; color: #9ca3af; margin: 4px 0 0; }
.empty-text { color: #9ca3af; padding: 40px 0; }
.section-label { font-size: 14px; font-weight: 600; color: #374151; margin-bottom: 12px; }
.section-label.dimmed { color: #9ca3af; margin-top: 24px; }
.strategy-list { display: flex; flex-direction: column; gap: 12px; }
.strategy-list.dimmed { opacity: 0.55; }
.strategy-card :deep(.n-card__content) { padding-top: 4px; }
.s-top { display: flex; align-items: center; gap: 8px; }
.s-situation { font-size: 14px; font-weight: 600; flex: 1; }
.s-label { font-size: 11px; margin-bottom: 2px; }
.s-label.g { color: #10b981; }
.s-label.b { color: #ef4444; }
.s-body { font-size: 13px; color: #4b5563; }
.s-reason { font-size: 12px; color: #9ca3af; font-style: italic; margin-top: 8px; }
</style>
