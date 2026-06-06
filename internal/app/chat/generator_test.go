package chat

import (
	"strings"
	"testing"

	"desktop-pet/internal/domain"
)

func TestBuildProactivePrompt(t *testing.T) {
	result := domain.SchedulerResult{
		ShouldAct:      true,
		Source:         domain.SourceCasual,
		Reason:         "主人刚完成工作",
		Score:          0.8,
		Escalation:     1,
		EmotionContext: "温暖自然",
		ContextAnchor:  "刚才在改代码",
	}
	prompt := BuildProactivePrompt(result, "我是诗音\n当前情绪: joy (愉悦度:0.8)", "", "在用VS Code", "", "", "", "", "", "", "")

	checks := []string{"诗音", "猫娘", "VS Code", "改代码", "温暖自然", "第一次提醒"}
	for _, c := range checks {
		if !strings.Contains(prompt, c) {
			t.Errorf("prompt missing %q", c)
		}
	}
}

func TestBuildProactivePrompt_Escalation(t *testing.T) {
	result := domain.SchedulerResult{Source: domain.SourceCare, Reason: "测试", EmotionContext: "自然"}
	prompt := BuildProactivePrompt(result, "", "", "", "", "", "", "", "", "", "")

	if !strings.Contains(prompt, "第一次提醒") {
		t.Errorf("escalation 0 should say '第一次提醒': %q", prompt)
	}

	result.Escalation = 3
	prompt = BuildProactivePrompt(result, "", "", "", "", "", "", "", "", "", "")
	if !strings.Contains(prompt, "傲娇") {
		t.Errorf("escalation 3 should mention 傲娇: %q", prompt)
	}
}

func TestBuildCareMessage_Fallback(t *testing.T) {
	gen := &Generator{}
	msg := gen.BuildCareMessage(domain.TriggerHydration, &domain.UserCareState{ContinuousWork: 120, StressLevel: 0.4, FocusLevel: 0.7}, nil, nil, "")
	if msg == "" {
		t.Error("fallback care message should not be empty")
	}
	if !strings.Contains(msg, "水") && !strings.Contains(msg, "喝") {
		t.Errorf("hydration message should mention drinking: %q", msg)
	}

	msg2 := gen.BuildCareMessage(domain.TriggerRest, &domain.UserCareState{ContinuousWork: 300, StressLevel: 0.8, FocusLevel: 0.9}, nil, nil, "")
	if msg2 == "" {
		t.Error("rest fallback should not be empty")
	}
}

func TestMoodToneGuide(t *testing.T) {
	vec := &domain.EmotionVector{Affection: 0.5, Annoyance: 0.85}
	sn := &domain.UserCareState{ContinuousWork: 300, StressLevel: 0.9}

	guide := MoodToneGuide(vec, sn, domain.TriggerRest)
	if guide == "" {
		t.Error("mood tone guide should not be empty")
	}

	// Nil vec should use default.
	guide2 := MoodToneGuide(nil, sn, domain.TriggerRest)
	if !strings.Contains(guide2, "强势") {
		t.Errorf("nil vec should use default tone: %q", guide2)
	}
}

func TestDefaultToneByCareType(t *testing.T) {
	tests := []struct {
		careType domain.CareTriggerType
		contains string
	}{
		{domain.TriggerRest, "坚决"},
		{domain.TriggerEncourage, "鼓励"},
		{domain.TriggerHydration, "俏皮"},
		{domain.TriggerMeal, "关心"},
		{domain.TriggerSocial, "好奇"},
		{domain.TriggerHealth, "健康"},
	}
	for _, tt := range tests {
		tone := DefaultToneByCareType(tt.careType)
		if !strings.Contains(tone, tt.contains) {
			t.Errorf("tone for %s missing %q: %q", tt.careType, tt.contains, tone)
		}
	}
}
