import { describe, it, expect, beforeEach } from 'vitest'
import { usePetStore } from './store'

function resetStore() {
  usePetStore.setState({
    isDragging: false,
    streaming: false,
    thinking: false,
    inputVisible: false,
  })
}

describe('PetStore', () => {
  beforeEach(() => resetStore())

  describe('streaming / thinking / input state', () => {
    it('setStreaming updates streaming flag', () => {
      expect(usePetStore.getState().streaming).toBe(false)
      usePetStore.getState().setStreaming(true)
      expect(usePetStore.getState().streaming).toBe(true)
    })

    it('setThinking updates thinking flag', () => {
      expect(usePetStore.getState().thinking).toBe(false)
      usePetStore.getState().setThinking(true)
      expect(usePetStore.getState().thinking).toBe(true)
    })

    it('setInputVisible updates input visible flag', () => {
      expect(usePetStore.getState().inputVisible).toBe(false)
      usePetStore.getState().setInputVisible(true)
      expect(usePetStore.getState().inputVisible).toBe(true)
    })
  })

  describe('drag state', () => {
    it('setDragging updates drag flag', () => {
      expect(usePetStore.getState().isDragging).toBe(false)
      usePetStore.getState().setDragging(true)
      expect(usePetStore.getState().isDragging).toBe(true)
    })
  })
})
