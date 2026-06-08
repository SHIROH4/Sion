<template>
  <div class="page-root">
    <div class="sub-header">
      <div class="sub-title">身份图谱 · 定义诗音的核心认知</div>
      <n-button size="small" @click="triggerSelfUpdate" secondary>触发自我更新</n-button>
    </div>

    <n-spin v-if="loading" style="display:flex;justify-content:center;padding:60px;" />
    <n-text v-else-if="error" type="error">{{ error }}</n-text>

    <n-grid v-else :cols="2" :x-gap="16" :y-gap="16">
      <n-gi v-for="cat in CATEGORIES" :key="cat.key">
        <n-card size="small" :bordered="false" :title="cat.label">
          <template #header-extra>
            <n-tag :color="{ color: cat.color + '18', textColor: cat.color }" size="tiny" :bordered="false" round>{{ nodesByCat(cat.key).length }}</n-tag>
          </template>
          <div v-if="nodesByCat(cat.key).length === 0" class="empty-cat">暂无</div>
          <div v-for="node in nodesByCat(cat.key)" :key="node.id" class="node-item">
            <template v-if="editing === node.id">
              <n-input v-model:value="editText" type="textarea" :autosize="{ minRows: 2, maxRows: 5 }" />
              <n-space style="margin-top: 6px;">
                <n-button size="tiny" type="primary" :loading="saving" @click="saveEdit(node)">保存</n-button>
                <n-button size="tiny" @click="cancelEdit">取消</n-button>
              </n-space>
            </template>
            <div v-else class="node-text" @click="startEdit(node)">{{ node.content }}</div>
          </div>
        </n-card>
      </n-gi>
    </n-grid>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { NCard, NGrid, NGi, NTag, NButton, NInput, NSpace, NSpin, NText } from 'naive-ui'

const API_BASE = 'http://127.0.0.1:19840'

interface IdentityNode { id: number; category: string; content: string; weight: number; updated_at: number }

const CATEGORIES = [
  { key: 'core_value', label: '核心价值观', color: '#ff4d4f' },
  { key: 'preference', label: '偏好', color: '#fa8c16' },
  { key: 'belief', label: '信念', color: '#1677ff' },
  { key: 'relationship', label: '关系', color: '#52c41a' },
  { key: 'behavior_rule', label: '行为准则', color: '#722ed1' },
  { key: 'goal', label: '目标', color: '#eb2f96' },
  { key: 'fear', label: '恐惧', color: '#595959' },
]

const nodes = ref<IdentityNode[]>([])
const loading = ref(true)
const error = ref<string | null>(null)
const editing = ref<number | null>(null)
const editText = ref('')
const saving = ref(false)

function nodesByCat(cat: string) { return nodes.value.filter(n => n.category === cat) }

async function load() {
  try {
    const res = await fetch(`${API_BASE}/api/identity`)
    if (!res.ok) throw new Error(`HTTP ${res.status}`)
    nodes.value = await res.json(); error.value = null
  } catch (e: any) { error.value = e?.message || '加载失败' }
  finally { loading.value = false }
}
onMounted(load)

function startEdit(node: IdentityNode) { editing.value = node.id; editText.value = node.content }
function cancelEdit() { editing.value = null; editText.value = '' }
async function saveEdit(node: IdentityNode) {
  saving.value = true
  try {
    const res = await fetch(`${API_BASE}/api/identity/${node.id}`, {
      method: 'PUT', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ content: editText.value, category: node.category }),
    })
    if (!res.ok) throw new Error(`HTTP ${res.status}`)
    nodes.value = nodes.value.map(n => n.id === node.id ? { ...n, content: editText.value } : n)
    editing.value = null
  } catch (e: any) { alert('保存失败: ' + (e?.message || e)) }
  finally { saving.value = false }
}
async function triggerSelfUpdate() {
  try {
    const res = await fetch(`${API_BASE}/api/identity/self-update`, { method: 'POST' })
    if (!res.ok) throw new Error(`HTTP ${res.status}`)
    alert('自我更新已触发')
  } catch { alert('自我更新尚未实现') }
}
</script>

<style scoped>
.page-root { height: 100%; overflow-y: auto; }
.sub-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 20px; }
.sub-title { font-size: 14px; color: #6b7280; }
.empty-cat { font-size: 13px; color: #9ca3af; padding: 8px 0; }
.node-item { margin-bottom: 10px; padding-bottom: 10px; border-bottom: 1px solid #f3f4f6; }
.node-item:last-child { border-bottom: none; margin-bottom: 0; }
.node-text { font-size: 13px; color: #1a1a2e; line-height: 1.6; cursor: pointer; border-radius: 4px; padding: 4px 6px; margin: -4px -6px; transition: background 0.15s; }
.node-text:hover { background: #f0f4ff; color: #4f6ef7; }
</style>
