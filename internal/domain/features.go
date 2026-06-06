// Package domain defines the core data types for the desktop pet application.
package domain

// QuantifiedFeatures holds all 46 computed factor values in a flat structure.
// Organized by dimension: User (主人), Agent (诗音), Environment, Relationship, Task Context.
//
// Value conventions:
//   - Normalized continuous: [0, 1]
//   - Trend / delta:        [-1, 1]
//   - PAD emotional:        [-1, 1]
//   - Categorical:          stored as string, with one-hot or embedding alongside
//   - Boolean flags:        {0, 1}
//
// Raw values (e.g. minutes, counts) are stored alongside normalized versions
// so the decision layer can choose which representation to use.
type QuantifiedFeatures struct {
	// === User (主人) — 14 factors ============================================

	// U1 — current app category: work / play / social / idle
	U1_AppCategory string `json:"u1_app_category"`

	// U2 — window title sub-type: debugging / meeting / coding / watching / ...
	U2_WindowSubtype string `json:"u2_window_subtype"`

	// U3 — whether the user is currently working (0 or 1)
	U3_IsWorking float64 `json:"u3_is_working"`

	// U4 — continuous work duration in minutes, and saturate-normalized at 180min
	U4_ContinuousWorkMins float64 `json:"u4_continuous_work_mins"`
	U4_ContinuousWorkNorm float64 `json:"u4_continuous_work_norm"`

	// U5 — app switch count in the last 30 minutes
	U5_AppSwitchCount float64 `json:"u5_app_switch_count"`
	U5_AppSwitchNorm  float64 `json:"u5_app_switch_norm"`

	// U7 — message length trend [-1, 1]; positive = getting longer
	U7_LengthTrend float64 `json:"u7_length_trend"`

	// U8 — EMA of response delay (seconds), and engagement normal [0, 1]
	U8_ResponseDelayEMA float64 `json:"u8_response_delay_ema"`
	U8_EngagementNorm   float64 `json:"u8_engagement_norm"`

	// U10 — acceptance rate in the current 3-hour time bucket
	U10_TimeWindowPref float64 `json:"u10_time_window_pref"`

	// U11 — meal time window (0 or 0.5)
	U11_MealTime float64 `json:"u11_meal_time"`

	// U12 — late-night window (0 or 0.6)
	U12_NightTime float64 `json:"u12_night_time"`

	// U13 — weekend flag (0 or 1)
	U13_IsWeekend float64 `json:"u13_is_weekend"`

	// U14 — minutes since last chat (any direction)
	U14_TimeSinceChatMins float64 `json:"u14_time_since_chat_mins"`

	// U15 — hours since user last mentioned fatigue/eating/rest
	U15_FatigueMentionHrs  float64 `json:"u15_fatigue_mention_hrs"`
	U15_FatigueMentionNorm float64 `json:"u15_fatigue_mention_norm"`

	// U16 — diversity of known user preferences (count of distinct preference categories)
	U16_PrefDiversity float64 `json:"u16_pref_diversity"`

	// === Agent (诗音 / 猫娘) — 13 factors ====================================

	// A1 — 8-dimension emotion vector (all 0~1)
	A1_Affection   float64 `json:"a1_affection"`
	A1_Worry       float64 `json:"a1_worry"`
	A1_Curiosity   float64 `json:"a1_curiosity"`
	A1_Sleepiness  float64 `json:"a1_sleepiness"`
	A1_Playfulness float64 `json:"a1_playfulness"`
	A1_Loneliness  float64 `json:"a1_loneliness"`
	A1_Confidence  float64 `json:"a1_confidence"`
	A1_Annoyance   float64 `json:"a1_annoyance"`

	// A2 — primary emotion label
	A2_PrimaryEmotion string `json:"a2_primary_emotion"`

	// A3 — emotional intensity [0, 1]
	A3_Intensity float64 `json:"a3_intensity"`

	// A4 — valence change over last hour [-2, 2], and vector euclidean displacement
	A4_ValenceTrend float64 `json:"a4_valence_trend"`
	A4_VecDelta     float64 `json:"a4_vec_delta"`

	// A5 — personality parameters learned from outcomes
	A5_AnnoySensitivity float64 `json:"a5_annoy_sensitivity"`
	A5_AffectWarmth     float64 `json:"a5_affect_warmth"`
	A5_WorryTendency    float64 `json:"a5_worry_tendency"`

	// A6 — number of proactive actions fired today
	A6_DailyActionCount float64 `json:"a6_daily_action_count"`

	// A7 — success rate per action type (keyed by CareTriggerType string)
	A7_ActionSuccessRate map[string]float64 `json:"a7_action_success_rate,omitempty"`

	// A8 — success rate per time block: 0=late_night, 1=morning, 2=afternoon, 3=evening
	A8_TimeBlockRate map[int]float64 `json:"a8_time_block_rate,omitempty"`

	// A10 — active conversation threads count
	A10_ActiveGoals     float64 `json:"a10_active_goals"`
	A10_ActiveGoalsNorm float64 `json:"a10_active_goals_norm"`

	// A11 — active curiosity inquiries count
	A11_ActiveInquiries float64 `json:"a11_active_inquiries"`

	// A12 — active knowledge gaps count
	A12_KnowledgeGaps float64 `json:"a12_knowledge_gaps"`

	// A13 — new facts learned in last 24h, and learning momentum [0, 1]
	A13_NewFacts24h      float64 `json:"a13_new_facts_24h"`
	A13_LearningMomentum float64 `json:"a13_learning_momentum"`

	// A14 — consecutive same-action count (repetition detection)
	A14_ConsecutiveCount float64 `json:"a14_consecutive_count"`

	// === Environment — 7 factors =============================================

	// E1 — current hour (0-23)
	E1_Hour float64 `json:"e1_hour"`

	// E2 — day of week with cyclical encoding
	E2_DayOfWeek float64 `json:"e2_day_of_week"` // 0=Sun .. 6=Sat
	E2_DOWSin    float64 `json:"e2_dow_sin"`
	E2_DOWCos    float64 `json:"e2_dow_cos"`

	// E3 — minutes since last proactive action, and cooldown factor
	E3_MinsSinceAction float64 `json:"e3_mins_since_action"`
	E3_CooldownNorm    float64 `json:"e3_cooldown_norm"`

	// E4 — remaining daily action quota
	E4_QuotaRemaining float64 `json:"e4_quota_remaining"`

	// E5 — minutes since last LLM decision
	E5_MinsSinceDecision float64 `json:"e5_mins_since_decision"`

	// E6 — system resource availability
	E6_LLMAvailable    bool `json:"e6_llm_available"`
	E6_VisionAvailable bool `json:"e6_vision_available"`

	// E7 — hours since last strategic reflection, and reflection-due factor
	E7_HoursSinceReflection float64 `json:"e7_hours_since_reflection"`
	E7_ReflectionDue        float64 `json:"e7_reflection_due"`

	// === Relationship (关系) — 8 factors =====================================

	// R1 — overall acceptance rate from last 20 outcomes
	R1_OverallAcceptRate float64 `json:"r1_overall_accept_rate"`
	R1_SampleCount       float64 `json:"r1_sample_count"`

	// R2 — acceptance rate in the current time window (±1 hour)
	R2_TimeWindowAccept float64 `json:"r2_time_window_accept"`

	// R3 — acceptance rate per proactive source (care / casual / knowledge_gap)
	R3_SourceAcceptRate map[string]float64 `json:"r3_source_accept_rate,omitempty"`

	// R4 — rejection count in last 5 outcomes, and severity [0, 1]
	R4_RecentRejections  float64 `json:"r4_recent_rejections"`
	R4_RejectionSeverity float64 `json:"r4_rejection_severity"`

	// R5 — hours since last user message (neglect), and normalized
	R5_NeglectHours float64 `json:"r5_neglect_hours"`
	R5_NeglectNorm  float64 `json:"r5_neglect_norm"`

	// R6 — conversation depth trend: avg turns per session, compared to baseline [-1, 1]
	R6_DepthTrend float64 `json:"r6_depth_trend"`

	// R7 — number of user-initiated conversations in last 24h
	R7_UserInitiative24h  float64 `json:"r7_user_initiative_24h"`
	R7_UserInitiativeNorm float64 `json:"r7_user_initiative_norm"`

	// R8 — affection 7-day moving average, and intimacy trend [-1, 1]
	// NOTE: R8 uses R1 trend as a proxy until daily emotion snapshots are persisted.
	R8_Affection7dMA float64 `json:"r8_affection_7d_ma"`
	R8_IntimacyTrend float64 `json:"r8_intimacy_trend"`

	// === Task Context (任务上下文) — 4 factors ===============================

	// T1 — count of active strategy principles
	T1_PrincipleCount     float64 `json:"t1_principle_count"`
	T1_PrincipleCountNorm float64 `json:"t1_principle_count_norm"`

	// T2 — count of active behavior patterns
	T2_PatternCount     float64 `json:"t2_pattern_count"`
	T2_PatternCountNorm float64 `json:"t2_pattern_count_norm"`

	// T3 — reflexion log entry count
	T3_ReflexionLogCount float64 `json:"t3_reflexion_log_count"`

	// T5 — today's activity session count, and data sufficiency
	T5_TodayActivityCount float64 `json:"t5_today_activity_count"`
	T5_ActivityDataNorm   float64 `json:"t5_activity_data_norm"`

	// === Metadata ============================================================
	ComputedAt int64 `json:"computed_at"` // unix timestamp of computation
}

// FeatureMeta describes a single factor for introspection and auto-registration.
type FeatureMeta struct {
	ID          string  // e.g. "u3_is_working"
	Dimension   string  // "user" | "agent" | "environment" | "relationship" | "task"
	Label       string  // human-readable name
	Range       string  // "[0,1]" | "[-1,1]" | "bool" | "categorical"
	Tier        int     // 1 = per-tick, 2 = cached aggregation, 3 = LLM-assisted
	Description string  // one-line description in Chinese
}

// FeatureRegistry returns metadata for all 46 factors.
// Used by debugging tools, dashboards, and LLM prompt generation.
func FeatureRegistry() []FeatureMeta {
	return []FeatureMeta{
		// User
		{ID: "u1_app_category", Dimension: "user", Label: "当前App分类", Range: "categorical", Tier: 2, Description: "Code/Bilibili/WeChat → work/play/social/idle"},
		{ID: "u2_window_subtype", Dimension: "user", Label: "窗口标题子类型", Range: "categorical", Tier: 3, Description: "通过正则/LLM理解窗口含义"},
		{ID: "u3_is_working", Dimension: "user", Label: "是否在工作", Range: "bool", Tier: 1, Description: "activity_sessions.is_working直接可用"},
		{ID: "u4_continuous_work_mins", Dimension: "user", Label: "连续工作时长", Range: "[0,∞)", Tier: 1, Description: "最近连续is_working=1的session时长累加"},
		{ID: "u5_app_switch_count", Dimension: "user", Label: "App切换频率", Range: "[0,20+]", Tier: 2, Description: "过去30分钟内切换了几次app"},
		{ID: "u7_length_trend", Dimension: "user", Label: "消息长度趋势", Range: "[-1,1]", Tier: 2, Description: "最近消息与baseline的长度变化率"},
		{ID: "u8_response_delay_ema", Dimension: "user", Label: "回复延迟EMA", Range: "[0,∞)", Tier: 2, Description: "用户回复的移动平均延迟秒数"},
		{ID: "u10_time_window_pref", Dimension: "user", Label: "时间窗口偏好", Range: "[0,1]", Tier: 2, Description: "当前3h桶的历史接受率"},
		{ID: "u11_meal_time", Dimension: "user", Label: "饭点状态", Range: "{0,0.5}", Tier: 1, Description: "11-13或17-20命中"},
		{ID: "u12_night_time", Dimension: "user", Label: "深夜状态", Range: "{0,0.6}", Tier: 1, Description: "23-2命中"},
		{ID: "u13_is_weekend", Dimension: "user", Label: "周末/工作日", Range: "bool", Tier: 1, Description: "DayOfWeek∈{0,6}"},
		{ID: "u14_time_since_chat_mins", Dimension: "user", Label: "距上次对话", Range: "[0,∞)", Tier: 1, Description: "距上次任何方向聊天的分钟数"},
		{ID: "u15_fatigue_mention_hrs", Dimension: "user", Label: "距上次提到吃饭/休息", Range: "[0,∞)", Tier: 2, Description: "facts中搜索关键词的时间差"},
		{ID: "u16_pref_diversity", Dimension: "user", Label: "已知用户偏好多样性", Range: "[0,1]", Tier: 2, Description: "偏好类别数归一化"},
		// Agent
		{ID: "a1_affection", Dimension: "agent", Label: "情感(affection)", Range: "[0,1]", Tier: 1, Description: "8维情绪向量之一"},
		{ID: "a1_worry", Dimension: "agent", Label: "担忧(worry)", Range: "[0,1]", Tier: 1, Description: "8维情绪向量之一"},
		{ID: "a1_curiosity", Dimension: "agent", Label: "好奇(curiosity)", Range: "[0,1]", Tier: 1, Description: "8维情绪向量之一"},
		{ID: "a1_sleepiness", Dimension: "agent", Label: "困倦(sleepiness)", Range: "[0,1]", Tier: 1, Description: "8维情绪向量之一"},
		{ID: "a1_playfulness", Dimension: "agent", Label: "贪玩(playfulness)", Range: "[0,1]", Tier: 1, Description: "8维情绪向量之一"},
		{ID: "a1_loneliness", Dimension: "agent", Label: "寂寞(loneliness)", Range: "[0,1]", Tier: 1, Description: "8维情绪向量之一"},
		{ID: "a1_confidence", Dimension: "agent", Label: "自信(confidence)", Range: "[0,1]", Tier: 1, Description: "8维情绪向量之一"},
		{ID: "a1_annoyance", Dimension: "agent", Label: "烦躁(annoyance)", Range: "[0,1]", Tier: 1, Description: "8维情绪向量之一"},
		{ID: "a2_primary_emotion", Dimension: "agent", Label: "主情绪标签", Range: "categorical", Tier: 1, Description: "joy/sadness/anger/fear/surprise/disgust/neutral"},
		{ID: "a3_intensity", Dimension: "agent", Label: "情绪强度", Range: "[0,1]", Tier: 1, Description: "当前情绪的强度"},
		{ID: "a4_valence_trend", Dimension: "agent", Label: "情绪变化趋势", Range: "[-2,2]", Tier: 1, Description: "Valence过去1h的变化量"},
		{ID: "a5_annoy_sensitivity", Dimension: "agent", Label: "烦躁敏感性", Range: "[0,1]", Tier: 1, Description: "人格参数: 对烦人行为的敏感度"},
		{ID: "a5_affect_warmth", Dimension: "agent", Label: "情感温暖度", Range: "[0,1]", Tier: 1, Description: "人格参数: 情感表达的温度"},
		{ID: "a5_worry_tendency", Dimension: "agent", Label: "担忧倾向", Range: "[0,1]", Tier: 1, Description: "人格参数: 担忧主人的倾向"},
		{ID: "a6_daily_action_count", Dimension: "agent", Label: "今日主动行动次数", Range: "[0,20]", Tier: 1, Description: "今天的proactive行动总数"},
		{ID: "a7_action_success_rate", Dimension: "agent", Label: "各action历史成功率", Range: "[0,1]", Tier: 2, Description: "按action_type分组的outcome>0比例"},
		{ID: "a8_time_block_rate", Dimension: "agent", Label: "各时段历史成功率", Range: "[0,1]", Tier: 2, Description: "按4时段分组的成功率"},
		{ID: "a10_active_goals", Dimension: "agent", Label: "活跃目标数", Range: "[0,∞)", Tier: 2, Description: "活跃的conversation_threads数"},
		{ID: "a11_active_inquiries", Dimension: "agent", Label: "活跃inquiries数", Range: "[0,∞)", Tier: 1, Description: "当前未回答的curiosity inquiries"},
		{ID: "a12_knowledge_gaps", Dimension: "agent", Label: "知识缺口数", Range: "[0,∞)", Tier: 1, Description: "当前活跃的knowledge gaps"},
		{ID: "a13_new_facts_24h", Dimension: "agent", Label: "最近学习的facts数", Range: "[0,∞)", Tier: 2, Description: "近24h created_at的facts count"},
		{ID: "a14_consecutive_count", Dimension: "agent", Label: "行为重复度", Range: "[0,∞)", Tier: 1, Description: "同一action连续执行次数"},
		// Environment
		{ID: "e1_hour", Dimension: "environment", Label: "当前小时", Range: "[0,23]", Tier: 1, Description: "time.Now().Hour()"},
		{ID: "e2_day_of_week", Dimension: "environment", Label: "星期几", Range: "[0,6]", Tier: 1, Description: "循环编码sin/cos"},
		{ID: "e3_mins_since_action", Dimension: "environment", Label: "距上次主动行动", Range: "[0,∞)", Tier: 1, Description: "time.Since(lastActionAt).Minutes()"},
		{ID: "e4_quota_remaining", Dimension: "environment", Label: "今日剩余配额", Range: "[0,20]", Tier: 1, Description: "maxDaily - DailyCount"},
		{ID: "e5_mins_since_decision", Dimension: "environment", Label: "距上次LLM决策", Range: "[0,∞)", Tier: 1, Description: "控制指数退避节奏"},
		{ID: "e6_llm_available", Dimension: "environment", Label: "LLM是否可用", Range: "bool", Tier: 1, Description: "影响是否走LLM兜底"},
		{ID: "e6_vision_available", Dimension: "environment", Label: "Vision LLM是否可用", Range: "bool", Tier: 1, Description: "影响视觉分析功能"},
		{ID: "e7_hours_since_reflection", Dimension: "environment", Label: "距上次反思", Range: "[0,∞)", Tier: 1, Description: "距上次StrategicAgent运行的小时数"},
		// Relationship
		{ID: "r1_overall_accept_rate", Dimension: "relationship", Label: "整体接受率", Range: "[0,1]", Tier: 2, Description: "最近20条outcome>0的占比"},
		{ID: "r2_time_window_accept", Dimension: "relationship", Label: "按时段接受率", Range: "[0,1]", Tier: 2, Description: "当前时段±1h的历史接受率"},
		{ID: "r3_source_accept_rate", Dimension: "relationship", Label: "按类型接受率", Range: "[0,1]", Tier: 2, Description: "care/casual/knowledge_gap各自接受率"},
		{ID: "r4_recent_rejections", Dimension: "relationship", Label: "最近拒绝次数", Range: "[0,5]", Tier: 2, Description: "最近5条中outcome=-1的count"},
		{ID: "r5_neglect_hours", Dimension: "relationship", Label: "冷落时长", Range: "[0,∞)", Tier: 2, Description: "最后一条user消息距现在的小时数"},
		{ID: "r6_depth_trend", Dimension: "relationship", Label: "对话深度趋势", Range: "[-1,1]", Tier: 2, Description: "最近N次对话平均轮次 vs baseline"},
		{ID: "r7_user_initiative_24h", Dimension: "relationship", Label: "主动问候频次", Range: "[0,∞)", Tier: 2, Description: "过去24h user-role消息数"},
		{ID: "r8_intimacy_trend", Dimension: "relationship", Label: "亲密度长期趋势", Range: "[-1,1]", Tier: 2, Description: "affection 7日MA vs 30日baseline"},
		// Task Context
		{ID: "t1_principle_count", Dimension: "task", Label: "策略原则可用数", Range: "[0,∞)", Tier: 1, Description: "活跃strategy_principles数"},
		{ID: "t2_pattern_count", Dimension: "task", Label: "行为模式可用数", Range: "[0,∞)", Tier: 1, Description: "活跃behavior_patterns数"},
		{ID: "t3_reflexion_log_count", Dimension: "task", Label: "反思记忆可用数", Range: "[0,20]", Tier: 1, Description: "reflexionLog条目数"},
		{ID: "t5_today_activity_count", Dimension: "task", Label: "新activity数据量", Range: "[0,∞)", Tier: 2, Description: "今日activity_sessions条数"},
	}
}
