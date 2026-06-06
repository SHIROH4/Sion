import { useEffect, useRef } from 'react'

export interface ChatMessage {
  role: 'user' | 'assistant'
  content: string
  source?: string
  observed?: boolean
  timestamp?: string
}

function sourceLabel(source: string): string {
  switch (source) {
    case 'care':          return '\u{2764}\u{FE0F} 关心'
    case 'knowledge_gap': return '\u{1F50D} 想了解你'
    case 'foresight':     return '\u{1F52E} 预测验证'
    case 'casual':        return '\u{1F4AC} 闲聊'
    default:              return ''
  }
}

interface ChatMessageListProps {
  messages: ChatMessage[]
  streaming?: boolean
}

function ChatMessageList({ messages, streaming }: ChatMessageListProps) {
  const endRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    endRef.current?.scrollIntoView({ behavior: 'auto' })
  }, [messages])

  return (
    <div style={{
      flex: 1,
      overflowY: 'auto',
      padding: '12px 16px',
      display: 'flex',
      flexDirection: 'column',
      gap: 8,
    }}>
      {messages.length === 0 && (
        <div style={{
          flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center',
          color: 'var(--text-muted)', fontSize: 14,
        }}>
          开始和诗音聊天吧
        </div>
      )}
      {messages.map((msg, i) => {
        const isUser = msg.role === 'user'
        const isLast = i === messages.length - 1
        return (
          <div
            key={i}
            style={{
              alignSelf: isUser ? 'flex-end' : 'flex-start',
              maxWidth: '80%',
              padding: '8px 12px',
              borderRadius: 12,
              fontSize: 14,
              lineHeight: 1.6,
              background: isUser ? 'var(--color-primary)' : '#f0f0f0',
              color: isUser ? '#fff' : 'var(--text-primary)',
              whiteSpace: 'pre-wrap',
              wordBreak: 'break-word',
            }}
          >
            {(msg.source || msg.observed) && (
              <div style={{ marginBottom: 4, display: 'flex', gap: 4, flexWrap: 'wrap' }}>
                {msg.source && (
                  <span style={{
                    display: 'inline-block',
                    fontSize: 10,
                    padding: '1px 6px',
                    borderRadius: 8,
                    background: isUser ? 'rgba(255,255,255,0.3)' : 'rgba(0,0,0,0.06)',
                    color: isUser ? 'rgba(255,255,255,0.85)' : 'var(--text-secondary)',
                  }}>
                    {sourceLabel(msg.source)}
                  </span>
                )}
                {msg.observed && (
                  <span style={{
                    display: 'inline-block',
                    fontSize: 10,
                    padding: '1px 6px',
                    borderRadius: 8,
                    background: 'rgba(124,77,255,0.12)',
                    color: '#7c4dff',
                  }}>
                    {'\u{1F441}\u{FE0F} 诗音看到了你的屏幕'}
                  </span>
                )}
              </div>
            )}
            {msg.content}
            {streaming && isLast && !isUser && (
              <span className="cursor-blink" style={{ background: '#333' }} />
            )}
          </div>
        )
      })}
      <div ref={endRef} />
    </div>
  )
}

export default ChatMessageList
