<template>
  <div ref="containerRef" class="pet-canvas">
    <div ref="bubbleElRef" class="pet-bubble" />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, shallowRef } from 'vue'

const props = defineProps<{
  modelPath: string
}>()
const emit = defineEmits<{
  ready: []
  poke: [areas: string[]]
  dragStart: []
  dragEnd: []
}>()

const POKE_THRESHOLD_MS = 250
const DRAG_THRESHOLD_PX = 4
const EXPRESSION_RESET_MS = 3000
const BUBBLE_HIDE_MS = 6000
const GAZE_RESET_MS = 3000

class ComboDetector {
  private clicks: number[] = []
  registerClick(): number {
    const now = Date.now()
    this.clicks = this.clicks.filter((t) => now - t < 2500)
    this.clicks.push(now)
    return this.clicks.length
  }
}

const containerRef = ref<HTMLDivElement | null>(null)
const bubbleElRef = ref<HTMLDivElement | null>(null)
const appRef = shallowRef<any>(null)
const modelRef = shallowRef<any>(null)
const bubbleQueueRef = ref<string[]>([])
let bubbleTimer: ReturnType<typeof setTimeout> | null = null

const timerRef = {
  lastExpressionAt: 0,
  lastBubbleAt: 0,
  lastGazeAt: 0,
  gazeCenterX: 0,
  gazeCenterY: 0,
  combo: new ComboDetector(),
}

function splitBubbleText(text: string): string[] {
  const MAX = 28
  if (text.length <= MAX) return text ? [text] : []
  const chunks: string[] = []
  let remaining = text
  while (remaining.length > 0) {
    if (remaining.length <= MAX) { chunks.push(remaining); break }
    let cut = -1
    for (const sep of ['。', '！', '？', '\n', '，', '、']) {
      const idx = remaining.lastIndexOf(sep, MAX)
      if (idx > MAX * 0.5) { cut = idx + 1; break }
    }
    if (cut < 0) cut = MAX
    chunks.push(remaining.slice(0, cut).trim())
    remaining = remaining.slice(cut).trim()
  }
  return chunks.filter((c) => c.length > 0)
}

function showNextBubble() {
  if (bubbleTimer) { clearTimeout(bubbleTimer); bubbleTimer = null }
  const q = bubbleQueueRef.value
  if (q.length === 0) {
    bubbleElRef.value?.classList.remove('show')
    return
  }
  const el = bubbleElRef.value
  if (el) { el.textContent = q[0]; el.classList.add('show') }
  q.shift()
  timerRef.lastBubbleAt = Date.now()
  bubbleTimer = setTimeout(showNextBubble, 3000)
}

function stopBubbleQueue() {
  bubbleQueueRef.value = []
  if (bubbleTimer) { clearTimeout(bubbleTimer); bubbleTimer = null }
}

// ---- Exposed methods ----
defineExpose({
  playExpression(id: string) {
    timerRef.lastExpressionAt = Date.now()
    modelRef.value?.expression?.(id)
  },
  playMotion(group: string, index: number) {
    modelRef.value?.motion?.(group, index)
  },
  showBubble(t: string) {
    timerRef.lastBubbleAt = Date.now()
    stopBubbleQueue()
    if (!t) return
    bubbleQueueRef.value = splitBubbleText(t)
    showNextBubble()
  },
  showBubbleLive(t: string) {
    timerRef.lastBubbleAt = Date.now()
    stopBubbleQueue()
    const el = bubbleElRef.value
    if (el && t) { el.textContent = t; el.classList.add('show') }
  },
  hideBubble() {
    timerRef.lastBubbleAt = 0
    stopBubbleQueue()
    bubbleElRef.value?.classList.remove('show')
  },
  setFocus(x: number, y: number, instant?: boolean) {
    modelRef.value?.focus?.(x, y, instant ?? false)
  },
})

// ---- PIXI lifecycle ----
onMounted(() => {
  const container = containerRef.value
  if (!container) return

  const P = (window as any).PIXI
  if (!P) { console.error('[PetCanvas] PIXI not loaded'); return }
  const LM = P.live2d?.Live2DModel
  if (!LM) { console.error('[PetCanvas] Live2DModel not loaded'); return }

  let destroyed = false

  const app = new P.Application({
    backgroundAlpha: 0, transparent: true,
    useContextAlpha: 'notMultiplied',
    resizeTo: container, antialias: true,
    resolution: window.devicePixelRatio || 1,
    autoDensity: true,
  })
  appRef.value = app
  container.appendChild(app.view as HTMLCanvasElement)

  const updateGazeTimer = () => { timerRef.lastGazeAt = Date.now() }

  const getCanvasPos = (e: PointerEvent) => {
    const r = (app.view as HTMLCanvasElement).getBoundingClientRect()
    return { x: e.clientX - r.left, y: e.clientY - r.top }
  }

  let drag = { active: false, startX: 0, startY: 0, startTime: 0 }

  const onPointerDown = (e: PointerEvent) => {
    const { x, y } = getCanvasPos(e)
    drag = { active: false, startX: x, startY: y, startTime: Date.now() }
    try { (window as any).go?.main?.App?.DragWindow() } catch {}
  }
  const onPointerMove = (e: PointerEvent) => {
    const { x, y } = getCanvasPos(e)
    modelRef.value?.focus?.(x, y, false)
    updateGazeTimer()
    if (!drag.active && (Math.abs(x - drag.startX) > DRAG_THRESHOLD_PX || Math.abs(y - drag.startY) > DRAG_THRESHOLD_PX)) {
      drag.active = true
      emit('dragStart')
    }
  }
  const onPointerUp = async (e: PointerEvent) => {
    if (drag.active) { emit('dragEnd'); return }
    if (Date.now() - drag.startTime < POKE_THRESHOLD_MS) {
      const count = timerRef.combo.registerClick()
      const { x, y } = getCanvasPos(e)
      const m = modelRef.value
      if (m) {
        const areas = await m.hitTest?.(x, y)
        if (areas?.length) {
          emit('poke', areas)
          if (count >= 10) {
            m.motion?.('special_01', 0)
          } else if (count >= 5) {
            m.expression?.('exp_06')
            timerRef.lastExpressionAt = Date.now()
          } else if (count >= 3) {
            m.expression?.('exp_04')
            timerRef.lastExpressionAt = Date.now()
          }
        }
      }
    }
  }

  const canvas = app.view as HTMLCanvasElement
  canvas.addEventListener('pointerdown', onPointerDown)
  canvas.addEventListener('pointermove', onPointerMove)
  canvas.addEventListener('pointerup', onPointerUp)

  const tick = setInterval(() => {
    if (destroyed) return
    const now = Date.now()
    const t = timerRef
    if (t.lastExpressionAt > 0 && now - t.lastExpressionAt > EXPRESSION_RESET_MS) {
      t.lastExpressionAt = 0
      modelRef.value?.expression?.('exp_01')
    }
    if (t.lastBubbleAt > 0 && now - t.lastBubbleAt > BUBBLE_HIDE_MS) {
      t.lastBubbleAt = 0
      stopBubbleQueue()
      bubbleElRef.value?.classList.remove('show')
    }
    if (t.lastGazeAt > 0 && now - t.lastGazeAt > GAZE_RESET_MS) {
      t.lastGazeAt = 0
      modelRef.value?.focus?.(t.gazeCenterX, t.gazeCenterY, true)
    }
  }, 200)

  const ro = new ResizeObserver(() => {
    const m = modelRef.value
    if (!m) return
    const cw = container.clientWidth
    const ch = container.clientHeight
    m.position?.set(cw / 2, ch / 2)
    timerRef.gazeCenterX = cw / 2
    timerRef.gazeCenterY = ch / 2
  })
  ro.observe(container)

  LM.from(props.modelPath).then((model: any) => {
    if (destroyed) return
    modelRef.value = model
    const cw = container.clientWidth
    const ch = container.clientHeight
    const s = (ch * 0.9) / model.height
    model.scale.set(s)
    const ctrl = model.internalModel?.focusController
    if (ctrl) { ctrl.acceleration = 0.04; ctrl.deceleration = 0.08 }
    model.anchor.set(0.5, 0.5)
    model.position.set(cw / 2, ch / 2)
    app.stage.addChildAt(model, 0)
    timerRef.gazeCenterX = cw / 2
    timerRef.gazeCenterY = ch / 2

    const b = model.getBounds()
    const margin = 20
    const newW = Math.ceil(b.width + margin * 2)
    const newH = Math.ceil(b.height + margin)
    try { (window as any).go?.main?.App?.ResizeWindow(newW, newH) } catch {}

    emit('ready')
  }).catch((err: any) => {
    console.error('[PetCanvas] Failed to load model:', err)
  })

  // Wails events
  const wr = (window as any).runtime
  if (wr?.EventsOn) {
    wr.EventsOn('mouse:move', (pos: any) => {
      if (!destroyed && modelRef.value) {
        modelRef.value.focus?.(pos.x, pos.y, false)
        updateGazeTimer()
      }
    })
    wr.EventsOn('pet:expression', (id: string) => {
      if (!destroyed && modelRef.value) {
        timerRef.lastExpressionAt = Date.now()
        modelRef.value.expression?.(id)
      }
    })
    wr.EventsOn('pet:motion', (data: any) => {
      if (!destroyed && modelRef.value) {
        modelRef.value.motion?.(data.group, data.index)
      }
    })
    wr.EventsOn('pet:bubble', (data: any) => {
      if (!destroyed) {
        timerRef.lastBubbleAt = Date.now()
        stopBubbleQueue()
        if (data.text) {
          bubbleQueueRef.value = splitBubbleText(data.text)
          showNextBubble()
        }
      }
    })
    wr.EventsOn('pet:hide_bubble', () => {
      bubbleElRef.value?.classList.remove('show')
      timerRef.lastBubbleAt = 0
    })
  }

  onUnmounted(() => {
    destroyed = true
    stopBubbleQueue()
    if (bubbleTimer) clearTimeout(bubbleTimer)
    ro.disconnect()
    clearInterval(tick)
    canvas.removeEventListener('pointerdown', onPointerDown)
    canvas.removeEventListener('pointermove', onPointerMove)
    canvas.removeEventListener('pointerup', onPointerUp)
    app.destroy(true, { children: true, texture: true })
    appRef.value = null
    modelRef.value = null
  })
})
</script>

<style scoped>
.pet-canvas {
  width: 100%; height: 100%; overflow: hidden;
  background: transparent; position: relative;
}
.pet-bubble {
  position: fixed; top: 2%; left: 50%;
  transform: translateX(-50%); max-width: 250px;
  padding: 10px 14px;
  background: rgba(255, 255, 255, 0.95);
  backdrop-filter: blur(8px); border-radius: 18px;
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
  font-size: 14px; line-height: 1.5; color: #444;
  box-shadow: 0 4px 15px rgba(0, 0, 0, 0.1);
  border: 1px solid rgba(255, 255, 255, 0.5);
  word-break: break-word; pointer-events: none;
  z-index: 10; opacity: 0; transition: opacity 0.3s ease;
}
.pet-bubble::after {
  content: ''; position: absolute; bottom: -8px; left: 50%;
  transform: translateX(-50%);
  border-left: 8px solid transparent;
  border-right: 8px solid transparent;
  border-top: 8px solid rgba(255, 255, 255, 0.95);
}
.pet-bubble.show { opacity: 1; }
</style>
