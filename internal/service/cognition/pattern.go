package cognition

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"desktop-pet/internal/domain"; "desktop-pet/internal/infra"
)

// Analyzer performs event segmentation and behavior pattern discovery.
// Activity observations are merged into continuous sessions: a new row is only
// written when app_name or window_title changes. Consecutive identical
// observations simply extend the current session's end_time.
type Analyzer struct {
	rawLLM      func([]domain.Message) (string, error)
	eventRepo   domain.ActivityEventRepository
	patternRepo domain.PatternRepository

	currentSession     *domain.ActivityEvent // active session being tracked
	currentSessionID   int64                 // DB id once persisted, 0 if not yet
	lastFlushAt        time.Time
	lastAnalyzeAt      time.Time
	minAnalyzeInterval time.Duration
}

// NewAnalyzer creates a pattern analyzer.
func NewAnalyzer(
	rawLLM func([]domain.Message) (string, error),
	eventRepo domain.ActivityEventRepository,
	patternRepo domain.PatternRepository,
) *Analyzer {
	return &Analyzer{
		rawLLM:             rawLLM,
		eventRepo:          eventRepo,
		patternRepo:        patternRepo,
		minAnalyzeInterval: 1 * time.Hour,
	}
}

// RecordActivity observes what the user is doing. If the app or window title
// changed, the previous session is persisted and a new one begins. Otherwise
// the current session's end_time is extended.
func (a *Analyzer) RecordActivity(appName, windowTitle string, isWorking bool) {
	now := time.Now().Unix()

	if a.currentSession != nil &&
		a.currentSession.AppName == appName &&
		a.currentSession.WindowTitle == windowTitle {
		// Same session — just extend.
		a.currentSession.EndTime = now
	} else {
		// App or title changed — persist the previous session first.
		a.flushCurrentSession()
		a.currentSession = &domain.ActivityEvent{
			AppName:     appName,
			WindowTitle: windowTitle,
			IsWorking:   isWorking,
			StartTime:   now,
			EndTime:     now,
		}
		a.currentSessionID = 0
	}

	// Periodic checkpoint: write current session's state to DB every 5 minutes
	// so data survives a crash.
	if time.Since(a.lastFlushAt) > 5*time.Minute {
		a.Flush()
	}
}

// Flush writes the current session to the repository.
func (a *Analyzer) Flush() {
	a.flushCurrentSession()
	a.lastFlushAt = time.Now()
}

func (a *Analyzer) flushCurrentSession() {
	if a.currentSession == nil {
		return
	}
	if a.currentSessionID > 0 {
		if err := a.eventRepo.UpdateSessionEnd(a.currentSessionID, a.currentSession.EndTime); err != nil {
			slog.Warn("pattern: failed to update session end", "err", err)
		}
	} else {
		id, err := a.eventRepo.RecordSession(*a.currentSession)
		if err != nil {
			slog.Warn("pattern: failed to record session", "err", err)
		} else {
			a.currentSessionID = id
		}
	}
}

// ShouldAnalyze returns true if it's time to run pattern discovery.
func (a *Analyzer) ShouldAnalyze() bool {
	return time.Since(a.lastAnalyzeAt) > a.minAnalyzeInterval
}

// Analyze runs the full analysis pipeline: event segmentation → pattern discovery.
func (a *Analyzer) Analyze() ([]domain.BehaviorPattern, error) {
	if a.rawLLM == nil {
		return nil, fmt.Errorf("pattern analyzer: no LLM available")
	}

	a.Flush()

	// Step 1: Segment today's events (in-memory).
	segments := a.segmentEvents()

	// Step 2: Discover patterns from segments.
	patterns, err := a.discoverPatterns(segments)
	if err != nil {
		return nil, err
	}

	// Step 3: Persist new patterns.
	for i := range patterns {
		p := &patterns[i]
		p.Active = true
		if _, err := a.patternRepo.SavePattern(*p); err != nil {
		slog.Warn("pattern: failed to save pattern", "err", err)
	}
	}

	a.lastAnalyzeAt = time.Now()
	return patterns, nil
}

// GetPatternTriggers checks if any active patterns suggest a preemptive action
// at the current time. Returns matching triggers sorted by priority.
func (a *Analyzer) GetPatternTriggers(now time.Time) []domain.PatternTrigger {
	active, _ := a.patternRepo.ListActive()
	if len(active) == 0 {
		return nil
	}

	var triggers []domain.PatternTrigger
	for i := range active {
		p := &active[i]
		trigger := a.patternToTrigger(p, now)
		if trigger != nil {
			triggers = append(triggers, *trigger)
		}
	}
	return triggers
}

// ---- Event Segmentation ----

func (a *Analyzer) segmentEvents() []domain.EventSegment {
	events, _ := a.eventRepo.ListToday()
	return a.segmentEventsFrom(events)
}

func (a *Analyzer) segmentEventsFrom(events []domain.ActivityEvent) []domain.EventSegment {
	if len(events) < 5 {
		return nil
	}

	// Heuristic: split at natural boundaries (app switch between work↔entertainment).
	var boundaries []int
	prevApp := events[0].AppName
	for i := 1; i < len(events); i++ {
		if domain.IsBoundaryCandidate(prevApp, events[i].AppName) {
			boundaries = append(boundaries, i)
		}
		// Also split at 15+ minute gaps between sessions (idle period).
		if events[i].StartTime-events[i-1].EndTime > 900 {
			boundaries = append(boundaries, i)
		}
		prevApp = events[i].AppName
	}

	if len(boundaries) == 0 {
		// Single segment for the whole day.
		return []domain.EventSegment{a.buildSegment(events, 0, len(events))}
	}

	var segments []domain.EventSegment
	start := 0
	for _, b := range boundaries {
		if b > start {
			segments = append(segments, a.buildSegment(events, start, b))
		}
		start = b
	}
	if start < len(events) {
		segments = append(segments, a.buildSegment(events, start, len(events)))
	}
	return segments
}

func (a *Analyzer) buildSegment(events []domain.ActivityEvent, start, end int) domain.EventSegment {
	if start >= end {
		start = 0
		end = len(events)
	}

	apps := make(map[string]int)
	workDuration := 0
	totalDuration := 0
	for i := start; i < end; i++ {
		d := int(events[i].EndTime - events[i].StartTime)
		if d <= 0 {
			d = 60 // minimum 1 minute per session
		}
		apps[events[i].AppName] += d
		totalDuration += d
		if events[i].IsWorking {
			workDuration += d
		}
	}

	// Most time-dominant app.
	topApp := ""
	topDur := 0
	for app, dur := range apps {
		if dur > topDur {
			topApp = app
			topDur = dur
		}
	}

	// Build app sequence.
	var appSeq []string
	seen := map[string]bool{}
	for i := start; i < end; i++ {
		if !seen[events[i].AppName] {
			appSeq = append(appSeq, events[i].AppName)
			seen[events[i].AppName] = true
		}
	}

	focused := totalDuration > 0 && float64(workDuration)/float64(totalDuration) > 0.7
	durationMin := 0
	if end > start {
		durationMin = int(events[end-1].EndTime-events[start].StartTime) / 60
	}

	t := time.Unix(events[start].StartTime, 0)
	label := buildSegmentLabel(topApp, focused, t.Hour())

	return domain.EventSegment{
		Label:       label,
		Summary:     fmt.Sprintf("%s: 主要使用%s, 共%d分钟", label, topApp, durationMin),
		AppSequence: strings.Join(appSeq, ","),
		DurationMin: durationMin,
		IsFocused:   focused,
		DayOfWeek:   int(t.Weekday()),
		HourStart:   t.Hour(),
		CreatedAt:   time.Now().Unix(),
	}
}

func buildSegmentLabel(topApp string, focused bool, hour int) string {
	slot := domain.TimeToSlot(hour)
	if !focused {
		return slot + "休闲"
	}
	switch {
	case strings.Contains(topApp, "Code") || strings.Contains(topApp, "Terminal") ||
		strings.Contains(topApp, "Xcode") || strings.Contains(topApp, "IntelliJ"):
		return slot + "开发"
	case strings.Contains(topApp, "Slack") || strings.Contains(topApp, "Zoom") ||
		strings.Contains(topApp, "Teams") || strings.Contains(topApp, "腾讯会议"):
		return slot + "会议"
	case strings.Contains(topApp, "Chrome") || strings.Contains(topApp, "Safari") ||
		strings.Contains(topApp, "Edge"):
		return slot + "浏览"
	default:
		return slot + "其他"
	}
}

// ---- Pattern Discovery ----

func (a *Analyzer) discoverPatterns(todaySegments []domain.EventSegment) ([]domain.BehaviorPattern, error) {
	// Compute segments from past 7 days of raw events (in-memory).
	all := todaySegments
	since := time.Now().AddDate(0, 0, -7).Unix()
	until := time.Now().Unix()
	if pastEvents, _ := a.eventRepo.ListRange(since, until); len(pastEvents) > 10 {
		all = append(a.segmentEventsFrom(pastEvents), todaySegments...)
	}
	if len(all) < 3 {
		return nil, fmt.Errorf("pattern discovery: need >=5 segments, got %d", len(all))
	}

	var sb strings.Builder
	sb.WriteString("过去7天的活动片段:\n")
	for _, s := range all {
		sb.WriteString(fmt.Sprintf("- [周%d %s] %s (专注:%v, %d分钟)\n",
			s.DayOfWeek, s.Label, s.Summary, s.IsFocused, s.DurationMin))
	}

	existing, _ := a.patternRepo.ListActive()
	var existingText strings.Builder
	for _, p := range existing {
		existingText.WriteString(fmt.Sprintf("- [%.0f%%] %s → %s\n", p.Confidence*100, p.Pattern, p.Implication))
	}
	existStr := existingText.String()
	if existStr == "" {
		existStr = "(暂无已有模式)"
	}

	prompt := fmt.Sprintf(patternDiscoveryPrompt, sb.String(), existStr)
	result, err := a.rawLLM([]domain.Message{{Role: "user", Content: prompt}})
	if err != nil {
		return nil, err
	}

	var discoveries []struct {
		Pattern     string  `json:"pattern"`
		Type        string  `json:"type"`
		Evidence    string  `json:"evidence"`
		Confidence  float64 `json:"confidence"`
		Implication string  `json:"implication"`
	}
	raw := infra.CleanJSON(result)
	if err := json.Unmarshal([]byte(raw), &discoveries); err != nil {
		return nil, fmt.Errorf("pattern discovery: JSON parse failed: %w (raw: %.200s)", err, raw)
	}

	var patterns []domain.BehaviorPattern
	now := time.Now().Unix()
	for _, d := range discoveries {
		if d.Pattern == "" || d.Confidence < 0.3 {
			continue
		}
		patterns = append(patterns, domain.BehaviorPattern{
			Pattern:     d.Pattern,
			Type:        d.Type,
			Evidence:    d.Evidence,
			Confidence:  d.Confidence,
			Implication: d.Implication,
			Active:      true,
			CreatedAt:   now,
			UpdatedAt:   now,
		})
	}
	return patterns, nil
}

func (a *Analyzer) patternToTrigger(p *domain.BehaviorPattern, now time.Time) *domain.PatternTrigger {
	if !p.Active {
		return nil
	}

	hour := now.Hour()
	dayOfWeek := int(now.Weekday())

	// Match known pattern types to current context.
	switch p.Type {
	case "daily_rhythm":
		// Check if pattern mentions current time slot.
		slot := domain.TimeToSlot(hour)
		if strings.Contains(p.Pattern, slot) {
			return &domain.PatternTrigger{
				Pattern:  p,
				Action:   string(domain.SourceCasual),
				Priority: p.Confidence * 0.7,
			}
		}
	case "work_habit":
		if domain.TimeToSlot(hour) == "上午" || domain.TimeToSlot(hour) == "下午" {
			return &domain.PatternTrigger{
				Pattern:  p,
				Action:   string(domain.SourceCare),
				Priority: p.Confidence * 0.6,
			}
		}
	case "social_pattern":
		// Only trigger on matching days.
		if strings.Contains(p.Pattern, fmt.Sprintf("周%d", dayOfWeek)) {
			return &domain.PatternTrigger{
				Pattern:  p,
				Action:   string(domain.SourceCasual),
				Priority: p.Confidence * 0.5,
			}
		}
	}

	return nil
}

const patternDiscoveryPrompt = `## 行为模式发现

你是诗音的模式分析模块。基于过去几天的用户活动片段，发现重复出现的行为规律。

### 活动片段
%s

### 已知模式（避免重复发现）
%s

### 分析任务

从以下四个维度寻找模式：

1. **daily_rhythm（日节律）**: 每天固定时间做什么。如"工作日10:30出现注意力分散（频繁切换应用）"
2. **work_habit（工作习惯）**: 工作流中的规律。如"git commit后80%%会休息5分钟"
3. **interest_shift（兴趣变化）**: 长期使用模式的变化。如"从Go开发逐渐转向Rust"
4. **social_pattern（社交模式）**: 社交/娱乐的规律。如"周三和周五下午QQ活动多"

### 输出格式
JSON 数组，每条模式包含:
- pattern: 模式描述（一句话）
- type: daily_rhythm / work_habit / interest_shift / social_pattern
- evidence: 支撑这个模式的证据（引用具体数据）
- confidence: 0~1，基于证据强度
- implication: 对系统行为的启示（应该怎么做）

示例:
[
  {
    "pattern": "工作日15:00左右出现注意力分散",
    "type": "daily_rhythm",
    "evidence": "周一15:05切到Bilibili, 周二15:12切到微博, 周三14:55切到YouTube",
    "confidence": 0.75,
    "implication": "在14:55左右主动送上一句轻松的问候，帮助主人度过注意力低谷"
  }
]

要求：
- 只输出置信度≥0.3的模式
- 每个模式必须有至少2天/次的具体证据
- 不要重复已有模式
- 只输出 JSON 数组。`

// simpleTextSim returns a jaccard-like word overlap ratio for basic dedup.
func simpleTextSim(a, b string) float64 {
	wordsA := strings.Fields(a)
	wordsB := strings.Fields(b)
	if len(wordsA) == 0 || len(wordsB) == 0 {
		return 0
	}
	overlap := 0
	for _, wa := range wordsA {
		for _, wb := range wordsB {
			if wa == wb {
				overlap++
				break
			}
		}
	}
	return float64(overlap) / float64(len(wordsA))
}

