const API_BASE = 'http://127.0.0.1:19840'

async function apiGet<T>(path: string): Promise<T> {
  const res = await fetch(API_BASE + path)
  if (!res.ok) throw new Error(`API ${path} returned ${res.status}`)
  return res.json()
}

async function apiPost<T>(path: string, body?: unknown): Promise<T> {
  const res = await fetch(API_BASE + path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: body ? JSON.stringify(body) : undefined,
  })
  if (!res.ok) throw new Error(`API ${path} returned ${res.status}`)
  return res.json()
}

async function apiPut<T>(path: string, body: unknown): Promise<T> {
  const res = await fetch(API_BASE + path, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) throw new Error(`API ${path} returned ${res.status}`)
  return res.json()
}

async function apiDelete(path: string): Promise<void> {
  const res = await fetch(API_BASE + path, { method: 'DELETE' })
  if (!res.ok) throw new Error(`API ${path} returned ${res.status}`)
}

// ---- types ----

export interface GlobalConfig {
  llm_provider: string
  llm_api_key: string
  llm_model: string
  llm_base_url: string
  vision_model: string
  vision_api_key: string
  vision_base_url: string
  emotion_model: string
  emotion_api_key: string
  emotion_base_url: string
  user_name: string
  user_tech_stack: string[]
}

export interface PluginInfo {
  name: string; version: string; description: string
  running: boolean; priority: number; requires: string[]
}

export interface DashboardStats {
  l0_message_count: number; l1_diary_count: number; l2_fact_count: number
  today_message_count: number; continuous_work_min: number; today_tokens: number
  active_plugins: string[]; emotion: EmotionViewModel
}

export interface EmotionViewModel {
  valence: number; arousal: number; dominance: number
  primary: string; intensity: number; vector: EmotionVectorModel
}

export interface EmotionVectorModel {
  affection: number; worry: number; curiosity: number
  sleepiness: number; playfulness: number; loneliness: number
  confidence: number; annoyance: number
}

export interface ModelInfo { name: string; path: string }

export interface LearningOverview {
  metrics: LearningMetrics; personality: PersonalityModel
  adaptive_params: AdaptiveParamsModel
  principles_count: number; active_threads: number
  active_inquiries: number; patterns_count: number
}

export interface LearningMetrics {
  accept_rate_pct: number; total_today: number; total_week: number
  by_source: Record<string, number>
}

export interface PersonalityModel {
  annoyance_sensitivity: number; affection_warmth: number; worry_tendency: number
}

export interface AdaptiveParamsModel {
  work_threshold: number; silence_threshold_min: number; loneliness_threshold: number
}

export interface ChatMessage { role: string; content: string }
export interface MemoryItem { id: number; content: string; importance: number; fact_role: string }
export interface DiaryEntry { id: number; title: string; summary: string; created_at: number }
export interface IdentityNode { id: number; node_type: string; content: string; confidence: number; active: boolean }

// ---- config ----

export const getConfig = () => apiGet<GlobalConfig>('/api/config')
export const saveConfig = (cfg: GlobalConfig) => apiPost<GlobalConfig>('/api/config', cfg)
export const testConnection = (target: string) => apiPost<{ ok: boolean }>('/api/test-connection', { target })

// ---- plugins ----

export const getPlugins = () => apiGet<PluginInfo[]>('/api/plugins')
export const togglePlugin = (name: string) => apiPost<{ status: string }>('/api/plugins/' + name + '/toggle')
export const getPluginConfig = (name: string) => apiGet<Record<string, unknown>>('/api/plugins/' + name)
export const savePluginConfig = (name: string, cfg: Record<string, unknown>) => apiPost('/api/plugins/' + name, cfg)

// ---- chat ----

export const sendMessage = (text: string) => apiPost<{ content: string; source?: string }>('/api/chat/send', { text })
export const getChatHistory = (limit: number) => apiGet<ChatMessage[]>('/api/chat/history?limit=' + limit)

// ---- stats + emotion ----

export const getStats = () => apiGet<DashboardStats>('/api/stats')
export const getEmotion = () => apiGet<EmotionViewModel>('/api/emotion')
export const getLearningOverview = () => apiGet<LearningOverview>('/api/learning/overview')
export const getModels = () => apiGet<ModelInfo[]>('/api/models')

// ---- memory ----

export const getMemories = () => apiGet<MemoryItem[]>('/api/memories')
export const getDiaries = () => apiGet<DiaryEntry[]>('/api/diaries')
export const deleteMemory = (id: number) => apiDelete('/api/memories/' + id)

// ---- identity ----

export const getIdentityNodes = () => apiGet<IdentityNode[]>('/api/identity')
export const upsertIdentityNode = (node: Partial<IdentityNode>) => apiPut('/api/identity', node)

// ---- proactive ----

export const getProactivePoll = () => apiGet<{ message: string }>('/api/proactive/poll')

// ---- features + strategies ----

export interface FeaturesViewModel {
  computed_at: number
  drives: DriveScores
  user: UserContext
  relationship: RelationshipContext
  needs: NeedsContext
  task: TaskContext
  last_decision?: DecisionSummary
}

export interface DriveScores { social: number; care: number; curious: number; quiet: number; explore: number }

export interface UserContext {
  app_category: string; window_subtype: string; is_working: boolean
  continuous_work_min: number; app_switch_count: number
  length_trend: number; engagement_norm: number
  meal_time: boolean; night_time: boolean; is_weekend: boolean
  time_since_chat_min: number; fatigue_mention_hrs: number; pref_diversity: number
}

export interface RelationshipContext {
  overall_accept_rate: number; sample_count: number; time_window_accept: number
  source_accept_rate: Record<string, number>; recent_rejections: number
  rejection_severity: number; neglect_hours: number
  depth_trend: number; user_initiative_24h: number; intimacy_trend: number
}

export interface NeedsContext { companionship: number; rest: number; play: number; curiosity: number; care: number; autonomy: number }

export interface TaskContext {
  principle_count: number; pattern_count: number; reflexion_log_size: number
  today_activity_count: number; quota_remaining: number; cooldown_norm: number; reflection_due: number
}

export interface DecisionSummary { action: string; score: number; source: string; routed_llm: boolean }

export interface StrategyViewModel {
  id: number; situation: string; good_strategy: string; bad_strategy: string
  reason: string; confidence: number; source: string; active: boolean
}

export const getFeatures = () => apiGet<FeaturesViewModel>('/api/features/current')
export const getStrategies = () => apiGet<StrategyViewModel[]>('/api/strategies')
