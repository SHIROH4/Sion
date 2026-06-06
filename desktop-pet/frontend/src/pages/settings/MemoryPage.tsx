import { useEffect, useState, useCallback } from 'react'
import LayerTabs, { LAYER_COLORS } from '../../components/settings/LayerTabs'
import { useSettingsStore } from '../../store/settingsStore'

const API_BASE = 'http://127.0.0.1:19840'

interface MemoryItem {
  id: string
  layer: string
  content: string
  weight: number
  created_at?: string
}

interface MemoryListResponse {
  memories: MemoryItem[]
  total: number
}

async function fetchMemories(layer: string, page: number, query?: string): Promise<MemoryListResponse> {
  const params = new URLSearchParams({ page: String(page), pageSize: '20' })
  if (layer) params.set('layer', layer)
  if (query) params.set('query', query)
  const res = await fetch(`${API_BASE}/api/memories?${params}`)
  if (!res.ok) throw new Error(`HTTP ${res.status}`)
  return res.json()
}

async function deleteMemory(id: string): Promise<void> {
  const res = await fetch(`${API_BASE}/api/memories/${id}`, { method: 'DELETE' })
  if (!res.ok) throw new Error(`HTTP ${res.status}`)
}

function MemoryPage() {
  const [layer, setLayer] = useState('')
  const [memories, setMemories] = useState<MemoryItem[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [query, setQuery] = useState('')
  const [searchInput, setSearchInput] = useState('')
  const [expanded, setExpanded] = useState<string | null>(null)
  const navigate = useSettingsStore((s) => s.navigate)

  const load = useCallback(async (l: string, p: number, q: string) => {
    setLoading(true)
    setError(null)
    try {
      const data = await fetchMemories(l, p, q || undefined)
      if (p === 0) {
        setMemories(data.memories)
      } else {
        setMemories((prev) => [...prev, ...data.memories])
      }
      setTotal(data.total)
    } catch (e: any) {
      setError(e?.message || '加载失败')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    setPage(0)
    load(layer, 0, query)
  }, [layer, query])

  const handleSearch = () => {
    setQuery(searchInput)
  }

  const handleDelete = async (id: string) => {
    if (!confirm('确定删除这条记忆？')) return
    try {
      await deleteMemory(id)
      setMemories((prev) => prev.filter((m) => m.id !== id))
      setTotal((t) => t - 1)
    } catch (e: any) {
      setError(e?.message || '删除失败')
    }
  }

  const loadMore = () => {
    const next = page + 1
    setPage(next)
    load(layer, next, query)
  }

  const hasMore = memories.length < total

  return (
    <div>
      <div style={{ fontSize: 20, fontWeight: 600, color: 'var(--text-primary)', marginBottom: 16 }}>
        {'\u{1F9E0} 记忆管理'}
        <span style={{ fontSize: 12, color: 'var(--text-muted)', marginLeft: 8 }}>
          共 {total} 条
        </span>
      </div>

      {/* Tabs */}
      <LayerTabs active={layer} onChange={setLayer} />

      {/* Search */}
      <div style={{ display: 'flex', gap: 8, marginBottom: 20 }}>
        <input
          type="text"
          value={searchInput}
          onChange={(e) => setSearchInput(e.target.value)}
          onKeyDown={(e) => { if (e.key === 'Enter') handleSearch() }}
          placeholder="搜索记忆..."
          style={{
            flex: 1, height: 36,
            border: '1px solid var(--border-color)',
            borderRadius: 'var(--radius-input)',
            padding: '0 12px', fontSize: 14, outline: 'none', maxWidth: 400,
          }}
        />
        <button
          onClick={handleSearch}
          style={{
            padding: '0 16px', height: 36,
            border: '1px solid var(--border-color)',
            borderRadius: 'var(--radius-input)',
            background: '#fff', fontSize: 13, cursor: 'pointer',
          }}
        >
          搜索
        </button>
      </div>

      {/* Content */}
      {error && (
        <div style={{ background: 'var(--bg-card)', borderRadius: 'var(--radius-card)', padding: 32, textAlign: 'center', marginBottom: 16 }}>
          <p style={{ color: 'var(--color-danger)' }}>{error}</p>
        </div>
      )}

      {!error && !loading && memories.length === 0 && (
        <div style={{ background: 'var(--bg-card)', borderRadius: 'var(--radius-card)', padding: 48, textAlign: 'center' }}>
          <p style={{ color: 'var(--text-muted)' }}>暂无记忆数据</p>
        </div>
      )}

      <div style={{
        display: 'grid',
        gridTemplateColumns: 'repeat(2, 1fr)',
        gap: 12,
      }}>
        {memories.map((m) => {
          const isOpen = expanded === m.id
          return (
            <div
              key={m.id}
              style={{
                background: 'var(--bg-card)',
                borderRadius: 'var(--radius-card)',
                padding: 16,
                boxShadow: 'var(--shadow-card)',
                borderLeft: `3px solid ${LAYER_COLORS[m.layer] || '#999'}`,
              }}
            >
              {/* Layer badge + time */}
              <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8 }}>
                <span style={{
                  fontSize: 11, fontWeight: 600, color: '#fff',
                  background: LAYER_COLORS[m.layer] || '#999',
                  padding: '2px 8px', borderRadius: 4,
                }}>
                  {m.layer}
                </span>
                {m.created_at && (
                  <span style={{ fontSize: 11, color: 'var(--text-muted)', marginLeft: 'auto' }}>
                    {m.created_at}
                  </span>
                )}
              </div>

              {/* Content */}
              <p style={{
                fontSize: 13, color: 'var(--text-primary)',
                lineHeight: 1.6, margin: '0 0 8px',
                whiteSpace: isOpen ? 'pre-wrap' : 'normal',
                overflow: isOpen ? 'visible' : 'hidden',
                display: isOpen ? 'block' : '-webkit-box',
                WebkitLineClamp: isOpen ? undefined : 3,
                WebkitBoxOrient: 'vertical',
              }}>
                {m.content}
              </p>

              {/* Weight bar */}
              <div style={{ height: 3, background: '#f0f0f0', borderRadius: 2, marginBottom: 10, overflow: 'hidden' }}>
                <div style={{
                  width: (m.weight * 100) + '%',
                  height: '100%',
                  background: LAYER_COLORS[m.layer] || '#999',
                  borderRadius: 2,
                  transition: 'width 0.3s',
                }} />
              </div>

              {/* Actions */}
              <div style={{ display: 'flex', gap: 8 }}>
                <button
                  onClick={() => setExpanded(isOpen ? null : m.id)}
                  style={{
                    border: '1px solid var(--border-color)',
                    borderRadius: 'var(--radius-input)',
                    background: '#fff', fontSize: 12, padding: '4px 10px',
                    cursor: 'pointer', color: 'var(--text-secondary)',
                  }}
                >
                  {isOpen ? '收起' : '展开'}
                </button>
                {m.layer === 'L2' && (
                  <button
                    onClick={() => handleDelete(m.id)}
                    style={{
                      border: 'none',
                      borderRadius: 'var(--radius-input)',
                      background: '#fff1f0', fontSize: 12, padding: '4px 10px',
                      cursor: 'pointer', color: 'var(--color-danger)',
                      marginLeft: 'auto',
                    }}
                  >
                    删除
                  </button>
                )}
              </div>
            </div>
          )
        })}
      </div>

      {loading && (
        <div style={{ textAlign: 'center', padding: 20, color: 'var(--text-muted)' }}>加载中...</div>
      )}
      {hasMore && !loading && (
        <div style={{ textAlign: 'center', padding: 20 }}>
          <button onClick={loadMore} style={{
            padding: '8px 24px',
            border: '1px solid var(--border-color)',
            borderRadius: 'var(--radius-input)',
            background: '#fff', fontSize: 13, cursor: 'pointer',
          }}>
            加载更多
          </button>
        </div>
      )}
    </div>
  )
}

export default MemoryPage
