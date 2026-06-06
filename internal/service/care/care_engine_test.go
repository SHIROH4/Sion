package care

import (
	"desktop-pet/internal/domain"
	emotion "desktop-pet/internal/service/emotion"
	"testing"
	"time"
)

func TestCareEngine_New(t *testing.T) {
	state := domain.NewUserCareState()
	engine := NewCareEngine(state, nil, func() emotion.EmotionState { return emotion.EmotionState{} })

	if engine.state != state {
		t.Error("expected engine.state to be the injected state")
	}
	if len(engine.triggers) != 6 {
		t.Errorf("expected 6 default triggers, got %d", len(engine.triggers))
	}
	if engine.AnnoyanceLevel() != 0 {
		t.Errorf("expected annoyanceLevel=0, got %.2f", engine.AnnoyanceLevel())
	}
	if len(engine.ActionLog(100)) != 0 {
		t.Error("expected empty action log on new engine")
	}
}

func TestCareEngine_Evaluate_NoTriggers(t *testing.T) {
	state := domain.NewUserCareState()
	engine := NewCareEngine(state, nil, func() emotion.EmotionState { return emotion.EmotionState{} })

	// Fresh state — no triggers should fire. Use fixed non-meal time.
	actions := engine.Evaluate(time.Date(2026, 6, 2, 10, 0, 0, 0, time.Local))
	if len(actions) != 0 {
		t.Errorf("expected 0 actions from fresh state, got %d", len(actions))
	}
}

func TestCareEngine_Evaluate_MultipleTriggers(t *testing.T) {
	state := domain.NewUserCareState()
	state.Mu.Lock()
	state.LastDrinkAt = time.Now().Add(-2 * time.Hour)
	state.LastMealAt = time.Now().Add(-6 * time.Hour)
	state.ContinuousWork = 200
	state.PostureWarning = true
	state.StressLevel = 0.7
	state.SocialActivity = 0.1
	state.IsolationHours = 10
	state.Mu.Unlock()

	// Emotion: low valence → encourage trigger.
	_ = emotion.EmotionState{Valence: -0.5}
	engine := NewCareEngine(state, nil, func() emotion.EmotionState { return emotion.EmotionState{} })

	// 1:00 AM — rest + hydration + health + encourage should be eligible.
	nightTime := time.Date(2026, 6, 1, 1, 0, 0, 0, time.Local)
	actions := engine.Evaluate(nightTime)

	if len(actions) == 0 {
		t.Fatal("expected multiple triggers at 1AM with poor state")
	}

	// Verify sorted by priority (1 = first).
	for i := 1; i < len(actions); i++ {
		if actions[i].Priority < actions[i-1].Priority {
			t.Errorf("actions not sorted by priority: action[%d].Priority=%d < action[%d].Priority=%d",
				i, actions[i].Priority, i-1, actions[i-1].Priority)
		}
	}

	// Rest should be first (priority 1).
	if actions[0].Type != domain.TriggerRest {
		t.Errorf("expected Rest (p1) first, got %s (p%d)", actions[0].Type, actions[0].Priority)
	}
}

func TestCareEngine_Evaluate_NightMode(t *testing.T) {
	state := domain.NewUserCareState()
	state.Mu.Lock()
	state.LastDrinkAt = time.Now().Add(-2 * time.Hour) // hydration condition met
	state.LastMealAt = time.Now().Add(-6 * time.Hour)  // meal condition met
	state.ContinuousWork = 200                         // rest + health condition met
	state.StressLevel = 0.7
	state.Mu.Unlock()

	_ = emotion.EmotionState{Valence: -0.5}
	engine := NewCareEngine(state, nil, func() emotion.EmotionState { return emotion.EmotionState{} })

	// 22:30 — night mode: only rest and health allowed.
	nightTime := time.Date(2026, 6, 1, 22, 30, 0, 0, time.Local)
	actions := engine.Evaluate(nightTime)

	for _, a := range actions {
		if a.Type != domain.TriggerRest && a.Type != domain.TriggerHealth {
			t.Errorf("night mode should filter out %s, only rest/health allowed", a.Type)
		}
	}
}

func TestCareEngine_Evaluate_FocusMode(t *testing.T) {
	state := domain.NewUserCareState()
	state.Mu.Lock()
	state.FocusLevel = 0.9 // deep focus
	state.LastDrinkAt = time.Now().Add(-2 * time.Hour)
	state.LastMealAt = time.Now().Add(-6 * time.Hour)
	state.ContinuousWork = 200
	state.Mu.Unlock()

	engine := NewCareEngine(state, nil, func() emotion.EmotionState { return emotion.EmotionState{} })

	// 1:00 AM + deep focus → only rest (priority 1) should pass.
	nightTime := time.Date(2026, 6, 1, 1, 0, 0, 0, time.Local)
	actions := engine.Evaluate(nightTime)

	for _, a := range actions {
		if a.Priority > 1 {
			t.Errorf("focus mode should filter out priority>1 actions, got %s (p%d)", a.Type, a.Priority)
		}
	}
}

func TestCareEngine_RecordResponse_Accepted(t *testing.T) {
	state := domain.NewUserCareState()
	engine := NewCareEngine(state, nil, func() emotion.EmotionState { return emotion.EmotionState{} })

	// Manually add an action to the log.
	engine.mu.Lock()
	engine.actionLog = append(engine.actionLog, domain.CareAction{ID: 1, Type: domain.TriggerHydration})
	engine.state.AnnoyanceLevel = 0.5
	engine.mu.Unlock()

	engine.RecordResponse(1, true, "谢谢关心")

	if engine.AnnoyanceLevel() >= 0.5 {
		t.Errorf("expected annoyanceLevel < 0.5 after acceptance, got %.2f", engine.AnnoyanceLevel())
	}

	// Verify the action was updated.
	log := engine.ActionLog(1)
	if len(log) != 1 {
		t.Fatal("expected 1 action in log")
	}
	if log[0].Accepted == nil || !*log[0].Accepted {
		t.Error("expected Accepted=true")
	}
	if log[0].Response != "谢谢关心" {
		t.Errorf("expected Response='谢谢关心', got %s", log[0].Response)
	}
}

func TestCareEngine_RecordResponse_Rejected(t *testing.T) {
	state := domain.NewUserCareState()
	engine := NewCareEngine(state, nil, func() emotion.EmotionState { return emotion.EmotionState{} })

	engine.mu.Lock()
	engine.actionLog = append(engine.actionLog, domain.CareAction{ID: 1, Type: domain.TriggerHydration})
	engine.state.AnnoyanceLevel = 0.2
	engine.mu.Unlock()

	engine.RecordResponse(1, false, "别烦我")

	if engine.AnnoyanceLevel() <= 0.2 {
		t.Errorf("expected annoyanceLevel > 0.2 after rejection, got %.2f", engine.AnnoyanceLevel())
	}

	log := engine.ActionLog(1)
	if log[0].Accepted == nil || *log[0].Accepted {
		t.Error("expected Accepted=false")
	}
}

func TestCareEngine_AnnoyanceLevel_Decays(t *testing.T) {
	state := domain.NewUserCareState()
	engine := NewCareEngine(state, nil, func() emotion.EmotionState { return emotion.EmotionState{} })

	// Start with high annoyance.
	engine.mu.Lock()
	engine.state.AnnoyanceLevel = 0.5
	engine.mu.Unlock()

	// Accept 5 times — should drop to near 0.
	for i := 0; i < 5; i++ {
		id := int64(i + 1)
		engine.mu.Lock()
		engine.actionLog = append(engine.actionLog, domain.CareAction{ID: id, Type: domain.TriggerHydration})
		engine.mu.Unlock()
		engine.RecordResponse(id, true, "谢谢")
	}

	if engine.AnnoyanceLevel() > 0.1 {
		t.Errorf("expected annoyanceLevel near 0 after 5 acceptances, got %.2f", engine.AnnoyanceLevel())
	}
}

func TestCareEngine_ShouldPoke_Cooldown(t *testing.T) {
	state := domain.NewUserCareState()
	engine := NewCareEngine(state, nil, func() emotion.EmotionState { return emotion.EmotionState{} })

	// Just poked — should not poke again.
	engine.mu.Lock()
	engine.lastPokeAt = time.Now()
	engine.mu.Unlock()

	if engine.ShouldPoke(time.Now()) {
		t.Error("expected ShouldPoke=false immediately after poke")
	}

	// After 3 minutes — should allow.
	if !engine.ShouldPoke(time.Now().Add(3 * time.Minute)) {
		t.Error("expected ShouldPoke=true after 3 minutes")
	}
}

func TestCareEngine_ShouldPoke_HighAnnoyance(t *testing.T) {
	state := domain.NewUserCareState()
	engine := NewCareEngine(state, nil, func() emotion.EmotionState { return emotion.EmotionState{} })

	engine.mu.Lock()
	engine.state.AnnoyanceLevel = 0.8
	engine.lastPokeAt = time.Now().Add(-5 * time.Minute)
	engine.mu.Unlock()

	if engine.ShouldPoke(time.Now()) {
		t.Error("expected ShouldPoke=false when annoyance > 0.7")
	}
}

func TestCareEngine_ShouldPoke_HighFocus(t *testing.T) {
	state := domain.NewUserCareState()
	state.Mu.Lock()
	state.FocusLevel = 0.9
	state.Mu.Unlock()

	engine := NewCareEngine(state, nil, func() emotion.EmotionState { return emotion.EmotionState{} })
	engine.mu.Lock()
	engine.lastPokeAt = time.Now().Add(-5 * time.Minute)
	engine.mu.Unlock()

	if engine.ShouldPoke(time.Now()) {
		t.Error("expected ShouldPoke=false when FocusLevel > 0.85")
	}
}

func TestCareEngine_ActionLog_Truncated(t *testing.T) {
	state := domain.NewUserCareState()
	engine := NewCareEngine(state, nil, func() emotion.EmotionState { return emotion.EmotionState{} })

	// Add 60 actions via RecordResponse, which triggers truncation at 50.
	for i := int64(1); i <= 60; i++ {
		engine.mu.Lock()
		engine.actionLog = append(engine.actionLog, domain.CareAction{ID: i, Type: domain.TriggerHydration})
		engine.mu.Unlock()
		// RecordResponse trims to 50 — call it for the last 10 to trigger truncation.
		if i > 50 {
			engine.RecordResponse(i, true, "ok")
		}
	}

	log := engine.ActionLog(100)
	if len(log) > 50 {
		t.Errorf("expected actionLog truncated to 50, got %d", len(log))
	}
}

func TestCareEngine_Poke(t *testing.T) {
	state := domain.NewUserCareState()
	// Use a fixed daytime to avoid night-mode filtering.
	now := time.Date(2026, 6, 1, 14, 0, 0, 0, time.Local)
	state.Mu.Lock()
	state.LastDrinkAt = now.Add(-2 * time.Hour)
	state.ContinuousWork = 90 // hydration condition met
	state.Mu.Unlock()

	var firedAction *domain.CareAction
	engine := NewCareEngine(state, func(a domain.CareAction) error {
		firedAction = &a
		return nil
	}, func() emotion.EmotionState { return emotion.EmotionState{} })

	// Allow time for poke cooldown.
	engine.mu.Lock()
	engine.lastPokeAt = now.Add(-5 * time.Minute)
	engine.mu.Unlock()

	action, err := engine.Poke(now)
	if err != nil {
		t.Fatalf("Poke returned error: %v", err)
	}
	if action == nil {
		t.Fatal("expected Poke to return an action")
	}
	if action.Type != domain.TriggerHydration {
		t.Errorf("expected hydration action, got %s", action.Type)
	}
	if action.Message == "" {
		t.Error("expected action.Message to be populated")
	}
	if firedAction == nil {
		t.Error("expected onCare callback to be called")
	}
}

func TestCareEngine_Poke_NoEligibleTriggers(t *testing.T) {
	state := domain.NewUserCareState() // fresh state, no triggers met
	engine := NewCareEngine(state, nil, func() emotion.EmotionState { return emotion.EmotionState{} })
	fixedTime := time.Date(2026, 6, 2, 10, 0, 0, 0, time.Local)
	engine.mu.Lock()
	engine.lastPokeAt = fixedTime.Add(-5 * time.Minute)
	engine.mu.Unlock()

	action, err := engine.Poke(fixedTime)
	if err != nil {
		t.Fatalf("Poke returned error: %v", err)
	}
	if action != nil {
		t.Errorf("expected nil action when no triggers fire, got %+v", action)
	}
}

func TestCareEngine_Poke_ShouldPokeBlocks(t *testing.T) {
	state := domain.NewUserCareState()
	state.Mu.Lock()
	state.LastDrinkAt = time.Now().Add(-2 * time.Hour)
	state.ContinuousWork = 90
	state.Mu.Unlock()

	engine := NewCareEngine(state, nil, func() emotion.EmotionState { return emotion.EmotionState{} })
	// lastPokeAt is now → ShouldPoke returns false.
	action, err := engine.Poke(time.Now())
	if err != nil {
		t.Fatalf("Poke returned error: %v", err)
	}
	if action != nil {
		t.Error("expected nil action when ShouldPoke returns false")
	}
}

func TestCareEngine_SetGenerateMessage(t *testing.T) {
	now := time.Date(2026, 6, 1, 14, 0, 0, 0, time.Local)
	state := domain.NewUserCareState()
	state.Mu.Lock()
	state.LastDrinkAt = now.Add(-2 * time.Hour)
	state.ContinuousWork = 90
	state.Mu.Unlock()

	engine := NewCareEngine(state, func(a domain.CareAction) error { return nil },
		func() emotion.EmotionState { return emotion.EmotionState{} })
	engine.mu.Lock()
	engine.lastPokeAt = now.Add(-5 * time.Minute)
	engine.mu.Unlock()

	customMsg := "喵~主人该喝水啦！(自定义消息)"
	engine.SetGenerateMessage(func(ct domain.CareTriggerType, _ *domain.UserCareState, _ *emotion.EmotionState, _ *emotion.EmotionVector, _ string) string {
		return customMsg
	})

	action, err := engine.Poke(now)
	if err != nil {
		t.Fatalf("Poke returned error: %v", err)
	}
	if action == nil {
		t.Fatal("expected Poke to return an action")
	}
	if action.Message != customMsg {
		t.Errorf("expected custom message, got %s", action.Message)
	}
}

func TestCareEngine_DefaultMessages_AllTypes(t *testing.T) {
	// Verify every trigger type has a non-empty default message.
	types := []domain.CareTriggerType{
		domain.TriggerHydration, domain.TriggerMeal, domain.TriggerRest,
		domain.TriggerEncourage, domain.TriggerSocial, domain.TriggerHealth,
	}
	sn := &domain.UserCareState{ContinuousWork: 120}

	for _, typ := range types {
		msg := DefaultCareMessage(typ, sn)
		if msg == "" {
			t.Errorf("DefaultCareMessage(%s) returned empty string", typ)
		}
	}
}
