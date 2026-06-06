import { useEffect, useState, useCallback } from 'react'

const API_BASE = 'http://127.0.0.1:19840'

interface DiaryItem {
  id: number
  content: string
  emotion_label: string
  emotion_score: number
  created_at: string
}

interface DiaryListResponse {
  diaries: DiaryItem[]
  total: number
}

async function fetchDiaries(page: number): Promise<DiaryListResponse> {
  const res = await fetch(`${API_BASE}/api/diaries?page=${page}&pageSize=20`)
  if (!res.ok) throw new Error(`HTTP ${res.status}`)
  return res.json()
}

function emotionEmoji(label: string): string {
  switch (label) {
    case 'happy': return '\u{1F60A}'
    case 'sad':   return '\u{1F615}'
    default:      return '\u{1F610}'
  }
}

function DiaryPage() {
  const [diaries, setDiaries] = useState<DiaryItem[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async (p: number) => {
    setLoading(true)
    setError(null)
    try {
      const data = await fetchDiaries(p)
      if (p === 0) {
        setDiaries(data.diaries)
      } else {
        setDiaries((prev) => [...prev, ...data.diaries])
      }
      setTotal(data.total)
    } catch (e: any) {
      setError(e?.message || '加载失败')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { load(0); setPage(0) }, [])

  const loadMore = () => {
    const next = page + 1
    setPage(next)
    load(next)
  }

  const hasMore = diaries.length < total

  return (
    <div>
      <div style={{ fontSize: 20, fontWeight: 600, color: 'var(--text-primary)', marginBottom: 24 }}>
        {'\u{1F4D3} 日记时间线'}
        <span style={{ fontSize: 12, color: 'var(--text-muted)', marginLeft: 8 }}>
          共 {total} 篇
        </span>
      </div>

      {error && (
        <div style={{ background: 'var(--bg-card)', borderRadius: 'var(--radius-card)', padding: 32, textAlign: 'center', marginBottom: 16 }}>
          <p style={{ color: 'var(--color-danger)' }}>{error}</p>
        </div>
      )}

      {!error && !loading && diaries.length === 0 && (
        <div style={{ background: 'var(--bg-card)', borderRadius: 'var(--radius-card)', padding: 48, textAlign: 'center' }}>
          <p style={{ color: 'var(--text-muted)' }}>暂无日记</p>
        </div>
      )}

      {/* Timeline */}
      <div style={{ position: 'relative', paddingLeft: 20 }}>
        {/* Vertical line */}
        <div style={{
          position: 'absolute', left: 5, top: 0, bottom: 0,
          width: 2, background: '#e8e8e8',
        }} />

        {diaries.map((d) => (
          <div key={d.id} style={{ position: 'relative', marginBottom: 20, paddingLeft: 20 }}>
            {/* Dot */}
            <div style={{
              position: 'absolute', left: -14, top: 8,
              width: 10, height: 10, borderRadius: '50%',
              background: 'var(--color-primary)',
              border: '2px solid #fff',
              boxShadow: '0 0 0 2px var(--color-primary)',
            }} />

            {/* Card */}
            <div style={{
              background: 'var(--bg-card)',
              borderRadius: 'var(--radius-card)',
              padding: 16,
              boxShadow: 'var(--shadow-card)',
            }}>
              <p style={{ fontSize: 14, color: 'var(--text-primary)', lineHeight: 1.7, margin: '0 0 10px' }}>
                {d.content}
              </p>
              <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                <span style={{ fontSize: 16 }}>
                  {emotionEmoji(d.emotion_label)}
                </span>
                <span style={{
                  fontSize: 13, fontWeight: 600,
                  color: d.emotion_score >= 0 ? 'var(--color-success)' : 'var(--color-warning)',
                }}>
                  {d.emotion_score > 0 ? '+' : ''}{d.emotion_score.toFixed(2)}
                </span>
              </div>
            </div>
          </div>
        ))}
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

export default DiaryPage
