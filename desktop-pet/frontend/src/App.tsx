import { useRef, useEffect, useCallback } from 'react'
import PetCanvas, { PetCanvasHandle } from './components/PetCanvas'
import ThinkingDots from './components/pet/ThinkingDots'
import FloatingInput from './components/pet/FloatingInput'
import { usePetStore } from './store'
import { getProactivePoll } from './store/api'
import './App.css'

function App() {
  const petRef = useRef<PetCanvasHandle>(null)
  const isStreamingRef = useRef(false)
  const streaming = usePetStore((s) => s.streaming)
  const thinking = usePetStore((s) => s.thinking)
  const inputVisible = usePetStore((s) => s.inputVisible)
  const setDragging = usePetStore((s) => s.setDragging)
  const setStreaming = usePetStore((s) => s.setStreaming)
  const setThinking = usePetStore((s) => s.setThinking)
  const setInputVisible = usePetStore((s) => s.setInputVisible)
  const modelPath = usePetStore((s) => s.modelPath)

  // ---- Wails chat events ----
  useEffect(() => {
    const wr = (window as any).runtime
    if (!wr?.EventsOn) return
    const onStream = (chunk: string) => {
      setThinking(false)
      petRef.current?.showBubble(chunk)
    }
    const onSent = () => {
      setStreaming(false)
      isStreamingRef.current = false
    }
    wr.EventsOn('chat:stream', onStream)
    wr.EventsOn('chat:sent', onSent)
    return () => {
      wr.EventsOff('chat:stream', onStream)
      wr.EventsOff('chat:sent', onSent)
    }
  }, [setStreaming, setThinking])

  // ---- SSE: real-time chat + proactive messages cross-window (same as Settings ChatPage) ----
  useEffect(() => {
    let es: EventSource | null = null
    const connectSSE = () => {
      es = new EventSource('http://127.0.0.1:19840/api/events')
      // Chat messages from Settings window → show bubble immediately.
      es.addEventListener('chat-message', (e: MessageEvent) => {
        if (isStreamingRef.current) return
        try {
          const msg = JSON.parse(e.data)
          if (msg.role === 'assistant' && msg.content) {
            petRef.current?.showBubble(msg.content)
          }
        } catch {}
      })
      // Care action messages (already handled by poll, but SSE is faster).
      es.addEventListener('care-action', () => {
        // Coalesced with poll — SSE gets it first, poll is fallback.
      })
      es.onerror = () => { es?.close(); setTimeout(connectSSE, 3000) }
    }
    connectSSE()
    return () => es?.close()
  }, [])

  // ---- Poll for proactive messages (fallback, care actions go through both paths) ----
  useEffect(() => {
    const poll = async () => {
      try {
        const data = await getProactivePoll()
        if (data?.message && !isStreamingRef.current) {
          petRef.current?.showBubble(data.message)
        }
      } catch {}
    }
    poll()
    const interval = setInterval(poll, 5000)
    return () => clearInterval(interval)
  }, [])

  // ---- Send message ----
  const handleSendMessage = useCallback(
    (text: string) => {
      setStreaming(true)
      setThinking(true)
      isStreamingRef.current = true
      petRef.current?.hideBubble()
      try { window.go?.main?.App?.SendMessage(text) } catch {}
    },
    [setStreaming, setThinking],
  )

  // ---- Mouse tracking ----
  const handleMouseMove = useCallback(
    (e: React.MouseEvent) => {
      const rect = (e.currentTarget as HTMLElement).getBoundingClientRect()
      setInputVisible(e.clientY - rect.top > rect.height * 0.5)
    },
    [setInputVisible],
  )

  return (
    <div
      onMouseMove={handleMouseMove}
      style={{ width: '100%', height: '100%', position: 'relative', overflow: 'hidden', background: 'transparent' }}
    >
      <PetCanvas
        ref={petRef}
        modelPath={modelPath}
        onPoke={(areas) => { try { window.go?.main?.App?.Poke(areas) } catch {} }}
        onDragStart={() => setDragging(true)}
        onDragEnd={() => setDragging(false)}
      />

      <ThinkingDots visible={thinking} />

      <FloatingInput visible={inputVisible} onSend={handleSendMessage} disabled={streaming} />

      {/* Settings gear */}
      <div
        onMouseEnter={(e) => { e.currentTarget.style.opacity = '1' }}
        onMouseLeave={(e) => { e.currentTarget.style.opacity = '0' }}
        onClick={() => { try { window.go?.main?.App?.OpenSettings() } catch {} }}
        title="打开设置"
        style={{
          position: 'absolute', bottom: 8, right: 8, zIndex: 30,
          opacity: 0, transition: 'opacity 0.2s', cursor: 'pointer',
          fontSize: 14, width: 28, height: 28, borderRadius: '50%',
          background: 'rgba(255,255,255,0.7)',
          display: 'flex', alignItems: 'center', justifyContent: 'center',
        }}
      >
        {'⚙️'}
      </div>
    </div>
  )
}

export default App
