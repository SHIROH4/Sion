import { useEffect, useState, useCallback } from 'react'
import { useSettingsStore } from '../../store/settingsStore'
import ParamToggle from '../../components/settings/ParamToggle'
import ParamNumber from '../../components/settings/ParamNumber'
import ParamSlider from '../../components/settings/ParamSlider'

type ConfigValue = boolean | number | string | ConfigObject
interface ConfigObject { [key: string]: ConfigValue }

function Collapse({ title, defaultOpen, children }: { title: string; defaultOpen?: boolean; children: React.ReactNode }) {
  const [open, setOpen] = useState(defaultOpen ?? false)

  return (
    <div style={{ marginBottom: 4 }}>
      <div
        onClick={() => setOpen(!open)}
        style={{
          display: 'flex', alignItems: 'center', gap: 6,
          padding: '8px 0',
          cursor: 'pointer',
          userSelect: 'none',
          borderBottom: '1px solid #f0f0f0',
        }}
      >
        <span style={{
          fontSize: 12, color: 'var(--text-muted)',
          transform: open ? 'rotate(90deg)' : 'rotate(0deg)',
          transition: 'transform 0.15s',
        }}>
          {'▶'}
        </span>
        <span style={{ fontSize: 14, fontWeight: 600, color: 'var(--text-primary)' }}>
          {title}
        </span>
      </div>
      {open && (
        <div style={{ paddingLeft: 16, paddingTop: 4 }}>
          {children}
        </div>
      )}
    </div>
  )
}

function isSliderValue(v: number): boolean {
  return v >= 0 && v <= 1
}

function isPasswordKey(key: string): boolean {
  return /secret|key|password/i.test(key)
}

function humanLabel(key: string): string {
  const map: Record<string, string> = {
    enabled: '启用',
    l0_threshold: 'L0 压缩阈值',
    merge_enabled: '合并开关',
    merge_similarity: '合并相似度阈值',
    qa_max_per_day: '每日提问上限',
    qa_max_pending: '待处理上限',
    history_days: '历史保留天数',
    diary_days: '日记保留天数',
    decay_threshold: '衰减阈值',
    interval_days: '更新间隔 (天)',
    interval_sec: '循环间隔 (秒)',
    interval_min: '间隔 (分钟)',
    screen_cool_down_sec: '截图冷却 (秒)',
    ocr_enabled: '本地 OCR',
    cloud_enabled: '云端分析',
    app_id: 'App ID',
    app_secret: 'App Secret',
    cooldown_min: '冷却 (分钟)',
    lunch_hour: '午餐整点',
    dinner_hour: '晚餐整点',
    continuous_work_min: '连续工作阈值 (分钟)',
    break_min: '建议休息 (分钟)',
    encourage: '鼓励',
    escalation: '升级提醒',
    max_daily: '每日上限',
    emotion_weight: '情绪权重',
    context_weight: '上下文权重',
    loneliness_threshold: '孤独触发阈值',
    worry_threshold: '担忧触发阈值',
    curiosity_threshold: '好奇触发阈值',
    dedup_window_min: '去重窗口 (分钟)',
    emotion_cloud_enabled: '云端情绪评估',
  }
  return map[key] ?? key
}

function humanUnit(key: string): string | undefined {
  if (/interval_sec|screen_cool_down_sec/.test(key)) return '秒'
  if (/interval_min|cooldown_min|continuous_work_min|break_min|dedup_window_min/.test(key)) return '分'
  if (/interval_days|history_days|diary_days/.test(key)) return '天'
  if (/hour/.test(key)) return '时'
  return undefined
}

function RenderField({ label, value, onChange }: {
  label: string
  value: ConfigValue
  onChange: (v: ConfigValue) => void
}) {
  if (typeof value === 'boolean') {
    return <ParamToggle label={humanLabel(label)} value={value} onChange={onChange} />
  }

  if (typeof value === 'number') {
    if (isSliderValue(value)) {
      return <ParamSlider label={humanLabel(label)} value={value} onChange={onChange} min={0} max={1} step={0.05} />
    }
    return (
      <ParamNumber
        label={humanLabel(label)}
        value={value}
        onChange={onChange}
        min={0}
        step={value >= 10 ? 1 : 1}
        unit={humanUnit(label)}
      />
    )
  }

  if (typeof value === 'string') {
    return (
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '6px 0' }}>
        <span style={{ fontSize: 13, color: 'var(--text-secondary)' }}>{humanLabel(label)}</span>
        <input
          type={isPasswordKey(label) ? 'password' : 'text'}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          style={{
            width: 200, height: 32,
            border: '1px solid var(--border-color)',
            borderRadius: 'var(--radius-input)',
            padding: '0 10px', fontSize: 13,
            outline: 'none',
          }}
          onFocus={(e) => {
            e.currentTarget.style.borderColor = 'var(--color-primary)'
            e.currentTarget.style.boxShadow = '0 0 0 2px rgba(22,119,255,0.1)'
          }}
          onBlur={(e) => {
            e.currentTarget.style.borderColor = 'var(--border-color)'
            e.currentTarget.style.boxShadow = 'none'
          }}
        />
      </div>
    )
  }

  if (typeof value === 'object' && value !== null) {
    return (
      <Collapse title={humanLabel(label)}>
        {Object.entries(value).map(([k, v]) => (
          <RenderField
            key={k}
            label={k}
            value={v}
            onChange={(newV) => {
              onChange({ ...value, [k]: newV })
            }}
          />
        ))}
      </Collapse>
    )
  }

  return null
}

function ConfigEditor({ config, onChange }: { config: ConfigObject; onChange: (c: ConfigObject) => void }) {
  const entries = Object.entries(config)
  const booleans = entries.filter(([, v]) => typeof v === 'boolean')
  const groups = entries.filter(([, v]) => typeof v === 'object' && v !== null)

  return (
    <div>
      {/* Master toggles at the top */}
      {booleans.length > 0 && (
        <div style={{ marginBottom: 12, paddingBottom: 12, borderBottom: '1px solid #f0f0f0' }}>
          {booleans.map(([key, value]) => (
            <RenderField
              key={key}
              label={key}
              value={value}
              onChange={(newVal) => onChange({ ...config, [key]: newVal })}
            />
          ))}
        </div>
      )}
      {/* Groups below */}
      {groups.map(([key, value]) => (
        <RenderField
          key={key}
          label={key}
          value={value}
          onChange={(newVal) => onChange({ ...config, [key]: newVal })}
        />
      ))}
    </div>
  )
}

// ---- API helpers (local until api.ts is extended) ----
const API_BASE = 'http://127.0.0.1:19840'

async function getPluginConfig(name: string): Promise<ConfigObject> {
  const res = await fetch(`${API_BASE}/api/plugins/${name}`)
  if (!res.ok) throw new Error(`API returned ${res.status}`)
  return res.json()
}

async function savePluginConfig(name: string, cfg: ConfigObject): Promise<void> {
  const res = await fetch(`${API_BASE}/api/plugins/${name}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(cfg),
  })
  if (!res.ok) throw new Error(`API returned ${res.status}`)
}

// ---- Page ----

function PluginConfigPage() {
  const pluginConfigName = useSettingsStore((s) => s.pluginConfigName)
  const navigate = useSettingsStore((s) => s.navigate)

  const [config, setConfig] = useState<ConfigObject | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [toast, setToast] = useState<{ type: 'success' | 'error'; msg: string } | null>(null)

  const showToast = (type: 'success' | 'error', msg: string) => {
    setToast({ type, msg })
    setTimeout(() => setToast(null), 2500)
  }

  useEffect(() => {
    if (!pluginConfigName) {
      navigate('plugins')
      return
    }
    getPluginConfig(pluginConfigName)
      .then(setConfig)
      .catch((e) => showToast('error', '加载失败: ' + (e?.message || e)))
      .finally(() => setLoading(false))
  }, [pluginConfigName])

  const handleSave = useCallback(async () => {
    if (!pluginConfigName || !config) return
    setSaving(true)
    try {
      await savePluginConfig(pluginConfigName, config)
      showToast('success', '配置已保存 ✓')
    } catch (e: any) {
      showToast('error', '保存失败: ' + (e?.message || e))
    } finally {
      setSaving(false)
    }
  }, [pluginConfigName, config])

  if (!pluginConfigName) return null

  if (loading) {
    return (
      <div style={{ background: 'var(--bg-card)', borderRadius: 'var(--radius-card)', padding: 48, textAlign: 'center' }}>
        <p style={{ color: 'var(--text-secondary)' }}>加载配置中...</p>
      </div>
    )
  }

  return (
    <div style={{ position: 'relative', maxWidth: 640 }}>
      {toast && (
        <div style={{
          position: 'fixed', top: 20, right: 20, zIndex: 1000,
          padding: '10px 20px', borderRadius: 6, fontSize: 14, fontWeight: 500,
          color: '#fff',
          background: toast.type === 'success' ? 'var(--color-success)' : 'var(--color-danger)',
          boxShadow: '0 4px 12px rgba(0,0,0,0.2)',
          animation: 'slideIn 0.3s ease',
        }}>
          {toast.msg}
        </div>
      )}

      {/* Back + title */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 24 }}>
        <button
          onClick={() => navigate('plugins')}
          style={{
            border: '1px solid var(--border-color)',
            borderRadius: 'var(--radius-input)',
            background: '#fff',
            padding: '6px 12px',
            cursor: 'pointer',
            fontSize: 13,
            color: 'var(--text-secondary)',
          }}
        >
          {'← 返回'}
        </button>
        <span style={{ fontSize: 18, fontWeight: 600, color: 'var(--text-primary)' }}>
          {'\u{1F9E9} '}{pluginConfigName} 配置
        </span>
      </div>

      {/* Config editor */}
      {config && (
        <div style={{
          background: 'var(--bg-card)',
          borderRadius: 'var(--radius-card)',
          padding: 20,
          boxShadow: 'var(--shadow-card)',
          marginBottom: 20,
        }}>
          <ConfigEditor config={config} onChange={setConfig} />
        </div>
      )}

      {/* Save */}
      <div style={{ paddingTop: 16, borderTop: '1px solid var(--border-color)' }}>
        <button
          onClick={handleSave}
          disabled={saving}
          style={{
            padding: '10px 28px',
            background: saving ? '#9cb8e0' : 'var(--color-primary)',
            color: '#fff',
            border: 'none',
            borderRadius: 'var(--radius-input)',
            fontSize: 14,
            fontWeight: 600,
            cursor: saving ? 'not-allowed' : 'pointer',
          }}
        >
          {saving ? '保存中...' : '保存配置'}
        </button>
      </div>
    </div>
  )
}

export default PluginConfigPage
