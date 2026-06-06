package api

// PluginInfoViewModel is the API-facing plugin metadata including runtime status.
type PluginInfoViewModel struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	Running     bool     `json:"running"`
	Priority    int      `json:"priority"`
	Requires    []string `json:"requires"`
}

// PluginComponentViewModel describes a plugin's custom settings panel component.
type PluginComponentViewModel struct {
	PluginName string         `json:"plugin_name"`
	Component  string         `json:"component"`
	Defaults   map[string]any `json:"defaults"`
}

// DashboardStats is the aggregated statistics returned by GET /api/stats.
type DashboardStats struct {
	L0MessageCount    int              `json:"l0_message_count"`
	L1DiaryCount      int              `json:"l1_diary_count"`
	L2FactCount       int              `json:"l2_fact_count"`
	TodayMessageCount int              `json:"today_message_count"`
	ContinuousWorkMin int              `json:"continuous_work_min"`
	TodayTokens       int              `json:"today_tokens"`
	ActivePlugins     []string         `json:"active_plugins"`
	Emotion           EmotionViewModel `json:"emotion"`
}

// EmotionViewModel is the API-facing emotion state.
type EmotionViewModel struct {
	Valence   float64            `json:"valence"`
	Arousal   float64            `json:"arousal"`
	Dominance float64            `json:"dominance"`
	Primary   string             `json:"primary"`
	Intensity float64            `json:"intensity"`
	Vector    EmotionVectorModel `json:"vector"`
}

// EmotionVectorModel is the API-facing emotion vector.
type EmotionVectorModel struct {
	Affection   float64 `json:"affection"`
	Worry       float64 `json:"worry"`
	Curiosity   float64 `json:"curiosity"`
	Sleepiness  float64 `json:"sleepiness"`
	Playfulness float64 `json:"playfulness"`
	Loneliness  float64 `json:"loneliness"`
	Confidence  float64 `json:"confidence"`
	Annoyance   float64 `json:"annoyance"`
}

// LearningOverview is the aggregated self-learning data returned by GET /api/learning/overview.
type LearningOverview struct {
	Metrics         LearningMetrics     `json:"metrics"`
	Personality     PersonalityModel    `json:"personality"`
	AdaptiveParams  AdaptiveParamsModel `json:"adaptive_params"`
	PrinciplesCount int                 `json:"principles_count"`
	ActiveThreads   int                 `json:"active_threads"`
	ActiveInquiries int                 `json:"active_inquiries"`
	PatternsCount   int                 `json:"patterns_count"`
}

// LearningMetrics holds proactive action outcome statistics.
type LearningMetrics struct {
	AcceptRatePct float64        `json:"accept_rate_pct"`
	TotalToday    int            `json:"total_today"`
	TotalWeek     int            `json:"total_week"`
	BySource      map[string]int `json:"by_source"`
}

// PersonalityModel exposes the learned personality scale.
type PersonalityModel struct {
	AnnoyanceSensitivity float64 `json:"annoyance_sensitivity"`
	AffectionWarmth      float64 `json:"affection_warmth"`
	WorryTendency        float64 `json:"worry_tendency"`
}

// AdaptiveParamsModel exposes the learned scheduler parameters.
type AdaptiveParamsModel struct {
	WorkThreshold       float64 `json:"work_threshold"`
	SilenceThresholdMin float64 `json:"silence_threshold_min"`
	LonelinessThreshold float64 `json:"loneliness_threshold"`
}

// FeaturesViewModel is the API-facing quantified features snapshot.
type FeaturesViewModel struct {
	ComputedAt int64 `json:"computed_at"`

	// Drives (computed on the fly from features).
	Drives DriveScores `json:"drives"`

	// User context.
	User UserContext `json:"user"`

	// Relationship.
	Relationship RelationshipContext `json:"relationship"`

	// Agent needs.
	Needs NeedsContext `json:"needs"`

	// Task context.
	Task TaskContext `json:"task"`

	// Last decision.
	LastDecision DecisionSummary `json:"last_decision,omitempty"`
}

type DriveScores struct {
	Social  float64 `json:"social"`
	Care    float64 `json:"care"`
	Curious float64 `json:"curious"`
	Quiet   float64 `json:"quiet"`
	Explore float64 `json:"explore"`
}

type UserContext struct {
	AppCategory        string  `json:"app_category"`
	WindowSubtype      string  `json:"window_subtype"`
	IsWorking          bool    `json:"is_working"`
	ContinuousWorkMin  float64 `json:"continuous_work_min"`
	AppSwitchCount     float64 `json:"app_switch_count"`
	LengthTrend        float64 `json:"length_trend"`
	EngagementNorm     float64 `json:"engagement_norm"`
	MealTime           bool    `json:"meal_time"`
	NightTime          bool    `json:"night_time"`
	IsWeekend          bool    `json:"is_weekend"`
	TimeSinceChatMin   float64 `json:"time_since_chat_min"`
	FatigueMentionHrs  float64 `json:"fatigue_mention_hrs"`
	PrefDiversity      float64 `json:"pref_diversity"`
}

type RelationshipContext struct {
	OverallAcceptRate  float64            `json:"overall_accept_rate"`
	SampleCount        int                `json:"sample_count"`
	TimeWindowAccept   float64            `json:"time_window_accept"`
	SourceAcceptRate   map[string]float64 `json:"source_accept_rate"`
	RecentRejections   int                `json:"recent_rejections"`
	RejectionSeverity  float64            `json:"rejection_severity"`
	NeglectHours       float64            `json:"neglect_hours"`
	DepthTrend         float64            `json:"depth_trend"`
	UserInitiative24h  float64            `json:"user_initiative_24h"`
	IntimacyTrend      float64            `json:"intimacy_trend"`
}

type NeedsContext struct {
	Companionship float64 `json:"companionship"`
	Rest          float64 `json:"rest"`
	Play          float64 `json:"play"`
	Curiosity     float64 `json:"curiosity"`
	Care          float64 `json:"care"`
	Autonomy      float64 `json:"autonomy"`
}

type TaskContext struct {
	PrincipleCount    int     `json:"principle_count"`
	PatternCount      int     `json:"pattern_count"`
	ReflexionLogSize  int     `json:"reflexion_log_size"`
	TodayActivityCount float64 `json:"today_activity_count"`
	QuotaRemaining    float64 `json:"quota_remaining"`
	CooldownNorm      float64 `json:"cooldown_norm"`
	ReflectionDue     float64 `json:"reflection_due"`
}

type DecisionSummary struct {
	Action    string  `json:"action"`
	Score     float64 `json:"score"`
	Source    string  `json:"source"`
	RoutedLLM bool    `json:"routed_llm"`
}

// StrategyViewModel is the API-facing strategy principle.
type StrategyViewModel struct {
	ID           int64   `json:"id"`
	Situation    string  `json:"situation"`
	GoodStrategy string  `json:"good_strategy"`
	BadStrategy  string  `json:"bad_strategy"`
	Reason       string  `json:"reason"`
	Confidence   float64 `json:"confidence"`
	Source       string  `json:"source"`
	Active       bool    `json:"active"`
}
