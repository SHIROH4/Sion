<template>
  <div class="page-root">
    <div class="sub-header">
      <div class="sub-title">记忆管理 · 共 {{ total }} 条</div>
      <n-input-group style="max-width: 340px;">
        <n-input v-model:value="searchInput" placeholder="搜索记忆..." size="small" clearable @keydown.enter="doSearch" />
        <n-button type="primary" size="small" @click="doSearch">搜索</n-button>
      </n-input-group>
    </div>

    <n-tabs v-model:value="layer" type="bar" animated @update:value="onLayerChange">
      <n-tab-pane name="" tab="全部" />
      <n-tab-pane name="L0" tab="L0 工作记忆" />
      <n-tab-pane name="L1" tab="L1 日记" />
      <n-tab-pane name="L2" tab="L2 语义事实" />
      <n-tab-pane name="L3" tab="L3 核心自我" />
    </n-tabs>

    <div v-if="error" class="msg-box"><n-text type="error">{{ error }}</n-text></div>
    <div v-if="!error && !loading && memories.length === 0" class="msg-box"><n-text depth="3">暂无记忆数据</n-text></div>

    <n-grid :cols="2" :x-gap="16" :y-gap="16" v-if="memories.length">
      <n-gi v-for="m in memories" :key="m.id">
        <n-card size="small" :bordered="false" class="mem-card">
          <template #header>
            <div class="mem-hd">
              <n-tag :color="{ color: LAYER_COLORS[m.layer] || '#999', textColor: '#fff' }" size="tiny" :bordered="false">{{ m.layer }}</n-tag>
              <span v-if="m.created_at" class="mem-time">{{ m.created_at }}</span>
            </div>
          </template>
          <n-ellipsis :line-clamp="expanded === m.id ? 20 : 3" :tooltip="false">
            {{ m.content }}
          </n-ellipsis>
          <n-progress :percentage="Math.round(m.weight * 100)" :height="3" :border-radius="2" :show-indicator="false" :color="LAYER_COLORS[m.layer] || '#999'" style="margin: 10px 0;" />
          <n-space>
            <n-button text size="tiny" @click="expanded = expanded === m.id ? null : m.id">{{ expanded === m.id ? '收起' : '展开' }}</n-button>
            <n-button v-if="m.layer === 'L2'" text size="tiny" type="error" @click="handleDelete(m.id)">删除</n-button>
          </n-space>
        </n-card>
      </n-gi>
    </n-grid>

    <n-spin v-if="loading" style="display:flex;justify-content:center;padding:24px;" />
    <div v-if="hasMore && !loading" style="text-align:center;padding:20px;">
      <n-button @click="loadMore" secondary>加载更多</n-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { NCard, NGrid, NGi, NTabs, NTabPane, NTag, NButton, NInput, NInputGroup, NProgress, NSpace, NSpin, NText, NEllipsis } from 'naive-ui'

const API_BASE = 'http://127.0.0.1:19840'
const LAYER_COLORS: Record<string, string> = { L0: '#52c41a', L1: '#1677ff', L2: '#722ed1', L3: '#eb2f96' }

interface MemoryItem { id: string; layer: string; content: string; weight: number; created_at?: string }

const layer = ref('')
const memories = ref<MemoryItem[]>([])
const total = ref(0)
const page = ref(0)
const loading = ref(true)
const error = ref<string | null>(null)
const query = ref('')
const searchInput = ref('')
const expanded = ref<string | null>(null)

const hasMore = computed(() => memories.value.length < total.value)

async function load(l: string, p: number, q: string) {
  loading.value = true; error.value = null
  try {
    const params = new URLSearchParams({ page: String(p), pageSize: '20' })
    if (l) params.set('layer', l)
    if (q) params.set('query', q)
    const res = await fetch(`${API_BASE}/api/memories?${params}`)
    if (!res.ok) throw new Error(`HTTP ${res.status}`)
    const data = await res.json()
    if (p === 0) memories.value = data.memories; else memories.value = [...memories.value, ...data.memories]
    total.value = data.total
  } catch (e: any) { error.value = e?.message || '加载失败' }
  finally { loading.value = false }
}

function onLayerChange(l: string) { layer.value = l; page.value = 0; load(l, 0, query.value) }
function doSearch() { query.value = searchInput.value; page.value = 0; load(layer.value, 0, query.value) }
async function handleDelete(id: string) {
  if (!confirm('确定删除？')) return
  try {
    const res = await fetch(`${API_BASE}/api/memories/${id}`, { method: 'DELETE' })
    if (!res.ok) throw new Error(`HTTP ${res.status}`)
    memories.value = memories.value.filter(m => m.id !== id); total.value--
  } catch (e: any) { error.value = e?.message || '删除失败' }
}
function loadMore() { const next = page.value + 1; page.value = next; load(layer.value, next, query.value) }

load('', 0, '')
</script>

<style scoped>
.page-root { height: 100%; overflow-y: auto; }
.sub-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 4px; }
.sub-title { font-size: 14px; color: #6b7280; }
.msg-box { text-align: center; padding: 40px; }
.mem-card :deep(.n-card__content) { padding-top: 0; }
.mem-hd { display: flex; align-items: center; justify-content: space-between; width: 100%; }
.mem-time { font-size: 11px; color: #9ca3af; }
</style>
