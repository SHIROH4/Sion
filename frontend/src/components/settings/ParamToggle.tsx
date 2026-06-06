interface ParamToggleProps {
  label: string
  value: boolean
  onChange: (v: boolean) => void
  disabled?: boolean
}

function ParamToggle({ label, value, onChange, disabled }: ParamToggleProps) {
  return (
    <div style={{
      display: 'flex', alignItems: 'center', justifyContent: 'space-between',
      padding: '6px 0',
    }}>
      <span style={{ fontSize: 13, color: 'var(--text-secondary)' }}>{label}</span>
      <button
        type="button"
        role="switch"
        aria-checked={value}
        disabled={disabled}
        onClick={() => onChange(!value)}
        style={{
          width: 40, height: 22,
          borderRadius: 11,
          border: 'none',
          background: value ? 'var(--color-primary)' : '#d9d9d9',
          cursor: disabled ? 'not-allowed' : 'pointer',
          opacity: disabled ? 0.5 : 1,
          position: 'relative',
          transition: 'background 0.2s',
          flexShrink: 0,
        }}
      >
        <span style={{
          position: 'absolute',
          top: 2, left: value ? 20 : 2,
          width: 18, height: 18,
          borderRadius: '50%',
          background: '#fff',
          boxShadow: '0 1px 3px rgba(0,0,0,0.2)',
          transition: 'left 0.2s',
        }} />
      </button>
    </div>
  )
}

export default ParamToggle
