import { create } from 'zustand'

function getStoredModel(): string {
  try { return localStorage.getItem('pet-model') || '/model/Mao/Mao.model3.json' } catch { return '/model/Mao/Mao.model3.json' }
}

export interface PetStore {
  isDragging: boolean
  streaming: boolean
  thinking: boolean
  inputVisible: boolean
  modelPath: string
  setDragging: (v: boolean) => void
  setStreaming: (v: boolean) => void
  setThinking: (v: boolean) => void
  setInputVisible: (v: boolean) => void
  setModelPath: (path: string) => void
}

export const usePetStore = create<PetStore>((set) => ({
  isDragging: false,
  streaming: false,
  thinking: false,
  inputVisible: false,
  modelPath: getStoredModel(),

  setDragging: (v) => set({ isDragging: v }),
  setStreaming: (v) => set({ streaming: v }),
  setThinking: (v) => set({ thinking: v }),
  setInputVisible: (v) => set({ inputVisible: v }),
  setModelPath: (path) => { try { localStorage.setItem('pet-model', path) } catch {}; set({ modelPath: path }) },
}))
