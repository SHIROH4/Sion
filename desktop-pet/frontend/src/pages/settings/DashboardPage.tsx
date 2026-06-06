import { useEffect, useState, useCallback } from 'react'
import { getFeatures, getLearningOverview, getEmotion, FeaturesViewModel, LearningOverview, EmotionViewModel } from '../../store/api'
import RadarChart from '../../components/settings/RadarChart'
import StatCard from '../../components/settings/StatCard'

type Tab = 'overview' | 'emotion' | 'decision'

export default function DashboardPage() {
  const [tab, setTab] = useState<Tab>('overview')
  const [features, setFeatures] = useState<FeaturesViewModel | null>(null)
  const [learning, setLearning] = useState<LearningOverview | null>(null)
  const [emotion, setEmotion] = useState<EmotionViewModel | null>(null)

  const refresh = useCallback(() => {
    getFeatures().then(setFeatures).catch(() => {})
    getLearningOverview().then(setLearning).catch(() => {})
    getEmotion().then(setEmotion).catch(() => {})
  }, [])

  useEffect(() => { refresh(); const t = setInterval(refresh, 5000); return () => clearInterval(t) }, [refresh])

  const tabs: { key: Tab; label: string }[] = [
    { key: 'overview', label: '概览' },
    { key: 'emotion', label: '情绪' },
    { key: 'decision', label: '决策' },
  ]

  return (
    <div>
      <h2 style={{ fontSize: 20, fontWeight: 700, marginBottom: 16 }}>仪表盘</h2>

      {/* Tab bar */}
      <div style={{ display: 'flex', gap: 4, marginBottom: 20, borderBottom: '1px solid var(--border-color)' }}>
        {tabs.map((t) => (
          <button key={t.key}
            onClick={() => setTab(t.key)}
            style={{
              padding: '8px 16px', border: 'none', background: 'none', cursor: 'pointer',
              fontSize: 14, fontWeight: tab === t.key ? 600 : 400,
              color: tab === t.key ? 'var(--color-primary)' : 'var(--text-secondary)',
              borderBottom: tab === t.key ? '2px solid var(--color-primary)' : '2px solid transparent',
              transition: 'all 0.15s',
            }}
          >{t.label}</button>
        ))}
      </div>

      {tab === 'overview' && <OverviewTab features={features} learning={learning} />}
      {tab === 'emotion' && <EmotionTab emotion={emotion} features={features} />}
      {tab === 'decision' && <DecisionTab features={features} learning={learning} />}
    </div>
  )
}

// ---- Overview Tab ----

function OverviewTab({ features, learning }: { features: FeaturesViewModel | null; learning: LearningOverview | null }) {
  if (!features) return <Loading />

  return (
    <div>
      {/* Top stats row */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 12, marginBottom: 20 }}>
        <StatCard icon="📊" title="接受率" value={`${features.relationship.overall_accept_rate > 0 ? (features.relationship.overall_accept_rate * 100).toFixed(0) : '--'}%`} color="#1677ff" />
        <StatCard icon="💻" title="连续工作" value={features.user.continuous_work_min > 0 ? `${features.user.continuous_work_min.toFixed(0)}分钟` : '未工作'} color="#f59e0b" />
        <StatCard icon="📨" title="今日行动" value={`${learning?.metrics.total_today ?? '--'}次`} color="#10b981" />
        <StatCard icon="⚠️" title="最近拒绝" value={`${features.relationship.recent_rejections}次`} color="#ef4444" />
      </div>

      {/* Drives radar + Needs bars */}
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 20 }}>
        <div>
          <h3 style={{ fontSize: 14, marginBottom: 8, color: 'var(--text-secondary)' }}>5维驱力</h3>
          <RadarChart
            size={180}
            data={[
              { label: '社交', value: features.drives?.social ?? 0, color: '#f472b6' },
              { label: '关怀', value: features.drives?.care ?? 0, color: '#ef4444' },
              { label: '好奇', value: features.drives?.curious ?? 0, color: '#60a5fa' },
              { label: '安静', value: features.drives?.quiet ?? 0, color: '#a78bfa' },
              { label: '探索', value: features.drives?.explore ?? 0, color: '#34d399' },
            ]}
          />
        </div>
        <div>
          <h3 style={{ fontSize: 14, marginBottom: 8, color: 'var(--text-secondary)' }}>6维内源需求</h3>
          <NeedBars needs={features.needs} />
        </div>
      </div>

      {/* Quick relationship */}
      <div style={{ marginTop: 20 }}>
        <h3 style={{ fontSize: 14, marginBottom: 8, color: 'var(--text-secondary)' }}>互动关系</h3>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 12 }}>
          <MiniItem label="冷落时长" value={`${(features.relationship.neglect_hours * 60).toFixed(0)}分钟`} />
          <MiniItem label="消息趋势" value={features.user.length_trend < -0.2 ? '变短 ⬇' : features.user.length_trend > 0.2 ? '变长 ⬆' : '稳定'} />
          <MiniItem label="亲密度趋势" value={features.relationship.intimacy_trend < -0.1 ? '降温 ⬇' : features.relationship.intimacy_trend > 0.1 ? '升温 ⬆' : '稳定'} />
        </div>
      </div>
    </div>
  )
}

// ---- Emotion Tab ----

function EmotionTab({ emotion, features }: { emotion: EmotionViewModel | null; features: FeaturesViewModel | null }) {
  if (!emotion) return <Loading />
  const v = emotion.vector

  const emotionBars = [
    { label: '情感', value: v.affection, color: '#f472b6' },
    { label: '担忧', value: v.worry, color: '#fbbf24' },
    { label: '好奇', value: v.curiosity, color: '#60a5fa' },
    { label: '困倦', value: v.sleepiness, color: '#a78bfa' },
    { label: '贪玩', value: v.playfulness, color: '#34d399' },
    { label: '寂寞', value: v.loneliness, color: '#f87171' },
    { label: '自信', value: v.confidence, color: '#818cf8' },
    { label: '烦躁', value: v.annoyance, color: '#fb923c' },
  ]

  return (
    <div>
      <div style={{ display: 'flex', gap: 16, marginBottom: 16 }}>
        <div style={{
          width: 64, height: 64, borderRadius: '50%', background: 'var(--color-primary-light)',
          display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: 28,
        }}>{emotionIcon(emotion.primary)}</div>
        <div>
          <div style={{ fontSize: 28, fontWeight: 700 }}>{labelPrimary(emotion.primary)}</div>
          <div style={{ fontSize: 13, color: 'var(--text-secondary)' }}>
            PAD: V{emotion.valence.toFixed(2)} A{emotion.arousal.toFixed(2)} D{emotion.dominance.toFixed(2)} · 强度{(emotion.intensity * 100).toFixed(0)}%
          </div>
          {features && <div style={{ fontSize: 12, color: 'var(--text-muted)', marginTop: 2 }}>
            趋势: {features.user?.length_trend ? (features.user.length_trend < 0 ? '↓ 变短' : '↑ 变长') : '--'}
          </div>}
        </div>
      </div>

      <h3 style={{ fontSize: 14, marginBottom: 8, color: 'var(--text-secondary)' }}>8维情绪向量</h3>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
        {emotionBars.map((e) => (
          <div key={e.label} style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <span style={{ width: 40, fontSize: 13, textAlign: 'right' }}>{e.label}</span>
            <div style={{ flex: 1, height: 16, background: 'var(--bg-muted)', borderRadius: 4, overflow: 'hidden' }}>
              <div style={{ height: '100%', width: `${(e.value * 100).toFixed(0)}%`, background: e.color, borderRadius: 4, transition: 'width 1s' }} />
            </div>
            <span style={{ width: 36, fontSize: 12, color: 'var(--text-muted)' }}>{(e.value * 100).toFixed(0)}%</span>
          </div>
        ))}
      </div>
    </div>
  )
}

// ---- Decision Tab ----

function DecisionTab({ features, learning }: { features: FeaturesViewModel | null; learning: LearningOverview | null }) {
  if (!features) return <Loading />

  const d = features.last_decision
  return (
    <div>
      <h3 style={{ fontSize: 14, marginBottom: 12, color: 'var(--text-secondary)' }}>最近决策溯源</h3>
      {d ? (
        <div style={{
          background: 'var(--bg-card)', border: '1px solid var(--border-color)',
          borderRadius: 8, padding: 16, marginBottom: 16,
        }}>
          <div style={{ fontSize: 18, fontWeight: 600, marginBottom: 8 }}>
            {actionLabel(d.action)} <span style={{ fontSize: 13, color: 'var(--text-muted)', fontWeight: 400 }}>score {d.score.toFixed(3)}</span>
          </div>
          <div style={{ fontSize: 13, color: 'var(--text-secondary)', lineHeight: 1.6 }}>
            {d.routed_llm && '🧠 LLM 兜底决策 · '}
            来源: {d.source || '--'}
          </div>
          <div style={{ display: 'flex', gap: 8, marginTop: 8, flexWrap: 'wrap' }}>
            {d.routed_llm && <Badge color="#f59e0b">LLM</Badge>}
            {d.source === 'care' && <Badge color="#ef4444">关怀</Badge>}
            {d.source === 'casual' && <Badge color="#3b82f6">闲聊</Badge>}
            {d.source === 'knowledge_gap' && <Badge color="#8b5cf6">好奇</Badge>}
          </div>
        </div>
      ) : (
        <div style={{ color: 'var(--text-muted)', marginBottom: 16 }}>等待首次决策...</div>
      )}

      {/* Factor snapshot */}
      <h3 style={{ fontSize: 14, marginBottom: 8, color: 'var(--text-secondary)' }}>当前关键因子</h3>
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 8 }}>
        <FactorRow label="App" value={`${features.user.app_category}(${features.user.window_subtype || '-'})`} />
        <FactorRow label="工作" value={features.user.is_working ? '是' : '否'} />
        <FactorRow label="时段接受率" value={`${(features.relationship.time_window_accept * 100).toFixed(0)}%`} />
        <FactorRow label="距上次消息" value={`${features.user.time_since_chat_min.toFixed(0)}分钟`} />
        <FactorRow label="策略/模式" value={`${features.task.principle_count}/${features.task.pattern_count}`} />
        <FactorRow label="冷却" value={`${(features.task.cooldown_norm * 100).toFixed(0)}%`} />
      </div>

      {/* Learning stats */}
      {learning && (
        <div style={{ marginTop: 20 }}>
          <h3 style={{ fontSize: 14, marginBottom: 8, color: 'var(--text-secondary)' }}>学习统计</h3>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 8 }}>
            <MiniItem label="策略" value={`${learning.principles_count}`} />
            <MiniItem label="线程" value={`${learning.active_threads}`} />
            <MiniItem label="inquiries" value={`${learning.active_inquiries}`} />
            <MiniItem label="模式" value={`${learning.patterns_count}`} />
          </div>
        </div>
      )}
    </div>
  )
}

// ---- Small Components ----

function NeedBars({ needs }: { needs: FeaturesViewModel['needs'] }) {
  const items = [
    { key: 'companionship', label: '陪伴', v: needs.companionship, color: '#f472b6' },
    { key: 'care', label: '关怀', v: needs.care, color: '#ef4444' },
    { key: 'play', label: '玩耍', v: needs.play, color: '#34d399' },
    { key: 'curiosity', label: '好奇', v: needs.curiosity, color: '#60a5fa' },
    { key: 'rest', label: '休息', v: needs.rest, color: '#a78bfa' },
    { key: 'autonomy', label: '自主', v: needs.autonomy, color: '#fbbf24' },
  ]
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
      {items.map((n) => (
        <div key={n.key} style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <span style={{ width: 36, fontSize: 12, textAlign: 'right' }}>{n.label}</span>
          <div style={{ flex: 1, height: 12, background: 'var(--bg-muted)', borderRadius: 3, overflow: 'hidden' }}>
            <div style={{ height: '100%', width: `${(n.v * 100).toFixed(0)}%`, background: n.color, borderRadius: 3, transition: 'width 0.8s' }} />
          </div>
          <span style={{ width: 32, fontSize: 11, color: 'var(--text-muted)' }}>
            {(n.v * 100).toFixed(0)}%{n.v > 0.7 ? ' ⚡' : ''}
          </span>
        </div>
      ))}
    </div>
  )
}

function MiniItem({ label, value }: { label: string; value: string }) {
  return (
    <div style={{ background: 'var(--bg-card)', border: '1px solid var(--border-color)', borderRadius: 6, padding: '8px 12px' }}>
      <div style={{ fontSize: 11, color: 'var(--text-muted)' }}>{label}</div>
      <div style={{ fontSize: 15, fontWeight: 600 }}>{value}</div>
    </div>
  )
}

function FactorRow({ label, value }: { label: string; value: string }) {
  return (
    <div style={{ fontSize: 13, display: 'flex', justifyContent: 'space-between', padding: '4px 0', borderBottom: '1px solid var(--border-color)' }}>
      <span style={{ color: 'var(--text-secondary)' }}>{label}</span>
      <span style={{ fontWeight: 500 }}>{value}</span>
    </div>
  )
}

function Badge({ color, children }: { color: string; children: string }) {
  return <span style={{ background: color + '18', color, padding: '2px 8px', borderRadius: 4, fontSize: 11, fontWeight: 600 }}>{children}</span>
}

function Loading() {
  return <div style={{ color: 'var(--text-muted)', padding: 40, textAlign: 'center' }}>加载中...</div>
}

function emotionIcon(primary: string) {
  const m: Record<string, string> = { joy: '😊', sadness: '😢', anger: '😠', fear: '😨', surprise: '😲', disgust: '🤢', neutral: '😐' }
  return m[primary] || '😐'
}

function labelPrimary(p: string) {
  const m: Record<string, string> = { joy: '开心', sadness: '难过', anger: '生气', fear: '恐惧', surprise: '惊讶', disgust: '厌恶', neutral: '平静' }
  return m[p] || p
}

function actionLabel(a: string) {
  const m: Record<string, string> = {
    speak_casual: '💬 闲聊', speak_care: '❤️ 关心', speak_inquiry: '❓ 提问',
    care_rest: '😴 催睡', care_meal: '🍚 催饭', care_hydration: '💧 催喝水',
    care_health: '🏃 健康提醒', care_encourage: '💪 鼓励', care_social: '👥 社交提醒',
    observe: '👀 观察', reflect: '🤔 反思', analyze_patterns: '📊 分析', none: '😴 安静',
  }
  return m[a] || a
}
