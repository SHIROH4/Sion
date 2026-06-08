<template>
  <div class="chat-root">
    <!-- Fixed header -->
    <div class="chat-header">
      <span class="chat-title">与诗音聊天</span>
      <n-space align="center">
        <n-tag v-if="error" type="error" size="small" round :bordered="false">{{ error }}</n-tag>
        <n-button v-if="messages.length" text size="small" @click="messages = []">清空</n-button>
      </n-space>
    </div>

    <!-- Scrollable messages -->
    <div class="msg-list" ref="listRef">
      <n-spin v-if="loading" style="flex:1;display:flex;align-items:center;justify-content:center;" />
      <template v-else>
        <div v-if="messages.length === 0" class="empty-chat">开始和诗音聊天吧</div>
        <div v-for="(msg, i) in messages" :key="i" :class="['bubble', msg.role === 'user' ? 'user' : 'assistant']">
          <div v-if="msg.source || msg.observed" class="bubble-tags">
            <n-tag v-if="msg.source" :bordered="false" size="tiny" round>{{ sourceLabel(msg.source) }}</n-tag>
            <n-tag v-if="msg.observed" :bordered="false" size="tiny" round type="info">屏幕观察</n-tag>
          </div>
          <div class="bubble-text">{{ msg.content }}</div>
          <span v-if="sending && i === messages.length - 1 && msg.role !== 'user'" class="cursor-blink">|</span>
        </div>
      </template>
    </div>

    <!-- Fixed input bar -->
    <div class="chat-footer">
      <n-input
        v-model:value="inputText"
        type="textarea"
        :autosize="{ minRows: 2, maxRows: 4 }"
        placeholder="输入消息... (Enter 发送)"
        :disabled="sending"
        @keydown="onKeydown"
        @compositionstart="composing = true"
        @compositionend="onCompositionEnd"
      />
      <n-button type="primary" :disabled="sending || !inputText.trim()" @click="handleSend" style="margin-top:8px;align-self:flex-end;">
        <template #icon><n-icon><SendOutline /></n-icon></template>
        发送
      </n-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, nextTick, watch } from 'vue'
import { NButton, NInput, NTag, NSpace, NSpin, NIcon } from 'naive-ui'
import { SendOutline } from '@vicons/ionicons5'

const API_BASE = 'http://127.0.0.1:19840'

interface ChatMessage { role: 'user' | 'assistant'; content: string; source?: string; observed?: boolean }

const messages = ref<ChatMessage[]>([])
const sending = ref(false)
const error = ref<string | null>(null)
const loading = ref(true)
const inputText = ref('')
const composing = ref(false)
const listRef = ref<HTMLElement | null>(null)
let sendingGuard = false

watch(() => messages.value.length, () => nextTick(() => {
  listRef.value?.lastElementChild?.scrollIntoView({ behavior: 'smooth' })
}))

function onCompositionEnd() { composing.value = false; setTimeout(() => composing.value = false, 300) }
function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter' && !e.shiftKey && !composing.value) {
    e.preventDefault(); handleSend()
  }
}

onMounted(async () => {
  try {
    const res = await fetch(`${API_BASE}/api/chat/history?page=0&pageSize=30`)
    if (res.ok) { const data = await res.json(); messages.value = data.messages ?? [] }
  } catch {}
  loading.value = false

  let es: EventSource | null = null
  const connectSSE = () => {
    es = new EventSource('http://127.0.0.1:19840/api/events')
    es.addEventListener('chat-message', (e: MessageEvent) => {
      if (sendingGuard) return
      try {
        const msg = JSON.parse(e.data)
        if (!msg.content) return
        const last = messages.value[messages.value.length - 1]
        if (last && last.content === msg.content && last.role === msg.role) return
        messages.value = [...messages.value, { role: msg.role, content: msg.content }]
      } catch {}
    })
    es.onerror = () => { es?.close(); setTimeout(connectSSE, 3000) }
  }
  connectSSE()
  onUnmounted(() => es?.close())
})

async function handleSend() {
  const text = inputText.value.trim()
  if (!text) return
  messages.value = [...messages.value, { role: 'user', content: text }]
  inputText.value = ''
  sending.value = true; sendingGuard = true; error.value = null
  try {
    const res = await fetch(`${API_BASE}/api/chat/send`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ text }),
    })
    if (!res.ok) { const err = await res.json().catch(() => ({ error: `HTTP ${res.status}` })); throw new Error(err.error || `HTTP ${res.status}`) }
    const result = await res.json()
    messages.value = [...messages.value, { role: 'assistant', content: result.content, source: result.source }]
  } catch (e: any) {
    messages.value = [...messages.value, { role: 'assistant', content: `[错误] ${e?.message || e}` }]
    error.value = e?.message || '发送失败'
  } finally { sending.value = false; setTimeout(() => { sendingGuard = false }, 500) }
}

function sourceLabel(s: string) { const m: Record<string, string> = { care: '关心', knowledge_gap: '想了解你', foresight: '预测验证', casual: '闲聊' }; return m[s] || '' }
</script>

<style scoped>
.chat-root {
  height: calc(100vh - 64px);
  display: flex;
  flex-direction: column;
  background: #fff;
  border-radius: 12px;
  box-shadow: 0 1px 3px rgba(0,0,0,0.06);
}

/* ── fixed top ── */
.chat-header {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 20px;
  border-bottom: 1px solid #f3f4f6;
}
.chat-title { font-size: 15px; font-weight: 600; }

/* ── scrollable middle ── */
.msg-list {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 16px 20px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.empty-chat {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #9ca3af;
  font-size: 14px;
}

/* ── fixed bottom ── */
.chat-footer {
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  padding: 12px 20px;
  border-top: 1px solid #f3f4f6;
}

/* ── bubbles ── */
.bubble { max-width: 78%; padding: 10px 14px; border-radius: 14px; font-size: 14px; line-height: 1.6; white-space: pre-wrap; word-break: break-word; }
.bubble.user { align-self: flex-end; background: #4f6ef7; color: #fff; border-bottom-right-radius: 4px; }
.bubble.assistant { align-self: flex-start; background: #f3f4f6; color: #1a1a2e; border-bottom-left-radius: 4px; }
.bubble-tags { margin-bottom: 4px; display: flex; gap: 4px; }
.bubble-text { font-size: 14px; }
.cursor-blink { animation: blink 1s step-end infinite; color: #999; }
@keyframes blink { 50% { opacity: 0; } }
</style>
