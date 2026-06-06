import { PluginInfo } from '../../store/api'

interface PluginCardProps {
  plugin: PluginInfo
  onConfigure: () => void
  onToggle: () => void
}

function PluginCard({ plugin, onConfigure, onToggle }: PluginCardProps) {
  const isCore = plugin.name === 'chat'

  return (
    <div style={{
      background: 'var(--bg-card)',
      borderRadius: 'var(--radius-card)',
      padding: 20,
      boxShadow: 'var(--shadow-card)',
      display: 'flex',
      flexDirection: 'column',
      transition: 'box-shadow 0.2s',
    }}
      onMouseEnter={(e) => { e.currentTarget.style.boxShadow = 'var(--shadow-card-hover)' }}
      onMouseLeave={(e) => { e.currentTarget.style.boxShadow = 'var(--shadow-card)' }}
    >
      {/* Header */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 10 }}>
        <span style={{
          width: 8, height: 8, borderRadius: '50%',
          background: plugin.running ? 'var(--color-success)' : '#d9d9d9',
          flexShrink: 0,
        }} />
        <span style={{ fontSize: 15, fontWeight: 600, color: 'var(--text-primary)' }}>
          {plugin.name}
        </span>
        <span style={{
          fontSize: 11, color: 'var(--text-muted)',
          background: '#f0f0f0', padding: '2px 6px', borderRadius: 4,
        }}>
          v{plugin.version}
        </span>
        <span style={{ fontSize: 12, color: plugin.running ? 'var(--color-success)' : 'var(--text-muted)', marginLeft: 'auto' }}>
          {plugin.running ? '运行中' : '已停止'}
        </span>
      </div>

      {/* Description */}
      <p style={{ fontSize: 13, color: 'var(--text-secondary)', margin: '0 0 8px', lineHeight: 1.5, flex: 1 }}>
        {plugin.description}
      </p>

      {/* Requires */}
      {(plugin.requires?.length ?? 0) > 0 && (
        <p style={{ fontSize: 11, color: 'var(--text-muted)', margin: '0 0 12px' }}>
          依赖: {plugin.requires.join(', ')}
        </p>
      )}

      {/* Actions */}
      <div style={{ display: 'flex', gap: 8 }}>
        <button
          onClick={onConfigure}
          style={{
            padding: '6px 14px',
            border: '1px solid var(--border-color)',
            borderRadius: 'var(--radius-input)',
            background: '#fff',
            color: 'var(--text-primary)',
            fontSize: 13,
            cursor: 'pointer',
            transition: 'border-color 0.15s',
          }}
          onMouseEnter={(e) => { e.currentTarget.style.borderColor = 'var(--color-primary)' }}
          onMouseLeave={(e) => { e.currentTarget.style.borderColor = 'var(--border-color)' }}
        >
          配置
        </button>
        <button
          onClick={onToggle}
          disabled={isCore}
          style={{
            padding: '6px 14px',
            border: 'none',
            borderRadius: 'var(--radius-input)',
            background: isCore ? '#f0f0f0' : plugin.running ? '#fff1f0' : 'var(--color-primary)',
            color: isCore ? 'var(--text-muted)' : plugin.running ? 'var(--color-danger)' : '#fff',
            fontSize: 13,
            fontWeight: 600,
            cursor: isCore ? 'not-allowed' : 'pointer',
            transition: 'opacity 0.15s',
          }}
        >
          {isCore ? '核心插件' : plugin.running ? '停止' : '启动'}
        </button>
      </div>
    </div>
  )
}

export default PluginCard
