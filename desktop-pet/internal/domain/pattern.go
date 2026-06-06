package domain

// ActivityEvent is a continuous session of the user using a single app.
// Consecutive identical observations are merged: a new row is only written when
// app_name or window_title changes. Each row covers a start→end time range.
type ActivityEvent struct {
	ID          int64  `json:"id"`
	AppName     string `json:"app_name"`
	WindowTitle string `json:"window_title"`
	IsWorking   bool   `json:"is_working"`
	StartTime   int64  `json:"start_time"`
	EndTime     int64  `json:"end_time"`
}

// BehaviorPattern is a discovered temporal regularity in the user's behavior.
type BehaviorPattern struct {
	ID          int64   `json:"id"`
	Pattern     string  `json:"pattern"`     // "工作日15:00左右出现注意力分散"
	Type        string  `json:"type"`        // daily_rhythm / work_habit / interest_shift / social_pattern
	Evidence    string  `json:"evidence"`    // what supports this pattern
	Confidence  float64 `json:"confidence"`  // 0~1
	Implication string  `json:"implication"` // 对系统的启示: "在14:55主动送鼓励"
	Active      bool    `json:"active"`
	CreatedAt   int64   `json:"created_at"`
	UpdatedAt   int64   `json:"updated_at"`
}

// PatternTrigger is a preemptive trigger derived from a behavior pattern.
// When the current time matches a pattern's expected window, the scheduler
// can fire a proactive action before the predicted event.
type PatternTrigger struct {
	Pattern  *BehaviorPattern
	Action   string // suggested proactive action type
	Message  string // pre-generated message hint
	Priority float64
}

// ActivityEventRepository persists activity sessions.
type ActivityEventRepository interface {
	RecordSession(session ActivityEvent) (int64, error)
	UpdateSessionEnd(id int64, endTime int64) error
	ListToday() ([]ActivityEvent, error)
	ListRange(since, until int64) ([]ActivityEvent, error)
	CleanOld(days int) int
}

// PatternRepository persists discovered behavior patterns.
type PatternRepository interface {
	SavePattern(p BehaviorPattern) (int64, error)
	ListActive() ([]BehaviorPattern, error)
	ListByType(patternType string) ([]BehaviorPattern, error)
	UpdateConfidence(id int64, delta float64) error
	Deactivate(id int64) error
	CleanInactive(days int) int
}

// Helpers.

// TimeToSlot maps an hour to a coarse time slot label.
func TimeToSlot(hour int) string {
	switch {
	case hour >= 23 || hour < 6:
		return "深夜"
	case hour >= 6 && hour < 9:
		return "早晨"
	case hour >= 9 && hour < 12:
		return "上午"
	case hour >= 12 && hour < 14:
		return "午间"
	case hour >= 14 && hour < 18:
		return "下午"
	default:
		return "晚间"
	}
}

// IsBoundaryCandidate returns true if an app switch likely indicates an event boundary.
func IsBoundaryCandidate(from, to string) bool {
	workApps := map[string]bool{
		"VS Code": true, "Terminal": true, "Xcode": true, "IntelliJ": true,
		"Slack": true, "Figma": true, "Notion": true, "Obsidian": true,
	}
	entertainmentApps := map[string]bool{
		"Bilibili": true, "YouTube": true, "Netflix": true, "Twitter": true,
		"WeChat": true, "QQ": true, "微博": true,
	}
	workToEnt := workApps[from] && entertainmentApps[to]
	entToWork := entertainmentApps[from] && workApps[to]
	return workToEnt || entToWork
}



// EventSegment is a meaningful chunk of user activity, split at natural boundaries
// (app switch patterns, idle periods, etc.). Inspired by Event Segmentation Theory
// as used in Nemori (2025).
type EventSegment struct {
	ID          int64  `json:"id"`
	Label       string `json:"label"`        // "上午开发" / "午休" / "开会" / "娱乐"
	Summary     string `json:"summary"`      // brief description
	AppSequence string `json:"app_sequence"` // comma-separated app names
	DurationMin int    `json:"duration_min"`
	IsFocused   bool   `json:"is_focused"`
	DayOfWeek   int    `json:"day_of_week"` // 0-6
	HourStart   int    `json:"hour_start"`  // 0-23
	CreatedAt   int64  `json:"created_at"`
}
