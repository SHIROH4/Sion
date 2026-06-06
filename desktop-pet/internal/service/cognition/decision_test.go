package cognition

import (
	"strings"
	"testing"
	"time"

	"desktop-pet/internal/domain"
)

func TestBuildFallbackPrompt_WithFullContext(t *testing.T) {
	e := NewDecisionEngine(nil)

	ctx := domain.DecisionContext{
		Now:               time.Date(2026, 6, 5, 14, 30, 0, 0, time.Local),
		EmotionVec:        domain.EmotionVector{Affection: 0.6, Worry: 0.4, Curiosity: 0.5, Sleepiness: 0.2, Playfulness: 0.5, Loneliness: 0.3, Confidence: 0.7, Annoyance: 0.1},
		EmotionState:      domain.EmotionState{Primary: "neutral", Intensity: 0.5, Valence: 0.3},
		TimeSinceLastChat: 10 * time.Minute,
		DailyActionCount:  3,
		ActivePrinciples: []domain.StrategyPrinciple{
			{Situation: "主人在深夜写代码", GoodStrategy: "用傲娇语气催睡"},
		},
		RecentOutcomes: []domain.ActionOutcome{
			{ActionSource: "care", ActionType: "rest", Outcome: 1},
			{ActionSource: "casual", ActionType: "social", Outcome: -1},
		},
		TacticalDirectives: []string{"关注主人作息"},
	}

	feats := &domain.QuantifiedFeatures{
		U1_AppCategory:       "work",
		U2_WindowSubtype:     "debugging",
		U3_IsWorking:         1.0,
		U4_ContinuousWorkMins: 107,
		U5_AppSwitchCount:    2,
		U7_LengthTrend:       -0.4,
		U8_EngagementNorm:    0.72,
		U10_TimeWindowPref:   0.70,
		U11_MealTime:         0.5,
		U12_NightTime:        0,
		U13_IsWeekend:        0,
		E1_Hour:              14,
		E4_QuotaRemaining:    17,
		A6_DailyActionCount:  3,
		A7_ActionSuccessRate: map[string]float64{"rest": 0.67, "meal": 1.0, "social": 0.5},
		R1_OverallAcceptRate: 0.70,
		R1_SampleCount:       10,
		R4_RecentRejections:  1,
		R4_RejectionSeverity: 0.33,
		R5_NeglectHours:      0.1,
		R6_DepthTrend:        0.0,
		T1_PrincipleCount:    4,
		T3_ReflexionLogCount: 5,
	}

	needs := &domain.IntrinsicNeeds{
		Companionship: 0.6, Care: 0.8, Play: 0.3,
		Curiosity: 0.4, Rest: 0.2, Autonomy: 0.3,
	}

	prompt := e.buildFallbackPrompt(ctx, feats, needs)

	// Verify all 4 structured blocks are present.
	blocks := []string{
		"[主人状态]",
		"[互动关系]",
		"[你的状态]",
		"[历史经验]",
	}
	for _, block := range blocks {
		if !strings.Contains(prompt, block) {
			t.Errorf("missing block: %q", block)
		}
	}

	// Verify key data points appear.
	checks := []string{
		"work",
		"debugging",
		"工作中",
		"107分钟",
		"2次",
		"饭点",
		"70%",
		"10条",
		"1次",
		"0.33",
		"变短",
		"内在需求",
		"陪伴60%",
		"关怀80%(高!)",
		"玩耍30%",
		"休息20%",
		"自主30%",
		"rest成功率: 67%",
		"meal成功率: 100%",
		"14h",
		"可用策略: 4条",
		"反思记忆: 5条",
		"从经验中学到的策略",
		"傲娇语气催睡",
		"最近行动结果",
		"可选: speak",
		"输出JSON",
	}
	for _, check := range checks {
		if !strings.Contains(prompt, check) {
			t.Errorf("missing content: %q", check)
		}
	}

	// Verify no nil pointer panics for nil feats/needs (degraded mode).
	t.Run("nil feats and needs", func(t *testing.T) {
		prompt2 := e.buildFallbackPrompt(ctx, nil, nil)
		if !strings.Contains(prompt2, "[主人状态]") {
			t.Error("should have user state block even with nil feats")
		}
		if strings.Contains(prompt2, "[互动关系]") {
			t.Error("should NOT have relationship block with nil feats")
		}
		if !strings.Contains(prompt2, "[你的状态]") {
			t.Error("should have agent state block even with nil feats")
		}
		if strings.Contains(prompt2, "[历史经验]") {
			t.Error("should NOT have history block with nil feats")
		}
	})

	// Verify nil needs doesn't crash.
	t.Run("nil needs only", func(t *testing.T) {
		prompt3 := e.buildFallbackPrompt(ctx, feats, nil)
		if strings.Contains(prompt3, "内在需求") {
			t.Error("should NOT have needs section with nil needs")
		}
	})

	t.Logf("\n=== Generated Prompt ===\n%s\n=== End Prompt ===", prompt)
}

func TestBuildFallbackPrompt_NoRecentOutcomes(t *testing.T) {
	e := NewDecisionEngine(nil)
	ctx := domain.DecisionContext{
		Now:          time.Now(),
		EmotionVec:   domain.EmotionVector{Affection: 0.5, Worry: 0.3, Curiosity: 0.4, Sleepiness: 0.1, Playfulness: 0.4, Loneliness: 0.2, Confidence: 0.6, Annoyance: 0.1},
		EmotionState: domain.EmotionState{Primary: "neutral", Intensity: 0.3},
	}
	prompt := e.buildFallbackPrompt(ctx, nil, nil)
	if !strings.Contains(prompt, "输出JSON") {
		t.Error("prompt should end with JSON instruction")
	}
}

func TestBuildFallbackPrompt_ReflexionLog(t *testing.T) {
	e := NewDecisionEngine(nil)
	e.reflexionLog = []reflexionEntry{
		{contextSummary: "深夜关心", outcome: "accepted", at: time.Now()},
		{contextSummary: "午间闲聊", outcome: "ignored", at: time.Now()},
	}
	ctx := domain.DecisionContext{
		Now:          time.Now(),
		EmotionVec:   domain.EmotionVector{Affection: 0.5, Worry: 0.3, Curiosity: 0.4, Sleepiness: 0.1, Playfulness: 0.4, Loneliness: 0.2, Confidence: 0.6, Annoyance: 0.1},
		EmotionState: domain.EmotionState{Primary: "neutral", Intensity: 0.3},
	}
	prompt := e.buildFallbackPrompt(ctx, nil, nil)
	if !strings.Contains(prompt, "反思记忆") {
		t.Error("should contain reflexion memory")
	}
	if !strings.Contains(prompt, "深夜关心") {
		t.Error("should contain reflexion entry")
	}
}

// TestDecide_LLMFallback_FullContext triggers the complete LLM fallback branch
// through Decide(), capturing the prompt sent to the mock LLM and verifying
// every structured block and data field is present without omission.
func TestDecide_LLMFallback_FullContext(t *testing.T) {
	var capturedPrompt string
	mockLLM := func(msgs []domain.Message) (string, error) {
		if len(msgs) > 0 {
			capturedPrompt = msgs[0].Content
		}
		// Return valid JSON so Decide completes the full flow.
		return `{"action":"speak","source":"care","reason":"主人连续工作107分钟需要休息","mood":"worried","priority":0.85}`, nil
	}

	e := NewDecisionEngine(mockLLM)

	ctx := domain.DecisionContext{
		Now:               time.Date(2026, 6, 5, 14, 30, 0, 0, time.Local),
		EmotionVec:        domain.EmotionVector{Affection: 0.6, Worry: 0.4, Curiosity: 0.5, Sleepiness: 0.2, Playfulness: 0.5, Loneliness: 0.3, Confidence: 0.7, Annoyance: 0.1},
		EmotionState:      domain.EmotionState{Primary: "neutral", Intensity: 0.5, Valence: 0.3},
		TimeSinceLastChat: 10 * time.Minute,
		DailyActionCount:  3,
		ActivePrinciples: []domain.StrategyPrinciple{
			{Situation: "主人在深夜写代码", GoodStrategy: "用傲娇语气催睡"},
		},
		RecentOutcomes: []domain.ActionOutcome{
			{ActionSource: "care", ActionType: "rest", Outcome: 1},
			{ActionSource: "casual", ActionType: "social", Outcome: -1},
		},
		ActiveInquiries:   2,
		KnowledgeGaps:     1,
		ScreenSummary:     "Code",
		RecentFactSample:  []string{"主人喜欢Go", "主人最近在学Rust"},
		TacticalDirectives: []string{"关注主人作息"},
	}

	feats := &domain.QuantifiedFeatures{
		U1_AppCategory:        "work",
		U2_WindowSubtype:      "debugging",
		U3_IsWorking:          1.0,
		U4_ContinuousWorkMins: 107,
		U5_AppSwitchCount:     2,
		U7_LengthTrend:        -0.4,
		U8_EngagementNorm:     0.72,
		U8_ResponseDelayEMA:   33.6,
		U10_TimeWindowPref:    0.70,
		U11_MealTime:          0.5,
		U12_NightTime:         0,
		U13_IsWeekend:         0,
		U14_TimeSinceChatMins: 10,
		U15_FatigueMentionHrs: 1.0,
		U15_FatigueMentionNorm: 0.25,
		U16_PrefDiversity:     0.66,
		A1_Affection:          0.6,
		A1_Worry:              0.4,
		A1_Curiosity:          0.5,
		A1_Sleepiness:         0.2,
		A1_Playfulness:        0.5,
		A1_Loneliness:         0.3,
		A1_Confidence:         0.7,
		A1_Annoyance:          0.1,
		A4_ValenceTrend:       0.1,
		A5_AnnoySensitivity:   0.5,
		A5_AffectWarmth:       0.6,
		A5_WorryTendency:      0.4,
		A6_DailyActionCount:   3,
		A7_ActionSuccessRate:  map[string]float64{"rest": 0.67, "meal": 1.0, "social": 0.5},
		A8_TimeBlockRate:      map[int]float64{2: 0.70},
		A10_ActiveGoals:       2,
		A10_ActiveGoalsNorm:   0.4,
		A11_ActiveInquiries:   2,
		A12_KnowledgeGaps:     1,
		A13_NewFacts24h:       3,
		A14_ConsecutiveCount:  1,
		E1_Hour:               14,
		E2_DayOfWeek:          5,
		E3_CooldownNorm:       0.8,
		E4_QuotaRemaining:     17,
		E7_ReflectionDue:      0.3,
		E6_LLMAvailable:       true,
		E6_VisionAvailable:    true,
		R1_OverallAcceptRate:  0.70,
		R1_SampleCount:        10,
		R2_TimeWindowAccept:   0.70,
		R3_SourceAcceptRate:   map[string]float64{"care": 0.8, "casual": 0.5},
		R4_RecentRejections:   1,
		R4_RejectionSeverity:  0.33,
		R5_NeglectHours:       0.1,
		R5_NeglectNorm:        0.004,
		R6_DepthTrend:         0.0,
		R7_UserInitiative24h:  5,
		R7_UserInitiativeNorm: 0.5,
		R8_IntimacyTrend:      0.05,
		T1_PrincipleCount:     4,
		T2_PatternCount:       2,
		T3_ReflexionLogCount:  5,
		T5_TodayActivityCount: 12,
	}

	needs := &domain.IntrinsicNeeds{
		Companionship: 0.6, Care: 0.8, Play: 0.3,
		Curiosity: 0.4, Rest: 0.2, Autonomy: 0.3,
	}

	// ---- Trigger Decide ----
	dec, err := e.Decide(ctx, feats, needs)
	if err != nil {
		t.Fatalf("Decide failed: %v", err)
	}
	if dec == nil {
		t.Fatal("expected non-nil DecisionOutput")
	}
	if dec.Action != "speak" {
		t.Errorf("action = %q, want speak", dec.Action)
	}
	if dec.Source != "care" {
		t.Errorf("source = %q, want care", dec.Source)
	}
	if dec.Mood != "worried" {
		t.Errorf("mood = %q, want worried", dec.Mood)
	}

	// ---- Verify prompt has all 4 structured blocks ----
	blocks := []string{"[主人状态]", "[互动关系]", "[你的状态]", "[历史经验]"}
	for _, b := range blocks {
		if !strings.Contains(capturedPrompt, b) {
			t.Errorf("MISSING BLOCK: %q", b)
		}
	}

	// ---- Verify every data field appears in the prompt ----
	type fieldCheck struct {
		value  string
		label  string // human-readable name for error messages
	}
	checks := []fieldCheck{
		// [主人状态]
		{"work", "U1_AppCategory"},
		{"工作中", "U3_IsWorking"},
		{"debugging", "U2_WindowSubtype"},
		{"107分钟", "U4_ContinuousWorkMins"},
		{"2次", "U5_AppSwitchCount"},
		{"饭点", "U11_MealTime"},
		// [互动关系]
		{"70%", "R1_OverallAcceptRate"},
		{"10条", "R1_SampleCount"},
		{"1次", "R4_RecentRejections"},
		{"0.33", "R4_RejectionSeverity"},
		{"变短", "U7_LengthTrend (negative)"},
		// [你的状态]
		{"主情绪", "A2_PrimaryEmotion"},
		{"强度50%", "A3_Intensity"},
		{"情感60%", "A1_Affection"},
		{"担忧40%", "A1_Worry"},
		{"好奇50%", "A1_Curiosity"},
		{"困倦20%", "A1_Sleepiness"},
		{"贪玩50%", "A1_Playfulness"},
		{"寂寞30%", "A1_Loneliness"},
		{"烦躁10%", "A1_Annoyance"},
		{"内在需求", "Needs section"},
		{"陪伴60%", "N_Companionship"},
		{"关怀80%(高!)", "N_Care (high)"},
		{"玩耍30%", "N_Play"},
		{"好奇40%", "N_Curiosity"},
		{"休息20%", "N_Rest"},
		{"自主30%", "N_Autonomy"},
		{"配额剩余17", "E4_QuotaRemaining"},
		// [历史经验]
		{"rest成功率: 67%", "A7 rest rate"},
		{"meal成功率: 100%", "A7 meal rate"},
		{"social成功率: 50%", "A7 social rate"},
		{"14h", "U10_TimeWindowPref hour"},
		{"可用策略: 4条", "T1_PrincipleCount"},
		{"反思记忆: 5条", "T3_ReflexionLogCount"},
		// Legacy sections
		{"从经验中学到的策略", "ActivePrinciples"},
		{"傲娇语气催睡", "Strategy content"},
		{"最近行动结果", "RecentOutcomes"},
		{"care/rest✓", "Outcome 1"},
		{"casual/social✗", "Outcome -1"},
		// Footer
		{"可选: speak", "Action options"},
		{"输出JSON", "JSON instruction"},
	}

	missing := 0
	for _, c := range checks {
		if !strings.Contains(capturedPrompt, c.value) {
			t.Errorf("MISSING %s: %q not found in prompt", c.label, c.value)
			missing++
		}
	}

	if missing > 0 {
		t.Errorf("%d field(s) missing from LLM fallback prompt", missing)
	}

	// Print the full prompt for manual inspection.
	t.Logf("\n========== LLM FALLBACK PROMPT (%d bytes) ==========\n%s\n========== END PROMPT ==========",
		len(capturedPrompt), capturedPrompt)
}

func TestBuildFallbackPrompt_WeekendAndNight(t *testing.T) {
	e := NewDecisionEngine(nil)
	ctx := domain.DecisionContext{
		Now:          time.Date(2026, 6, 7, 1, 0, 0, 0, time.Local), // Sunday 1am
		EmotionVec:   domain.EmotionVector{Affection: 0.5, Worry: 0.3, Curiosity: 0.4, Sleepiness: 0.1, Playfulness: 0.4, Loneliness: 0.2, Confidence: 0.6, Annoyance: 0.1},
		EmotionState: domain.EmotionState{Primary: "neutral", Intensity: 0.3},
	}
	feats := &domain.QuantifiedFeatures{
		U12_NightTime: 0.6,
		U13_IsWeekend: 1.0,
		E1_Hour:       1,
	}
	prompt := e.buildFallbackPrompt(ctx, feats, nil)
	if !strings.Contains(prompt, "深夜") {
		t.Error("should contain 深夜 tag")
	}
	if !strings.Contains(prompt, "周末") {
		t.Error("should contain 周末 tag")
	}
}
