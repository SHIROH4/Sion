package cognition

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"math"
	"os"
	"strings"
	"sync"
	"time"

	"desktop-pet/internal/domain"
	"desktop-pet/internal/infra/storage"
)

// ---- FeatureComputer ----

// FeatureComputer computes all 46 quantifiable factors for the decision layer.
// Tier 1 factors are computed in ComputeFast every tick (~1ms, no SQL).
// Tier 2 factors are computed in ComputeSlow on a TTL cache (~50ms, SQL aggregations).
type FeatureComputer struct {
	mu sync.RWMutex

	// In-memory state for Tier 1 factors.
	lastActionAt     time.Time
	lastDecisionAt   time.Time
	lastReflectionAt time.Time
	emotionHistory     *ringBuffer
	emotionHistoryPath string // JSON persist path for ring buffer
	llmAvailable       bool
	visionAvailable    bool

	// Tier 2 data access.
	db          *sql.DB
	outcomeRepo domain.ActionOutcomeRepository

	// LLM-assisted factor classification (Phase 3).
	llmCall    func(string) (string, error) // LLM callback for U1/U2 classification
	vectorizer domain.Vectorizer            // embedding service for U15 semantic search

	// App category mapping cache for U1.
	appCategoryCache map[string]string
	appCategoryMu    sync.RWMutex // protects appCategoryCache reads/writes
}

// NewFeatureComputer creates a FeatureComputer with default state.
// Pass nil for db if Tier 2 features are not yet needed.
func NewFeatureComputer(db *sql.DB, outcomeRepo domain.ActionOutcomeRepository) *FeatureComputer {
	return &FeatureComputer{
		lastActionAt:     time.Now(),
		lastDecisionAt:   time.Now(),
		lastReflectionAt: time.Now(),
		emotionHistory:   newRingBuffer(24), // 2 hours of 5-min ticks
		llmAvailable:     true,
		visionAvailable:  true,
		db:               db,
		outcomeRepo:      outcomeRepo,
		appCategoryCache: defaultAppCategories(),
	}
}

// ---- State Setters ----

// NoteAction updates the last-action timestamp (called after any proactive action).
func (fc *FeatureComputer) NoteAction() {
	fc.mu.Lock()
	fc.lastActionAt = time.Now()
	fc.mu.Unlock()
}

// NoteDecision updates the last-decision timestamp (called after each System 2 tick).
func (fc *FeatureComputer) NoteDecision() {
	fc.mu.Lock()
	fc.lastDecisionAt = time.Now()
	fc.mu.Unlock()
}

// NoteReflection updates the last-reflection timestamp.
func (fc *FeatureComputer) NoteReflection(at time.Time) {
	fc.mu.Lock()
	fc.lastReflectionAt = at
	fc.mu.Unlock()
}

// SetEmotionHistoryPath sets the JSON file path for persisting emotion snapshots.
// On set, it attempts to load previously persisted history.
func (fc *FeatureComputer) SetEmotionHistoryPath(path string) {
	fc.mu.Lock()
	fc.emotionHistoryPath = path
	fc.mu.Unlock()
	fc.loadEmotionHistory()
}

// PushEmotionSnapshot records an emotion snapshot for trend computation (A4).
// Persists the full buffer to disk after each push if a path is configured.
func (fc *FeatureComputer) PushEmotionSnapshot(valence float64, vec domain.EmotionVector) {
	fc.mu.Lock()
	fc.emotionHistory.Push(emotionSnapshot{
		valence: valence,
		vec:     vec,
		at:      time.Now(),
	})
	path := fc.emotionHistoryPath
	fc.mu.Unlock()

	if path != "" {
		fc.saveEmotionHistory(path)
	}
}

// loadEmotionHistory restores the ring buffer from a JSON file.
func (fc *FeatureComputer) loadEmotionHistory() {
	fc.mu.RLock()
	path := fc.emotionHistoryPath
	fc.mu.RUnlock()
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var snapshots []emotionSnapshot
	if json.Unmarshal(data, &snapshots) != nil {
		return
	}
	fc.mu.Lock()
	for _, s := range snapshots {
		fc.emotionHistory.Push(s)
	}
	fc.mu.Unlock()
}

// saveEmotionHistory writes the ring buffer contents to a JSON file.
func (fc *FeatureComputer) saveEmotionHistory(path string) {
	fc.mu.RLock()
	snapshots := fc.emotionHistory.All()
	fc.mu.RUnlock()
	data, err := json.Marshal(snapshots)
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0644)
}

// SetLLMAvailable updates the LLM availability flag (E6).
func (fc *FeatureComputer) SetLLMAvailable(ok bool) {
	fc.mu.Lock()
	fc.llmAvailable = ok
	fc.mu.Unlock()
}

// SetVisionAvailable updates the Vision LLM availability flag (E6).
func (fc *FeatureComputer) SetVisionAvailable(ok bool) {
	fc.mu.Lock()
	fc.visionAvailable = ok
	fc.mu.Unlock()
}

// SetLLM wires an LLM callback for U1/U2 classification of unknown apps/titles.
func (fc *FeatureComputer) SetLLM(fn func(string) (string, error)) {
	fc.mu.Lock()
	fc.llmCall = fn
	fc.mu.Unlock()
}

// SetVectorizer wires the embedding service for U15 semantic fatigue search.
func (fc *FeatureComputer) SetVectorizer(v domain.Vectorizer) {
	fc.mu.Lock()
	fc.vectorizer = v
	fc.mu.Unlock()
}

// ---- Tier 1: Fast Features (per-tick, no SQL) ----

// ComputeFast computes all Tier 1 factors (~22 factors) from in-memory state
// and passed-in parameters. Runs every tick in <1ms.
func (fc *FeatureComputer) ComputeFast(
	now time.Time,
	emotionVec domain.EmotionVector,
	emotionState domain.EmotionState,
	timeSinceLastChat time.Duration,
	isWorking bool,
	continuousWorkMin float64,
	dailyActionCount int,
	maxDaily int,
	consecutiveCount int,
	activeInquiries int,
	knowledgeGaps int,
	principleCount int,
	patternCount int,
	reflexionLogSize int,
	annoySensitivity float64,
	affectWarmth float64,
	worryTendency float64,
) *domain.QuantifiedFeatures {
	fc.mu.RLock()
	lastAction := fc.lastActionAt
	lastDecision := fc.lastDecisionAt
	lastReflection := fc.lastReflectionAt
	snap1h := fc.emotionHistory.SnapshotNHoursAgo(1)
	llmOK := fc.llmAvailable
	visOK := fc.visionAvailable
	fc.mu.RUnlock()

	f := &domain.QuantifiedFeatures{ComputedAt: now.Unix()}

	// === User factors ===

	f.U3_IsWorking = boolToFloat(isWorking)
	f.U4_ContinuousWorkMins = continuousWorkMin
	f.U4_ContinuousWorkNorm = saturateNorm(continuousWorkMin, 180)

	hour := now.Hour()
	if (hour >= 11 && hour <= 13) || (hour >= 17 && hour <= 20) {
		f.U11_MealTime = 0.5
	}
	if hour >= 23 || hour <= 4 {
		f.U12_NightTime = 0.6
	}

	dow := now.Weekday()
	if dow == 0 || dow == 6 {
		f.U13_IsWeekend = 1.0
	}
	f.U14_TimeSinceChatMins = timeSinceLastChat.Minutes()

	// === Agent factors ===

	f.A1_Affection = storage.Clamp01(emotionVec.Affection)
	f.A1_Worry = storage.Clamp01(emotionVec.Worry)
	f.A1_Curiosity = storage.Clamp01(emotionVec.Curiosity)
	f.A1_Sleepiness = storage.Clamp01(emotionVec.Sleepiness)
	f.A1_Playfulness = storage.Clamp01(emotionVec.Playfulness)
	f.A1_Loneliness = storage.Clamp01(emotionVec.Loneliness)
	f.A1_Confidence = storage.Clamp01(emotionVec.Confidence)
	f.A1_Annoyance = storage.Clamp01(emotionVec.Annoyance)

	f.A2_PrimaryEmotion = emotionState.Primary
	f.A3_Intensity = storage.Clamp01(emotionState.Intensity)

	if snap1h != nil {
		f.A4_ValenceTrend = emotionState.Valence - snap1h.valence
		f.A4_VecDelta = vecDistance(emotionVec, snap1h.vec)
	}

	f.A5_AnnoySensitivity = storage.Clamp01(annoySensitivity)
	f.A5_AffectWarmth = storage.Clamp01(affectWarmth)
	f.A5_WorryTendency = storage.Clamp01(worryTendency)

	f.A6_DailyActionCount = float64(dailyActionCount)
	f.A11_ActiveInquiries = float64(activeInquiries)
	f.A12_KnowledgeGaps = float64(knowledgeGaps)
	f.A14_ConsecutiveCount = float64(consecutiveCount)

	// === Environment factors ===

	f.E1_Hour = float64(hour)
	f.E2_DayOfWeek = float64(dow)
	f.E2_DOWSin = math.Sin(2 * math.Pi * float64(dow) / 7)
	f.E2_DOWCos = math.Cos(2 * math.Pi * float64(dow) / 7)

	minsSinceAction := now.Sub(lastAction).Minutes()
	f.E3_MinsSinceAction = minsSinceAction
	f.E3_CooldownNorm = saturateNorm(minsSinceAction, 30)

	remaining := maxDaily - dailyActionCount
	if remaining < 0 {
		remaining = 0
	}
	f.E4_QuotaRemaining = float64(remaining)

	minsSinceDecision := now.Sub(lastDecision).Minutes()
	f.E5_MinsSinceDecision = minsSinceDecision

	f.E6_LLMAvailable = llmOK
	f.E6_VisionAvailable = visOK

	hoursSinceReflection := now.Sub(lastReflection).Hours()
	f.E7_HoursSinceReflection = hoursSinceReflection
	f.E7_ReflectionDue = saturateNorm(hoursSinceReflection, 24)

	// === Task Context factors ===

	f.T1_PrincipleCount = float64(principleCount)
	f.T1_PrincipleCountNorm = saturateNorm(float64(principleCount), 10)
	f.T2_PatternCount = float64(patternCount)
	f.T2_PatternCountNorm = saturateNorm(float64(patternCount), 5)
	f.T3_ReflexionLogCount = float64(reflexionLogSize)

	return f
}

// ---- Tier 2: Slow Features (SQL aggregations, cached) ----

// Slow TTL constants (seconds).
const (
	ttl1Hour  = 3600
	ttl6Hour  = 21600
	ttl24Hour = 86400
)

// ComputeSlow fills in all Tier 2 factors on the given features struct.
// Uses feature_cache for TTL-based caching. Runs in ~50ms when cache is cold.
func (fc *FeatureComputer) ComputeSlow(f *domain.QuantifiedFeatures, now time.Time) {
	// Each compute function handles nil db internally with sensible defaults.
	// No early return — Tier 2 defaults must always be populated.

	// --- U1: App category (1h TTL) ---
	f.U1_AppCategory = fc.getCachedString("u1_app_category", ttl1Hour, func() string {
		return fc.computeAppCategory()
	})

	// --- U2: Window subtype (6h TTL) ---
	f.U2_WindowSubtype = fc.getCachedString("u2_window_subtype", ttl6Hour, func() string {
		return fc.computeWindowSubtype()
	})

	// --- U4: Continuous work minutes (1h TTL, overwrites Fast value) ---
	cwm := fc.getCachedFloat("u4_continuous_work_mins", ttl1Hour, func() (float64, int) {
		return fc.computeContinuousWorkMins(now)
	})
	f.U4_ContinuousWorkMins = cwm
	f.U4_ContinuousWorkNorm = saturateNorm(cwm, 180)

	// --- U5: App switch count (1h TTL) ---
	f.U5_AppSwitchCount = fc.getCachedFloat("u5_app_switch_count", ttl1Hour, func() (float64, int) {
		return fc.computeAppSwitchCount(now)
	})
	f.U5_AppSwitchNorm = saturateNorm(f.U5_AppSwitchCount, 15)

	// --- U7: Message length trend (1h TTL) ---
	f.U7_LengthTrend = fc.getCachedFloat("u7_length_trend", ttl1Hour, func() (float64, int) {
		return fc.computeLengthTrend()
	})

	// --- U8: Response delay EMA (1h TTL) ---
	delayEMA, delayN := fc.computeResponseDelayEMA()
	f.U8_ResponseDelayEMA = fc.getCachedFloat("u8_response_delay_ema", ttl1Hour, func() (float64, int) {
		return delayEMA, delayN
	})
	f.U8_EngagementNorm = 1.0 - saturateNorm(f.U8_ResponseDelayEMA, 300)

	// --- U10 / R2: Time window preference (1h TTL) ---
	f.U10_TimeWindowPref = fc.getCachedFloat("u10_time_window_pref", ttl1Hour, func() (float64, int) {
		return fc.computeTimeWindowPref(now)
	})
	f.R2_TimeWindowAccept = f.U10_TimeWindowPref // same signal

	// --- U15: Fatigue mention hours (6h TTL) ---
	hrs, hrsN := fc.computeFatigueMentionHrs(now)
	f.U15_FatigueMentionHrs = fc.getCachedFloat("u15_fatigue_mention_hrs", ttl6Hour, func() (float64, int) {
		return hrs, hrsN
	})
	f.U15_FatigueMentionNorm = saturateNorm(f.U15_FatigueMentionHrs, 8)

	// --- U16: Preference diversity (6h TTL) ---
	f.U16_PrefDiversity = fc.getCachedFloat("u16_pref_diversity", ttl6Hour, func() (float64, int) {
		return fc.computePrefDiversity()
	})

	// --- A7: Action success rate by type (1h TTL) ---
	f.A7_ActionSuccessRate = fc.getCachedMapSI("a7_action_success_rate", ttl1Hour, func() map[string]float64 {
		return fc.computeActionSuccessByType()
	})

	// --- A8: Time block success rate (1h TTL) ---
	f.A8_TimeBlockRate = fc.getCachedMapII("a8_time_block_rate", ttl1Hour, func() map[int]float64 {
		return fc.computeTimeBlockRate()
	})

	// --- A10: Active goals (1h TTL) ---
	agoals, agoalsN := fc.computeActiveGoals(now)
	f.A10_ActiveGoals = fc.getCachedFloat("a10_active_goals", ttl1Hour, func() (float64, int) {
		return agoals, agoalsN
	})
	f.A10_ActiveGoalsNorm = saturateNorm(f.A10_ActiveGoals, 5)

	// --- A13: New facts in 24h (6h TTL) ---
	nf, nfN := fc.computeNewFacts24h(now)
	f.A13_NewFacts24h = fc.getCachedFloat("a13_new_facts_24h", ttl6Hour, func() (float64, int) {
		return nf, nfN
	})
	f.A13_LearningMomentum = saturateNorm(f.A13_NewFacts24h, 20)

	// --- R1: Overall acceptance rate (1h TTL) ---
	f.R1_OverallAcceptRate, f.R1_SampleCount = fc.computeOverallAcceptRate()
	f.R1_OverallAcceptRate = fc.getCachedFloat("r1_overall_accept_rate", ttl1Hour, func() (float64, int) {
		return f.R1_OverallAcceptRate, int(f.R1_SampleCount)
	})

	// --- R3: Acceptance rate by source (1h TTL) ---
	f.R3_SourceAcceptRate = fc.getCachedMapSI("r3_source_accept_rate", ttl1Hour, func() map[string]float64 {
		return fc.computeSourceAcceptRate()
	})

	// --- R4: Recent rejections (1h TTL) ---
	rej, rejN := fc.computeRecentRejections()
	f.R4_RecentRejections = fc.getCachedFloat("r4_recent_rejections", ttl1Hour, func() (float64, int) {
		return rej, rejN
	})
	f.R4_RejectionSeverity = saturateNorm(f.R4_RecentRejections, 3)

	// --- R5: Neglect hours (5min TTL — changes frequently with chat) ---
	nh, nhN := fc.computeNeglectHours(now)
	f.R5_NeglectHours = fc.getCachedFloat("r5_neglect_hours", 300, func() (float64, int) {
		return nh, nhN
	})
	f.R5_NeglectNorm = saturateNorm(f.R5_NeglectHours, 24)

	// --- R6: Conversation depth trend (6h TTL) ---
	f.R6_DepthTrend = fc.getCachedFloat("r6_depth_trend", ttl6Hour, func() (float64, int) {
		return fc.computeDepthTrend()
	})

	// --- R7: User initiative in 24h (1h TTL) ---
	ui, uiN := fc.computeUserInitiative24h(now)
	f.R7_UserInitiative24h = fc.getCachedFloat("r7_user_initiative_24h", ttl1Hour, func() (float64, int) {
		return ui, uiN
	})
	f.R7_UserInitiativeNorm = saturateNorm(f.R7_UserInitiative24h, 10)

	// --- R8: Intimacy trend (24h TTL) — proxy via R1 trend ---
	f.R8_Affection7dMA = fc.getCachedFloat("r8_affection_7d_ma", ttl24Hour, func() (float64, int) {
		return fc.computeAffection7dMA()
	})
	f.R8_IntimacyTrend = fc.getCachedFloat("r8_intimacy_trend", ttl24Hour, func() (float64, int) {
		return fc.computeIntimacyTrend(f.R8_Affection7dMA)
	})

	// --- T5: Today activity count (1h TTL) ---
	tac, tacN := fc.computeTodayActivityCount(now)
	f.T5_TodayActivityCount = fc.getCachedFloat("t5_today_activity_count", ttl1Hour, func() (float64, int) {
		return tac, tacN
	})
	f.T5_ActivityDataNorm = saturateNorm(f.T5_TodayActivityCount, 30)
}

// ComputeFull computes all 46 factors by merging Tier 1 (fast) and Tier 2 (slow).
// This is the primary entry point for the decision layer.
func (fc *FeatureComputer) ComputeFull(
	now time.Time,
	emotionVec domain.EmotionVector,
	emotionState domain.EmotionState,
	timeSinceLastChat time.Duration,
	isWorking bool,
	dailyActionCount int,
	maxDaily int,
	consecutiveCount int,
	activeInquiries int,
	knowledgeGaps int,
	principleCount int,
	patternCount int,
	reflexionLogSize int,
	annoySensitivity float64,
	affectWarmth float64,
	worryTendency float64,
) *domain.QuantifiedFeatures {
	// Tier 1: fast features (always fresh).
	f := fc.ComputeFast(
		now, emotionVec, emotionState, timeSinceLastChat, isWorking, 0,
		dailyActionCount, maxDaily, consecutiveCount,
		activeInquiries, knowledgeGaps,
		principleCount, patternCount, reflexionLogSize,
		annoySensitivity, affectWarmth, worryTendency,
	)

	// Tier 2: slow features (cached with TTL).
	fc.ComputeSlow(f, now)

	return f
}

// ---- TTL Cache Helpers ----

func (fc *FeatureComputer) getCachedFloat(name string, ttl int, compute func() (float64, int)) float64 {
	if cached, ok := fc.loadFromCache(name, ttl); ok {
		var v float64
		if json.Unmarshal([]byte(cached), &v) == nil {
			return v
		}
	}
	value, sampleCount := compute()
	fc.saveToCache(name, value, 1.0, sampleCount, ttl)
	return value
}

func (fc *FeatureComputer) getCachedString(name string, ttl int, compute func() string) string {
	if cached, ok := fc.loadFromCache(name, ttl); ok {
		var v string
		if json.Unmarshal([]byte(cached), &v) == nil {
			return v
		}
	}
	value := compute()
	fc.saveToCache(name, value, 1.0, 1, ttl)
	return value
}

func (fc *FeatureComputer) getCachedMapSI(name string, ttl int, compute func() map[string]float64) map[string]float64 {
	if cached, ok := fc.loadFromCache(name, ttl); ok {
		var v map[string]float64
		if json.Unmarshal([]byte(cached), &v) == nil {
			return v
		}
	}
	value := compute()
	fc.saveToCache(name, value, 1.0, len(value), ttl)
	return value
}

func (fc *FeatureComputer) getCachedMapII(name string, ttl int, compute func() map[int]float64) map[int]float64 {
	if cached, ok := fc.loadFromCache(name, ttl); ok {
		var v map[int]float64
		if json.Unmarshal([]byte(cached), &v) == nil {
			return v
		}
	}
	value := compute()
	fc.saveToCache(name, value, 1.0, len(value), ttl)
	return value
}

func (fc *FeatureComputer) loadFromCache(name string, ttl int) (string, bool) {
	if fc.db == nil {
		return "", false
	}
	var valueJSON string
	var computedAt int64
	err := fc.db.QueryRow(
		`SELECT value_json, computed_at FROM feature_cache WHERE feature_name = ?`,
		name,
	).Scan(&valueJSON, &computedAt)
	if err != nil {
		return "", false
	}
	if time.Now().Unix()-computedAt > int64(ttl) {
		return "", false
	}
	return valueJSON, true
}

func (fc *FeatureComputer) saveToCache(name string, value any, confidence float64, sampleCount int, ttl int) {
	if fc.db == nil {
		return
	}
	data, err := json.Marshal(value)
	if err != nil {
		return
	}
	if _, err := fc.db.Exec(
		`INSERT INTO feature_cache (feature_name, value_json, confidence, sample_count, computed_at, ttl_seconds)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(feature_name) DO UPDATE SET
			value_json=excluded.value_json,
			confidence=excluded.confidence,
			sample_count=excluded.sample_count,
			computed_at=excluded.computed_at,
			ttl_seconds=excluded.ttl_seconds`,
		name, string(data), confidence, sampleCount, time.Now().Unix(), ttl,
	); err != nil {
		slog.Warn("features: failed to save feature cache", "name", name, "err", err)
	}
}

// ---- Tier 2 Computation Functions ----

// U1: classify the current app. Uses hardcoded map → in-memory cache → LLM fallback.
func (fc *FeatureComputer) computeAppCategory() string {
	if fc.db == nil {
		return "idle"
	}
	var appName string
	err := fc.db.QueryRow(
		`SELECT app_name FROM activity_sessions ORDER BY end_time DESC LIMIT 1`,
	).Scan(&appName)
	if err != nil || appName == "" {
		return "idle"
	}

	cat := categorizeApp(appName)
	if cat != "idle" {
		return cat
	}

	// Check in-memory learned cache.
	fc.appCategoryMu.RLock()
	if learned, ok := fc.appCategoryCache[strings.ToLower(appName)]; ok {
		fc.appCategoryMu.RUnlock()
		return learned
	}
	fc.appCategoryMu.RUnlock()

	// Try LLM classification.
	learned := fc.learnAppCategory(appName)
	if learned != "" {
		fc.appCategoryMu.Lock()
		fc.appCategoryCache[strings.ToLower(appName)] = learned
		fc.appCategoryMu.Unlock()
		return learned
	}

	return "idle"
}

// learnAppCategory asks the LLM to classify an unknown app as work/play/social/idle.
func (fc *FeatureComputer) learnAppCategory(appName string) string {
	fc.mu.RLock()
	llm := fc.llmCall
	fc.mu.RUnlock()
	if llm == nil {
		return ""
	}
	prompt := "请将以下应用名称分类为 work(工作), play(娱乐), social(社交), 或 idle(其他)。只输出一个英文单词，不要解释。\n应用名称: " + appName
	result, err := llm(prompt)
	if err != nil {
		return ""
	}
	result = strings.TrimSpace(strings.ToLower(result))
	// Validate result is one of the 4 categories.
	switch result {
	case "work", "play", "social", "idle":
		return result
	}
	// Strip any extra text — take first word.
	if words := strings.Fields(result); len(words) > 0 {
		if word := words[0]; word == "work" || word == "play" || word == "social" || word == "idle" {
			return word
		}
	}
	return ""
}

// U2: extract window subtitle via regex keyword matching → LLM fallback.
func (fc *FeatureComputer) computeWindowSubtype() string {
	if fc.db == nil {
		return ""
	}
	var title string
	err := fc.db.QueryRow(
		`SELECT window_title FROM activity_sessions ORDER BY end_time DESC LIMIT 1`,
	).Scan(&title)
	if err != nil || title == "" {
		return ""
	}

	subtype := matchWindowSubtype(title)
	if subtype != "" {
		return subtype
	}

	// LLM fallback for novel window titles.
	return fc.classifyWindowWithLLM(title)
}

// classifyWindowWithLLM asks the LLM to classify a window title into an activity subtype.
func (fc *FeatureComputer) classifyWindowWithLLM(title string) string {
	fc.mu.RLock()
	llm := fc.llmCall
	fc.mu.RUnlock()
	if llm == nil {
		return ""
	}
	prompt := `请根据窗口标题判断用户当前的活动类型。只输出一个英文标签，不要解释。

可选标签: debugging(调试), code_review(代码审查), meeting(会议), email(邮件), writing(写作), watching(看视频), gaming(游戏), shopping(购物), coding(编程), terminal(终端操作), other(其他)

窗口标题: ` + title
	result, err := llm(prompt)
	if err != nil {
		return ""
	}
	result = strings.TrimSpace(strings.ToLower(result))
	// Take first word only.
	if words := strings.Fields(result); len(words) > 0 && words[0] != "" {
		return words[0]
	}
	return ""
}

// U4: continuous work minutes from activity_sessions.
func (fc *FeatureComputer) computeContinuousWorkMins(now time.Time) (float64, int) {
	if fc.db == nil {
		return 0, 0
	}
	// Find the last non-working session end time in the last 24h.
	dayAgo := now.Unix() - 86400
	var lastBreakEnd int64
	fc.db.QueryRow(
		`SELECT COALESCE(MAX(end_time), ?) FROM activity_sessions
		WHERE is_working = 0 AND end_time > ?`,
		dayAgo, dayAgo,
	).Scan(&lastBreakEnd)

	var totalSec int64
	fc.db.QueryRow(
		`SELECT COALESCE(SUM(end_time - start_time), 0) FROM activity_sessions
		WHERE is_working = 1 AND start_time > ?`,
		lastBreakEnd,
	).Scan(&totalSec)
	return float64(totalSec) / 60.0, 1
}

// U5: app switch count in the last 30 minutes.
func (fc *FeatureComputer) computeAppSwitchCount(now time.Time) (float64, int) {
	if fc.db == nil {
		return 0, 0
	}
	since := now.Unix() - 1800
	var count int
	fc.db.QueryRow(
		`SELECT COUNT(*) FROM (
			SELECT app_name, LAG(app_name) OVER (ORDER BY start_time) as prev
			FROM activity_sessions WHERE start_time > ?
		) WHERE app_name != prev`,
		since,
	).Scan(&count)
	return float64(count), 1
}

// U7: message length trend comparing recent 5 vs previous 20 user messages.
func (fc *FeatureComputer) computeLengthTrend() (float64, int) {
	if fc.db == nil {
		return 0, 0
	}
	rows, err := fc.db.Query(
		`SELECT LENGTH(content) FROM chat_history WHERE role = 'user' ORDER BY created_at DESC LIMIT 25`,
	)
	if err != nil {
		return 0, 0
	}
	defer rows.Close()

	var lengths []float64
	for rows.Next() {
		var l int
		if rows.Scan(&l) == nil {
			lengths = append(lengths, float64(l))
		}
	}
	if len(lengths) < 6 {
		return 0, len(lengths)
	}
	recent := average(lengths[:5])
	baseline := average(lengths[5:])
	if baseline == 0 {
		return 0, len(lengths)
	}
	trend := (recent - baseline) / baseline
	return clampNeg1_1(trend), len(lengths)
}

// U8: EMA of response delays from recent outcomes.
func (fc *FeatureComputer) computeResponseDelayEMA() (float64, int) {
	if fc.db == nil {
		return 0, 0
	}
	rows, err := fc.db.Query(
		`SELECT response_delay FROM action_outcomes WHERE outcome > 0 ORDER BY created_at DESC LIMIT 10`,
	)
	if err != nil {
		return 0, 0
	}
	defer rows.Close()

	var delays []float64
	for rows.Next() {
		var d int
		if rows.Scan(&d) == nil {
			delays = append(delays, float64(d))
		}
	}
	if len(delays) == 0 {
		return 0, 0
	}
	// EMA with α=0.3
	ema := delays[0]
	for i := 1; i < len(delays); i++ {
		ema = 0.3*delays[i] + 0.7*ema
	}
	return ema, len(delays)
}

// U10 / R2: acceptance rate in the current ±1h time window.
func (fc *FeatureComputer) computeTimeWindowPref(now time.Time) (float64, int) {
	if fc.outcomeRepo == nil {
		return 0.5, 0
	}
	ctx := domain.ActionContext{HourOfDay: now.Hour(), DayOfWeek: int(now.Weekday())}
	accepts, total := fc.outcomeRepo.SuccessRate(ctx, 30)
	if total == 0 {
		return 0.5, 0
	}
	return float64(accepts) / float64(total), total
}

// U15: hours since user last mentioned fatigue/rest.
// Tier 1: keyword LIKE search. Tier 2: embedding semantic search.
func (fc *FeatureComputer) computeFatigueMentionHrs(now time.Time) (float64, int) {
	if fc.db == nil {
		return 999, 0
	}

	// Tier 1: keyword search.
	keywords := []string{"%吃饭%", "%休息%", "%累%", "%困%", "%饿%", "%疲惫%", "%午休%", "%睡觉%", "%加班%"}
	var maxCreated int64
	for _, kw := range keywords {
		var ts int64
		fc.db.QueryRow(`SELECT COALESCE(MAX(created_at), 0) FROM facts WHERE content LIKE ?`, kw).Scan(&ts)
		if ts > maxCreated {
			maxCreated = ts
		}
	}
	if maxCreated > 0 {
		return time.Since(time.Unix(maxCreated, 0)).Hours(), 1
	}

	// Tier 2: embedding semantic search.
	if ts, ok := fc.searchFatigueByEmbedding(); ok {
		return time.Since(time.Unix(ts, 0)).Hours(), 1
	}

	return 999, 0
}

// searchFatigueByEmbedding uses embedding similarity to find facts about
// fatigue/rest/eating when keyword search fails.
func (fc *FeatureComputer) searchFatigueByEmbedding() (int64, bool) {
	fc.mu.RLock()
	vec := fc.vectorizer
	fc.mu.RUnlock()
	if vec == nil || fc.db == nil {
		return 0, false
	}

	queryEmbedding, err := vec.Vectorize("主人提到吃饭、休息、累了、困了、饿了、疲惫、午休、睡觉")
	if err != nil || len(queryEmbedding) == 0 {
		return 0, false
	}

	// Load recent facts with vectors.
	since := time.Now().Unix() - 604800 // 7 days
	rows, err := fc.db.Query(
		`SELECT content, vector, created_at FROM facts
		WHERE vector IS NOT NULL AND created_at > ?
		ORDER BY created_at DESC LIMIT 200`, since,
	)
	if err != nil {
		return 0, false
	}
	defer rows.Close()

	type factRow struct {
		content   string
		vec       []float32
		createdAt int64
	}

	var bestMatch *factRow
	bestSim := 0.0
	threshold := 0.65 // minimum cosine similarity to consider a match

	for rows.Next() {
		var f factRow
		var vecBlob []byte
		if rows.Scan(&f.content, &vecBlob, &f.createdAt) != nil {
			continue
		}
		f.vec = storage.DecodeVector(vecBlob)
		if len(f.vec) == 0 {
			continue
		}
		sim := storage.CosineSimilarity(queryEmbedding, f.vec)
		if sim > bestSim && sim >= threshold {
			bestSim = sim
			cp := f
			bestMatch = &cp
		}
	}

	if bestMatch == nil {
		return 0, false
	}
	return bestMatch.createdAt, true
}

// computePrefDiversity counts distinct preference categories from identity_nodes
// using embedding-based greedy clustering.
func (fc *FeatureComputer) computePrefDiversity() (float64, int) {
	if fc.db == nil {
		return 0, 0
	}

	rows, err := fc.db.Query(
		`SELECT content, embedding FROM identity_nodes WHERE node_type = 'preference' AND active = 1 AND embedding IS NOT NULL`,
	)
	if err != nil {
		return 0, 0
	}
	defer rows.Close()

	var nodes []prefNode
	for rows.Next() {
		var n prefNode
		var blob []byte
		if rows.Scan(&n.content, &blob) == nil {
			n.vec = storage.DecodeVector(blob)
			nodes = append(nodes, n)
		}
	}
	_ = rows.Err()

	if len(nodes) == 0 {
		// Fallback: count nodes without embeddings.
		var count int
		fc.db.QueryRow(
			`SELECT COUNT(*) FROM identity_nodes WHERE node_type = 'preference' AND active = 1`,
		).Scan(&count)
		return saturateNorm(float64(count), 5), int(count)
	}

	// Greedy clustering by cosine similarity (threshold 0.70).
	clusters := clusterBySimilarity(nodes, 0.70)
	return saturateNorm(float64(len(clusters)), 5), int(len(clusters))
}

type prefNode struct {
	content string
	vec     []float32
}

// clusterBySimilarity performs greedy single-pass clustering on preference nodes.
// Each node is assigned to the first cluster whose centroid similarity exceeds the threshold.
func clusterBySimilarity(nodes []prefNode, threshold float64) [][]prefNode {
	if len(nodes) == 0 {
		return nil
	}
	clusters := [][]prefNode{{nodes[0]}}
	centroids := [][]float32{copyVec(nodes[0].vec)}

	for i := 1; i < len(nodes); i++ {
		n := nodes[i]
		bestCluster := -1
		bestSim := 0.0

		for j, centroid := range centroids {
			sim := storage.CosineSimilarity(n.vec, centroid)
			if sim > bestSim {
				bestSim = sim
				bestCluster = j
			}
		}

		if bestSim >= threshold && bestCluster >= 0 {
			clusters[bestCluster] = append(clusters[bestCluster], n)
			centroids[bestCluster] = clusterCentroid(clusters[bestCluster])
		} else {
			clusters = append(clusters, []prefNode{n})
			centroids = append(centroids, copyVec(n.vec))
		}
	}
	return clusters
}

func copyVec(v []float32) []float32 {
	out := make([]float32, len(v))
	copy(out, v)
	return out
}

func clusterCentroid(nodes []prefNode) []float32 {
	if len(nodes) == 0 {
		return nil
	}
	dim := len(nodes[0].vec)
	centroid := make([]float32, dim)
	for _, n := range nodes {
		for j := range n.vec {
			centroid[j] += n.vec[j]
		}
	}
	for j := range centroid {
		centroid[j] /= float32(len(nodes))
	}
	return centroid
}

// A7: success rate per action type (14-day window).
func (fc *FeatureComputer) computeActionSuccessByType() map[string]float64 {
	if fc.outcomeRepo == nil {
		return nil
	}
	rates, err := fc.outcomeRepo.SuccessRateByType(14)
	if err != nil {
		return nil
	}
	result := make(map[string]float64, len(rates))
	for k, v := range rates {
		result[string(k)] = v
	}
	return result
}

// A8: success rate per 4 time blocks (30-day window for sufficient samples).
func (fc *FeatureComputer) computeTimeBlockRate() map[int]float64 {
	if fc.outcomeRepo == nil {
		return nil
	}
	rates, err := fc.outcomeRepo.SuccessRateByTimeBlock(30)
	if err != nil {
		return nil
	}
	return rates
}

// A10: count of active conversation threads touched within 7 days.
func (fc *FeatureComputer) computeActiveGoals(now time.Time) (float64, int) {
	if fc.db == nil {
		return 0, 0
	}
	since := now.Unix() - 604800
	var count int
	fc.db.QueryRow(
		`SELECT COUNT(*) FROM conversation_threads WHERE status = 'active' AND last_touched_at > ?`,
		since,
	).Scan(&count)
	return float64(count), count
}

// A13: new facts learned in the last 24 hours.
func (fc *FeatureComputer) computeNewFacts24h(now time.Time) (float64, int) {
	if fc.db == nil {
		return 0, 0
	}
	since := now.Unix() - 86400
	var count int
	fc.db.QueryRow(`SELECT COUNT(*) FROM facts WHERE created_at > ?`, since).Scan(&count)
	return float64(count), count
}

// R1: overall acceptance rate from the last 20 outcomes.
func (fc *FeatureComputer) computeOverallAcceptRate() (float64, float64) {
	if fc.db == nil {
		return 0.5, 0
	}
	var accepts, total int
	fc.db.QueryRow(
		`SELECT COALESCE(SUM(CASE WHEN outcome > 0 THEN 1 ELSE 0 END), 0), COUNT(*)
		FROM (SELECT outcome FROM action_outcomes ORDER BY created_at DESC LIMIT 20)`,
	).Scan(&accepts, &total)
	if total == 0 {
		return 0.5, 0
	}
	return float64(accepts) / float64(total), float64(total)
}

// R3: acceptance rate per proactive source (14-day window).
func (fc *FeatureComputer) computeSourceAcceptRate() map[string]float64 {
	if fc.outcomeRepo == nil {
		return nil
	}
	rates, err := fc.outcomeRepo.SuccessRateBySource(14)
	if err != nil {
		return nil
	}
	result := make(map[string]float64, len(rates))
	for k, v := range rates {
		result[string(k)] = v
	}
	return result
}

// R4: rejection count in the last 5 outcomes.
func (fc *FeatureComputer) computeRecentRejections() (float64, int) {
	if fc.db == nil {
		return 0, 0
	}
	var count int
	fc.db.QueryRow(
		`SELECT COUNT(*) FROM (SELECT outcome FROM action_outcomes ORDER BY created_at DESC LIMIT 5) WHERE outcome = -1`,
	).Scan(&count)
	return float64(count), count
}

// R5: hours since the last user message (neglect).
func (fc *FeatureComputer) computeNeglectHours(now time.Time) (float64, int) {
	if fc.db == nil {
		return 0, 0
	}
	var lastUserMsg int64
	err := fc.db.QueryRow(
		`SELECT MAX(created_at) FROM chat_history WHERE role = 'user'`,
	).Scan(&lastUserMsg)
	if err != nil || lastUserMsg == 0 {
		return 0, 0
	}
	return time.Since(time.Unix(lastUserMsg, 0)).Hours(), 1
}

// R6: conversation depth trend — avg turns per session, recent vs baseline.
func (fc *FeatureComputer) computeDepthTrend() (float64, int) {
	if fc.db == nil {
		return 0, 0
	}
	// Sample up to 500 recent messages for better session coverage.
	rows, err := fc.db.Query(
		`SELECT created_at FROM chat_history ORDER BY created_at DESC LIMIT 500`,
	)
	if err != nil {
		return 0, 0
	}
	defer rows.Close()

	var timestamps []int64
	for rows.Next() {
		var ts int64
		if rows.Scan(&ts) == nil {
			timestamps = append(timestamps, ts)
		}
	}
	if len(timestamps) < 20 {
		return 0, len(timestamps) // insufficient data
	}

	// Split into sessions: gap > 60 min = new conversation.
	const sessionGapSec = 3600
	var turns []int
	currentTurns := 1
	for i := 1; i < len(timestamps); i++ {
		if timestamps[i-1]-timestamps[i] > sessionGapSec {
			turns = append(turns, currentTurns)
			currentTurns = 1
		} else {
			currentTurns++
		}
	}
	turns = append(turns, currentTurns)

	// Need at least 3 sessions for a meaningful trend.
	if len(turns) < 3 {
		return 0, len(turns)
	}

	// EMA-smoothed comparison: recent third vs earlier two-thirds.
	split := len(turns) / 3
	if split < 1 {
		split = 1
	}
	recentAvg := averageInt(turns[:split])
	baselineAvg := averageInt(turns[split:])
	if baselineAvg == 0 {
		return 0, len(turns)
	}
	trend := (recentAvg - baselineAvg) / baselineAvg
	return clampNeg1_1(trend), len(turns)
}

// R7: user-initiated messages in the last 24h.
func (fc *FeatureComputer) computeUserInitiative24h(now time.Time) (float64, int) {
	if fc.db == nil {
		return 0, 0
	}
	since := now.Unix() - 86400
	var count int
	fc.db.QueryRow(
		`SELECT COUNT(*) FROM chat_history WHERE role = 'user' AND created_at > ?`,
		since,
	).Scan(&count)
	return float64(count), count
}

// R8 proxy: 7-day average affection (uses R1 7-day success rate as proxy).
func (fc *FeatureComputer) computeAffection7dMA() (float64, int) {
	if fc.outcomeRepo == nil {
		return 0.5, 0
	}
	accepts, total := fc.outcomeRepo.SuccessRate(domain.ActionContext{}, 7)
	if total == 0 {
		return 0.5, 0
	}
	return float64(accepts) / float64(total), total
}

// R8: intimacy trend compares 7-day vs 30-day acceptance.
func (fc *FeatureComputer) computeIntimacyTrend(affection7d float64) (float64, int) {
	if fc.outcomeRepo == nil {
		return 0, 0
	}
	accepts30, total30 := fc.outcomeRepo.SuccessRate(domain.ActionContext{}, 30)
	if total30 == 0 {
		return 0, 0
	}
	baseline := float64(accepts30) / float64(total30)
	return clampNeg1_1(affection7d - baseline), total30
}

// T5: today's activity session count.
func (fc *FeatureComputer) computeTodayActivityCount(now time.Time) (float64, int) {
	if fc.db == nil {
		return 0, 0
	}
	todayStart := now.Truncate(24 * time.Hour).Unix()
	var count int
	fc.db.QueryRow(
		`SELECT COUNT(*) FROM activity_sessions WHERE start_time > ?`,
		todayStart,
	).Scan(&count)
	return float64(count), count
}

// ---- App Category & Window Subtype Helpers ----

func defaultAppCategories() map[string]string {
	return map[string]string{
		"code": "work", "terminal": "work", "xcode": "work", "android studio": "work",
		"intellij": "work", "pycharm": "work", "sublime": "work", "vim": "work",
		"vscode": "work", "visual studio code": "work", "emacs": "work",
		"slack": "work", "figma": "work", "notion": "work", "obsidian": "work",
		"钉钉": "work", "飞书": "work", "teams": "work", "zoom": "social",
		"bilibili": "play", "netflix": "play", "youtube": "play", "steam": "play",
		"epic": "play", "spotify": "play", "apple music": "play",
		"wechat": "social", "qq": "social", "discord": "social", "telegram": "social",
		"微信": "social", "微博": "social", "twitter": "social",
	}
}

func categorizeApp(appName string) string {
	lower := strings.ToLower(appName)
	for prefix, cat := range defaultAppCategories() {
		if strings.Contains(lower, prefix) {
			return cat
		}
	}
	return "idle"
}

// windowSubtypePatterns maps regex-like keywords to activity subtypes.
var windowSubtypePatterns = []struct {
	keywords []string
	subtype  string
}{
	{[]string{"bug", "debug", "fix", "修复", "调试", "defect"}, "debugging"},
	{[]string{"pr ", " pr", "review", "code review", "审查", "评审"}, "code_review"},
	{[]string{"会议", "meeting", "zoom", "腾讯会议", "teams"}, "meeting"},
	{[]string{"邮件", "mail", "inbox", "gmail", "outlook"}, "email"},
	{[]string{"文档", "doc", "wiki", "notion", "confluence"}, "writing"},
	{[]string{"视频", "video", "bilibili", "youtube", "netflix", "播放"}, "watching"},
	{[]string{"游戏", "game", "league", "原神", "星铁", "steam"}, "gaming"},
	{[]string{"购物", "taobao", "jd", "amazon", "淘宝", "京东"}, "shopping"},
	{[]string{"vscode", "code", "coding", "编程", "编辑器"}, "coding"},
	{[]string{"terminal", "终端", "iterm", "warp", "zsh", "bash"}, "terminal"},
}

func matchWindowSubtype(title string) string {
	lower := strings.ToLower(title)
	for _, p := range windowSubtypePatterns {
		for _, kw := range p.keywords {
			if strings.Contains(lower, kw) {
				return p.subtype
			}
		}
	}
	return ""
}

// ---- Math Helpers ----

func average(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}

func averageInt(vals []int) float64 {
	if len(vals) == 0 {
		return 0
	}
	sum := 0
	for _, v := range vals {
		sum += v
	}
	return float64(sum) / float64(len(vals))
}

func clampNeg1_1(v float64) float64 {
	if v < -1 {
		return -1
	}
	if v > 1 {
		return 1
	}
	return v
}

func saturateNorm(value, saturationPoint float64) float64 {
	if saturationPoint <= 0 {
		return 0
	}
	return math.Tanh(value / saturationPoint * 2.0)
}

func boolToFloat(b bool) float64 {
	if b {
		return 1.0
	}
	return 0.0
}

func vecDistance(a, b domain.EmotionVector) float64 {
	diff := [8]float64{
		a.Affection - b.Affection,
		a.Worry - b.Worry,
		a.Curiosity - b.Curiosity,
		a.Sleepiness - b.Sleepiness,
		a.Playfulness - b.Playfulness,
		a.Loneliness - b.Loneliness,
		a.Confidence - b.Confidence,
		a.Annoyance - b.Annoyance,
	}
	sum := 0.0
	for _, d := range diff {
		sum += d * d
	}
	return math.Sqrt(sum)
}

// ---- Ring Buffer for Emotion History ----

type emotionSnapshot struct {
	valence float64
	vec     domain.EmotionVector
	at      time.Time
}

type ringBuffer struct {
	buf  []emotionSnapshot
	head int
	size int
}

func newRingBuffer(cap int) *ringBuffer {
	return &ringBuffer{buf: make([]emotionSnapshot, cap)}
}

func (rb *ringBuffer) Push(s emotionSnapshot) {
	rb.buf[rb.head] = s
	rb.head = (rb.head + 1) % len(rb.buf)
	if rb.size < len(rb.buf) {
		rb.size++
	}
}

// All returns all snapshots in chronological order (oldest first).
func (rb *ringBuffer) All() []emotionSnapshot {
	if rb.size == 0 {
		return nil
	}
	result := make([]emotionSnapshot, rb.size)
	start := (rb.head - rb.size + len(rb.buf)) % len(rb.buf)
	for i := 0; i < rb.size; i++ {
		result[i] = rb.buf[(start+i)%len(rb.buf)]
	}
	return result
}

func (rb *ringBuffer) SnapshotNHoursAgo(hours int) *emotionSnapshot {
	if rb.size == 0 {
		return nil
	}
	target := time.Now().Add(-time.Duration(hours) * time.Hour)

	var best *emotionSnapshot
	bestDiff := time.Hour * 24
	for i := 0; i < rb.size; i++ {
		idx := (rb.head - 1 - i + len(rb.buf)) % len(rb.buf)
		s := &rb.buf[idx]
		diff := s.at.Sub(target)
		if diff < 0 {
			diff = -diff
		}
		if diff < bestDiff {
			bestDiff = diff
			best = s
		}
	}
	if bestDiff > 30*time.Minute {
		return nil
	}
	return best
}
