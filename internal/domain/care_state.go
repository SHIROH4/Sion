package domain

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// NewUserCareState creates a default state with neutral mid-values.
func NewUserCareState() *UserCareState {
	return &UserCareState{
		MoodTrend:      "stable",
		TaskComplexity: "medium",
	}
}

// UpdateFromObservation updates the state from a single Observation.
func (s *UserCareState) UpdateFromObservation(obs Observation) {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	switch obs.Source {
	case ObsScreen:
		s.updateFromScreen(obs)
	case ObsQQ:
		s.updateFromQQ(obs)
	case ObsChat:
		s.updateFromChat(obs)
	}

	s.LastUpdated = time.Now()
}

// UpdateStress sets the user's stress level (0-1), derived from emotion model.
func (s *UserCareState) UpdateStress(stress float64) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	s.StressLevel = clampTo01(stress)
}

// IncrementIsolation adds hours to the isolation counter.
func (s *UserCareState) IncrementIsolation(hours float64) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	s.IsolationHours += hours
}

// UpdateFromLLM overwrites fields from an LLM state inference result.
func (s *UserCareState) UpdateFromLLM(result *LLMCareStateResult) {
	if result == nil {
		return
	}
	s.Mu.Lock()
	defer s.Mu.Unlock()

	if result.StressLevel != 0 {
		s.StressLevel = clampTo01(result.StressLevel)
	}
	if result.MoodTrend != "" {
		s.MoodTrend = result.MoodTrend
	}
	if result.BurnoutRisk != 0 {
		s.BurnoutRisk = clampTo01(result.BurnoutRisk)
	}
	if result.FocusLevel != 0 {
		s.FocusLevel = clampTo01(result.FocusLevel)
	}
	if result.TaskComplexity != "" {
		s.TaskComplexity = result.TaskComplexity
	}
	if result.SocialActivity != 0 {
		s.SocialActivity = clampTo01(result.SocialActivity)
	}

	s.LastUpdated = time.Now()
}

// Snapshot returns an independent copy of the state.
func (s *UserCareState) Snapshot() UserCareState {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	return UserCareState{
		LastMealAt:     s.LastMealAt,
		LastDrinkAt:    s.LastDrinkAt,
		ContinuousWork: s.ContinuousWork,
		PostureWarning: s.PostureWarning,
		StressLevel:    s.StressLevel,
		MoodTrend:      s.MoodTrend,
		BurnoutRisk:    s.BurnoutRisk,
		SocialActivity: s.SocialActivity,
		IsolationHours: s.IsolationHours,
		FocusLevel:     s.FocusLevel,
		TaskComplexity: s.TaskComplexity,
		DeadlinesNear:  s.DeadlinesNear,
		AnnoyanceLevel: s.AnnoyanceLevel,
		LastUpdated:    s.LastUpdated,
	}
}

// UpdateContinuousWork adds seconds to the continuous work counter.
func (s *UserCareState) UpdateContinuousWork(seconds int) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	s.ContinuousWork += seconds
	if s.ContinuousWork > 180 && s.FocusLevel > 0.65 {
		s.BurnoutRisk = clampTo01(float64(s.ContinuousWork) / 480.0 * s.FocusLevel)
	}
}

// ResetContinuousWork zeroes the continuous work counter and burnout risk.
func (s *UserCareState) ResetContinuousWork() {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	s.ContinuousWork = 0
	s.BurnoutRisk = 0
}

// ---- internal helpers ----

var (
	screenWorkRe  = regexp.MustCompile(`活跃\s*(\d+)\s*分钟`)
	mealKeywords  = []string{"吃了", "吃过了", "吃饭", "吃了饭", "吃过饭", "刚吃", "午饭", "晚饭", "早餐", "午餐", "晚餐", "外卖", "点餐"}
	drinkKeywords = []string{"喝了", "喝水", "喝杯水", "去倒水", "接水"}
)

func (s *UserCareState) updateFromScreen(obs Observation) {
	content := obs.Content

	if matches := screenWorkRe.FindStringSubmatch(content); len(matches) > 1 {
		if mins, err := strconv.Atoi(matches[1]); err == nil {
			s.ContinuousWork = mins
		}
	}

	lower := strings.ToLower(content)
	switch {
	case strings.Contains(lower, "ide") ||
		strings.Contains(lower, "vscode") ||
		strings.Contains(lower, "编辑器") ||
		strings.Contains(lower, "代码") ||
		strings.Contains(lower, "terminal") ||
		strings.Contains(lower, "终端"):
		s.FocusLevel = clampTo01(0.3*0.7 + 0.7*s.FocusLevel)
	case strings.Contains(lower, "浏览器") ||
		strings.Contains(lower, "文档") ||
		strings.Contains(lower, "document") ||
		strings.Contains(lower, "stackoverflow") ||
		strings.Contains(lower, "docs"):
		s.FocusLevel = clampTo01(0.3*0.5 + 0.7*s.FocusLevel)
	case strings.Contains(lower, "社交") ||
		strings.Contains(lower, "视频") ||
		strings.Contains(lower, "游戏") ||
		strings.Contains(lower, "youtube") ||
		strings.Contains(lower, "bilibili"):
		s.FocusLevel = clampTo01(0.3*0.2 + 0.7*s.FocusLevel)
	}

	if strings.Contains(lower, "坐姿") || strings.Contains(lower, "距离屏幕") {
		s.PostureWarning = true
	}

	if s.ContinuousWork > 180 && s.FocusLevel > 0.65 {
		s.BurnoutRisk = clampTo01(float64(s.ContinuousWork) / 480.0 * s.FocusLevel)
	}
}

func (s *UserCareState) updateFromQQ(obs Observation) {
	s.SocialActivity = clampTo01(0.3*1.0 + 0.7*s.SocialActivity)
	s.IsolationHours = 0
}

func (s *UserCareState) updateFromChat(obs Observation) {
	content := obs.Content

	for _, kw := range mealKeywords {
		if strings.Contains(content, kw) {
			s.LastMealAt = obs.Timestamp
			break
		}
	}

	for _, kw := range drinkKeywords {
		if strings.Contains(content, kw) {
			s.LastDrinkAt = obs.Timestamp
			break
		}
	}
}

func clampTo01(v float64) float64 {
	if v > 1 {
		return 1
	}
	if v < 0 {
		return 0
	}
	return v
}

// MarshalCareState serialises the mutable fields of UserCareState to JSON.
func MarshalCareState(s *UserCareState) ([]byte, error) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	return json.Marshal(map[string]interface{}{
		"last_meal_at":    s.LastMealAt.Unix(),
		"last_drink_at":   s.LastDrinkAt.Unix(),
		"continuous_work": s.ContinuousWork,
		"posture_warning": s.PostureWarning,
		"stress_level":    s.StressLevel,
		"mood_trend":      s.MoodTrend,
		"burnout_risk":    s.BurnoutRisk,
		"social_activity": s.SocialActivity,
		"isolation_hours": s.IsolationHours,
		"focus_level":     s.FocusLevel,
		"task_complexity": s.TaskComplexity,
		"deadlines_near":  s.DeadlinesNear,
		"annoyance_level": s.AnnoyanceLevel,
	})
}

// UnmarshalCareState deserialises JSON into the mutable fields of UserCareState.
func UnmarshalCareState(s *UserCareState, data []byte) error {
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	s.Mu.Lock()
	defer s.Mu.Unlock()
	if v, ok := m["last_meal_at"].(float64); ok {
		s.LastMealAt = time.Unix(int64(v), 0)
	}
	if v, ok := m["last_drink_at"].(float64); ok {
		s.LastDrinkAt = time.Unix(int64(v), 0)
	}
	if v, ok := m["continuous_work"].(float64); ok {
		s.ContinuousWork = int(v)
	}
	if v, ok := m["posture_warning"].(bool); ok {
		s.PostureWarning = v
	}
	if v, ok := m["stress_level"].(float64); ok {
		s.StressLevel = v
	}
	if v, ok := m["mood_trend"].(string); ok {
		s.MoodTrend = v
	}
	if v, ok := m["burnout_risk"].(float64); ok {
		s.BurnoutRisk = v
	}
	if v, ok := m["social_activity"].(float64); ok {
		s.SocialActivity = v
	}
	if v, ok := m["isolation_hours"].(float64); ok {
		s.IsolationHours = v
	}
	if v, ok := m["focus_level"].(float64); ok {
		s.FocusLevel = v
	}
	if v, ok := m["task_complexity"].(string); ok {
		s.TaskComplexity = v
	}
	if v, ok := m["deadlines_near"].(bool); ok {
		s.DeadlinesNear = v
	}
	if v, ok := m["annoyance_level"].(float64); ok {
		s.AnnoyanceLevel = v
	}
	return nil
}
