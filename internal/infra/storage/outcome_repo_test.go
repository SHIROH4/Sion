package storage

import (
	"testing"
	"time"

	"desktop-pet/internal/domain"
)

func TestSuccessRateByType(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()
	repo := NewActionOutcomeRepo(store.db)

	// Insert test data.
	now := time.Now()
	insert := func(src domain.ProactiveSource, typ domain.CareTriggerType, outcome domain.OutcomeResult, hour int) {
		_ = repo.SaveOutcome(domain.ActionOutcome{
			ActionSource: src, ActionType: typ, Outcome: outcome,
			HourOfDay: hour, DayOfWeek: int(now.Weekday()),
			EmotionBucket: "neutral", CreatedAt: now.Unix(),
		})
	}
	insert(domain.SourceCare, domain.TriggerRest, domain.OutcomeReplied, 14)    // rest accepted
	insert(domain.SourceCare, domain.TriggerRest, domain.OutcomeRejected, 15)  // rest rejected
	insert(domain.SourceCare, domain.TriggerMeal, domain.OutcomeReplied, 12)   // meal accepted
	insert(domain.SourceCare, domain.TriggerMeal, domain.OutcomeReplied, 19)   // meal accepted
	insert(domain.SourceCasual, domain.TriggerSocial, domain.OutcomeIgnored, 10)

	rates, err := repo.SuccessRateByType(1) // 1-day window
	if err != nil {
		t.Fatal("SuccessRateByType:", err)
	}

	// rest: 1/2 = 0.5
	if r, ok := rates[domain.TriggerRest]; !ok || r < 0.4 || r > 0.6 {
		t.Errorf("rest rate = %.2f, want ~0.5", r)
	}
	// meal: 2/2 = 1.0
	if r, ok := rates[domain.TriggerMeal]; !ok || r < 0.9 {
		t.Errorf("meal rate = %.2f, want ~1.0", r)
	}
	// social: 0/1 = 0.0 (ignored counts as not accepted)
	if r, ok := rates[domain.TriggerSocial]; !ok || r > 0.1 {
		t.Errorf("social rate = %.2f, want ~0.0", r)
	}
}

func TestSuccessRateByTimeBlock(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()
	repo := NewActionOutcomeRepo(store.db)

	now := time.Now()
	insert := func(outcome domain.OutcomeResult, hour int) {
		_ = repo.SaveOutcome(domain.ActionOutcome{
			ActionSource: domain.SourceCare, ActionType: domain.TriggerRest,
			Outcome: outcome, HourOfDay: hour, DayOfWeek: int(now.Weekday()),
			EmotionBucket: "neutral", CreatedAt: now.Unix(),
		})
	}
	// Block 0 (late_night 0-5): 1/2 accepted.
	insert(domain.OutcomeReplied, 1)
	insert(domain.OutcomeRejected, 3)
	// Block 1 (morning 6-11): 2/2 accepted.
	insert(domain.OutcomeReplied, 8)
	insert(domain.OutcomeReplied, 10)
	// Block 2 (afternoon 12-17): 0/1.
	insert(domain.OutcomeIgnored, 14)
	// Block 3 (evening 18-23): 1/1 accepted.
	insert(domain.OutcomeReplied, 20)

	rates, err := repo.SuccessRateByTimeBlock(1)
	if err != nil {
		t.Fatal("SuccessRateByTimeBlock:", err)
	}

	if rates[0] < 0.4 || rates[0] > 0.6 {
		t.Errorf("block 0 = %.2f, want ~0.5", rates[0])
	}
	if rates[1] < 0.9 {
		t.Errorf("block 1 = %.2f, want ~1.0", rates[1])
	}
	if rates[2] > 0.1 {
		t.Errorf("block 2 = %.2f, want ~0.0", rates[2])
	}
	if rates[3] < 0.9 {
		t.Errorf("block 3 = %.2f, want ~1.0", rates[3])
	}
}

func TestSuccessRateBySource(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()
	repo := NewActionOutcomeRepo(store.db)

	now := time.Now()
	insert := func(src domain.ProactiveSource, outcome domain.OutcomeResult) {
		_ = repo.SaveOutcome(domain.ActionOutcome{
			ActionSource: src, ActionType: domain.TriggerRest,
			Outcome: outcome, HourOfDay: 14, DayOfWeek: int(now.Weekday()),
			EmotionBucket: "neutral", CreatedAt: now.Unix(),
		})
	}
	insert(domain.SourceCare, domain.OutcomeReplied)
	insert(domain.SourceCare, domain.OutcomeReplied)
	insert(domain.SourceCare, domain.OutcomeRejected) // care: 2/3
	insert(domain.SourceCasual, domain.OutcomeReplied)
	insert(domain.SourceCasual, domain.OutcomeIgnored) // casual: 1/2
	insert(domain.SourceKnowledgeGap, domain.OutcomeRejected) // gap: 0/1

	rates, err := repo.SuccessRateBySource(1)
	if err != nil {
		t.Fatal("SuccessRateBySource:", err)
	}

	if r, ok := rates[domain.SourceCare]; !ok || r < 0.6 || r > 0.75 {
		t.Errorf("care rate = %.2f, want ~0.67", r)
	}
	if r, ok := rates[domain.SourceCasual]; !ok || r < 0.4 || r > 0.6 {
		t.Errorf("casual rate = %.2f, want ~0.5", r)
	}
	if r, ok := rates[domain.SourceKnowledgeGap]; !ok || r > 0.1 {
		t.Errorf("gap rate = %.2f, want ~0.0", r)
	}
}

func TestSuccessRateByType_Empty(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()
	repo := NewActionOutcomeRepo(store.db)

	rates, err := repo.SuccessRateByType(7)
	if err != nil {
		t.Fatal(err)
	}
	if len(rates) != 0 {
		t.Errorf("expected empty, got %d entries", len(rates))
	}
}
