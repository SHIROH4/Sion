package care

import (
	"desktop-pet/internal/domain"
	emotion "desktop-pet/internal/service/emotion"
	"testing"
	"time"
)

func TestCareTrigger_Hydration_Fires(t *testing.T) {
	s := domain.NewUserCareState()
	// Set state: 90+ min since last drink, 60+ min continuous work.
	s.Mu.Lock()
	s.LastDrinkAt = time.Now().Add(-2 * time.Hour)
	s.ContinuousWork = 90
	s.Mu.Unlock()

	triggers := DefaultCareTriggers()
	var ht *CareTrigger
	for _, tr := range triggers {
		if tr.Type == domain.TriggerHydration {
			ht = tr
			break
		}
	}
	if ht == nil {
		t.Fatal("hydration trigger not found in defaults")
	}

	if !ht.Evaluate(s, nil, time.Now()) {
		t.Error("expected hydration trigger to fire: 2h no drink + 90min work")
	}
}

func TestCareTrigger_Hydration_NotThirsty(t *testing.T) {
	s := domain.NewUserCareState()
	// Just drank 10 min ago — should not fire.
	s.Mu.Lock()
	s.LastDrinkAt = time.Now().Add(-10 * time.Minute)
	s.ContinuousWork = 90
	s.Mu.Unlock()

	triggers := DefaultCareTriggers()
	var ht *CareTrigger
	for _, tr := range triggers {
		if tr.Type == domain.TriggerHydration {
			ht = tr
			break
		}
	}
	if ht == nil {
		t.Fatal("hydration trigger not found in defaults")
	}

	if ht.Evaluate(s, nil, time.Now()) {
		t.Error("expected hydration trigger NOT to fire: just drank 10min ago")
	}
}

func TestCareTrigger_Rest_Priority(t *testing.T) {
	triggers := DefaultCareTriggers()
	var rt *CareTrigger
	for _, tr := range triggers {
		if tr.Type == domain.TriggerRest {
			rt = tr
			break
		}
	}
	if rt == nil {
		t.Fatal("rest trigger not found in defaults")
	}
	if rt.Priority != 1 {
		t.Errorf("expected Rest Priority=1 (highest), got %d", rt.Priority)
	}
}

func TestCareTrigger_Rest_NightTime(t *testing.T) {
	s := domain.NewUserCareState()
	s.Mu.Lock()
	s.ContinuousWork = 150 // 2.5h
	s.Mu.Unlock()

	triggers := DefaultCareTriggers()
	var rt *CareTrigger
	for _, tr := range triggers {
		if tr.Type == domain.TriggerRest {
			rt = tr
			break
		}
	}
	if rt == nil {
		t.Fatal("rest trigger not found in defaults")
	}

	// Simulate 1:00 AM.
	nightTime := time.Date(2026, 6, 1, 1, 0, 0, 0, time.Local)
	if !rt.Evaluate(s, nil, nightTime) {
		t.Error("expected rest trigger to fire at 1AM with 2.5h work")
	}
}

func TestCareTrigger_Rest_DayTime(t *testing.T) {
	s := domain.NewUserCareState()
	s.Mu.Lock()
	s.ContinuousWork = 150
	s.Mu.Unlock()

	triggers := DefaultCareTriggers()
	var rt *CareTrigger
	for _, tr := range triggers {
		if tr.Type == domain.TriggerRest {
			rt = tr
			break
		}
	}
	if rt == nil {
		t.Fatal("rest trigger not found in defaults")
	}

	// 3:00 PM — not night time, should not fire.
	dayTime := time.Date(2026, 6, 1, 15, 0, 0, 0, time.Local)
	if rt.Evaluate(s, nil, dayTime) {
		t.Error("expected rest trigger NOT to fire at 3PM")
	}
}

func TestCareTrigger_Encourage_LowValence(t *testing.T) {
	s := domain.NewUserCareState()
	s.Mu.Lock()
	s.StressLevel = 0.7
	s.Mu.Unlock()

	emo := &emotion.EmotionState{Valence: -0.5}

	triggers := DefaultCareTriggers()
	var et *CareTrigger
	for _, tr := range triggers {
		if tr.Type == domain.TriggerEncourage {
			et = tr
			break
		}
	}
	if et == nil {
		t.Fatal("encourage trigger not found in defaults")
	}

	if !et.Evaluate(s, emo, time.Now()) {
		t.Error("expected encourage trigger to fire: Valence=-0.5 + Stress=0.7")
	}
}

func TestCareTrigger_Cooldown_BlocksReTrigger(t *testing.T) {
	s := domain.NewUserCareState()
	s.Mu.Lock()
	s.LastDrinkAt = time.Now().Add(-3 * time.Hour)
	s.ContinuousWork = 90
	s.Mu.Unlock()

	ht := &CareTrigger{
		Type:     domain.TriggerHydration,
		Priority: 3,
		Condition: func(s *domain.UserCareState, _ *emotion.EmotionState, now time.Time) bool {
			sn := s.Snapshot()
			return now.Sub(sn.LastDrinkAt) > 90*time.Minute && sn.ContinuousWork > 60
		},
		Cooldown: 2 * time.Hour,
		MaxDaily: 5,
	}

	now := time.Now()
	// First fire — should succeed.
	if !ht.Evaluate(s, nil, now) {
		t.Fatal("expected first hydration to fire")
	}
	// Immediate re-evaluate — should be blocked by cooldown.
	if ht.Evaluate(s, nil, now) {
		t.Error("expected cooldown to block immediate re-trigger")
	}
	// Remaining cooldown should be ~2h.
	rem := ht.CooldownRemaining(now)
	if rem < 119*time.Minute {
		t.Errorf("expected cooldown remaining ~2h, got %v", rem)
	}
}

func TestCareTrigger_MaxDaily_Enforced(t *testing.T) {
	s := domain.NewUserCareState()
	s.Mu.Lock()
	s.LastDrinkAt = time.Now().Add(-3 * time.Hour)
	s.ContinuousWork = 90
	s.Mu.Unlock()

	ht := &CareTrigger{
		Type:     domain.TriggerHydration,
		Priority: 3,
		Condition: func(s *domain.UserCareState, _ *emotion.EmotionState, now time.Time) bool {
			sn := s.Snapshot()
			return now.Sub(sn.LastDrinkAt) > 90*time.Minute && sn.ContinuousWork > 60
		},
		Cooldown: 0, // no cooldown for this test
		MaxDaily: 3,
	}

	// Advance time by 1ns per fire to pass the zero-cooldown gate.
	now := time.Now()
	for i := 0; i < 3; i++ {
		if !ht.Evaluate(s, nil, now.Add(time.Duration(i)*time.Nanosecond)) {
			t.Fatalf("expected fire %d to succeed", i+1)
		}
	}
	// 4th fire — should be blocked by daily cap.
	if ht.Evaluate(s, nil, now.Add(4*time.Nanosecond)) {
		t.Error("expected 4th fire to be blocked by MaxDaily=3")
	}
}

func TestCareTrigger_ResetDaily(t *testing.T) {
	day1 := time.Date(2026, 6, 1, 10, 0, 0, 0, time.Local)

	s := domain.NewUserCareState()
	s.Mu.Lock()
	s.LastDrinkAt = day1.Add(-3 * time.Hour)
	s.ContinuousWork = 90
	s.Mu.Unlock()

	ht := &CareTrigger{
		Type:     domain.TriggerHydration,
		Priority: 3,
		Condition: func(s *domain.UserCareState, _ *emotion.EmotionState, now time.Time) bool {
			sn := s.Snapshot()
			return now.Sub(sn.LastDrinkAt) > 90*time.Minute && sn.ContinuousWork > 60
		},
		Cooldown: 1 * time.Nanosecond,
		MaxDaily: 2,
	}
	// Fire twice on day 1.
	ht.Evaluate(s, nil, day1)
	ht.Evaluate(s, nil, day1)
	if ht.Evaluate(s, nil, day1) {
		t.Error("expected 3rd fire on day 1 to be blocked")
	}

	// Reset to day 2 — should allow fires again.
	day2 := time.Date(2026, 6, 2, 10, 0, 0, 0, time.Local)
	ht.ResetDaily(day2)
	if !ht.Evaluate(s, nil, day2) {
		t.Error("expected fire to succeed on day 2 after reset")
	}
}

func TestCareTrigger_AllDefaults_Valid(t *testing.T) {
	triggers := DefaultCareTriggers()
	if len(triggers) != 6 {
		t.Fatalf("expected 6 default triggers, got %d", len(triggers))
	}

	for _, tr := range triggers {
		if tr.Priority < 1 || tr.Priority > 5 {
			t.Errorf("trigger %s: Priority %d out of range [1,5]", tr.Type, tr.Priority)
		}
		if tr.MaxDaily < 1 {
			t.Errorf("trigger %s: MaxDaily %d must be >= 1", tr.Type, tr.MaxDaily)
		}
		if tr.Cooldown <= 0 {
			t.Errorf("trigger %s: Cooldown %v must be positive", tr.Type, tr.Cooldown)
		}
		if tr.Condition == nil {
			t.Errorf("trigger %s: Condition must not be nil", tr.Type)
		}
		if tr.Type == "" {
			t.Error("trigger has empty Type")
		}
	}
}
