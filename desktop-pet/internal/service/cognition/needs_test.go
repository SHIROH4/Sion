package cognition

import (
	"math"
	"testing"
	"time"

	"desktop-pet/internal/domain"
)

func TestNewNeedModel_Defaults(t *testing.T) {
	m := NewNeedModel()
	s := m.Snapshot()

	// All needs should be positive and in [0,1].
	checks := map[string]float64{
		"companionship": s.Companionship,
		"rest":          s.Rest,
		"play":          s.Play,
		"curiosity":     s.Curiosity,
		"care":          s.Care,
		"autonomy":      s.Autonomy,
	}
	for name, v := range checks {
		if v <= 0 || v > 1 {
			t.Errorf("%s = %.2f out of (0,1]", name, v)
		}
	}
}

func TestGrow_IncreasesNeeds(t *testing.T) {
	m := NewNeedModel()
	before := m.Snapshot()

	// Grow by 2 hours with long idle to trigger companion/autonomy bonuses.
	now := time.Now()
	m.Grow(now.Add(2*time.Hour), false, 14, 3*time.Hour)

	after := m.Snapshot()
	if after.Companionship <= before.Companionship {
		t.Error("companionship should increase")
	}
	if after.Rest <= before.Rest {
		t.Error("rest should increase")
	}
	if after.Play <= before.Play {
		t.Error("play should increase")
	}
	if after.Care <= before.Care {
		t.Error("care should increase")
	}
	if after.Autonomy <= before.Autonomy {
		t.Error("autonomy should increase")
	}
}

func TestGrow_NightAcceleratesRest(t *testing.T) {
	m := NewNeedModel()
	night := time.Date(2026, 6, 5, 1, 0, 0, 0, time.Local) // 1am
	m.Grow(night, false, 1, 30*time.Minute)
	nightRest := m.Snapshot().Rest

	m2 := NewNeedModel()
	day := time.Date(2026, 6, 5, 14, 0, 0, 0, time.Local) // 2pm
	m2.Grow(day, false, 14, 30*time.Minute)
	dayRest := m2.Snapshot().Rest

	if nightRest <= dayRest {
		t.Errorf("rest should grow faster at night: night=%.3f day=%.3f", nightRest, dayRest)
	}
}

func TestGrow_WorkingAcceleratesCare(t *testing.T) {
	m := NewNeedModel()
	now := time.Now()
	m.Grow(now, true, 14, 30*time.Minute)   // working
	workCare := m.Snapshot().Care

	m2 := NewNeedModel()
	m2.Grow(now, false, 14, 30*time.Minute) // not working
	restCare := m2.Snapshot().Care

	if workCare <= restCare {
		t.Errorf("care should grow faster when working: work=%.3f rest=%.3f", workCare, restCare)
	}
}

func TestGrow_IdleAcceleratesCompanionship(t *testing.T) {
	m := NewNeedModel()
	now := time.Now()
	m.Grow(now, false, 14, 3*time.Hour) // long idle

	m2 := NewNeedModel()
	m2.Grow(now, false, 14, 1*time.Minute) // recent chat

	if m.Snapshot().Companionship <= m2.Snapshot().Companionship {
		t.Error("companionship should grow faster when idle")
	}
}

func TestGrow_ClampedTo1(t *testing.T) {
	m := NewNeedModel()
	// Grow for a very long time.
	now := time.Now()
	m.Grow(now.Add(100*time.Hour), false, 14, 100*time.Hour)
	s := m.Snapshot()
	if s.Companionship > 1.0 || s.Rest > 1.0 || s.Care > 1.0 {
		t.Error("needs should be clamped to 1.0")
	}
}

func TestSatisfy_ReducesNeeds(t *testing.T) {
	m := NewNeedModel()

	// Artificially set needs high.
	m.mu.Lock()
	m.needs.Companionship = 0.8
	m.needs.Play = 0.8
	m.needs.Care = 0.8
	m.needs.Curiosity = 0.8
	m.needs.Autonomy = 0.8
	m.needs.Rest = 0.8
	m.mu.Unlock()

	// Satisfy via speak_casual.
	m.Satisfy("speak_casual", domain.OutcomeReplied)
	s := m.Snapshot()

	if s.Play >= 0.8 {
		t.Error("play should decrease after speak_casual")
	}
	if s.Companionship >= 0.8 {
		t.Error("companionship should decrease after reply")
	}
}

func TestSatisfy_ReflectReducesAutonomy(t *testing.T) {
	m := NewNeedModel()
	m.mu.Lock()
	m.needs.Autonomy = 0.9
	m.mu.Unlock()

	m.Satisfy("reflect", domain.OutcomeIgnored)
	if s := m.Snapshot(); s.Autonomy >= 0.9 {
		t.Error("autonomy should decrease after reflect")
	}
}

func TestSatisfy_NoneReducesRest(t *testing.T) {
	m := NewNeedModel()
	m.mu.Lock()
	m.needs.Rest = 0.7
	m.mu.Unlock()

	m.Satisfy("none", domain.OutcomeIgnored)
	if s := m.Snapshot(); s.Rest >= 0.7 {
		t.Error("rest should decrease after none")
	}
}

func TestSatisfy_RejectionLessSatisfying(t *testing.T) {
	// Rejection should satisfy less than a positive reply.
	m1 := NewNeedModel()
	m1.mu.Lock()
	m1.needs.Companionship = 0.5
	m1.needs.Care = 0.5
	m1.mu.Unlock()
	m1.Satisfy("speak_care", domain.OutcomeRejected)
	rejected := m1.Snapshot()

	m2 := NewNeedModel()
	m2.mu.Lock()
	m2.needs.Companionship = 0.5
	m2.needs.Care = 0.5
	m2.mu.Unlock()
	m2.Satisfy("speak_care", domain.OutcomeReplied)
	replied := m2.Snapshot()

	// Rejection should leave needs higher (less satisfied) than a positive reply.
	if rejected.Companionship <= replied.Companionship {
		t.Errorf("rejected comp=%.3f should be > replied comp=%.3f", rejected.Companionship, replied.Companionship)
	}
	if rejected.Care <= replied.Care {
		t.Errorf("rejected care=%.3f should be > replied care=%.3f", rejected.Care, replied.Care)
	}
}

func TestSatisfy_EngagementDeeplySatisfies(t *testing.T) {
	m := NewNeedModel()
	m.mu.Lock()
	m.needs.Companionship = 0.8
	m.mu.Unlock()

	// Engaged outcome gives extra satisfaction.
	m.Satisfy("speak_casual", domain.OutcomeEngaged)
	engaged := m.Snapshot().Companionship

	m.mu.Lock()
	m.needs.Companionship = 0.8
	m.mu.Unlock()
	m.Satisfy("speak_casual", domain.OutcomeReplied)
	replied := m.Snapshot().Companionship

	if engaged >= replied {
		t.Errorf("engaged(%.3f) should satisfy more than replied(%.3f)", engaged, replied)
	}
}

func TestModulation_Ranges(t *testing.T) {
	m := NewNeedModel()
	mod := m.Modulation()

	// All multipliers should be in reasonable ranges.
	if mod.LonelinessDecayMul < 0.5 || mod.LonelinessDecayMul > 1.0 {
		t.Errorf("LonelinessDecayMul=%.3f out of [0.5,1.0]", mod.LonelinessDecayMul)
	}
	if mod.SleepinessGrowthMul < 1.0 || mod.SleepinessGrowthMul > 1.5 {
		t.Errorf("SleepinessGrowthMul=%.3f out of [1.0,1.5]", mod.SleepinessGrowthMul)
	}
	if mod.PlayfulnessDecayMul < 0.5 || mod.PlayfulnessDecayMul > 1.0 {
		t.Errorf("PlayfulnessDecayMul=%.3f out of [0.5,1.0]", mod.PlayfulnessDecayMul)
	}
	if mod.CuriosityDecayMul < 0.6 || mod.CuriosityDecayMul > 1.0 {
		t.Errorf("CuriosityDecayMul=%.3f out of [0.6,1.0]", mod.CuriosityDecayMul)
	}
	if mod.WorryDecayMul < 0.6 || mod.WorryDecayMul > 1.0 {
		t.Errorf("WorryDecayMul=%.3f out of [0.6,1.0]", mod.WorryDecayMul)
	}
	if mod.ConfidenceDecayMul < 1.0 || mod.ConfidenceDecayMul > 1.3 {
		t.Errorf("ConfidenceDecayMul=%.3f out of [1.0,1.3]", mod.ConfidenceDecayMul)
	}
}

func TestModulation_HighNeedsModulateStrongly(t *testing.T) {
	m := NewNeedModel()
	// Set all needs to max.
	m.mu.Lock()
	m.needs.Companionship = 1.0
	m.needs.Rest = 1.0
	m.needs.Play = 1.0
	m.needs.Curiosity = 1.0
	m.needs.Care = 1.0
	m.needs.Autonomy = 1.0
	m.mu.Unlock()

	mod := m.Modulation()

	// At max needs, modulation should be at extremes.
	if math.Abs(mod.LonelinessDecayMul-0.5) > 0.01 {
		t.Errorf("max companionship: LonelinessDecayMul=%.3f, want 0.5", mod.LonelinessDecayMul)
	}
	if math.Abs(mod.SleepinessGrowthMul-1.5) > 0.01 {
		t.Errorf("max rest: SleepinessGrowthMul=%.3f, want 1.5", mod.SleepinessGrowthMul)
	}
	if math.Abs(mod.PlayfulnessDecayMul-0.5) > 0.01 {
		t.Errorf("max play: PlayfulnessDecayMul=%.3f, want 0.5", mod.PlayfulnessDecayMul)
	}
}

func TestNeedSatisfactionForAction_AllActions(t *testing.T) {
	actions := []string{"speak_casual", "speak_care", "speak_inquiry", "observe", "reflect", "analyze_patterns", "none"}
	for _, action := range actions {
		s := domain.NeedSatisfactionForAction(action, domain.OutcomeIgnored)
		sum := s.Companionship + s.Rest + s.Play + s.Curiosity + s.Care + s.Autonomy
		if math.IsNaN(sum) {
			t.Errorf("action %q produced NaN satisfaction", action)
		}
	}
}

func TestClampNeed(t *testing.T) {
	if clampNeed(1.5) != 1.0 {
		t.Error("1.5→1.0")
	}
	if clampNeed(-0.5) != 0.0 {
		t.Error("-0.5→0.0")
	}
	if clampNeed(0.5) != 0.5 {
		t.Error("0.5→0.5")
	}
}
