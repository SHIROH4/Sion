package cognition

import (
	"math"
	"os"
	"testing"
	"time"

	"desktop-pet/internal/domain"
)

// ---- Test Helpers ----

func defaultFeats() *domain.QuantifiedFeatures {
	return &domain.QuantifiedFeatures{
		A1_Loneliness: 0.5, A1_Playfulness: 0.5, A1_Affection: 0.5,
		A1_Annoyance: 0.1, A1_Worry: 0.5, A1_Curiosity: 0.5, A1_Sleepiness: 0.1,
		A1_Confidence: 0.5,
		U14_TimeSinceChatMins: 30,
		E1_Hour:               14,
		R1_OverallAcceptRate:  0.7, R1_SampleCount: 10,
		U10_TimeWindowPref: 0.7,
	}
}

func defaultNeeds() *domain.IntrinsicNeeds {
	return &domain.IntrinsicNeeds{
		Companionship: 0.3, Care: 0.3, Play: 0.3,
		Curiosity: 0.4, Rest: 0.2, Autonomy: 0.3,
	}
}

func defaultCtx() domain.DecisionContext {
	return domain.DecisionContext{
		Now:               time.Now(),
		EmotionVec:        domain.EmotionVector{Affection: 0.5, Curiosity: 0.5, Loneliness: 0.5, Worry: 0.5, Sleepiness: 0.1, Playfulness: 0.5, Annoyance: 0.1, Confidence: 0.5},
		EmotionState:      domain.EmotionState{Primary: "neutral", Intensity: 0.5, Valence: 0.0},
		TimeSinceLastChat: 30 * time.Minute,
		DailyActionCount:  2,
	}
}

// ---- ComputeDrives Tests ----

func TestComputeDrives_NilFeats(t *testing.T) {
	s, c, cur, q, e := ComputeDrives(nil, nil)
	assertRange(t, "social", s, 0.2, 0.5)
	assertRange(t, "care", c, 0.1, 0.4)
	assertRange(t, "curious", cur, 0.15, 0.5)
	assertRange(t, "quiet", q, 0.0, 0.3)
	assertRange(t, "explore", e, 0.1, 0.5)
}

func TestComputeDrives_Neutral(t *testing.T) {
	f := defaultFeats()
	n := defaultNeeds()
	s, c, cur, q, e := ComputeDrives(f, n)
	assertRange(t, "social", s, 0.3, 0.7)
	assertRange(t, "care", c, 0.2, 0.6)
	assertRange(t, "curious", cur, 0.2, 0.6)
	assertRange(t, "quiet", q, 0.0, 0.35)
	assertRange(t, "explore", e, 0.2, 0.6)
}

func TestComputeDrives_LonelyAndPlayful(t *testing.T) {
	f := defaultFeats()
	f.A1_Loneliness = 0.9
	f.A1_Playfulness = 0.9
	f.A1_Affection = 0.9
	f.A1_Annoyance = 0.0
	f.U14_TimeSinceChatMins = 60

	s, _, _, _, _ := ComputeDrives(f, defaultNeeds())
	if s < 0.5 {
		t.Errorf("expected social >0.5 with high loneliness+play+affection, got %.3f", s)
	}
}

func TestComputeDrives_AnnoyedAndSleepy(t *testing.T) {
	f := defaultFeats()
	f.A1_Annoyance = 0.9
	f.A1_Sleepiness = 0.9
	f.U14_TimeSinceChatMins = 1

	_, _, _, q, _ := ComputeDrives(f, defaultNeeds())
	if q < 0.4 {
		t.Errorf("expected quiet >0.4 with high annoyance+sleepiness, got %.3f", q)
	}
}

func TestComputeDrives_WorriedAtNight(t *testing.T) {
	f := defaultFeats()
	f.A1_Worry = 0.9
	f.A1_Affection = 0.8
	f.E1_Hour = 1 // 1am

	_, c, _, _, _ := ComputeDrives(f, defaultNeeds())
	if c < 0.5 {
		t.Errorf("expected care >0.5 at night with worry+affection, got %.3f", c)
	}
}

func TestComputeDrives_CuriousWithInquiries(t *testing.T) {
	f := defaultFeats()
	f.A1_Curiosity = 0.9
	f.A11_ActiveInquiries = 3
	f.A12_KnowledgeGaps = 2

	_, _, cur, _, _ := ComputeDrives(f, defaultNeeds())
	if cur < 0.5 {
		t.Errorf("expected curious >0.5, got %.3f", cur)
	}
}

func TestComputeDrives_IsWorkingSuppressesSocial(t *testing.T) {
	fWork := defaultFeats()
	fWork.U3_IsWorking = 1.0
	sWork, _, _, _, _ := ComputeDrives(fWork, defaultNeeds())

	fPlay := defaultFeats()
	fPlay.U3_IsWorking = 0.0
	sPlay, _, _, _, _ := ComputeDrives(fPlay, defaultNeeds())

	if sWork >= sPlay {
		t.Errorf("working social(%.3f) should be < non-working social(%.3f)", sWork, sPlay)
	}
}

func TestComputeDrives_RejectionSuppressesSocial(t *testing.T) {
	f := defaultFeats()
	f.A1_Loneliness = 0.8
	sNormal, _, _, _, _ := ComputeDrives(f, defaultNeeds())

	f.R4_RejectionSeverity = 0.9
	sRejected, _, _, _, _ := ComputeDrives(f, defaultNeeds())

	if sRejected >= sNormal {
		t.Errorf("rejected social(%.3f) should be < normal social(%.3f)", sRejected, sNormal)
	}
}

func TestComputeDrives_LowAcceptanceGate(t *testing.T) {
	fGood := defaultFeats()
	fGood.R1_OverallAcceptRate = 0.8
	fGood.R1_SampleCount = 10
	sGood, _, _, _, _ := ComputeDrives(fGood, defaultNeeds())

	fBad := defaultFeats()
	fBad.R1_OverallAcceptRate = 0.2
	fBad.R1_SampleCount = 10
	sBad, _, _, _, _ := ComputeDrives(fBad, defaultNeeds())

	if sBad >= sGood {
		t.Errorf("low acceptance social(%.3f) should be < high acceptance social(%.3f)", sBad, sGood)
	}
}

func TestComputeDrives_NeedsPushDrives(t *testing.T) {
	f := defaultFeats()

	nHighComp := defaultNeeds()
	nHighComp.Companionship = 0.9
	sHigh, _, _, _, _ := ComputeDrives(f, nHighComp)

	nLowComp := defaultNeeds()
	nLowComp.Companionship = 0.1
	sLow, _, _, _, _ := ComputeDrives(f, nLowComp)

	if sHigh <= sLow {
		t.Errorf("high companionship social(%.3f) should be > low(%.3f)", sHigh, sLow)
	}
}

func TestComputeDrives_ContinuousWorkBoostsCare(t *testing.T) {
	f := defaultFeats()
	f.A1_Worry = 0.6

	f.U4_ContinuousWorkNorm = 0.9 // ~3h work
	_, cLong, _, _, _ := ComputeDrives(f, defaultNeeds())

	f.U4_ContinuousWorkNorm = 0.0
	_, cShort, _, _, _ := ComputeDrives(f, defaultNeeds())

	if cLong <= cShort {
		t.Errorf("long work care(%.3f) should be > short work care(%.3f)", cLong, cShort)
	}
}

func TestComputeDrives_AllInRange(t *testing.T) {
	f := defaultFeats()
	n := defaultNeeds()
	for _, hour := range []float64{0, 3, 6, 9, 12, 15, 18, 21} {
		f.E1_Hour = hour
		s, c, cur, q, e := ComputeDrives(f, n)
		for name, v := range map[string]float64{"social": s, "care": c, "curious": cur, "quiet": q, "explore": e} {
			if v < 0.0 || v > 1.0 {
				t.Errorf("drive %s out of range at hour %.0f: %.3f", name, hour, v)
			}
		}
	}
}

// ---- ScoreActions Tests ----

func TestScoreActions_ReturnsValidAction(t *testing.T) {
	m := NewMotivator()
	f := defaultFeats()
	s, c, cur, q, e := ComputeDrives(f, defaultNeeds())
	action, score , _ := m.ScoreActions(s, c, cur, q, e, f, nil)
	if action == "" {
		t.Fatal("expected non-empty action")
	}
	if math.IsInf(score, -1) || score < 0 || score > 1 {
		t.Errorf("score out of range: %.3f", score)
	}
}

func TestScoreActions_NoneWinsWhenQuiet(t *testing.T) {
	m := NewMotivator()
	f := defaultFeats()
	f.A1_Sleepiness = 0.9
	f.A1_Annoyance = 0.9
	f.U14_TimeSinceChatMins = 1
	s, c, cur, q, e := ComputeDrives(f, defaultNeeds())
	action, _ , _ := m.ScoreActions(s, c, cur, q, e, f, nil)
	if action != "none" {
		t.Errorf("expected 'none' when sleepy+annoyed, got %q", action)
	}
}

func TestScoreActions_SpeakWinsWhenLonely(t *testing.T) {
	m := NewMotivator()
	f := defaultFeats()
	f.A1_Loneliness = 0.9
	f.A1_Playfulness = 0.8
	f.A1_Affection = 0.8
	f.U14_TimeSinceChatMins = 60
	s, c, cur, q, e := ComputeDrives(f, defaultNeeds())
	action, _ , _ := m.ScoreActions(s, c, cur, q, e, f, nil)
	if action == "none" {
		t.Errorf("expected a speak action, got 'none'")
	}
	t.Logf("action: %s", action)
}

func TestScoreActions_ContextModulatorBoostsSuccessfulAction(t *testing.T) {
	m := NewMotivator()
	f := defaultFeats()
	f.A7_ActionSuccessRate = map[string]float64{"social": 0.9} // speak_casual → social type, 90% success
	f.R3_SourceAcceptRate = map[string]float64{"casual": 0.9}
	s, c, cur, q, e := ComputeDrives(f, defaultNeeds())
	action, score , _ := m.ScoreActions(s, c, cur, q, e, f, nil)
	if action == "none" {
		t.Errorf("successful action should win, got %q (score=%.3f)", action, score)
	}
}

func TestScoreActions_ContextModulatorPenalizesFailingAction(t *testing.T) {
	m := NewMotivator()
	f := defaultFeats()
	f.A1_Loneliness = 0.9
	f.A1_Playfulness = 0.8
	// speak_casual → social type, 10% success → should be penalized.
	f.A7_ActionSuccessRate = map[string]float64{"social": 0.1}
	f.R3_SourceAcceptRate = map[string]float64{"casual": 0.1}
	s, c, cur, q, e := ComputeDrives(f, defaultNeeds())
	action, _ , _ := m.ScoreActions(s, c, cur, q, e, f, nil)
	t.Logf("with failing social history, action=%s", action)
	// Should still return a valid action, just lower score.
	if action == "" {
		t.Error("should still return an action")
	}
}

// ---- RouteToLLM Tests ----

func TestRouteToLLM_EmotionSpike(t *testing.T) {
	ctx := defaultCtx()
	ctx.EmotionState.Primary = "fear"
	ctx.EmotionState.Intensity = 0.9
	if !RouteToLLM(ctx, "speak_casual", 1, nil, nil, "speak_casual") {
		t.Error("fear+intensity → LLM")
	}
}

func TestRouteToLLM_Sleepiness(t *testing.T) {
	ctx := defaultCtx()
	ctx.EmotionVec.Sleepiness = 0.9
	if !RouteToLLM(ctx, "speak_casual", 1, nil, nil, "speak_casual") {
		t.Error("sleepiness>0.85 → LLM")
	}
}

func TestRouteToLLM_RepetitionLoop(t *testing.T) {
	ctx := defaultCtx()
	if !RouteToLLM(ctx, "speak_casual", 3, nil, nil, "speak_casual") {
		t.Error("3 consecutive → LLM")
	}
	// "none" excluded from repetition detection.
	if RouteToLLM(ctx, "none", 3, nil, nil, "none") {
		t.Error("consecutive none should not route to LLM")
	}
}

func TestRouteToLLM_Normal(t *testing.T) {
	ctx := defaultCtx()
	if RouteToLLM(ctx, "speak_casual", 1, nil, nil, "speak_casual") {
		t.Error("normal → no LLM")
	}
}

func TestRouteToLLM_RejectionCascade(t *testing.T) {
	ctx := defaultCtx()
	f := defaultFeats()
	f.R4_RecentRejections = 4
	if !RouteToLLM(ctx, "speak_casual", 1, f, nil, "speak_casual") {
		t.Error("3+ rejections → LLM")
	}
}

func TestRouteToLLM_AcceptanceCollapse_WithTrend(t *testing.T) {
	ctx := defaultCtx()
	f := defaultFeats()
	f.R1_OverallAcceptRate = 0.2
	f.R1_SampleCount = 10
	f.R8_IntimacyTrend = -0.3 // getting worse over time
	if !RouteToLLM(ctx, "speak_casual", 1, f, nil, "speak_casual") {
		t.Error("low rate + negative intimacy trend → collapse → LLM")
	}
}

func TestRouteToLLM_AcceptanceCollapse_NoTrend(t *testing.T) {
	ctx := defaultCtx()
	f := defaultFeats()
	f.R1_OverallAcceptRate = 0.2
	f.R1_SampleCount = 10
	f.R8_IntimacyTrend = 0.0 // stable, just consistently low
	if RouteToLLM(ctx, "speak_casual", 1, f, nil, "speak_casual") {
		t.Error("low rate without negative trend → NOT a collapse (cold start)")
	}
}

func TestRouteToLLM_AcceptanceCollapse_InsufficientSamples(t *testing.T) {
	ctx := defaultCtx()
	f := defaultFeats()
	f.R1_OverallAcceptRate = 0.2
	f.R1_SampleCount = 5 // too few samples
	f.R8_IntimacyTrend = -0.3
	if RouteToLLM(ctx, "speak_casual", 1, f, nil, "speak_casual") {
		t.Error("insufficient samples → no collapse detection")
	}
}

func TestRouteToLLM_NeedConflict_Companionship(t *testing.T) {
	ctx := defaultCtx()
	f := defaultFeats()
	f.R4_RejectionSeverity = 0.1
	f.U3_IsWorking = 0.0
	n := defaultNeeds()
	n.Companionship = 0.9
	// topAction="none" → conflict between high companionship and choosing silence.
	if !RouteToLLM(ctx, "speak_casual", 1, f, n, "none") {
		t.Error("high companionship + topAction=none → LLM")
	}
}

func TestRouteToLLM_NeedConflict_Care(t *testing.T) {
	ctx := defaultCtx()
	f := defaultFeats()
	f.R4_RejectionSeverity = 0.1
	f.U3_IsWorking = 0.0
	n := defaultNeeds()
	n.Care = 0.9
	if !RouteToLLM(ctx, "speak_casual", 1, f, n, "none") {
		t.Error("high care + topAction=none → LLM")
	}
}

func TestRouteToLLM_NeedConflict_NoConflictWhenSpeaking(t *testing.T) {
	ctx := defaultCtx()
	f := defaultFeats()
	f.R4_RejectionSeverity = 0.1
	f.U3_IsWorking = 0.0
	n := defaultNeeds()
	n.Companionship = 0.9
	// topAction="speak_casual" → no conflict, cat is already speaking.
	if RouteToLLM(ctx, "speak_casual", 1, f, n, "speak_casual") {
		t.Error("high companionship + topAction=speak → no conflict")
	}
}

func TestRouteToLLM_NeedConflict_BlockedByRejection(t *testing.T) {
	ctx := defaultCtx()
	f := defaultFeats()
	f.R4_RejectionSeverity = 0.5 // being rejected
	f.U3_IsWorking = 0.0
	n := defaultNeeds()
	n.Companionship = 0.9
	// Rejection severity >0.3 → "available" is false → no LLM route.
	if RouteToLLM(ctx, "speak_casual", 1, f, n, "none") {
		t.Error("high companionship but rejected → should not route (respecting rejection)")
	}
}

func TestRouteToLLM_ValenceCrash(t *testing.T) {
	ctx := defaultCtx()
	f := defaultFeats()
	f.A4_ValenceTrend = -0.7
	if !RouteToLLM(ctx, "speak_casual", 1, f, nil, "speak_casual") {
		t.Error("valence crash → LLM")
	}
}

func TestRouteToLLM_LongSilence(t *testing.T) {
	ctx := defaultCtx()
	f := defaultFeats()
	f.U14_TimeSinceChatMins = 300 // 5h idle (exceeds 4h threshold)
	// consecutiveCount <= 1 → early re-engagement tick.
	if !RouteToLLM(ctx, "speak_casual", 1, f, nil, "speak_casual") {
		t.Error(">4h idle + early tick → LLM re-engagement")
	}
}

func TestRouteToLLM_LongSilence_NotFirstTick(t *testing.T) {
	ctx := defaultCtx()
	f := defaultFeats()
	f.U14_TimeSinceChatMins = 180
	// consecutiveCount=2 → not first tick, already decided before.
	if RouteToLLM(ctx, "speak_casual", 2, f, nil, "speak_casual") {
		t.Error("long idle but not first tick → no LLM (already handled)")
	}
}

// ---- Weight Persistence ----

func TestWeightPersistence(t *testing.T) {
	tmp := t.TempDir() + "/weights.json"
	m1 := NewMotivator()
	m1.weights["speak_casual"] = ActionWeight{Social: 0.99}
	m1.SetStoragePath(tmp)
	m1.Save()

	m2 := NewMotivator()
	m2.SetStoragePath(tmp)
	if m2.weights["speak_casual"].Social != 0.99 {
		t.Errorf("expected 0.99, got %.4f", m2.weights["speak_casual"].Social)
	}
}

func TestUpdateWeightsFromOutcome_Positive(t *testing.T) {
	m := NewMotivator()
	old := m.weights["speak_casual"].Social
	m.UpdateWeightsFromOutcome("speak_casual", 1.0, 0.5, 0.3, 0.2, 0.1, 0.1)
	if m.weights["speak_casual"].Social <= old {
		t.Errorf("expected increased social weight: old=%.4f new=%.4f", old, m.weights["speak_casual"].Social)
	}
}

func TestUpdateWeightsFromOutcome_Negative(t *testing.T) {
	m := NewMotivator()
	old := m.weights["speak_care"].Care
	m.UpdateWeightsFromOutcome("speak_care", -1.0, 0.2, 0.7, 0.1, 0.0, 0.0)
	if m.weights["speak_care"].Care >= old {
		t.Errorf("expected decreased care weight: old=%.4f new=%.4f", old, m.weights["speak_care"].Care)
	}
}

func TestUpdateWeightsFromOutcome_NoneDoesNothing(t *testing.T) {
	m := NewMotivator()
	old := m.weights["none"].Quiet
	m.UpdateWeightsFromOutcome("none", 1.0, 0.5, 0.5, 0.5, 0.5, 0.5)
	if m.weights["none"].Quiet != old {
		t.Error("none action weights should not change")
	}
}

func TestDefaultWeightsAllActionsPresent(t *testing.T) {
	m := NewMotivator()
	expected := []string{
		"speak_casual", "speak_care", "speak_inquiry",
		"care_rest", "care_meal", "care_hydration", "care_health", "care_encourage", "care_social",
		"observe", "reflect", "analyze_patterns", "none",
	}
	for _, name := range expected {
		if _, ok := m.weights[name]; !ok {
			t.Errorf("missing default weight for action %q", name)
		}
	}
}

func TestMotivator_NoPathNoCrash(t *testing.T) {
	m := NewMotivator()
	m.Save()
	f := defaultFeats()
	s, c, cur, q, e := ComputeDrives(f, defaultNeeds())
	_, score , _ := m.ScoreActions(s, c, cur, q, e, f, nil)
	if score < 0 || score > 1 {
		t.Error("score out of range")
	}
}

func TestUpdateWeight_InvalidField(t *testing.T) {
	m := NewMotivator()
	old := m.weights["speak_casual"].Social
	m.UpdateWeight("speak_casual", "nonexistent", 1.0)
	if m.weights["speak_casual"].Social != old {
		t.Error("should not change for invalid field")
	}
}

func TestUpdateWeight_InvalidAction(t *testing.T) {
	m := NewMotivator()
	m.UpdateWeight("nonexistent", "social", 1.0) // should not panic
}

func TestWeightClamp(t *testing.T) {
	if clampWeight(2.0) != 1.0 || clampWeight(-2.0) != -1.0 {
		t.Error("clampWeight")
	}
}

func TestRouteToLLMWithScores(t *testing.T) {
	if !RouteToLLMWithScores(0.50, 0.48, "speak_casual") {
		t.Error("close scores → conflict")
	}
	if RouteToLLMWithScores(0.80, 0.30, "speak_casual") {
		t.Error("distant scores → no conflict")
	}
	if RouteToLLMWithScores(0.35, 0.31, "none") {
		t.Error("non-speak winner → no LLM needed")
	}
}

func TestScoreActions_ReturnsSecondScore(t *testing.T) {
	m := NewMotivator()
	f := defaultFeats()
	s, c, cur, q, e := ComputeDrives(f, defaultNeeds())
	_, score, second := m.ScoreActions(s, c, cur, q, e, f, nil)
	if second > score {
		t.Errorf("second(%.3f) should be <= winner(%.3f)", second, score)
	}
	// With normal params, there should be a meaningful second score.
	if second <= 0 {
		t.Error("second score should be >0 with normal params")
	}
}

func TestComputeDrives_CooldownBoost(t *testing.T) {
	f := defaultFeats()
	// Just acted (cooldown fresh).
	f.E3_CooldownNorm = 0.1 // ~3 min since last action
	_, _, _, qFresh, _ := ComputeDrives(f, defaultNeeds())

	// Long cooldown elapsed.
	f.E3_CooldownNorm = 0.95 // ~30 min since last action
	_, _, _, qStale, _ := ComputeDrives(f, defaultNeeds())

	if qFresh <= qStale {
		t.Errorf("fresh cooldown quiet(%.3f) should be > stale cooldown quiet(%.3f)", qFresh, qStale)
	}
}

func TestContextModulator_NilFeats(t *testing.T) {
	if contextModulator("speak_casual", nil) != 1.0 {
		t.Error("nil feats → 1.0")
	}
}

func TestContextModulator_Clamped(t *testing.T) {
	f := defaultFeats()
	f.A7_ActionSuccessRate = map[string]float64{"social": 0.0}
	f.R3_SourceAcceptRate = map[string]float64{"casual": 0.0}
	f.U10_TimeWindowPref = 0.0
	f.U8_EngagementNorm = 0.0
	f.U7_LengthTrend = -1.0
	m := contextModulator("speak_casual", f)
	if m < 0.1 || m > 1.5 {
		t.Errorf("modulator should be clamped to [0.1, 1.5], got %.3f", m)
	}
}

func TestActionMappings(t *testing.T) {
	// isSpeakAction includes all care actions.
	for _, a := range []string{"speak_casual", "speak_care", "speak_inquiry",
		"care_rest", "care_meal", "care_hydration", "care_health", "care_encourage", "care_social"} {
		if !isSpeakAction(a) {
			t.Errorf("isSpeakAction(%q) should be true", a)
		}
	}
	if isSpeakAction("observe") || isSpeakAction("reflect") {
		t.Error("non-speak should be false")
	}
	// actionToType checks.
	if ActionByName("speak_casual").OutcomeType != "social" || ActionByName("speak_care").OutcomeType != "encourage" {
		t.Error("actionToType speak")
	}
	if ActionByName("care_rest").OutcomeType != "rest" || ActionByName("care_meal").OutcomeType != "meal" {
		t.Error("actionToType care")
	}
	// actionToSource checks.
	if ActionByName("speak_casual").Source != "casual" || ActionByName("speak_care").Source != "care" {
		t.Error("actionToSource speak")
	}
	if ActionByName("care_rest").Source != "care" || ActionByName("care_meal").Source != "care" {
		t.Error("actionToSource care")
	}
}

func TestScoreActions_CareSuggestionBonus(t *testing.T) {
	m := NewMotivator()
	f := defaultFeats()
	// Boost care drive, suppress explore so care_rest can compete with reflect.
	f.A1_Worry = 0.7
	f.U4_ContinuousWorkNorm = 0.8
	f.E1_Hour = 1 // night → nightBonus → care boost
	s, c, cur, q, e := ComputeDrives(f, defaultNeeds())
	t.Logf("drives: s=%.3f c=%.3f cur=%.3f q=%.3f e=%.3f", s, c, cur, q, e)

	// Without suggestions.
	actionNoSugg, _, _ := m.ScoreActions(s, c, cur, q, e, f, nil)
	t.Logf("without suggestion: %s", actionNoSugg)

	// With a priority-1 rest suggestion: care_rest should win.
	suggestions := []domain.CareSuggestion{
		{Type: domain.TriggerRest, Priority: 1},
	}
	actionWithSugg, scoreWith, _ := m.ScoreActions(s, c, cur, q, e, f, suggestions)
	t.Logf("with rest suggestion: %s (score=%.3f)", actionWithSugg, scoreWith)

	if actionWithSugg != "care_rest" {
		t.Errorf("priority-1 rest suggestion should make care_rest win, got %q", actionWithSugg)
	}
}

func TestScoreActions_CareSuggestionLowPriorityLoses(t *testing.T) {
	m := NewMotivator()
	f := defaultFeats()
	f.A1_Loneliness = 0.8
	f.A1_Playfulness = 0.8
	f.U14_TimeSinceChatMins = 60
	s, c, cur, q, e := ComputeDrives(f, defaultNeeds())

	// Low-priority social suggestion vs high social drive — speak_casual should still win.
	suggestions := []domain.CareSuggestion{
		{Type: domain.TriggerSocial, Priority: 4},
	}
	action, _, _ := m.ScoreActions(s, c, cur, q, e, f, suggestions)
	// With high social drive, a low-priority care_social (bonus 0.10) shouldn't beat
	// speak_casual or other higher-scoring actions.
	if action == "care_social" {
		t.Error("low-priority social suggestion should not dominate")
	}
	t.Logf("low priority vs high drive: winner=%s", action)
}

func TestCareSuggestion_ActionName(t *testing.T) {
	tests := []struct {
		triggerType domain.CareTriggerType
		want        string
	}{
		{domain.TriggerRest, "care_rest"},
		{domain.TriggerMeal, "care_meal"},
		{domain.TriggerHydration, "care_hydration"},
		{domain.TriggerHealth, "care_health"},
		{domain.TriggerEncourage, "care_encourage"},
		{domain.TriggerSocial, "care_social"},
	}
	for _, tt := range tests {
		s := domain.CareSuggestion{Type: tt.triggerType}
		if s.ActionName() != tt.want {
			t.Errorf("%s → %q, want %q", tt.triggerType, s.ActionName(), tt.want)
		}
	}
}

func TestInteractionGate(t *testing.T) {
	if interactionGate(0.8) != 1.0 {
		t.Error("high rate→1.0")
	}
	if interactionGate(0) != 1.0 {
		t.Error("no data→1.0")
	}
	gate := interactionGate(0.25)
	if gate < 0.7 || gate > 0.8 {
		t.Errorf("0.25→~0.75, got %.3f", gate)
	}
}

// ---- Helpers ----

func assertRange(t *testing.T, name string, v, lo, hi float64) {
	t.Helper()
	if v < lo || v > hi {
		t.Errorf("%s = %.3f, want in [%.1f, %.1f]", name, v, lo, hi)
	}
}

func TestMotivator_TempFileCleanup(t *testing.T) {
	tmp := t.TempDir() + "/cleanup_test.json"
	m := NewMotivator()
	m.SetStoragePath(tmp)
	m.Save()
	if _, err := os.Stat(tmp); os.IsNotExist(err) {
		t.Error("weight file should exist after Save")
	}
}
