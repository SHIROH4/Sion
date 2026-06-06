package care

import (
	"desktop-pet/internal/domain"
	"sync"
	"time"

	emotion "desktop-pet/internal/service/emotion"
)

// CareTrigger defines a care trigger condition with cooldown and daily cap.
type CareTrigger struct {
	Type      domain.CareTriggerType
	Priority  int // 1 (urgent) ~ 5 (optional)
	Condition func(state *domain.UserCareState, emotion *emotion.EmotionState, now time.Time) bool
	Cooldown  time.Duration
	MaxDaily  int

	mu           sync.Mutex
	lastFiredAt  time.Time
	dailyCount   int
	lastResetDay int64
}

// Evaluate checks whether the trigger condition is met, including cooldown and
// daily cap. When all three pass, lastFiredAt and dailyCount are updated.
func (t *CareTrigger) Evaluate(state *domain.UserCareState, emotion *emotion.EmotionState, now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	// 1. Condition check.
	if t.Condition == nil || !t.Condition(state, emotion, now) {
		return false
	}

	// 2. Cooldown check.
	if now.Sub(t.lastFiredAt) < t.Cooldown {
		return false
	}

	// 3. Daily cap check.
	if t.dailyCount >= t.MaxDaily {
		return false
	}

	// All three passed — record the fire.
	t.lastFiredAt = now
	t.dailyCount++
	return true
}

// ResetDaily resets the daily counter when crossing into a new day.
func (t *CareTrigger) ResetDaily(now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()

	today := now.Truncate(24 * time.Hour).Unix()
	if today != t.lastResetDay {
		t.dailyCount = 0
		t.lastResetDay = today
	}
}

// CheckCondition evaluates the trigger condition without side effects.
// Returns (triggered, cooldownRemaining, dailyRemaining).
func (t *CareTrigger) CheckCondition(state *domain.UserCareState, emotion *emotion.EmotionState, now time.Time) (bool, time.Duration, int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.Condition == nil || !t.Condition(state, emotion, now) {
		return false, 0, 0
	}

	cooldownRemaining := t.Cooldown - now.Sub(t.lastFiredAt)
	if cooldownRemaining < 0 {
		cooldownRemaining = 0
	}

	dailyRemaining := t.MaxDaily - t.dailyCount
	if dailyRemaining < 0 {
		dailyRemaining = 0
	}

	triggered := cooldownRemaining == 0 && dailyRemaining > 0
	return triggered, cooldownRemaining, dailyRemaining
}

// CooldownRemaining returns the remaining cooldown duration, floored at zero.
func (t *CareTrigger) CooldownRemaining(now time.Time) time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()

	remaining := t.Cooldown - now.Sub(t.lastFiredAt)
	if remaining < 0 {
		return 0
	}
	return remaining
}

// ---- internal helpers for DefaultCareTriggers ----

// isMealTime returns true during typical meal hours.
func isMealTime(hour int) bool {
	return (hour >= 11 && hour <= 13) || (hour >= 17 && hour <= 20)
}

// isNightTime returns true during quiet hours (22:00-08:00).
// Consistent with CareEngine.Evaluate/Suggestions nightMode.
func isNightTime(hour int) bool {
	return hour >= 22 || hour < 8
}

// DefaultCareTriggers returns all six default care triggers with their
// conditions, cooldowns, and daily limits.
func DefaultCareTriggers() []*CareTrigger {
	return []*CareTrigger{
		{
			Type:     domain.TriggerHydration,
			Priority: 3,
			Condition: func(s *domain.UserCareState, _ *emotion.EmotionState, now time.Time) bool {
				sn := s.Snapshot()
				return now.Sub(sn.LastDrinkAt) > 90*time.Minute && sn.ContinuousWork > 60
			},
			Cooldown: 2 * time.Hour,
			MaxDaily: 5,
		},
		{
			Type:     domain.TriggerMeal,
			Priority: 2,
			Condition: func(s *domain.UserCareState, _ *emotion.EmotionState, now time.Time) bool {
				sn := s.Snapshot()
				return isMealTime(now.Hour()) && now.Sub(sn.LastMealAt) > 5*time.Hour
			},
			Cooldown: 4 * time.Hour,
			MaxDaily: 3,
		},
		{
			Type:     domain.TriggerRest,
			Priority: 1, // highest priority
			Condition: func(s *domain.UserCareState, _ *emotion.EmotionState, now time.Time) bool {
				sn := s.Snapshot()
				return isNightTime(now.Hour()) && sn.ContinuousWork > 120
			},
			Cooldown: 3 * time.Hour,
			MaxDaily: 2,
		},
		{
			Type:     domain.TriggerEncourage,
			Priority: 4,
			Condition: func(s *domain.UserCareState, e *emotion.EmotionState, _ time.Time) bool {
				if e == nil {
					return false
				}
				sn := s.Snapshot()
				// P1-2: trigger on low valence+stress, OR high stress alone.
				return (e.Valence < -0.3 && sn.StressLevel > 0.5) ||
					(sn.StressLevel > 0.6)
			},
			Cooldown: 4 * time.Hour,
			MaxDaily: 3,
		},
		{
			Type:     domain.TriggerSocial,
			Priority: 4,
			Condition: func(s *domain.UserCareState, _ *emotion.EmotionState, _ time.Time) bool {
				sn := s.Snapshot()
				return sn.SocialActivity < 0.2 && sn.IsolationHours > 8
			},
			Cooldown: 6 * time.Hour,
			MaxDaily: 2,
		},
		{
			Type:     domain.TriggerHealth,
			Priority: 3,
			Condition: func(s *domain.UserCareState, _ *emotion.EmotionState, _ time.Time) bool {
				sn := s.Snapshot()
				return sn.PostureWarning || sn.ContinuousWork > 180
			},
			Cooldown: 90 * time.Minute,
			MaxDaily: 4,
		},
	}
}
