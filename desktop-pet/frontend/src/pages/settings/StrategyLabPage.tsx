import { useEffect, useState, useCallback } from 'react'
import { getStrategies, StrategyViewModel } from '../../store/api'

export default function StrategyLabPage() {
  const [strategies, setStrategies] = useState<StrategyViewModel[]>([])

  const refresh = useCallback(() => {
    getStrategies().then(setStrategies).catch(() => {})
  }, [])

  useEffect(() => { refresh() }, [refresh])

  const active = strategies.filter((s) => s.active)
  const inactive = strategies.filter((s) => !s.active)

  return (
    <div>
      <h2 style={{ fontSize: 20, fontWeight: 700, marginBottom: 4 }}>策略实验室</h2>
      <p style={{ fontSize: 13, color: 'var(--text-secondary)', marginBottom: 20 }}>
        猫娘从互动中自主学习的行为策略。活跃策略影响 LLM 兜底决策。
      </p>

      {strategies.length === 0 && (
        <div style={{ color: 'var(--text-muted)', padding: 40, textAlign: 'center' }}>
          还没有学到任何策略。多互动几天，猫娘会从经验中总结策略。
        </div>
      )}

      {active.length > 0 && (
        <>
          <h3 style={{ fontSize: 14, marginBottom: 8, color: 'var(--text-secondary)' }}>
            活跃策略 ({active.length})
          </h3>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 12, marginBottom: 24 }}>
            {active.map((s) => <StrategyCard key={s.id} s={s} />)}
          </div>
        </>
      )}

      {inactive.length > 0 && (
        <>
          <h3 style={{ fontSize: 14, marginBottom: 8, color: 'var(--text-muted)' }}>
            已停用 ({inactive.length})
          </h3>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 12, opacity: 0.6 }}>
            {inactive.map((s) => <StrategyCard key={s.id} s={s} />)}
          </div>
        </>
      )}
    </div>
  )
}

function StrategyCard({ s }: { s: StrategyViewModel }) {
  return (
    <div style={{
      background: 'var(--bg-card)', border: '1px solid var(--border-color)',
      borderRadius: 8, padding: 16,
    }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: 8 }}>
        <div style={{ fontSize: 14, fontWeight: 600, flex: 1 }}>{s.situation}</div>
        <div style={{ display: 'flex', gap: 6, alignItems: 'center', flexShrink: 0, marginLeft: 12 }}>
          <span style={{
            fontSize: 11, padding: '2px 8px', borderRadius: 4,
            background: s.confidence > 0.7 ? '#dcfce7' : s.confidence > 0.4 ? '#fef9c3' : '#fee2e2',
            color: s.confidence > 0.7 ? '#166534' : s.confidence > 0.4 ? '#854d0e' : '#991b1b',
            fontWeight: 600,
          }}>置信度 {(s.confidence * 100).toFixed(0)}%</span>
          <span style={{ fontSize: 10, color: 'var(--text-muted)', background: 'var(--bg-muted)', padding: '2px 6px', borderRadius: 3 }}>
            {sourceLabel(s.source)}
          </span>
        </div>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 8, marginBottom: 6 }}>
        <div>
          <div style={{ fontSize: 11, color: '#16a34a', marginBottom: 2 }}>✅ 好策略</div>
          <div style={{ fontSize: 13 }}>{s.good_strategy}</div>
        </div>
        {s.bad_strategy && (
          <div>
            <div style={{ fontSize: 11, color: '#dc2626', marginBottom: 2 }}>❌ 坏策略</div>
            <div style={{ fontSize: 13 }}>{s.bad_strategy}</div>
          </div>
        )}
      </div>

      {s.reason && (
        <div style={{ fontSize: 12, color: 'var(--text-muted)', fontStyle: 'italic' }}>
          {s.reason}
        </div>
      )}
    </div>
  )
}

function sourceLabel(s: string) {
  const m: Record<string, string> = { daily_reflection: '日反思', immediate_feedback: '即时反馈' }
  return m[s] || s
}
