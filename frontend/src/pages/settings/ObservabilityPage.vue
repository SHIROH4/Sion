<template>
  <div class="page-root">
    <div class="page-header">
      <h2 class="page-title">执行状态</h2>
      <n-tag :type="connected ? 'success' : 'default'" size="small" round :bordered="false">
        <template #icon><span class="live-dot" :class="{ on: connected }" /></template>
        {{ connected ? '实时连接中' : '未连接' }}
      </n-tag>
    </div>

    <n-card :bordered="false" size="small" style="margin-bottom:20px;">
      <div class="summary">
        <n-space :size="16">
          <n-statistic label="事件" :value="events.length" />
          <n-statistic label="成功"><span style="color:#10b981">{{ okCount }}</span></n-statistic>
          <n-statistic label="失败"><span style="color:#ef4444">{{ failCount }}</span></n-statistic>
        </n-space>
        <n-space v-if="phases.length">
          <n-tag v-for="p in phases" :key="p" :color="{ color: phaseColor(p) + '18', textColor: phaseColor(p) }" size="tiny" round :bordered="false">{{ phaseLabel(p) }}</n-tag>
        </n-space>
      </div>
    </n-card>

    <n-card :bordered="false" size="small">
      <div ref="containerRef" class="event-list">
        <div v-if="events.length === 0" class="empty-list">等待第一个事件...</div>
        <div v-for="(e, i) in events" :key="i" class="event-item">
          <span class="evt-icon">{{ statusIcon(e.status) }}</span>
          <span class="evt-time">{{ e.time }}</span>
          <n-tag :color="{ color: phaseColor(e.phase) + '18', textColor: phaseColor(e.phase) }" size="tiny" :bordered="false" class="evt-phase">{{ phaseLabel(e.phase) }}</n-tag>
          <span class="evt-msg" :class="{ fail: e.status === 'fail' }">{{ e.message }}</span>
        </div>
      </div>
    </n-card>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch, nextTick } from 'vue'
import { NCard, NTag, NSpace, NStatistic } from 'naive-ui'

interface StatusEvent { time: string; phase: string; status: string; message: string }

const events = ref<StatusEvent[]>([])
const connected = ref(false)
const containerRef = ref<HTMLElement | null>(null)

const phases = computed(() => [...new Set(events.value.map(e => e.phase))])
const okCount = computed(() => events.value.filter(e => e.status === 'ok').length)
const failCount = computed(() => events.value.filter(e => e.status === 'fail').length)

watch(events, () => nextTick(() => {
  if (containerRef.value) containerRef.value.scrollTop = containerRef.value.scrollHeight
}), { deep: true })

function phaseLabel(p: string) { const m: Record<string, string> = { decision: '决策', observe: '观察', curiosity: '好奇心', memory: '记忆', chat: '对话', system: '系统' }; return m[p] || p }
function phaseColor(p: string) { const m: Record<string, string> = { decision: '#722ed1', observe: '#52c41a', curiosity: '#1677ff', memory: '#fa8c16', chat: '#13c2c2', system: '#8c8c8c' }; return m[p] || '#999' }
function statusIcon(s: string) { const m: Record<string, string> = { start: '▶️', ok: '✅', fail: '❌', info: 'ℹ️' }; return m[s] || '·' }

onMounted(() => {
  fetch('http://127.0.0.1:19840/api/status/recent').then(r => r.json()).then(data => { if (Array.isArray(data)) events.value = data }).catch(() => {})
  const es = new EventSource('http://127.0.0.1:19840/api/events')
  es.addEventListener('connected', () => connected.value = true)
  es.addEventListener('status', (e: MessageEvent) => {
    try { const evt: StatusEvent = JSON.parse(e.data); events.value = [...events.value.slice(-199), evt] } catch {}
  })
  es.onerror = () => connected.value = false
  onUnmounted(() => es.close())
})
</script>

<style scoped>
.page-root { height: 100%; overflow-y: auto; }
.page-header { display: flex; align-items: center; gap: 12px; margin-bottom: 20px; }
.page-title { font-size: 22px; font-weight: 700; margin: 0; color: #1a1a2e; }
.live-dot { display: inline-block; width: 6px; height: 6px; border-radius: 3px; background: #d1d5db; margin-right: 2px; }
.live-dot.on { background: #10b981; }
.summary { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 12px; }
.event-list { max-height: calc(100vh - 280px); overflow-y: auto; }
.empty-list { padding: 48px; text-align: center; color: #9ca3af; }
.event-item { display: flex; align-items: center; gap: 10px; padding: 10px 16px; border-bottom: 1px solid #f3f4f6; }
.event-item:last-child { border-bottom: none; }
.evt-icon { font-size: 14px; width: 24px; text-align: center; flex-shrink: 0; }
.evt-time { font-size: 11px; font-family: monospace; color: #9ca3af; width: 56px; flex-shrink: 0; }
.evt-phase { flex-shrink: 0; }
.evt-msg { font-size: 13px; color: #4b5563; line-height: 1.4; word-break: break-all; flex: 1; }
.evt-msg.fail { color: #ef4444; }
</style>
