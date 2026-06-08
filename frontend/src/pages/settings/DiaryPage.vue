<template>
  <div class="page-root">
    <div class="page-header">
      <h2 class="page-title">日记时间线</h2>
      <n-tag size="small" round :bordered="false">共 {{ total }} 篇</n-tag>
    </div>

    <div v-if="error" class="msg"><n-text type="error">{{ error }}</n-text></div>
    <div v-if="!error && !loading && diaries.length === 0" class="msg"><n-text depth="3">暂无日记</n-text></div>

    <n-timeline v-if="diaries.length">
      <n-timeline-item
        v-for="d in diaries" :key="d.id"
        :color="d.emotion_score >= 0 ? '#4f6ef7' : '#f59e0b'"
        :time="d.created_at"
      >
        <n-card size="small" :bordered="false">
          <p class="diary-content">{{ d.content }}</p>
          <div class="diary-meta">
            <span class="diary-emoji">{{ emotionEmoji(d.emotion_label) }}</span>
            <span :class="['diary-score', d.emotion_score >= 0 ? 'positive' : 'negative']">{{ d.emotion_score > 0 ? '+' : '' }}{{ d.emotion_score.toFixed(2) }}</span>
          </div>
        </n-card>
      </n-timeline-item>
    </n-timeline>

    <n-spin v-if="loading" style="display:flex;justify-content:center;padding:24px;" />
    <div v-if="hasMore && !loading" style="text-align:center;padding:20px;">
      <n-button @click="loadMore" secondary>加载更多</n-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { NCard, NTimeline, NTimelineItem, NButton, NTag, NSpin, NText } from 'naive-ui'

const API_BASE = 'http://127.0.0.1:19840'

interface DiaryItem { id: number; content: string; emotion_label: string; emotion_score: number; created_at: string }

const diaries = ref<DiaryItem[]>([])
const total = ref(0)
const page = ref(0)
const loading = ref(true)
const error = ref<string | null>(null)

const hasMore = computed(() => diaries.value.length < total.value)

async function load(p: number) {
  loading.value = true; error.value = null
  try {
    const res = await fetch(`${API_BASE}/api/diaries?page=${p}&pageSize=20`)
    if (!res.ok) throw new Error(`HTTP ${res.status}`)
    const data = await res.json()
    if (p === 0) diaries.value = data.diaries; else diaries.value = [...diaries.value, ...data.diaries]
    total.value = data.total
  } catch (e: any) { error.value = e?.message || '加载失败' }
  finally { loading.value = false }
}
onMounted(() => load(0))
function loadMore() { const next = page.value + 1; page.value = next; load(next) }
function emotionEmoji(label: string) { switch (label) { case 'happy': return '😊'; case 'sad': return '😕'; default: return '😐' } }
</script>

<style scoped>
.page-root { height: 100%; overflow-y: auto; }
.page-header { display: flex; align-items: center; gap: 12px; margin-bottom: 24px; }
.page-title { font-size: 22px; font-weight: 700; margin: 0; color: #1a1a2e; }
.msg { text-align: center; padding: 60px; }
.diary-content { font-size: 14px; color: #374151; line-height: 1.7; margin: 0 0 8px; }
.diary-meta { display: flex; align-items: center; gap: 6px; }
.diary-emoji { font-size: 16px; }
.diary-score { font-size: 13px; font-weight: 600; }
.diary-score.positive { color: #10b981; }
.diary-score.negative { color: #f59e0b; }
</style>
