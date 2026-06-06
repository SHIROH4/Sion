package care

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"desktop-pet/internal/domain"
	emotion "desktop-pet/internal/service/emotion"
)

// DetermineCareMood picks the emotional tone based on the AI's current emotion
// vector and the user's state.
func DetermineCareMood(vector emotion.EmotionVector, state *domain.UserCareState) domain.CareMood {
	if vector.Annoyance > 0.4 && vector.Affection > 0.5 {
		return domain.MoodTsundere // 又气又关心 → 傲娇
	}
	if state.StressLevel > 0.7 && vector.Worry > 0.5 {
		return domain.MoodWorried // 主人压力大 + 担心 → 担忧模式
	}
	if vector.Playfulness > 0.6 {
		return domain.MoodPlayful // 心情好 → 调皮
	}
	if state.ContinuousWork > 240 && vector.Worry > 0.4 {
		return domain.MoodFirm // 工作太久 + 担心 → 强势
	}
	if vector.Affection > 0.7 && vector.Worry > 0.3 {
		return domain.MoodGentle // 亲密度高 + 担忧 → 温柔
	}
	return domain.MoodNeutral
}

// CareEngine manages the full lifecycle of proactive care: state tracking,
// trigger evaluation, action execution, and feedback adaptation.
type CareEngine struct {
	mu       sync.Mutex
	state    *domain.UserCareState
	triggers []*CareTrigger

	// Action log — last 50 entries.
	actionLog []domain.CareAction
	nextID    int64

	// Adaptive parameters.
	lastPokeAt      time.Time
	consecutiveSkip int // consecutive skipped care opportunities

	// Callbacks injected by MemoryPlugin.
	onCare           func(domain.CareAction) error
	getEmotion       func() emotion.EmotionState
	getEmotionVector func() emotion.EmotionVector
	generateMessage  func(domain.CareTriggerType, *domain.UserCareState, *emotion.EmotionState, *emotion.EmotionVector, string) string
}

// NewCareEngine creates a care engine with default triggers and neutral state.
func NewCareEngine(
	state *domain.UserCareState,
	onCare func(domain.CareAction) error,
	getEmotion func() emotion.EmotionState,
) *CareEngine {
	return &CareEngine{
		state:      state,
		triggers:   DefaultCareTriggers(),
		actionLog:  make([]domain.CareAction, 0, 50),
		onCare:     onCare,
		getEmotion: getEmotion,
		lastPokeAt: time.Now(),
	}
}

// Evaluate runs all triggers through safety filters and returns a priority-sorted
// list of care actions that should fire.
func (e *CareEngine) Evaluate(now time.Time) []domain.CareAction {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Reset daily counters when crossing midnight.
	for _, t := range e.triggers {
		t.ResetDaily(now)
	}

	sn := e.state.Snapshot()

	// Safety boundary: night mode (22:00-08:00) — only rest and health.
	nightMode := now.Hour() >= 22 || now.Hour() < 8

	// Safety boundary: focus mode — only priority 1 (life-critical).
	focusMode := sn.FocusLevel > 0.8

	var actions []domain.CareAction
	emo := e.getEmotion()

	// Emotion-vector-driven adjustments.
	vector := emotion.EmotionVector{}
	if e.getEmotionVector != nil {
		vector = e.getEmotionVector()
	}

	// Annoyance-based skip: higher annoyance → more triggers skipped.
	skipChance := e.state.AnnoyanceLevel * 0.7 // max 70% skip rate

	for _, t := range e.triggers {
		// Night mode: only rest and health.
		if nightMode && t.Type != domain.TriggerRest && t.Type != domain.TriggerHealth {
			continue
		}
		// Focus mode: only priority 1.
		if focusMode && t.Priority > 1 {
			continue
		}

		// Emotion-vector-driven suppress: annoyed → skip non-urgent.
		if vector.Annoyance > 0.5 && t.Priority > 1 {
			continue
		}

		// Annoyance skip: priority 1 never skipped.
		if t.Priority > 1 && skipChance > 0 {
			if int64(skipChance*100) > int64(now.UnixNano()%100) {
				continue
			}
		}

		// Emotion-vector boost: lonely → lower social trigger threshold.
		triggered := t.Evaluate(&sn, &emo, now)
		if !triggered && t.Type == domain.TriggerSocial && vector.Loneliness > 0.6 {
			triggered = true
		}

		if triggered {
			e.nextID++
			actions = append(actions, domain.CareAction{
				ID:          e.nextID,
				Type:        t.Type,
				Priority:    t.Priority,
				TriggeredAt: now,
			})
		}
	}

	// Sort by priority ascending (1 = highest).
	sort.Slice(actions, func(i, j int) bool { return actions[i].Priority < actions[j].Priority })

	return actions
}

// UpdateState delegates to domain.UserCareState.UpdateFromObservation.
func (e *CareEngine) UpdateState(obs domain.Observation) {
	if e.state != nil {
		e.state.UpdateFromObservation(obs)
	}
}

// SaveState serialises care state to JSON for persistence across restarts.
func (e *CareEngine) SaveState() ([]byte, error) {
	if e.state == nil {
		return nil, nil
	}
	return domain.MarshalCareState(e.state)
}

// LoadState restores care state from persisted JSON.
func (e *CareEngine) LoadState(data []byte) error {
	if e.state == nil || len(data) == 0 {
		return nil
	}
	return domain.UnmarshalCareState(e.state, data)
}

// IncrementWork increments the user's continuous work counter by seconds.
func (e *CareEngine) IncrementWork(seconds int) {
	if e.state != nil {
		e.state.UpdateContinuousWork(seconds)
	}
}

// ResetWork zeroes the user's continuous work counter.
func (e *CareEngine) ResetWork() {
	if e.state != nil {
		e.state.ResetContinuousWork()
	}
}

// UpdateStress sets the user's stress level, derived from emotion model.
func (e *CareEngine) UpdateStress(stress float64) {
	if e.state != nil {
		e.state.UpdateStress(stress)
	}
}

// TickIsolation increments the user's isolation counter. Call from background loop
// on each tick (~5 min). When IsolationHours > 8, domain.TriggerSocial may fire.
func (e *CareEngine) TickIsolation(interval time.Duration) {
	if e.state != nil {
		e.state.IncrementIsolation(interval.Hours())
	}
}

// UpdateStateFromLLM delegates to domain.UserCareState.UpdateFromLLM.
func (e *CareEngine) UpdateStateFromLLM(result *domain.LLMCareStateResult) {
	if e.state != nil {
		e.state.UpdateFromLLM(result)
	}
}

// RecordResponse records the user's feedback to a care action and adapts the
// annoyance level accordingly.
func (e *CareEngine) RecordResponse(actionID int64, accepted bool, response string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	for i := range e.actionLog {
		if e.actionLog[i].ID == actionID {
			e.actionLog[i].Accepted = &accepted
			e.actionLog[i].Response = response

			if accepted {
				e.state.AnnoyanceLevel = max(0, e.state.AnnoyanceLevel-0.1)
			} else {
				e.state.AnnoyanceLevel = min(1.0, e.state.AnnoyanceLevel+0.15)
			}
			break
		}
	}

	// Trim to last 50 entries.
	if len(e.actionLog) > 50 {
		e.actionLog = e.actionLog[len(e.actionLog)-50:]
	}
}

// Suggestions evaluates all care triggers non-destructively and returns
// triggered suggestions for the System 2 decision pipeline.
func (e *CareEngine) Suggestions(now time.Time) []domain.CareSuggestion {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Reset daily counters if crossing midnight.
	for _, t := range e.triggers {
		t.ResetDaily(now)
	}

	// Safety gates (same as ShouldPoke).
	if e.state.AnnoyanceLevel > 0.7 {
		return nil
	}
	sn := e.state.Snapshot()
	if sn.FocusLevel > 0.85 {
		return nil
	}

	emo := e.getEmotion()
	var vec emotion.EmotionVector
	if e.getEmotionVector != nil {
		vec = e.getEmotionVector()
	}

	nightMode := now.Hour() >= 22 || now.Hour() < 8
	focusMode := sn.FocusLevel > 0.8

	var suggestions []domain.CareSuggestion
	for _, t := range e.triggers {
		// Night mode: only rest and health.
		if nightMode && t.Type != domain.TriggerRest && t.Type != domain.TriggerHealth {
			continue
		}
		// Focus mode: only priority 1.
		if focusMode && t.Priority > 1 {
			continue
		}
		// Annoyance skip for non-urgent.
		if vec.Annoyance > 0.5 && t.Priority > 1 {
			continue
		}

		triggered, _, dailyLeft := t.CheckCondition(&sn, &emo, now)
		// Loneliness boost for social trigger.
		if !triggered && t.Type == domain.TriggerSocial && vec.Loneliness > 0.6 {
			triggered = true
		}

		if triggered && dailyLeft > 0 {
			suggestions = append(suggestions, domain.CareSuggestion{
				Type:     t.Type,
				Priority: t.Priority,
				Reason:   string(t.Type) + " trigger conditions met",
			})
		}
	}
	return suggestions
}

// ShouldPoke decides whether it's a good time to initiate care, based on
// cooldown, annoyance, and focus level.
func (e *CareEngine) ShouldPoke(now time.Time) bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Minimum 2 minutes between pokes.
	if now.Sub(e.lastPokeAt) < 2*time.Minute {
		return false
	}

	// Too annoyed → stay quiet.
	if e.state.AnnoyanceLevel > 0.7 {
		return false
	}

	// Deep focus → don't interrupt.
	sn := e.state.Snapshot()
	if sn.FocusLevel > 0.85 {
		return false
	}

	return true
}

// SetGenerateMessage injects an LLM-based message generation function. When nil,
// Poke falls back to built-in default messages.
func (e *CareEngine) SetGenerateMessage(fn func(domain.CareTriggerType, *domain.UserCareState, *emotion.EmotionState, *emotion.EmotionVector, string) string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.generateMessage = fn
}

// SetEmotionVector injects a callback that returns the current emotion vector.
func (e *CareEngine) SetEmotionVector(fn func() emotion.EmotionVector) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.getEmotionVector = fn
}

// Poke evaluates triggers and, if any are ready, generates a message and executes
// the onCare callback. Returns the action that was fired, or nil if nothing to do.
func (e *CareEngine) Poke(now time.Time) (*domain.CareAction, error) {
	if !e.ShouldPoke(now) {
		return nil, nil
	}

	actions := e.Evaluate(now)
	if len(actions) == 0 {
		return nil, nil
	}

	// Take the highest-priority action.
	action := actions[0]

	// Generate message.
	e.mu.Lock()
	genMsg := e.generateMessage
	e.mu.Unlock()

	sn := e.state.Snapshot()
	emo := e.getEmotion()
	var emotionVec emotion.EmotionVector
	if e.getEmotionVector != nil {
		emotionVec = e.getEmotionVector()
	}

	// Determine emotional tone for this care action.
	action.Mood = DetermineCareMood(emotionVec, &sn)

	if genMsg != nil {
		action.Message = genMsg(action.Type, &sn, &emo, &emotionVec, "")
	} else {
		action.Message = DefaultCareMessage(action.Type, &sn)
	}

	// Execute callback.
	e.mu.Lock()
	onCare := e.onCare
	e.mu.Unlock()

	if onCare != nil {
		if err := onCare(action); err != nil {
			return &action, err
		}
	}

	// Record the poke.
	e.mu.Lock()
	e.lastPokeAt = now
	e.actionLog = append(e.actionLog, action)
	if len(e.actionLog) > 50 {
		e.actionLog = e.actionLog[len(e.actionLog)-50:]
	}
	e.mu.Unlock()

	return &action, nil
}


// State returns the underlying domain.UserCareState pointer for external read access.
func (e *CareEngine) State() *domain.UserCareState {
	return e.state
}

// ActionLog returns the most recent n care action records.
func (e *CareEngine) ActionLog(n int) []domain.CareAction {
	e.mu.Lock()
	defer e.mu.Unlock()

	if n <= 0 || n > len(e.actionLog) {
		n = len(e.actionLog)
	}
	start := len(e.actionLog) - n
	out := make([]domain.CareAction, n)
	copy(out, e.actionLog[start:])
	return out
}

// defaultCareMessage returns a built-in fallback message when no LLM generator
// has been injected.
func DefaultCareMessage(t domain.CareTriggerType, sn *domain.UserCareState) string {
	workMin := sn.ContinuousWork
	switch t {
	case domain.TriggerHydration:
		return fmt.Sprintf("主人已经连续工作了%d分钟了喵…快去喝点水吧，脱水会变笨的！", workMin)
	case domain.TriggerMeal:
		return "到饭点了喵！主人不饿吗？要不要休息一下去吃点东西？"
	case domain.TriggerRest:
		return "主人！！都这么晚了！！明天还有bug要改呢，现在不睡明天脑子转不动的喵。关掉编辑器，去洗漱，立刻！"
	case domain.TriggerEncourage:
		return "主人，我注意到你最近压力有点大。你一直是很厉害的程序员，这次也一定能搞定的喵~"
	case domain.TriggerSocial:
		return "主人好久没和群里的小伙伴聊天了喵，要不要去看看大家在聊什么？"
	case domain.TriggerHealth:
		return fmt.Sprintf("主人已经连续工作了%d分钟了。起来活动一下！伸懒腰~扭扭腰~转转头~", workMin)
	default:
		return "主人~我在哦！"
	}
}

// AnnoyanceLevel returns the current annoyance level (0~1).
func (e *CareEngine) AnnoyanceLevel() float64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.state.AnnoyanceLevel
}
