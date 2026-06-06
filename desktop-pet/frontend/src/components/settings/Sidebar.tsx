import { useEffect, useState } from 'react'
import { useSettingsStore } from '../../store/settingsStore'
import { getModels, ModelInfo } from '../../store/api'
import { usePetStore } from '../../store'

const menuItems = [
  { key: 'dashboard',      icon: '\u{1F4CA}', label: '仪表盘' },
  { key: 'observability',  icon: '\u{1F52D}', label: '执行状态' },
  { key: 'chat',           icon: '\u{1F4AC}', label: '对话面板' },
  { key: 'diary',          icon: '\u{1F4D6}', label: '日记' },
  { key: 'memory',         icon: '\u{1F9E0}', label: '记忆图谱' },
  { key: 'strategy-lab',   icon: '\u{1F9EA}', label: '策略实验室' },
  { key: 'api',            icon: '\u{2699}\u{FE0F}', label: 'API 配置' },
  { key: 'plugins',        icon: '\u{1F9E9}', label: '插件' },
]

function Sidebar() {
  const currentPage = useSettingsStore((s) => s.currentPage)
  const navigate = useSettingsStore((s) => s.navigate)

  const handleOpenPet = () => {
    try {
      const go = (window as any).go
      if (go?.main?.SettingsApp?.OpenPet) {
        go.main.SettingsApp.OpenPet()
      }
    } catch {
      // silently ignore if Wails binding not available
    }
  }

  return (
    <aside style={{
      width: 220, minWidth: 220, height: '100vh',
      background: 'var(--bg-sidebar)', borderRight: '1px solid var(--border-color)',
      display: 'flex', flexDirection: 'column', userSelect: 'none',
    }}>
      <div style={{ padding: 20 }}>
        <div style={{ fontSize: 16, fontWeight: 700, color: 'var(--text-primary)', display: 'flex', alignItems: 'center', gap: 8 }}>
          <span style={{ fontSize: 20 }}>{'\u{1F431}'}</span>
          {'诗音 · 设置'}
        </div>
        <div style={{ fontSize: 11, color: 'var(--text-muted)', marginTop: 2 }}>Desktop Pet v0.5.0</div>
      </div>

      <nav style={{ flex: 1, overflowY: 'auto', padding: '8px 0' }}>
        {menuItems.map((item) => {
          const active = currentPage === item.key
          return (
            <div key={item.key} onClick={() => navigate(item.key)}
              style={{
                display: 'flex', alignItems: 'center', padding: '10px 16px', margin: '2px 8px',
                borderRadius: 8, cursor: 'pointer', transition: 'all 0.15s ease',
                color: active ? 'var(--color-primary)' : 'var(--text-secondary)',
                background: active ? 'var(--color-primary-light)' : 'transparent',
                fontWeight: active ? 600 : 400,
              }}
              onMouseEnter={(e) => { if (!active) { e.currentTarget.style.background = '#e5e7eb'; e.currentTarget.style.color = 'var(--text-primary)' } }}
              onMouseLeave={(e) => { if (!active) { e.currentTarget.style.background = 'transparent'; e.currentTarget.style.color = 'var(--text-secondary)' } }}
            >
              <span style={{ fontSize: 16, width: 20, textAlign: 'center', flexShrink: 0 }}>{item.icon}</span>
              <span style={{ marginLeft: 12, fontSize: 14 }}>{item.label}</span>
            </div>
          )
        })}
      </nav>

      <ModelSelector />

      <div style={{ padding: '0 8px 16px' }}>
        <div style={{ borderTop: '1px solid var(--border-color)', margin: '8px 8px 12px' }} />
        <button onClick={handleOpenPet} style={{
          width: '100%', padding: '10px 16px', border: 'none', borderRadius: 8,
          background: 'var(--color-primary)', color: '#fff', fontWeight: 600,
          fontSize: 14, cursor: 'pointer', transition: 'background 0.15s ease',
        }} onMouseEnter={(e) => { e.currentTarget.style.background = '#0958d9' }}
          onMouseLeave={(e) => { e.currentTarget.style.background = 'var(--color-primary)' }}>
          {'\u{1F431} 打开宠物'}
        </button>
      </div>
    </aside>
  )
}

function ModelSelector() {
  const [models, setModels] = useState<ModelInfo[]>([])
  const modelPath = usePetStore((s) => s.modelPath)
  const setModelPath = usePetStore((s) => s.setModelPath)

  useEffect(() => { getModels().then(setModels).catch(() => {}) }, [])
  if (models.length < 2) return null

  return (
    <div style={{ padding: '8px 16px' }}>
      <div style={{ fontSize: 11, color: 'var(--text-muted)', marginBottom: 4 }}>模型</div>
      <select value={modelPath} onChange={(e) => { setModelPath(e.target.value); setTimeout(() => window.location.reload(), 200) }}
        style={{ width: '100%', padding: '6px 8px', borderRadius: 6, border: '1px solid var(--border-color)', fontSize: 12, background: 'var(--bg-card)', color: 'var(--text-primary)', cursor: 'pointer' }}>
        {models.map((m) => <option key={m.path} value={m.path}>{m.name}</option>)}
      </select>
    </div>
  )
}

export default Sidebar
