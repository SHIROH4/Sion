interface ParamSliderProps {
  label: string
  value: number
  onChange: (v: number) => void
  min?: number
  max?: number
  step?: number
  disabled?: boolean
}

function ParamSlider({ label, value, onChange, min = 0, max = 1, step = 0.05, disabled }: ParamSliderProps) {
  return (
    <div style={{ padding: '6px 0' }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 6 }}>
        <span style={{ fontSize: 13, color: 'var(--text-secondary)' }}>{label}</span>
        <span style={{ width: 44, textAlign: 'right', fontSize: 13, fontWeight: 600, color: 'var(--text-primary)' }}>
          {value.toFixed(2)}
        </span>
      </div>
      <input
        type="range"
        min={min}
        max={max}
        step={step}
        value={value}
        onChange={(e) => onChange(parseFloat(e.target.value))}
        disabled={disabled}
        style={{
          width: '100%', height: 4,
          WebkitAppearance: 'none',
          appearance: 'none',
          background: '#e8e8e8',
          borderRadius: 2,
          outline: 'none',
          opacity: disabled ? 0.5 : 1,
          cursor: disabled ? 'not-allowed' : 'pointer',
        }}
        className="param-slider"
      />
    </div>
  )
}

export default ParamSlider
