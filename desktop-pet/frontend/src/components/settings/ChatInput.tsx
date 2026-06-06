import { useState, useRef } from 'react'

interface ChatInputProps {
  onSend: (text: string) => void
  disabled?: boolean
  placeholder?: string
}

function ChatInput({ onSend, disabled, placeholder = '输入消息...' }: ChatInputProps) {
  const [text, setText] = useState('')
  const composingUntil = useRef(0)
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  const handleSubmit = () => {
    const trimmed = text.trim()
    if (!trimmed || disabled) return
    onSend(trimmed)
    setText('')
    if (textareaRef.current) {
      textareaRef.current.style.height = 'auto'
    }
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      // Block Enter during IME composition + 300ms buffer for WKWebView.
      if (composingUntil.current > Date.now()) {
        return
      }
      e.preventDefault()
      handleSubmit()
    }
  }

  return (
    <div style={{
      display: 'flex', gap: 8, padding: '12px 16px',
      borderTop: '1px solid var(--border-color)',
      background: '#fafafa',
    }}>
      <textarea
        ref={textareaRef}
        value={text}
        onChange={(e) => setText(e.target.value)}
        onKeyDown={handleKeyDown}
        onCompositionStart={() => { composingUntil.current = 1e16 }}
        onCompositionEnd={() => { composingUntil.current = Date.now() + 300 }}
        placeholder={placeholder}
        disabled={disabled}
        rows={2}
        style={{
          flex: 1,
          resize: 'none',
          border: '1px solid var(--border-color)',
          borderRadius: 'var(--radius-input)',
          padding: '8px 12px',
          fontSize: 14,
          fontFamily: 'inherit',
          outline: 'none',
          lineHeight: 1.5,
        }}
        onFocus={(e) => {
          e.currentTarget.style.borderColor = 'var(--color-primary)'
          e.currentTarget.style.boxShadow = '0 0 0 2px rgba(22,119,255,0.1)'
        }}
        onBlur={(e) => {
          e.currentTarget.style.borderColor = 'var(--border-color)'
          e.currentTarget.style.boxShadow = 'none'
        }}
      />
      <button
        onClick={handleSubmit}
        disabled={disabled || !text.trim()}
        style={{
          border: 'none',
          borderRadius: 'var(--radius-input)',
          background: disabled || !text.trim() ? '#ccc' : 'var(--color-primary)',
          color: '#fff',
          padding: '8px 20px',
          fontSize: 14,
          fontWeight: 600,
          cursor: disabled || !text.trim() ? 'not-allowed' : 'pointer',
          alignSelf: 'flex-end',
          transition: 'background 0.15s',
        }}
      >
        发送
      </button>
    </div>
  )
}

export default ChatInput
