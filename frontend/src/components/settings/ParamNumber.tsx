interface ParamNumberProps {
  label: string
  value: number
  onChange: (v: number) => void
  min?: number
  max?: number
  step?: number
  unit?: string
  disabled?: boolean
}

function ParamNumber({ label, value, onChange, min, max, step = 1, unit, disabled }: ParamNumberProps) {
  const dec = () => {
    const next = value - step
    if (min !== undefined && next < min) return
    onChange(next)
  }
  const inc = () => {
    const next = value + step
    if (max !== undefined && next > max) return
    onChange(next)
  }

  return (
    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '6px 0' }}>
      <span style={{ fontSize: 13, color: 'var(--text-secondary)' }}>{label}</span>
      <div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
        <button
          type="button"
          onClick={dec}
          disabled={disabled || (min !== undefined && value <= min)}
          style={{
            width: 28, height: 28, borderRadius: '50%',
            border: '1px solid var(--border-color)', background: '#fff',
            cursor: 'pointer', fontSize: 16, lineHeight: 1,
            display: 'flex', alignItems: 'center', justifyContent: 'center',
            color: 'var(--text-primary)',
          }}
        >
          {'−'}
        </button>
        <span style={{
          width: 48, textAlign: 'center',
          fontSize: 14, fontWeight: 600, color: 'var(--text-primary)',
        }}>
          {value}
        </span>
        <button
          type="button"
          onClick={inc}
          disabled={disabled || (max !== undefined && value >= max)}
          style={{
            width: 28, height: 28, borderRadius: '50%',
            border: '1px solid var(--border-color)', background: '#fff',
            cursor: 'pointer', fontSize: 16, lineHeight: 1,
            display: 'flex', alignItems: 'center', justifyContent: 'center',
            color: 'var(--text-primary)',
          }}
        >
          {'+'}
        </button>
        {unit && (
          <span style={{ fontSize: 12, color: 'var(--text-muted)', marginLeft: 4, width: 24 }}>
            {unit}
          </span>
        )}
      </div>
    </div>
  )
}

export default ParamNumber
