import { useState } from 'react'
import MemoryPage from './MemoryPage'
import IdentityPage from './IdentityPage'

export default function MemoryIdentityPage() {
  const [tab, setTab] = useState<'memory' | 'identity'>('memory')

  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 16 }}>
        <h2 style={{ fontSize: 20, fontWeight: 700, margin: 0 }}>记忆图谱</h2>
        <div style={{ display: 'flex', gap: 2, background: 'var(--bg-muted)', borderRadius: 6, padding: 2 }}>
          <TabBtn active={tab === 'memory'} onClick={() => setTab('memory')} label="记忆" />
          <TabBtn active={tab === 'identity'} onClick={() => setTab('identity')} label="身份" />
        </div>
      </div>
      {tab === 'memory' ? <MemoryPage /> : <IdentityPage />}
    </div>
  )
}

function TabBtn({ active, onClick, label }: { active: boolean; onClick: () => void; label: string }) {
  return (
    <button onClick={onClick} style={{
      padding: '6px 14px', border: 'none', borderRadius: 4, cursor: 'pointer',
      fontSize: 13, fontWeight: active ? 600 : 400,
      background: active ? 'var(--bg-card)' : 'transparent',
      color: active ? 'var(--color-primary)' : 'var(--text-secondary)',
    }}>{label}</button>
  )
}
