const LAYERS = [
  { key: '',     label: '全部' },
  { key: 'L0',   label: 'L0 工作记忆' },
  { key: 'L1',   label: 'L1 日记' },
  { key: 'L2',   label: 'L2 语义事实' },
  { key: 'L3',   label: 'L3 核心自我' },
]

const LAYER_COLORS: Record<string, string> = {
  L0: '#52c41a',
  L1: '#1677ff',
  L2: '#722ed1',
  L3: '#eb2f96',
}

interface LayerTabsProps {
  active: string
  onChange: (layer: string) => void
}

function LayerTabs({ active, onChange }: LayerTabsProps) {
  return (
    <div style={{ display: 'flex', gap: 0, marginBottom: 20 }}>
      {LAYERS.map((l) => {
        const isActive = active === l.key
        return (
          <button
            key={l.key}
            onClick={() => onChange(l.key)}
            style={{
              padding: '8px 16px',
              border: 'none',
              borderBottom: isActive ? '2px solid var(--color-primary)' : '2px solid transparent',
              background: 'transparent',
              color: isActive ? 'var(--color-primary)' : 'var(--text-secondary)',
              fontSize: 13,
              fontWeight: isActive ? 600 : 400,
              cursor: 'pointer',
              transition: 'color 0.15s, border-color 0.15s',
            }}
          >
            {l.label}
          </button>
        )
      })}
    </div>
  )
}

export { LAYER_COLORS }
export default LayerTabs
