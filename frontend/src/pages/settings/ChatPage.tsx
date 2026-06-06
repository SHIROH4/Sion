import { useState, useCallback, useEffect, useRef } from 'react'
import ChatMessageList, { ChatMessage } from '../../components/settings/ChatMessageList'
import ChatInput from '../../components/settings/ChatInput'

const API_BASE = 'http://127.0.0.1:19840'

async function sendChatMessage(text: string): Promise<{ content: string; source?: string }> {
  const res = await fetch(`${API_BASE}/api/chat/send`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ text }),
  })
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: `HTTP ${res.status}` }))
    throw new Error(err.error || `HTTP ${res.status}`)
  }
  return res.json()
}

interface HistoryResponse {
  messages: ChatMessage[]
  total: number
}

async function fetchHistory(): Promise<ChatMessage[]> {
  const res = await fetch(`${API_BASE}/api/chat/history?page=0&pageSize=30`)
  if (!res.ok) return []
  const data: HistoryResponse = await res.json()
  // API already returns oldest-first.
  return data.messages ?? []
}

function ChatPage() {
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [sending, setSending] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const sendingRef = useRef(false)

  // Load history on mount, then subscribe to SSE for real-time messages.
  useEffect(() => {
    let cancelled = false
    let es: EventSource | null = null

    fetchHistory()
      .then((msgs) => { if (!cancelled) setMessages(msgs) })
      .catch(() => {})
      .finally(() => { if (!cancelled) setLoading(false) })

    // Subscribe to SSE for real-time cross-window sync.
    const connectSSE = () => {
      es = new EventSource('http://127.0.0.1:19840/api/events')
      es.addEventListener('chat-message', (e: MessageEvent) => {
        if (cancelled || sendingRef.current) return
        try {
          const msg = JSON.parse(e.data)
          if (!msg.content) return
          // Use functional update to avoid race with handleSend's local add.
          setMessages((prev) => {
            const last = prev[prev.length - 1]
            if (last && last.content === msg.content && last.role === msg.role) return prev
            return [...prev, { role: msg.role, content: msg.content }]
          })
        } catch {}
      })
      es.onerror = () => {
        es?.close()
        if (!cancelled) setTimeout(connectSSE, 3000)
      }
    }
    connectSSE()

    return () => { cancelled = true; es?.close() }
  }, [])

  const handleSend = useCallback(async (text: string) => {
    setMessages((prev) => [...prev, { role: 'user', content: text }])
    setSending(true)
    sendingRef.current = true
    setError(null)
    try {
      const result = await sendChatMessage(text)
      setMessages((prev) => [...prev, {
        role: 'assistant',
        content: result.content,
        source: result.source,
      }])
    } catch (e: any) {
      setMessages((prev) => [...prev, {
        role: 'assistant',
        content: `[错误] ${e?.message || e}`,
      }])
      setError(e?.message || '发送失败')
    } finally {
      setSending(false)
      // Keep guard up briefly for late SSE events.
      setTimeout(() => { sendingRef.current = false }, 500)
    }
  }, [])

  return (
    <div style={{
      display: 'flex', flexDirection: 'column', height: '100%',
      background: 'var(--bg-card)', borderRadius: 'var(--radius-card)',
      boxShadow: 'var(--shadow-card)', overflow: 'hidden',
    }}>
      {/* Header */}
      <div style={{
        display: 'flex', alignItems: 'center', justifyContent: 'space-between',
        padding: '12px 16px', borderBottom: '1px solid var(--border-color)',
      }}>
        <span style={{ fontSize: 15, fontWeight: 600, color: 'var(--text-primary)' }}>
          {'\u{1F4AC} 与诗音聊天'}
        </span>
        {error && (
          <span style={{ fontSize: 12, color: 'var(--color-danger)' }}>
            {error}
          </span>
        )}
        {messages.length > 0 && (
          <button
            onClick={() => setMessages([])}
            style={{
              border: 'none', background: 'none',
              color: 'var(--text-muted)', fontSize: 12,
              cursor: 'pointer',
            }}
          >
            清空
          </button>
        )}
      </div>

      {/* Messages */}
      {loading ? (
        <div style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', color: 'var(--text-muted)' }}>
          加载对话记录...
        </div>
      ) : (
        <ChatMessageList messages={messages} streaming={sending} />
      )}

      {/* Input */}
      <ChatInput
        onSend={handleSend}
        disabled={sending}
        placeholder="输入消息... (Enter 发送, Shift+Enter 换行)"
      />
    </div>
  )
}

export default ChatPage
