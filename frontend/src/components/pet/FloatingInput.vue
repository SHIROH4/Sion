<template>
  <div class="floating-input" :class="{ visible }">
    <div class="input-row">
      <input
        v-model="text"
        type="text"
        :disabled="disabled"
        placeholder="和诗音说点什么..."
        @keydown="onKeydown"
        @compositionstart="composing = true"
        @compositionend="onCompositionEnd"
      />
      <button :disabled="disabled || !text.trim()" @click="handleSend">➤</button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'

const props = defineProps<{
  visible: boolean
  disabled: boolean
}>()
const emit = defineEmits<{ send: [text: string] }>()

const text = ref('')
const composing = ref(false)
let composingTimer: ReturnType<typeof setTimeout> | null = null

function onCompositionEnd() {
  composing.value = false
  if (composingTimer) clearTimeout(composingTimer)
  composingTimer = setTimeout(() => { composing.value = false }, 300)
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter' && !e.shiftKey) {
    if (composing.value) return
    e.preventDefault()
    handleSend()
  }
}

function handleSend() {
  const trimmed = text.value.trim()
  if (!trimmed || props.disabled) return
  emit('send', trimmed)
  text.value = ''
}
</script>

<style scoped>
.floating-input {
  position: absolute; bottom: 20px; left: 50%;
  transform: translateX(-50%); width: 85%; max-width: 320px;
  z-index: 25; opacity: 0; pointer-events: none;
  transition: opacity 0.2s ease;
}
.floating-input.visible { opacity: 1; pointer-events: auto; }
.input-row { display: flex; align-items: center; gap: 8px; }
input {
  flex: 1; height: 36px; border-radius: 20px;
  border: 1px solid rgba(255,255,255,0.3);
  background: rgba(255,255,255,0.85);
  padding: 0 16px; font-size: 14px;
  outline: none; color: #333; box-sizing: border-box;
}
button {
  width: 36px; height: 36px; border-radius: 50%;
  background: var(--color-primary); color: #fff;
  border: none; cursor: pointer; flex-shrink: 0;
  font-size: 16px; display: flex; align-items: center;
  justify-content: center; padding: 0;
  transition: background 0.15s;
}
button:disabled { background: #ccc; cursor: not-allowed; }
</style>
