import { useEffect, useState } from 'react'

const API_BASE = 'http://127.0.0.1:19840'

interface IdentityNode {
  id: number
  category: string
  content: string
  weight: number
  updated_at: number
}

const CATEGORIES: { key: string; label: string; color: string }[] = [
  { key: 'core_value',    label: '\u{6838}\u{5FC3}\u{4EF7}\u{503C}\u{89C2}', color: '#ff4d4f' },
  { key: 'preference',    label: '\u{504F}\u{597D}', color: '#fa8c16' },
  { key: 'belief',        label: '\u{4FE1}\u{5FF5}', color: '#1677ff' },
  { key: 'relationship',  label: '\u{5173}\u{7CFB}', color: '#52c41a' },
  { key: 'behavior_rule', label: '\u{884C}\u{4E3A}\u{51C6}\u{5219}', color: '#722ed1' },
  { key: 'goal',          label: '\u{76EE}\u{6807}', color: '#eb2f96' },
  { key: 'fear',          label: '\u{6050}\u{60E7}', color: '#595959' },
]

function IdentityPage() {
  const [nodes, setNodes] = useState<IdentityNode[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [editing, setEditing] = useState<number | null>(null)
  const [editText, setEditText] = useState('')
  const [saving, setSaving] = useState(false)

  const load = async () => {
    try {
      const res = await fetch(`${API_BASE}/api/identity`)
      if (!res.ok) throw new Error(`HTTP ${res.status}`)
      const data = await res.json()
      setNodes(data)
      setError(null)
    } catch (e: any) {
      setError(e?.message || '加载失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { load() }, [])

  const startEdit = (node: IdentityNode) => {
    setEditing(node.id)
    setEditText(node.content)
  }

  const cancelEdit = () => {
    setEditing(null)
    setEditText('')
  }

  const saveEdit = async (node: IdentityNode) => {
    setSaving(true)
    try {
      const res = await fetch(`${API_BASE}/api/identity/${node.id}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ content: editText, category: node.category }),
      })
      if (!res.ok) throw new Error(`HTTP ${res.status}`)
      setNodes((prev) => prev.map((n) => n.id === node.id ? { ...n, content: editText } : n))
      setEditing(null)
    } catch (e: any) {
      alert('保存失败: ' + (e?.message || e))
    } finally {
      setSaving(false)
    }
  }

  if (loading) {
    return (
      <div style={{ background: 'var(--bg-card)', borderRadius: 'var(--radius-card)', padding: 48, textAlign: 'center' }}>
        <p style={{ color: 'var(--text-secondary)' }}>加载中...</p>
      </div>
    )
  }

  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 24 }}>
        <span style={{ fontSize: 20, fontWeight: 600, color: 'var(--text-primary)' }}>
          {'\u{1FAAA} \u{8EAB}\u{4EFD}\u{56FE}\u{8C31}'}
        </span>
        <button
          onClick={async () => {
            try {
              const res = await fetch(`${API_BASE}/api/identity/self-update`, { method: 'POST' })
              if (!res.ok) throw new Error(`HTTP ${res.status}`)
              alert('\u{81EA}\u{6211}\u{66F4}\u{65B0}\u{5DF2}\u{89E6}\u{53D1}')
            } catch { alert('\u{81EA}\u{6211}\u{66F4}\u{65B0}\u{5C1A}\u{672A}\u{5B9E}\u{73B0}') }
          }}
          style={{
            padding: '8px 16px', border: '1px solid var(--color-primary)',
            borderRadius: 'var(--radius-input)', background: '#fff',
            color: 'var(--color-primary)', fontSize: 13, cursor: 'pointer',
          }}
        >
          {'\u{89E6}\u{53D1}\u{81EA}\u{6211}\u{66F4}\u{65B0}'}
        </button>
      </div>

      {error && (
        <div style={{ background: 'var(--bg-card)', borderRadius: 'var(--radius-card)', padding: 32, textAlign: 'center', marginBottom: 16 }}>
          <p style={{ color: 'var(--color-danger)' }}>{error}</p>
        </div>
      )}

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(2, 1fr)', gap: 16 }}>
        {CATEGORIES.map((cat) => {
          const catNodes = nodes.filter((n) => n.category === cat.key)
          return (
            <div
              key={cat.key}
              style={{
                background: 'var(--bg-card)',
                borderRadius: 'var(--radius-card)',
                padding: 16,
                boxShadow: 'var(--shadow-card)',
                borderLeft: `3px solid ${cat.color}`,
              }}
            >
              <div style={{ fontSize: 15, fontWeight: 600, color: 'var(--text-primary)', marginBottom: 12 }}>
                {cat.label}
                <span style={{ fontSize: 11, color: 'var(--text-muted)', marginLeft: 6 }}>
                  {catNodes.length}
                </span>
              </div>
              {catNodes.length === 0 && (
                <p style={{ fontSize: 13, color: 'var(--text-muted)' }}>\u{6682}\u{65E0}</p>
              )}
              {catNodes.map((node) => (
                <div key={node.id} style={{ marginBottom: 8, paddingBottom: 8, borderBottom: '1px solid #f5f5f5' }}>
                  {editing === node.id ? (
                    <div>
                      <textarea
                        value={editText}
                        onChange={(e) => setEditText(e.target.value)}
                        rows={3}
                        style={{
                          width: '100%', resize: 'vertical',
                          border: '1px solid var(--border-color)', borderRadius: 'var(--radius-input)',
                          padding: 8, fontSize: 13, outline: 'none', fontFamily: 'inherit',
                        }}
                      />
                      <div style={{ display: 'flex', gap: 6, marginTop: 6 }}>
                        <button
                          onClick={() => saveEdit(node)}
                          disabled={saving}
                          style={{
                            padding: '4px 12px', border: 'none',
                            borderRadius: 'var(--radius-input)',
                            background: 'var(--color-primary)', color: '#fff',
                            fontSize: 12, cursor: 'pointer',
                          }}
                        >
                          {saving ? '...' : '\u{4FDD}\u{5B58}'}
                        </button>
                        <button
                          onClick={cancelEdit}
                          style={{
                            padding: '4px 12px', border: '1px solid var(--border-color)',
                            borderRadius: 'var(--radius-input)',
                            background: '#fff', fontSize: 12, cursor: 'pointer',
                          }}
                        >
                          {'\u{53D6}\u{6D88}'}
                        </button>
                      </div>
                    </div>
                  ) : (
                    <div
                      onClick={() => startEdit(node)}
                      style={{ cursor: 'pointer', fontSize: 13, color: 'var(--text-primary)', lineHeight: 1.6 }}
                      title={'\u{70B9}\u{51FB}\u{7F16}\u{8F91}'}
                    >
                      {node.content}
                    </div>
                  )}
                </div>
              ))}
            </div>
          )
        })}
      </div>
    </div>
  )
}

export default IdentityPage
