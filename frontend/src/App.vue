<template>
  <div class="pet-root" @mousemove="onMouseMove">
    <PetCanvas
      ref="petRef"
      :model-path="modelPath"
      @poke="onPoke"
      @drag-start="pet.setDragging(true)"
      @drag-end="pet.setDragging(false)"
    />
    <ThinkingDots :visible="pet.thinking" />
    <FloatingInput :visible="pet.inputVisible" :disabled="pet.streaming" @send="handleSend" />
    <div
      class="settings-gear"
      @mouseenter="gearVisible = true"
      @mouseleave="gearVisible = false"
      :style="{ opacity: gearVisible ? 1 : 0 }"
      @click="openSettings"
      title="打开设置"
    >⚙️</div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, nextTick } from 'vue'
import PetCanvas from './components/pet/PetCanvas.vue'
import ThinkingDots from './components/pet/ThinkingDots.vue'
import FloatingInput from './components/pet/FloatingInput.vue'
import { usePetStore } from './stores/petStore'
import { getProactivePoll } from './stores/api'

const pet = usePetStore()
const petRef = ref<InstanceType<typeof PetCanvas> | null>(null)
const isStreamingRef = ref(false)
const gearVisible = ref(false)

const modelPath = pet.modelPath

// Wails chat events
onMounted(() => {
  const wr = (window as any).runtime
  if (wr?.EventsOn) {
    const onStream = (chunk: string) => {
      pet.setThinking(false)
      petRef.value?.showBubble(chunk)
    }
    const onSent = () => {
      pet.setStreaming(false)
      isStreamingRef.value = false
    }
    wr.EventsOn('chat:stream', onStream)
    wr.EventsOn('chat:sent', onSent)
    onUnmounted(() => {
      wr.EventsOff('chat:stream', onStream)
      wr.EventsOff('chat:sent', onSent)
    })
  }

  // SSE for real-time chat sync
  let es: EventSource | null = null
  const connectSSE = () => {
    es = new EventSource('http://127.0.0.1:19840/api/events')
    es.addEventListener('chat-message', (e: MessageEvent) => {
      if (isStreamingRef.value) return
      try {
        const msg = JSON.parse(e.data)
        if (msg.role === 'assistant' && msg.content) {
          petRef.value?.showBubble(msg.content)
        }
      } catch {}
    })
    es.onerror = () => { es?.close(); setTimeout(connectSSE, 3000) }
  }
  connectSSE()
  onUnmounted(() => es?.close())
})

// Poll for proactive messages
let pollTimer: ReturnType<typeof setInterval> | null = null
onMounted(() => {
  const poll = async () => {
    try {
      const data = await getProactivePoll()
      if (data?.message && !isStreamingRef.value) {
        petRef.value?.showBubble(data.message)
      }
    } catch {}
  }
  poll()
  pollTimer = setInterval(poll, 5000)
  onUnmounted(() => { if (pollTimer) clearInterval(pollTimer) })
})

function handleSend(text: string) {
  pet.setStreaming(true)
  pet.setThinking(true)
  isStreamingRef.value = true
  petRef.value?.hideBubble()
  try { (window as any).go?.main?.App?.SendMessage(text) } catch {}
}

function onMouseMove(e: MouseEvent) {
  const target = e.currentTarget as HTMLElement
  const rect = target.getBoundingClientRect()
  pet.setInputVisible(e.clientY - rect.top > rect.height * 0.5)
}

function onPoke(areas: string[]) {
  try { (window as any).go?.main?.App?.Poke(areas) } catch {}
}

function openSettings() {
  try { (window as any).go?.main?.App?.OpenSettings() } catch {}
}
</script>

<style scoped>
.pet-root {
  width: 100%; height: 100%;
  position: relative; overflow: hidden;
  background: transparent;
}
.settings-gear {
  position: absolute; bottom: 8px; right: 8px; z-index: 30;
  transition: opacity 0.2s; cursor: pointer;
  font-size: 14px; width: 28px; height: 28px; border-radius: 50%;
  background: rgba(255,255,255,0.7);
  display: flex; align-items: center; justify-content: center;
}
</style>
