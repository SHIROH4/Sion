import { useState, useRef } from 'react'

interface FloatingInputProps {
  visible: boolean
  onSend: (text: string) => void
  disabled: boolean
}

function FloatingInput({ visible, onSend, disabled }: FloatingInputProps) {
  const [text, setText] = useState('')
  const composingUntil = useRef(0)
  const inputRef = useRef<HTMLInputElement>(null)

  const handleSend = () => {
    const trimmed = text.trim()
    if (!trimmed || disabled) return
    onSend(trimmed)
    setText('')
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      if (composingUntil.current > Date.now()) return
      e.preventDefault()
      handleSend()
    }
  }

  return (
    <div style={{
      position: 'absolute',
      bottom: 20,
      left: '50%',
      transform: 'translateX(-50%)',
      width: '85%',
      maxWidth: 320,
      zIndex: 25,
      opacity: visible ? 1 : 0,
      pointerEvents: visible ? 'auto' : 'none',
      transition: 'opacity 0.2s ease',
    }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
        <input
          ref={inputRef}
          type="text"
          value={text}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={handleKeyDown}
          onCompositionStart={() => { composingUntil.current = 1e16 }}
          onCompositionEnd={() => { composingUntil.current = Date.now() + 300 }}
          placeholder="和诗音说点什么..."
          disabled={disabled}
          style={{
            flex: 1, height: 36, borderRadius: 20,
            border: '1px solid rgba(255,255,255,0.3)',
            background: 'rgba(255,255,255,0.85)',
            padding: '0 16px', fontSize: 14,
            outline: 'none', color: '#333', boxSizing: 'border-box',
          }}
        />
        <button
          onClick={handleSend}
          disabled={disabled || !text.trim()}
          style={{
            width: 36, height: 36, borderRadius: '50%',
            background: disabled || !text.trim() ? '#ccc' : '#1677ff',
            color: '#fff', border: 'none',
            cursor: disabled || !text.trim() ? 'not-allowed' : 'pointer',
            flexShrink: 0, fontSize: 16,
            display: 'flex', alignItems: 'center', justifyContent: 'center',
            padding: 0, transition: 'background 0.15s',
          }}
        >
          {'➤'}
        </button>
      </div>
    </div>
  )
}

export default FloatingInput
