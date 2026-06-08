<template>
  <div class="page-root">
    <div class="page-header">
      <h2 class="page-title">仪表盘</h2>
      <n-tag type="info" size="small" round :bordered="false">实时</n-tag>
    </div>

    <n-tabs v-model:value="tab" type="segment" animated>
      <n-tab-pane name="overview" tab="概览" />
      <n-tab-pane name="emotion" tab="情绪" />
      <n-tab-pane name="decision" tab="决策" />
    </n-tabs>

    <n-skeleton v-if="!features" text :repeat="4" style="margin-top: 24px;" />

    <!-- Overview -->
    <template v-if="tab === 'overview' && features">
      <n-grid :cols="4" :x-gap="16" :y-gap="16" style="margin-top: 20px;">
        <n-gi>
          <n-card size="small" :bordered="false" class="stat-card">
            <n-statistic label="接受率" :value="features.relationship.overall_accept_rate > 0 ? (features.relationship.overall_accept_rate * 100).toFixed(0) : '--'" />
            <template #header-extra><n-icon size="20" color="#4f6ef7"><TrendingUpOutline /></n-icon></template>
          </n-card>
        </n-gi>
        <n-gi>
          <n-card size="small" :bordered="false" class="stat-card">
            <n-statistic label="连续工作">
              {{ features.user.continuous_work_min > 0 ? features.user.continuous_work_min.toFixed(0) + ' 分钟' : '未工作' }}
            </n-statistic>
            <template #header-extra><n-icon size="20" color="#f59e0b"><DesktopOutline /></n-icon></template>
          </n-card>
        </n-gi>
        <n-gi>
          <n-card size="small" :bordered="false" class="stat-card">
            <n-statistic label="今日行动">
              {{ learning?.metrics.total_today ?? '--' }} 次
            </n-statistic>
            <template #header-extra><n-icon size="20" color="#10b981"><FlashOutline /></n-icon></template>
          </n-card>
        </n-gi>
        <n-gi>
          <n-card size="small" :bordered="false" class="stat-card">
            <n-statistic label="最近拒绝">
              {{ features.relationship.recent_rejections }} 次
            </n-statistic>
            <template #header-extra><n-icon size="20" color="#ef4444"><AlertCircleOutline /></n-icon></template>
          </n-card>
        </n-gi>
      </n-grid>

      <n-grid :cols="2" :x-gap="20" style="margin-top: 20px;">
        <n-gi>
          <n-card title="5 维驱力" :bordered="false" size="small">
            <div class="drive-bars">
              <div v-for="d in driveBars" :key="d.label" class="drive-row">
                <span class="drive-label">{{ d.label }}</span>
                <n-progress
                  :percentage="Math.round(d.value * 100)"
                  :color="d.color"
                  :height="8"
                  :border-radius="4"
                  :show-indicator="false"
                  processing
                />
                <span class="drive-val">{{ (d.value * 100).toFixed(0) }}%</span>
              </div>
            </div>
          </n-card>
        </n-gi>
        <n-gi>
          <n-card title="6 维内源需求" :bordered="false" size="small">
            <div class="drive-bars">
              <div v-for="n in needBars" :key="n.label" class="drive-row">
                <span class="drive-label">{{ n.label }}</span>
                <n-progress
                  :percentage="Math.round(n.value * 100)"
                  :color="n.color"
                  :height="8"
                  :border-radius="4"
                  :show-indicator="false"
                  processing
                />
                <span class="drive-val">{{ (n.value * 100).toFixed(0) }}%</span>
              </div>
            </div>
          </n-card>
        </n-gi>
      </n-grid>

      <n-card title="互动关系" :bordered="false" size="small" style="margin-top: 20px;">
        <n-grid :cols="3" :x-gap="16">
          <n-gi><div class="rel-item"><span class="rel-label">冷落时长</span><span class="rel-value">{{ (features.relationship.neglect_hours * 60).toFixed(0) }} 分钟</span></div></n-gi>
          <n-gi><div class="rel-item"><span class="rel-label">消息趋势</span><span class="rel-value">{{ features.user.length_trend < -0.2 ? '变短 ↓' : features.user.length_trend > 0.2 ? '变长 ↑' : '稳定' }}</span></div></n-gi>
          <n-gi><div class="rel-item"><span class="rel-label">亲密度趋势</span><span class="rel-value">{{ features.relationship.intimacy_trend < -0.1 ? '降温 ↓' : features.relationship.intimacy_trend > 0.1 ? '升温 ↑' : '稳定' }}</span></div></n-gi>
        </n-grid>
      </n-card>
    </template>

    <!-- Emotion -->
    <template v-if="tab === 'emotion' && emotion">
      <n-card :bordered="false" size="small" style="margin-top: 20px;">
        <div class="emotion-header">
          <n-avatar :size="64" color="#e6f0ff">{{ emotionIcon(emotion.primary) }}</n-avatar>
          <div>
            <div class="emotion-label">{{ primaryLabel(emotion.primary) }}</div>
            <div class="emotion-pad">愉悦 {{ emotion.valence.toFixed(2) }} · 唤醒 {{ emotion.arousal.toFixed(2) }} · 支配 {{ emotion.dominance.toFixed(2) }}</div>
            <div class="emotion-strength">强度 {{ (emotion.intensity * 100).toFixed(0) }}%</div>
          </div>
        </div>
      </n-card>

      <n-card title="8 维情绪向量" :bordered="false" size="small" style="margin-top: 20px;">
        <div class="drive-bars">
          <div v-for="e in emotionBars" :key="e.label" class="drive-row">
            <span class="drive-label">{{ e.label }}</span>
            <n-progress :percentage="Math.round(e.value * 100)" :color="e.color" :height="8" :border-radius="4" :show-indicator="false" processing />
            <span class="drive-val">{{ (e.value * 100).toFixed(0) }}%</span>
          </div>
        </div>
      </n-card>
    </template>

    <!-- Decision -->
    <template v-if="tab === 'decision' && features">
      <n-card title="最近决策溯源" :bordered="false" size="small" style="margin-top: 20px;">
        <template v-if="features.last_decision">
          <div class="decision-main">
            <span class="decision-action">{{ actionLabel(features.last_decision.action) }}</span>
            <span class="decision-score">score {{ features.last_decision.score.toFixed(3) }}</span>
          </div>
          <div class="decision-meta">
            {{ features.last_decision.routed_llm ? 'LLM 兜底决策' : 'System 1 快速决策' }} · 来源: {{ features.last_decision.source || '--' }}
          </div>
          <n-space style="margin-top: 8px;">
            <n-tag v-if="features.last_decision.routed_llm" type="warning" size="small" round :bordered="false">LLM</n-tag>
            <n-tag v-if="features.last_decision.source === 'care'" type="error" size="small" round :bordered="false">关怀</n-tag>
            <n-tag v-if="features.last_decision.source === 'casual'" type="info" size="small" round :bordered="false">闲聊</n-tag>
            <n-tag v-if="features.last_decision.source === 'knowledge_gap'" type="success" size="small" round :bordered="false">好奇</n-tag>
          </n-space>
        </template>
        <div v-else class="empty-text">等待首次决策...</div>
      </n-card>

      <n-grid :cols="2" :x-gap="20" style="margin-top: 20px;">
        <n-gi>
          <n-card title="当前关键因子" :bordered="false" size="small">
            <div v-for="f in factorItems" :key="f.label" class="factor-row">
              <span class="factor-label">{{ f.label }}</span>
              <span class="factor-value">{{ f.value }}</span>
            </div>
          </n-card>
        </n-gi>
        <n-gi>
          <n-card title="学习统计" :bordered="false" size="small" v-if="learning">
            <n-grid :cols="2" :x-gap="12" :y-gap="12">
              <n-gi><div class="metric-box"><div class="metric-num">{{ learning.principles_count }}</div><div class="metric-lbl">策略</div></div></n-gi>
              <n-gi><div class="metric-box"><div class="metric-num">{{ learning.active_threads }}</div><div class="metric-lbl">线程</div></div></n-gi>
              <n-gi><div class="metric-box"><div class="metric-num">{{ learning.active_inquiries }}</div><div class="metric-lbl">探索目标</div></div></n-gi>
              <n-gi><div class="metric-box"><div class="metric-num">{{ learning.patterns_count }}</div><div class="metric-lbl">模式</div></div></n-gi>
            </n-grid>
          </n-card>
        </n-gi>
      </n-grid>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import {
  NGrid, NGi, NCard, NTabs, NTabPane, NTag, NProgress,
  NSpace, NAvatar, NStatistic, NIcon, NSkeleton,
} from 'naive-ui'
import {
  TrendingUpOutline, DesktopOutline, FlashOutline, AlertCircleOutline,
} from '@vicons/ionicons5'
import { getFeatures, getLearningOverview, getEmotion, FeaturesViewModel, LearningOverview, EmotionViewModel } from '../../stores/api'

type Tab = 'overview' | 'emotion' | 'decision'
const tab = ref<Tab>('overview')

const features = ref<FeaturesViewModel | null>(null)
const learning = ref<LearningOverview | null>(null)
const emotion = ref<EmotionViewModel | null>(null)

function refresh() {
  getFeatures().then(f => features.value = f).catch(() => {})
  getLearningOverview().then(l => learning.value = l).catch(() => {})
  getEmotion().then(e => emotion.value = e).catch(() => {})
}
let timer: ReturnType<typeof setInterval>
onMounted(() => { refresh(); timer = setInterval(refresh, 5000) })
onUnmounted(() => clearInterval(timer))

const driveBars = computed(() => {
  if (!features.value) return []
  const d = features.value.drives
  return [
    { label: '社交', value: d.social, color: '#f472b6' },
    { label: '关怀', value: d.care, color: '#ef4444' },
    { label: '好奇', value: d.curious ?? 0, color: '#60a5fa' },
    { label: '安静', value: d.quiet, color: '#a78bfa' },
    { label: '探索', value: d.explore, color: '#34d399' },
  ]
})

const needBars = computed(() => {
  if (!features.value) return []
  const n = features.value.needs
  return [
    { label: '陪伴', value: n.companionship, color: '#f472b6' },
    { label: '关怀', value: n.care, color: '#ef4444' },
    { label: '玩耍', value: n.play, color: '#34d399' },
    { label: '好奇', value: n.curiosity, color: '#60a5fa' },
    { label: '休息', value: n.rest, color: '#a78bfa' },
    { label: '自主', value: n.autonomy, color: '#fbbf24' },
  ]
})

const emotionBars = computed(() => {
  if (!emotion.value) return []
  const v = emotion.value.vector
  return [
    { label: '情感', value: v.affection, color: '#f472b6' },
    { label: '担忧', value: v.worry, color: '#fbbf24' },
    { label: '好奇', value: v.curiosity, color: '#60a5fa' },
    { label: '困倦', value: v.sleepiness, color: '#a78bfa' },
    { label: '贪玩', value: v.playfulness, color: '#34d399' },
    { label: '寂寞', value: v.loneliness, color: '#f87171' },
    { label: '自信', value: v.confidence, color: '#818cf8' },
    { label: '烦躁', value: v.annoyance, color: '#fb923c' },
  ]
})

const factorItems = computed(() => {
  if (!features.value) return []
  const f = features.value
  return [
    { label: 'App', value: `${f.user.app_category} (${f.user.window_subtype || '-'})` },
    { label: '工作中', value: f.user.is_working ? '是' : '否' },
    { label: '时段接受率', value: `${(f.relationship.time_window_accept * 100).toFixed(0)}%` },
    { label: '距上次消息', value: `${f.user.time_since_chat_min.toFixed(0)} 分钟` },
    { label: '策略 / 模式', value: `${f.task.principle_count} / ${f.task.pattern_count}` },
    { label: '冷却', value: `${(f.task.cooldown_norm * 100).toFixed(0)}%` },
  ]
})

function emotionIcon(p: string) { const m: Record<string, string> = { joy: '😊', sadness: '😢', anger: '😠', fear: '😨', surprise: '😲', disgust: '🤢', neutral: '😐' }; return m[p] || '😐' }
function primaryLabel(p: string) { const m: Record<string, string> = { joy: '开心', sadness: '难过', anger: '生气', fear: '恐惧', surprise: '惊讶', disgust: '厌恶', neutral: '平静' }; return m[p] || p }
function actionLabel(a: string) { const m: Record<string, string> = { speak_casual: '闲聊', speak_care: '关心', speak_inquiry: '提问', care_rest: '催睡', care_meal: '催饭', care_hydration: '催喝水', care_health: '健康提醒', care_encourage: '鼓励', care_social: '社交提醒', observe: '观察', reflect: '反思', analyze_patterns: '分析', none: '安静' }; return m[a] || a }
</script>

<style scoped>
.page-root { height: 100%; overflow-y: auto; }
.page-header { display: flex; align-items: center; gap: 12px; margin-bottom: 4px; }
.page-title { font-size: 22px; font-weight: 700; margin: 0; color: #1a1a2e; }
.stat-card :deep(.n-card__content) { padding: 16px 20px; }
.drive-bars { display: flex; flex-direction: column; gap: 12px; }
.drive-row { display: flex; align-items: center; gap: 10px; }
.drive-label { width: 36px; font-size: 13px; color: #6b7280; text-align: right; flex-shrink: 0; }
.drive-val { width: 36px; font-size: 12px; color: #9ca3af; text-align: right; flex-shrink: 0; }
.rel-item { text-align: center; }
.rel-label { display: block; font-size: 12px; color: #9ca3af; margin-bottom: 4px; }
.rel-value { font-size: 15px; font-weight: 600; color: #1a1a2e; }
.emotion-header { display: flex; align-items: center; gap: 16px; }
.emotion-label { font-size: 24px; font-weight: 700; color: #1a1a2e; }
.emotion-pad { font-size: 13px; color: #6b7280; margin-top: 2px; }
.emotion-strength { font-size: 12px; color: #9ca3af; margin-top: 2px; }
.decision-main { margin-bottom: 4px; }
.decision-action { font-size: 20px; font-weight: 700; }
.decision-score { font-size: 13px; color: #9ca3af; margin-left: 8px; font-weight: 400; }
.decision-meta { font-size: 13px; color: #6b7280; }
.factor-row { display: flex; justify-content: space-between; padding: 8px 0; border-bottom: 1px solid #f3f4f6; }
.factor-row:last-child { border-bottom: none; }
.factor-label { font-size: 13px; color: #6b7280; }
.factor-value { font-size: 13px; font-weight: 500; color: #1a1a2e; }
.metric-box { text-align: center; padding: 12px; background: #f8fafc; border-radius: 8px; }
.metric-num { font-size: 22px; font-weight: 700; color: #1a1a2e; }
.metric-lbl { font-size: 12px; color: #9ca3af; margin-top: 2px; }
.empty-text { color: #9ca3af; padding: 20px 0; }
</style>
