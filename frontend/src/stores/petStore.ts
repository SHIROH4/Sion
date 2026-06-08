import { defineStore } from 'pinia'
import { ref } from 'vue'

function getStoredModel(): string {
  try {
    return localStorage.getItem('pet-model') || '/model/Mao/Mao.model3.json'
  } catch {
    return '/model/Mao/Mao.model3.json'
  }
}

export const usePetStore = defineStore('pet', () => {
  const isDragging = ref(false)
  const streaming = ref(false)
  const thinking = ref(false)
  const inputVisible = ref(false)
  const modelPath = ref(getStoredModel())

  function setDragging(v: boolean) { isDragging.value = v }
  function setStreaming(v: boolean) { streaming.value = v }
  function setThinking(v: boolean) { thinking.value = v }
  function setInputVisible(v: boolean) { inputVisible.value = v }
  function setModelPath(path: string) {
    try { localStorage.setItem('pet-model', path) } catch {}
    modelPath.value = path
  }

  return {
    isDragging, streaming, thinking, inputVisible, modelPath,
    setDragging, setStreaming, setThinking, setInputVisible, setModelPath,
  }
})
