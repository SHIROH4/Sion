import { useEffect, useState, useCallback } from 'react'
import { getConfig, saveConfig, GlobalConfig } from '../../store/api'

function SectionTitle({ children }: { children: string }) {
  return (
    <h3 style={{
      fontSize: 14, fontWeight: 600, color: 'var(--text-primary)',
      borderLeft: '3px solid var(--color-primary)',
      paddingLeft: 10, marginBottom: 16,
    }}>
      {children}
    </h3>
  )
}

function ConfigRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div style={{ display: 'flex', alignItems: 'center', marginBottom: 16 }}>
      <label style={{ width: 120, flexShrink: 0, fontSize: 14, color: 'var(--text-secondary)' }}>
        {label}
      </label>
      <div style={{ flex: 1 }}>{children}</div>
    </div>
  )
}

const inputStyle: React.CSSProperties = {
  width: '100%',
  height: 38,
  border: '1px solid var(--border-color)',
  borderRadius: 'var(--radius-input)',
  padding: '0 12px',
  fontSize: 14,
  outline: 'none',
  boxSizing: 'border-box',
  transition: 'border-color 0.15s, box-shadow 0.15s',
}

function TagInput({ tags, onChange }: { tags: string[]; onChange: (t: string[]) => void }) {
  const [input, setInput] = useState('')

  const addTag = useCallback((val: string) => {
    const tag = val.trim()
    if (tag && !tags.includes(tag)) {
      onChange([...tags, tag])
    }
    setInput('')
  }, [tags, onChange])

  const removeTag = useCallback((idx: number) => {
    onChange(tags.filter((_, i) => i !== idx))
  }, [tags, onChange])

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') {
      e.preventDefault()
      addTag(input)
    } else if (e.key === ',' && input === '') {
      e.preventDefault()
    }
  }

  const handlePaste = (e: React.ClipboardEvent) => {
    const text = e.clipboardData.getData('text')
    if (text.includes(',')) {
      e.preventDefault()
      const parts = text.split(',').map((s) => s.trim()).filter(Boolean)
      const newTags = [...tags]
      parts.forEach((t) => { if (!newTags.includes(t)) newTags.push(t) })
      onChange(newTags)
    }
  }

  return (
    <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8, alignItems: 'center' }}>
      {tags.map((tag, i) => (
        <span key={i} style={{
          display: 'inline-flex', alignItems: 'center',
          background: '#f0f0f0', borderRadius: 4,
          padding: '2px 8px', fontSize: 13, color: 'var(--text-primary)',
        }}>
          {tag}
          <span
            onClick={() => removeTag(i)}
            style={{ marginLeft: 4, cursor: 'pointer', color: '#999', fontSize: 14, lineHeight: 1 }}
            title="删除"
          >
            {'×'}
          </span>
        </span>
      ))}
      <input
        value={input}
        onChange={(e) => {
          const v = e.target.value
          if (v.endsWith(',')) {
            addTag(v.slice(0, -1))
          } else {
            setInput(v)
          }
        }}
        onKeyDown={handleKeyDown}
        onPaste={handlePaste}
        placeholder={tags.length === 0 ? '输入后回车添加' : '继续添加'}
        style={{ border: 'none', outline: 'none', minWidth: 120, flex: 1, fontSize: 13, background: 'transparent' }}
      />
    </div>
  )
}

function ApiConfigPage() {
  const [cfg, setCfg] = useState<GlobalConfig | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [toast, setToast] = useState<{ type: 'success' | 'error'; msg: string } | null>(null)

  useEffect(() => {
    getConfig()
      .then(setCfg)
      .catch((e) => showToast('error', '加载配置失败: ' + (e?.message || e)))
      .finally(() => setLoading(false))
  }, [])

  const showToast = (type: 'success' | 'error', msg: string) => {
    setToast({ type, msg })
    setTimeout(() => setToast(null), 2500)
  }

  const handleSave = async () => {
    if (!cfg) return
    if (!cfg.llm_provider || !cfg.llm_model || !cfg.llm_base_url) {
      showToast('error', '请填写 LLM Provider、Model 和 Base URL')
      return
    }
    setSaving(true)
    try {
      await saveConfig(cfg)
      showToast('success', '配置已保存 ✓')
    } catch (e: any) {
      showToast('error', '保存失败: ' + (e?.message || e))
    } finally {
      setSaving(false)
    }
  }

  const [testing, setTesting] = useState<string | null>(null)

  const testConnection = async (label: string, target: string) => {
    setTesting(label)
    try {
      const res = await fetch('http://127.0.0.1:19840/api/test-connection', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ target }),
      })
      const data = await res.json()
      if (data.ok) {
        showToast('success', `${label} 连接成功 ✓`)
      } else {
        showToast('error', `${label} 失败: ${data.error}`)
      }
    } catch (e: any) {
      showToast('error', `${label} 失败: ${e?.message || e}`)
    } finally {
      setTesting(null)
    }
  }

  const update = (key: keyof GlobalConfig, value: any) => {
    if (!cfg) return
    setCfg({ ...cfg, [key]: value })
  }

  if (loading) {
    return (
      <div style={{ background: 'var(--bg-card)', borderRadius: 'var(--radius-card)', padding: 48, textAlign: 'center' }}>
        <p style={{ color: 'var(--text-secondary)' }}>加载中...</p>
      </div>
    )
  }

  if (!cfg) {
    return (
      <div style={{ background: 'var(--bg-card)', borderRadius: 'var(--radius-card)', padding: 48, textAlign: 'center' }}>
        <p style={{ color: 'var(--color-danger)' }}>无法加载配置，请检查主进程是否运行</p>
      </div>
    )
  }

  return (
    <div style={{ position: 'relative' }}>
      {/* Toast */}
      {toast && (
        <div style={{
          position: 'fixed', top: 20, right: 20, zIndex: 1000,
          padding: '10px 20px', borderRadius: 6, fontSize: 14, fontWeight: 500,
          color: '#fff',
          background: toast.type === 'success' ? 'var(--color-success)' : 'var(--color-danger)',
          boxShadow: '0 4px 12px rgba(0,0,0,0.2)',
          animation: 'slideIn 0.3s ease',
        }}>
          {toast.msg}
        </div>
      )}

      <div style={{ maxWidth: 640 }}>
        {/* Section 1: LLM */}
        <SectionTitle>{'\u{1F916} LLM 对话配置'}</SectionTitle>
        <ConfigRow label="LLM Provider">
          <input
            type="text"
            value={cfg.llm_provider}
            onChange={(e) => update('llm_provider', e.target.value)}
            placeholder="deepseek"
            style={inputStyle}
            onFocus={(e) => {
              e.currentTarget.style.borderColor = 'var(--color-primary)'
              e.currentTarget.style.boxShadow = '0 0 0 2px rgba(22,119,255,0.1)'
            }}
            onBlur={(e) => {
              e.currentTarget.style.borderColor = 'var(--border-color)'
              e.currentTarget.style.boxShadow = 'none'
            }}
          />
        </ConfigRow>
        <ConfigRow label="LLM Base URL">
          <input
            type="text"
            value={cfg.llm_base_url}
            onChange={(e) => update('llm_base_url', e.target.value)}
            placeholder="https://api.deepseek.com"
            style={inputStyle}
            onFocus={(e) => {
              e.currentTarget.style.borderColor = 'var(--color-primary)'
              e.currentTarget.style.boxShadow = '0 0 0 2px rgba(22,119,255,0.1)'
            }}
            onBlur={(e) => {
              e.currentTarget.style.borderColor = 'var(--border-color)'
              e.currentTarget.style.boxShadow = 'none'
            }}
          />
        </ConfigRow>
        <ConfigRow label="LLM Model">
          <input
            type="text"
            value={cfg.llm_model}
            onChange={(e) => update('llm_model', e.target.value)}
            placeholder="deepseek-chat"
            style={inputStyle}
            onFocus={(e) => {
              e.currentTarget.style.borderColor = 'var(--color-primary)'
              e.currentTarget.style.boxShadow = '0 0 0 2px rgba(22,119,255,0.1)'
            }}
            onBlur={(e) => {
              e.currentTarget.style.borderColor = 'var(--border-color)'
              e.currentTarget.style.boxShadow = 'none'
            }}
          />
        </ConfigRow>
        <ConfigRow label="API Key">
          <input
            type="password"
            value={cfg.llm_api_key}
            onChange={(e) => update('llm_api_key', e.target.value)}
            placeholder="sk-..."
            style={inputStyle}
            onFocus={(e) => {
              e.currentTarget.style.borderColor = 'var(--color-primary)'
              e.currentTarget.style.boxShadow = '0 0 0 2px rgba(22,119,255,0.1)'
            }}
            onBlur={(e) => {
              e.currentTarget.style.borderColor = 'var(--border-color)'
              e.currentTarget.style.boxShadow = 'none'
            }}
          />
        </ConfigRow>

        {/* Section 2: Vision */}
        <SectionTitle>{'\u{1F441}\u{FE0F} Vision 配置（截图分析）'}</SectionTitle>
        <ConfigRow label="Vision Model">
          <input
            type="text"
            value={cfg.vision_model}
            onChange={(e) => update('vision_model', e.target.value)}
            placeholder="qwen-vl-max"
            style={inputStyle}
            onFocus={(e) => {
              e.currentTarget.style.borderColor = 'var(--color-primary)'
              e.currentTarget.style.boxShadow = '0 0 0 2px rgba(22,119,255,0.1)'
            }}
            onBlur={(e) => {
              e.currentTarget.style.borderColor = 'var(--border-color)'
              e.currentTarget.style.boxShadow = 'none'
            }}
          />
        </ConfigRow>
        <ConfigRow label="Vision Base URL">
          <input
            type="text"
            value={cfg.vision_base_url}
            onChange={(e) => update('vision_base_url', e.target.value)}
            placeholder="默认同 LLM Base URL"
            style={inputStyle}
            onFocus={(e) => {
              e.currentTarget.style.borderColor = 'var(--color-primary)'
              e.currentTarget.style.boxShadow = '0 0 0 2px rgba(22,119,255,0.1)'
            }}
            onBlur={(e) => {
              e.currentTarget.style.borderColor = 'var(--border-color)'
              e.currentTarget.style.boxShadow = 'none'
            }}
          />
        </ConfigRow>
        <ConfigRow label="Vision API Key">
          <input
            type="password"
            value={cfg.vision_api_key}
            onChange={(e) => update('vision_api_key', e.target.value)}
            placeholder="默认同 LLM API Key"
            style={inputStyle}
            onFocus={(e) => {
              e.currentTarget.style.borderColor = 'var(--color-primary)'
              e.currentTarget.style.boxShadow = '0 0 0 2px rgba(22,119,255,0.1)'
            }}
            onBlur={(e) => {
              e.currentTarget.style.borderColor = 'var(--border-color)'
              e.currentTarget.style.boxShadow = 'none'
            }}
          />
        </ConfigRow>

        {/* Section 3: Emotion */}
        <SectionTitle>{'\u{1F9E0} Emotion 配置（情绪云端评估）'}</SectionTitle>
        <ConfigRow label="Emotion Model">
          <input
            type="text"
            value={cfg.emotion_model ?? ''}
            onChange={(e) => update('emotion_model', e.target.value)}
            placeholder="DeepSeek-V4-Flash"
            style={inputStyle}
            onFocus={(e) => { e.currentTarget.style.borderColor = 'var(--color-primary)'; e.currentTarget.style.boxShadow = '0 0 0 2px rgba(22,119,255,0.1)' }}
            onBlur={(e) => { e.currentTarget.style.borderColor = 'var(--border-color)'; e.currentTarget.style.boxShadow = 'none' }}
          />
        </ConfigRow>
        <ConfigRow label="Emotion Base URL">
          <input
            type="text"
            value={cfg.emotion_base_url ?? ''}
            onChange={(e) => update('emotion_base_url', e.target.value)}
            placeholder="https://api.siliconflow.cn/v1"
            style={inputStyle}
            onFocus={(e) => { e.currentTarget.style.borderColor = 'var(--color-primary)'; e.currentTarget.style.boxShadow = '0 0 0 2px rgba(22,119,255,0.1)' }}
            onBlur={(e) => { e.currentTarget.style.borderColor = 'var(--border-color)'; e.currentTarget.style.boxShadow = 'none' }}
          />
        </ConfigRow>
        <ConfigRow label="Emotion API Key">
          <input
            type="password"
            value={cfg.emotion_api_key ?? ''}
            onChange={(e) => update('emotion_api_key', e.target.value)}
            placeholder="sk-..."
            style={inputStyle}
            onFocus={(e) => { e.currentTarget.style.borderColor = 'var(--color-primary)'; e.currentTarget.style.boxShadow = '0 0 0 2px rgba(22,119,255,0.1)' }}
            onBlur={(e) => { e.currentTarget.style.borderColor = 'var(--border-color)'; e.currentTarget.style.boxShadow = 'none' }}
          />
        </ConfigRow>

        {/* Section 4: User Info */}
        <SectionTitle>{'\u{1F464} 用户信息'}</SectionTitle>
        <ConfigRow label="称呼">
          <input
            type="text"
            value={cfg.user_name}
            onChange={(e) => update('user_name', e.target.value)}
            placeholder="主人"
            style={inputStyle}
            onFocus={(e) => {
              e.currentTarget.style.borderColor = 'var(--color-primary)'
              e.currentTarget.style.boxShadow = '0 0 0 2px rgba(22,119,255,0.1)'
            }}
            onBlur={(e) => {
              e.currentTarget.style.borderColor = 'var(--border-color)'
              e.currentTarget.style.boxShadow = 'none'
            }}
          />
        </ConfigRow>
        <ConfigRow label="技术栈">
          <TagInput
            tags={cfg.user_tech_stack ?? []}
            onChange={(t) => update('user_tech_stack', t)}
          />
        </ConfigRow>

        {/* Test + Save buttons */}
        <div style={{ marginTop: 24, paddingTop: 16, borderTop: '1px solid var(--border-color)', display: 'flex', gap: 10, flexWrap: 'wrap', alignItems: 'center' }}>
          <button
            onClick={() => testConnection('Chat', 'chat')}
            disabled={!!testing || !cfg.llm_model}
            style={{
              padding: '10px 20px', background: '#fff', color: 'var(--text-primary)',
              border: '1px solid var(--border-color)', borderRadius: 'var(--radius-input)',
              fontSize: 13, fontWeight: 500, cursor: testing || !cfg.llm_model ? 'not-allowed' : 'pointer',
              opacity: testing || !cfg.llm_model ? 0.5 : 1,
            }}
          >
            {testing === 'Chat' ? '测试中...' : '测试 Chat'}
          </button>
          {cfg.vision_model && (
            <button
              onClick={() => testConnection('Vision', 'vision')}
              disabled={!!testing}
              style={{
                padding: '10px 20px', background: '#fff', color: 'var(--text-primary)',
                border: '1px solid var(--border-color)', borderRadius: 'var(--radius-input)',
                fontSize: 13, fontWeight: 500, cursor: testing ? 'not-allowed' : 'pointer',
                opacity: testing ? 0.5 : 1,
              }}
            >
              {testing === 'Vision' ? '测试中...' : '测试 Vision'}
            </button>
          )}
          {cfg.emotion_model && (
            <button
              onClick={() => testConnection('Emotion', 'emotion')}
              disabled={!!testing}
              style={{
                padding: '10px 20px', background: '#fff', color: 'var(--text-primary)',
                border: '1px solid var(--border-color)', borderRadius: 'var(--radius-input)',
                fontSize: 13, fontWeight: 500, cursor: testing ? 'not-allowed' : 'pointer',
                opacity: testing ? 0.5 : 1,
              }}
            >
              {testing === 'Emotion' ? '测试中...' : '测试 Emotion'}
            </button>
          )}
          <button
            onClick={handleSave}
            disabled={saving}
            style={{
              padding: '10px 28px', marginLeft: 'auto',
              background: saving ? '#9cb8e0' : 'var(--color-primary)',
              color: '#fff', border: 'none', borderRadius: 'var(--radius-input)',
              fontSize: 14, fontWeight: 600, cursor: saving ? 'not-allowed' : 'pointer',
              transition: 'background 0.15s',
            }}
            onMouseEnter={(e) => { if (!saving) e.currentTarget.style.background = '#0958d9' }}
            onMouseLeave={(e) => { if (!saving) e.currentTarget.style.background = 'var(--color-primary)' }}
          >
            {saving ? '保存中...' : '保存配置'}
          </button>
        </div>
      </div>
    </div>
  )
}

export default ApiConfigPage
