interface StatCardProps {
  icon: string
  title: string
  value: string
  color: string
}

function StatCard({ icon, title, value, color }: StatCardProps) {
  return (
    <div
      style={{
        background: 'var(--bg-card)',
        borderRadius: 'var(--radius-card)',
        padding: 20,
        borderLeft: `3px solid ${color}`,
        boxShadow: 'var(--shadow-card)',
        cursor: 'default',
        transition: 'transform 0.2s ease, box-shadow 0.2s ease',
      }}
      onMouseEnter={(e) => {
        e.currentTarget.style.transform = 'translateY(-2px)'
        e.currentTarget.style.boxShadow = 'var(--shadow-card-hover)'
      }}
      onMouseLeave={(e) => {
        e.currentTarget.style.transform = ''
        e.currentTarget.style.boxShadow = 'var(--shadow-card)'
      }}
    >
      <div style={{ display: 'flex', alignItems: 'center', marginBottom: 8 }}>
        <span style={{ fontSize: 18, marginRight: 6 }}>{icon}</span>
        <span style={{ fontSize: 13, color: 'var(--text-secondary)' }}>{title}</span>
      </div>
      <div style={{ fontSize: 28, fontWeight: 700, color: 'var(--text-primary)' }}>
        {value}
      </div>
    </div>
  )
}

export default StatCard
