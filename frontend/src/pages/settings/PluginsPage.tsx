import { useEffect, useState } from 'react'
import { getPlugins, togglePlugin, PluginInfo } from '../../store/api'
import { useSettingsStore } from '../../store/settingsStore'
import PluginCard from '../../components/settings/PluginCard'

function PluginsPage() {
  const [plugins, setPlugins] = useState<PluginInfo[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const navigate = useSettingsStore((s) => s.navigate)
  const openPluginConfig = useSettingsStore((s) => s.openPluginConfig)

  const loadPlugins = () => {
    getPlugins()
      .then(setPlugins)
      .catch((e) => setError(e?.message || '加载失败'))
      .finally(() => setLoading(false))
  }

  useEffect(() => { loadPlugins() }, [])

  const handleToggle = (p: PluginInfo) => {
    togglePlugin(p.name)
      .then(() => loadPlugins())
      .catch((e) => console.warn('Toggle failed:', e))
  }

  if (loading) {
    return (
      <div style={{ background: 'var(--bg-card)', borderRadius: 'var(--radius-card)', padding: 48, textAlign: 'center' }}>
        <p style={{ color: 'var(--text-secondary)' }}>加载中...</p>
      </div>
    )
  }

  if (error) {
    return (
      <div style={{ background: 'var(--bg-card)', borderRadius: 'var(--radius-card)', padding: 48, textAlign: 'center' }}>
        <p style={{ color: 'var(--color-danger)' }}>{error}</p>
      </div>
    )
  }

  return (
    <div>
      <div style={{ fontSize: 20, fontWeight: 600, color: 'var(--text-primary)', marginBottom: 24 }}>
        {'\u{1F9E9} 插件中心'}
      </div>
      <div style={{
        display: 'grid',
        gridTemplateColumns: 'repeat(2, 1fr)',
        gap: 16,
      }}>
        {plugins.map((p) => (
          <PluginCard
            key={p.name}
            plugin={p}
            onConfigure={() => {
              navigate('plugin-config')
              openPluginConfig(p.name)
            }}
            onToggle={() => handleToggle(p)}
          />
        ))}
      </div>
    </div>
  )
}

export default PluginsPage
