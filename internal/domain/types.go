// Package domain defines the core data types and interfaces for the desktop pet
// application. It has zero dependencies on other desktop-pet packages — it only
// imports the Go standard library.
package domain

import (
	"sync"
	"time"
)

// ---- Fact Types ----

// FactRole labels an AtomicFact's relationship to its parent Episode.
type FactRole string

const (
	RoleCore     FactRole = "core"
	RoleContext  FactRole = "context"
	RoleDetail   FactRole = "detail"
	RoleTemporal FactRole = "temporal"
	RoleCausal   FactRole = "causal"
)

// FactEntry is the complete information for a stored fact.
type FactEntry struct {
	ID             int64
	Content        string
	Importance     float64
	FactRole       FactRole
	StartTime      int64
	EndTime        int64
	LastRecalledAt int64
	RecallCount    int
	Vector         []float32
	EpisodeID      int64
	Source         string
	CreatedAt      int64
	UpdatedAt      int64
}

// AtomicFactInput is the LLM-extracted atomic fact before persistence.
type AtomicFactInput struct {
	Content    string   `json:"content"`
	Importance float64  `json:"importance"`
	Confidence float64  `json:"confidence"` // 0-1: how certain the LLM is that this is a real fact
	FactRole   FactRole `json:"fact_role"`
	StartTime  int64    `json:"start_time"`
	EndTime    int64    `json:"end_time"`
	Source     string   `json:"source"`
}

// ---- Emotion Types ----

// EmotionState represents a PAD three-dimensional emotion plus the primary
// emotion label and intensity.
type EmotionState struct {
	Valence   float64 `json:"valence"`   // -1 (unpleasant) ~ +1 (pleasant)
	Arousal   float64 `json:"arousal"`   // -1 (calm) ~ +1 (excited)
	Dominance float64 `json:"dominance"` // -1 (controlled) ~ +1 (in-control)
	Primary   string  `json:"primary"`   // joy/sadness/anger/fear/surprise/disgust/neutral
	Intensity float64 `json:"intensity"` // 0 ~ 1
}

// EmotionVector represents simultaneous multi-dimensional emotions.
type EmotionVector struct {
	Affection   float64 `json:"affection"`   // 0~1
	Worry       float64 `json:"worry"`       // 0~1
	Curiosity   float64 `json:"curiosity"`   // 0~1
	Sleepiness  float64 `json:"sleepiness"`  // 0~1
	Playfulness float64 `json:"playfulness"` // 0~1
	Loneliness  float64 `json:"loneliness"`  // 0~1
	Confidence  float64 `json:"confidence"`  // 0~1
	Annoyance   float64 `json:"annoyance"`   // 0~1
}

// EmotionHistoryEntry is a historical emotion state record.
type EmotionHistoryEntry = EmotionState

// ---- Care Types ----

// CareMood describes the emotional tone of a care action.
type CareMood int

const (
	MoodNeutral  CareMood = iota
	MoodGentle            // 温柔模式
	MoodPlayful           // 调皮模式
	MoodFirm              // 强势模式
	MoodWorried           // 担忧模式
	MoodTsundere          // 傲娇模式
)

// CareTriggerType is the type of a care trigger.
type CareTriggerType string

const (
	TriggerHydration CareTriggerType = "hydration"
	TriggerMeal      CareTriggerType = "meal"
	TriggerRest      CareTriggerType = "rest"
	TriggerEncourage CareTriggerType = "encourage"
	TriggerSocial    CareTriggerType = "social"
	TriggerHealth    CareTriggerType = "health"
)

// ProactiveSource tags the origin of a proactive action.
type ProactiveSource string

const (
	SourceCare         ProactiveSource = "care"
	SourceKnowledgeGap ProactiveSource = "knowledge_gap"
	SourceCasual       ProactiveSource = "casual"
)

// CareAction is a record of one care behavior.
type CareAction struct {
	ID          int64
	Type        CareTriggerType
	Priority    int
	Mood        CareMood        `json:"mood"`
	Source      ProactiveSource `json:"source"`
	Message     string
	TriggeredAt time.Time
	Accepted    *bool  // nil=no feedback yet, true=accepted, false=rejected
	Response    string // user reply summary
	Observed    bool   `json:"observed"` // set when screen observation was used
}

// CareTrigger defines a care trigger condition with cooldown and daily cap.
type CareTrigger struct {
	Type      CareTriggerType
	Priority  int
	Condition func(state *UserCareState, emotion *EmotionState, now time.Time) bool
	Cooldown  time.Duration
	MaxDaily  int

	mu           sync.Mutex
	lastFiredAt  time.Time
	dailyCount   int
	lastResetDay int64
}

// ObservationSource tags where an observation came from.
type ObservationSource string

const (
	ObsChat   ObservationSource = "chat"
	ObsQQ     ObservationSource = "qq"
	ObsScreen ObservationSource = "screen"
	ObsWeb    ObservationSource = "web"
	ObsSystem ObservationSource = "system"
)

// Observation is a generic data point from any source.
type Observation struct {
	Source    ObservationSource `json:"source"`
	Content   string            `json:"content"`
	Timestamp time.Time         `json:"timestamp"`
	Metadata  map[string]any    `json:"metadata,omitempty"`
}

// UserCareState describes the user's current well-being across six dimensions.
type UserCareState struct {
	Mu sync.Mutex // exported for cross-package access

	// Physiological
	LastMealAt     time.Time
	LastDrinkAt    time.Time
	ContinuousWork int
	PostureWarning bool

	// Psychological
	StressLevel float64
	MoodTrend   string // "rising" | "falling" | "stable"
	BurnoutRisk float64

	// Social
	SocialActivity float64
	IsolationHours float64

	// Work
	FocusLevel     float64
	TaskComplexity string
	DeadlinesNear  bool

	// Agent relationship
	AnnoyanceLevel float64 // 0~1, rises on rejection, falls on acceptance

	LastUpdated time.Time
}

// LLMCareStateResult is the JSON output of LLM state inference.
type LLMCareStateResult struct {
	StressLevel    float64 `json:"stress_level"`
	StressEvidence string  `json:"stress_evidence"`
	MoodTrend      string  `json:"mood_trend"`
	BurnoutRisk    float64 `json:"burnout_risk"`
	FocusLevel     float64 `json:"focus_level"`
	FocusEvidence  string  `json:"focus_evidence"`
	TaskComplexity string  `json:"task_complexity"`
	SocialActivity float64 `json:"social_activity"`
	LikelyActivity string  `json:"likely_activity"`
	CareSuggestion string  `json:"care_suggestion"`
}

// ---- Screen Observation ----

// ScreenObservation captures a snapshot of what the user is doing on screen.
type ScreenObservation struct {
	AppName     string
	WindowTitle string
	OCRText     string
	CapturedAt  time.Time
	IsWorking   bool
}

// ---- Scheduler Types ----

// SchedulerInput is the snapshot of state used to decide whether to act.
type SchedulerInput struct {
	Now                time.Time
	TimeSinceLastChat  time.Duration
	EmotionVec         EmotionVector
	EmotionState       EmotionState
	UserState          *UserCareState
	RecentUserMsg      string
	LastScreenObs      ScreenObservation
	TacticalDirectives []string // from StrategicAgent, influences pickAction
}

// SchedulerResult describes what the Scheduler decided.
type SchedulerResult struct {
	ShouldAct      bool
	Source         ProactiveSource
	Reason         string
	Score          float64
	Escalation     int
	EmotionContext string
	ContextAnchor  string
}

// ---- Search Types ----

// SearchResult is a keyword search result from archives or facts.
type SearchResult struct {
	Name    string
	Level   int
	Summary string
	Source  string // "archive" or "fact"
}

// UnifiedResult is a unified search result from multiple sources.
type UnifiedResult struct {
	Source    string // "fact" | "diary" | "episode"
	ID        int64
	Content   string
	Score     float64
	DecayW    float64
	CreatedAt int64
}

// ---- Diary Types ----

// DiaryEntry is a single diary record with emotional annotation and embedding.
type DiaryEntry struct {
	ID             int64
	Title          string
	Summary        string
	Vector         []float32
	EmotionValence float64
	EmotionArousal float64
	StartTime      int64
	EndTime        int64
	CreatedAt      int64
}

// ---- Episode & Topic Types ----

// EpisodeEntry is a semantic cluster of related AtomicFacts.
type EpisodeEntry struct {
	ID         int64
	Title      string
	Summary    string
	Centroid   []float32
	TopicID    int64
	Importance float64
	FactCount  int
	StartTime  int64
	EndTime    int64
	CreatedAt  int64
	UpdatedAt  int64
}

// TopicEntry is a high-level theme grouping related Episodes.
type TopicEntry struct {
	ID           int64
	Name         string
	Centroid     []float32
	Description  string
	EpisodeCount int
	CreatedAt    int64
	UpdatedAt    int64
}

// ---- MemCell Types ----

// MemCellType is the category of an atomic memory cell.
type MemCellType string

const (
	CellFact     MemCellType = "fact"
	CellPrefer   MemCellType = "prefer"
	CellEvent    MemCellType = "event"
	CellEmotion  MemCellType = "emotion"
	CellSkill    MemCellType = "skill"
	CellRelation MemCellType = "relation"
)

// MemCell is an atomic memory unit extracted from a conversation turn.
type MemCell struct {
	ID         int64        `json:"-"`
	Type       MemCellType  `json:"type"`
	Content    string       `json:"content"`
	Importance float64      `json:"importance"`
	Emotion    EmotionState `json:"-"`
	SourceMsg  string       `json:"-"`
	Vector     []float32    `json:"-"`
	CreatedAt  int64        `json:"-"`
}

// ---- Action Outcome (Adaptive Learning) ----

// OutcomeResult records whether a proactive action was accepted.
type OutcomeResult int

const (
	OutcomeIgnored  OutcomeResult = 0  // user didn't respond
	OutcomeReplied  OutcomeResult = 1  // user replied
	OutcomeEngaged  OutcomeResult = 2  // user replied positively / continued the topic
	OutcomeRejected OutcomeResult = -1 // user explicitly rejected ("别烦" etc.)
)

// ActionOutcome records the result of one proactive action for adaptive learning.
type ActionOutcome struct {
	ID            int64
	ActionSource  ProactiveSource // care / casual / knowledge_gap
	ActionType    CareTriggerType // rest / meal / social / encourage / ...
	HourOfDay     int             // 0-23
	DayOfWeek     int             // 0-6
	AppContext    string          // what app the user was using
	EmotionBucket string          // discretized emotion vector key
	EscalationLvl int             // escalation level at time of action
	Outcome       OutcomeResult   // 0=ignored 1=replied 2=engaged -1=rejected
	ResponseDelay int             // seconds until response (0 if ignored)
	CreatedAt     int64
}

// ActionContext is the query key for finding similar historical outcomes.
type ActionContext struct {
	Source        ProactiveSource
	Type          CareTriggerType
	HourOfDay     int
	DayOfWeek     int
	AppContext    string
	EmotionBucket string
}

// ---- User Profile ----

// UserProfile holds learned information about the user.
type UserProfile struct {
	Name      string
	TechStack []string
}

// ---- Vectorizer ----

// Vectorizer is the interface for text-to-vector embedding.
type Vectorizer interface {
	Vectorize(text string) ([]float32, error)
}

// ---- Chat Types ----

// Message represents a single chat message.
type Message struct {
	Role       string
	Content    string
	Images     []Image
	ToolCallID string      `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
	Meta       interface{} // 用于内嵌标记压缩
}

// ToolCall is a tool invocation from the LLM.
type ToolCall struct {
	Index    int              `json:"index"`
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction holds the name and JSON-encoded arguments.
type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Image holds base64-encoded image data.
type Image struct {
	Base64 string
	Format string
}

// ---- Strategy Principle (PRINCIPLES-inspired) ----

// StrategyPrinciple is a reusable behavioral rule extracted from interaction outcomes.
// Inspired by PRINCIPLES (Kim et al., EMNLP 2025).
type StrategyPrinciple struct {
	ID           int64     `json:"id"`
	Situation    string    `json:"situation"`     // "当主人在深夜还在写代码时"
	GoodStrategy string    `json:"good_strategy"` // "用傲娇的语气催睡"
	BadStrategy  string    `json:"bad_strategy"`  // "温和提醒"
	Reason       string    `json:"reason"`        // "主人对温和提醒免疫，但傲娇催睡有效"
	Confidence   float64   `json:"confidence"`
	Source       string    `json:"source"` // "daily_reflection" / "immediate_feedback"
	Tags         string    `json:"tags"`   // comma-separated: "深夜,关心,傲娇"
	Embedding    []float32 `json:"-"`
	Active       bool      `json:"active"`
	CreatedAt    int64     `json:"created_at"`
	UpdatedAt    int64     `json:"updated_at"`
}

// ThreadRecommendation is a conversation thread lifecycle decision from the strategic agent.
type ThreadRecommendation struct {
	Action       string  `json:"action"`        // "create" / "resolve" / "stale"
	Type         string  `json:"type"`          // follow_up / exploration / care / entertainment
	Goal         string  `json:"goal"`          // required for "create"
	BestApproach string  `json:"best_approach"` // how to bring it up naturally
	Priority     float64 `json:"priority"`
	ThreadID     int64   `json:"thread_id"` // required for "resolve" / "stale"
	Outcome      string  `json:"outcome"`   // for "resolve"
	Learnings    string  `json:"learnings"` // for "resolve"
}

// DailyReflectionInput is the snapshot of state used for the daily strategic reflection.
type DailyReflectionInput struct {
	YesterdayOutcomes   []ActionOutcome
	YesterdayFacts      []string // summary of newly learned facts
	CurrentSelfModel    string
	ActivePrinciples    []StrategyPrinciple
	ActiveThreads       []ConversationThread
	RecentDiaries       []string // last 7 days' diary summaries
	EmotionHistoryToday string   // brief emotion trajectory
	InteractionCount    int
	ProactiveAcceptRate float64
}

// DailyReflectionOutput is the result of the daily strategic reflection.
type DailyReflectionOutput struct {
	SelfModelUpdate        string                 `json:"self_model_update"`
	NewPrinciples          []StrategyPrinciple    `json:"new_principles"`
	DeactivatePrincipleIDs []int64                `json:"deactivate_principle_ids"`
	TacticalDirectives     []string               `json:"tactical_directives"`
	ThreadRecommendations  []ThreadRecommendation `json:"thread_recommendations"`
	NarrativeSummary       string                 `json:"narrative_summary"`
}

// ChatContext holds the state of a single chat interaction.
type ChatContext struct {
	Input     string
	Messages  []Message
	Output    string
	Compacted bool
	Source    string // "chat" or "qq"
}

// ---- System 2 Decision Types ----

// DecisionContext is the full input to the System 2 LLM decision prompt.
type DecisionContext struct {
	Now               time.Time
	EmotionVec        EmotionVector
	EmotionState      EmotionState
	TimeSinceLastChat time.Duration
	ScreenSummary     string // current app + title
	RecentUserMsg     string
	DailyActionCount  int
	ActivePatterns    []BehaviorPattern
	ActivePrinciples  []StrategyPrinciple
	ActiveInquiries   int // count of unanswered inquiries
	RecentOutcomes    []ActionOutcome
	KnowledgeGaps     int // count of active gaps
	RecentFactSample  []string
	TacticalDirectives []string
	SelfSummary       string   // AI's self-model summary
}

// DecisionOutput is the LLM's autonomous decision.
type DecisionOutput struct {
	ShouldAct bool   `json:"should_act"`
	Action    string `json:"action"`  // "speak" | "observe" | "learn" | "reflect" | "none"
	Source    string `json:"source"`  // "care" | "casual" | "knowledge_gap" (if action=speak)
	Reason    string `json:"reason"`  // why the LLM chose this
	Mood      string `json:"mood"`    // "gentle" | "playful" | "firm" | "tsundere" | "worried"
	Priority  float64 `json:"priority"`
	ToolName  string `json:"tool"`    // which tool to invoke
	ToolInput string `json:"tool_input"` // URL for browse, query for search, etc.
}

// ToolResult is the outcome of executing a tool.
type ToolResult struct {
	ToolName string
	Success  bool
	Output   string
	Error    string
	Duration time.Duration
}
