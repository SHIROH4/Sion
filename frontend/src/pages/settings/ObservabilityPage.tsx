import { useEffect, useState, useRef } from 'react'

interface StatusEvent {
  time: string
  phase: string
  status: string // "start" | "ok" | "fail" | "info"
  message: string
}

const PHASE_LABELS: Record<string, string> = {
  decision: '决策', observe: '观察', curiosity: '好奇心',
  memory: '记忆', chat: '对话', system: '系统',
}

const PHASE_COLORS: Record<string, string> = {
  decision: '#722ed1', observe: '#52c41a', curiosity: '#1677ff',
  memory: '#fa8c16', chat: '#13c2c2', system: '#8c8c8c',
}

const STATUS_ICONS: Record<string, string> = {
  start: '▶️', ok: '✅', fail: '❌', info: 'ℹ️',
}

function ObservabilityPage() {
  const [events, setEvents] = useState<StatusEvent[]>([])
  const [connected, setConnected] = useState(false)
  const containerRef = useRef<HTMLDivElement>(null)

  // Load recent history on mount
  useEffect(() => {
    fetch('http://127.0.0.1:19840/api/status/recent')
      .then(r => r.json())
      .then(data => { if (Array.isArray(data)) setEvents(data) })
      .catch(() => {})
  }, [])

  // Subscribe to SSE for live events
  useEffect(() => {
    const es = new EventSource('http://127.0.0.1:19840/api/events')
    es.addEventListener('connected', () => setConnected(true))
    es.addEventListener('status', (e: MessageEvent) => {
      try {
        const evt: StatusEvent = JSON.parse(e.data)
        setEvents(prev => [...prev.slice(-199), evt])
      } catch {}
    })
    es.onerror = () => setConnected(false)
    return () => es.close()
  }, [])

  // Auto-scroll to bottom
  useEffect(() => {
    if (containerRef.current) {
      containerRef.current.scrollTop = containerRef.current.scrollHeight
    }
  }, [events])

  const phases = [...new Set(events.map(e => e.phase))]
  const okCount = events.filter(e => e.status === 'ok').length
  const failCount = events.filter(e => e.status === 'fail').length

  return (
    <div>
      <div style={{ fontSize: 20, fontWeight: 600, color: 'var(--text-primary)', marginBottom: 24 }}>
        {'🔭'} 执行状态 · 实时链路
      </div>

      {/* Summary bar */}
      <div style={{
        display: 'flex', gap: 12, marginBottom: 16,
        background: 'var(--bg-card)', borderRadius: 'var(--radius-card)',
        padding: '12px 20px', boxShadow: 'var(--shadow-card)',
      }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
          <div style={{
            width: 8, height: 8, borderRadius: 4,
            background: connected ? '#52c41a' : '#ff4d4f',
            transition: 'background 0.3s',
          }} />
          <span style={{ fontSize: 13, color: 'var(--text-secondary)' }}>
            {connected ? '实时连接中' : '未连接'}
          </span>
        </div>
        <div style={{ fontSize: 13, color: 'var(--text-muted)' }}>
          事件: {events.length} | ✅ {okCount} | ❌ {failCount}
        </div>
        <div style={{ display: 'flex', gap: 6, flex: 1, justifyContent: 'flex-end' }}>
          {phases.map(p => (
            <span key={p} style={{
              padding: '1px 8px', borderRadius: 8, fontSize: 11,
              background: (PHASE_COLORS[p] ?? '#999') + '15',
              color: PHASE_COLORS[p] ?? '#999',
            }}>
              {PHASE_LABELS[p] ?? p}
            </span>
          ))}
        </div>
      </div>

      {/* Event timeline */}
      <div
        ref={containerRef}
        style={{
          background: 'var(--bg-card)', borderRadius: 'var(--radius-card)',
          padding: 0, boxShadow: 'var(--shadow-card)',
          maxHeight: 'calc(100vh - 200px)', overflowY: 'auto',
        }}
      >
        {events.length === 0 && (
          <div style={{ padding: 48, textAlign: 'center', color: 'var(--text-muted)', fontSize: 14 }}>
            等待第一个事件...
          </div>
        )}
        {events.map((e, i) => (
          <div key={i} style={{
            display: 'flex', alignItems: 'flex-start', gap: 10,
            padding: '8px 16px', borderBottom: '1px solid #f5f5f5',
            transition: 'background 0.2s',
          }}>
            <span style={{ fontSize: 14, width: 24, textAlign: 'center', flexShrink: 0 }}>
              {STATUS_ICONS[e.status] ?? '·'}
            </span>
            <span style={{
              fontSize: 11, fontFamily: 'monospace', color: 'var(--text-muted)',
              width: 60, flexShrink: 0, marginTop: 2,
            }}>
              {e.time}
            </span>
            <span style={{
              padding: '1px 6px', borderRadius: 6, fontSize: 10,
              background: (PHASE_COLORS[e.phase] ?? '#999') + '18',
              color: PHASE_COLORS[e.phase] ?? '#999',
              fontWeight: 600, flexShrink: 0, minWidth: 44, textAlign: 'center',
            }}>
              {PHASE_LABELS[e.phase] ?? e.phase}
            </span>
            <span style={{
              fontSize: 12, color: e.status === 'fail' ? '#ff4d4f' : 'var(--text-secondary)',
              lineHeight: 1.4, wordBreak: 'break-all',
            }}>
              {e.message}
            </span>
          </div>
        ))}
      </div>
    </div>
  )
}

export default ObservabilityPage
