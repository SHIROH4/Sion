package emotion

import (
	"strings"
	"testing"
)

func TestEmotionModel_Evaluate(t *testing.T) {
	mockLLM := func(prompt string) (string, error) {
		return `{"valence": 0.8, "arousal": 0.6, "dominance": 0.4, "primary": "joy", "intensity": 0.7, "emotion_vector": {"affection": 0.7, "worry": 0.1, "curiosity": 0.5, "sleepiness": 0.2, "playfulness": 0.6, "loneliness": 0.1, "confidence": 0.8, "annoyance": 0.0}}`, nil
	}

	em := NewEmotionModel(mockLLM)
	em.ruleEval = nil

	if err := em.Evaluate("主人夸我代码写得好喵~"); err != nil {
		t.Fatal(err)
	}

	cur := em.Current()
	// After EMA smoothing, Valence should have moved upward from initial 0.25.
	if cur.Valence <= 0.2 {
		t.Errorf("Valence should increase from 0.25, got %.4f", cur.Valence)
	}
	// LLM's primary emotion is now preserved.
	if cur.Primary != "joy" {
		t.Errorf("expected Primary 'joy' (LLM judgment preserved), got %q", cur.Primary)
	}
}

func TestEmotionModel_EMA_Smoothing(t *testing.T) {
	// Mock that returns a constant high-valence state.
	mockLLM := func(prompt string) (string, error) {
		return `{"valence": 0.9, "arousal": 0.5, "dominance": 0.6, "primary": "joy", "intensity": 0.8, "emotion_vector": {"affection": 0.8, "worry": 0.0, "curiosity": 0.4, "sleepiness": 0.1, "playfulness": 0.7, "loneliness": 0.0, "confidence": 0.9, "annoyance": 0.0}}`, nil
	}

	em := NewEmotionModel(mockLLM)
	em.ruleEval = nil // test cloud LLM path directly

	// Run 5 evaluations. EMA should converge toward the target but never fully
	// reach it — smoothing resists sudden changes.
	var vals []float64
	for i := 0; i < 5; i++ {
		if err := em.Evaluate("开心！"); err != nil {
			t.Fatal(err)
		}
		vals = append(vals, em.Current().Valence)
	}

	// Each step should be monotonic (increasing toward 0.9).
	for i := 1; i < len(vals); i++ {
		if vals[i] <= vals[i-1] {
			t.Errorf("Valence should increase monotonically, step %d: %.4f → %.4f", i, vals[i-1], vals[i])
		}
	}

	// After 5 steps, should be close to but below target 0.9.
	final := vals[len(vals)-1]
	if final < 0.5 || final > 0.9 {
		t.Errorf("expected Valence 0.5~0.9 after 5 EMAs, got %.4f", final)
	}

	// History should contain 5 entries.
	if len(em.History()) != 5 {
		t.Errorf("expected 5 history entries, got %d", len(em.History()))
	}
}

func TestBuildEmotionPrompt(t *testing.T) {
	prompt := BuildEmotionPrompt("主人: 今天好累\n诗音: 主人辛苦了喵，早点休息吧~")

	checks := []string{
		"Kardia-R1",
		"情绪评估",
		"诗音",
		"最近对话",
		"主人: 今天好累",
		"诗音: 主人辛苦了喵",
		"valence",
		"arousal",
		"dominance",
		"primary",
		"intensity",
		"emotion_vector",
		"affection",
		"worry",
		"curiosity",
		"只输出 JSON",
	}

	for _, c := range checks {
		if !strings.Contains(prompt, c) {
			t.Errorf("prompt missing %q", c)
		}
	}
}

func TestEmotionModel_Evaluate_NilLLM(t *testing.T) {
	em := NewEmotionModel(nil)
	em.ruleEval = nil
	if err := em.Evaluate("anything"); err != nil {
		t.Errorf("nil llmEval should be no-op, got error: %v", err)
	}
	cur := em.Current()
	// Model starts with defaults (Valence 0.25). With nil LLM and nil ruleEval,
	// state should remain at initialization values.
	if cur.Valence != 0.25 || cur.Arousal != 0.1 || cur.Dominance != 0.15 {
		t.Errorf("state should remain at defaults, got V=%.2f A=%.2f D=%.2f",
			cur.Valence, cur.Arousal, cur.Dominance)
	}
}

func TestEmotionModel_Evaluate_ParseError(t *testing.T) {
	mockLLM := func(prompt string) (string, error) {
		return "not json", nil
	}
	em := NewEmotionModel(mockLLM)
	em.ruleEval = nil
	// Parse errors now degrade gracefully — no error returned, state unchanged.
	if err := em.Evaluate("test"); err != nil {
		t.Errorf("parse error should not cause Evaluate to fail, got: %v", err)
	}
}

func TestEmotionModel_History_Cap(t *testing.T) {
	mockLLM := func(prompt string) (string, error) {
		return `{"valence": 0.5, "arousal": 0.0, "dominance": 0.0, "primary": "neutral", "intensity": 0.5, "emotion_vector": {"affection": 0.5, "worry": 0.0, "curiosity": 0.2, "sleepiness": 0.1, "playfulness": 0.3, "loneliness": 0.1, "confidence": 0.6, "annoyance": 0.0}}`, nil
	}
	em := NewEmotionModel(mockLLM)
	em.ruleEval = nil

	for i := 0; i < 25; i++ {
		em.Evaluate("test")
	}

	if len(em.History()) != 25 {
		t.Errorf("expected 25 history entries, got %d (maxHistory=100)", len(em.History()))
	}
}

func TestParseEmotionJSON_CodeFence(t *testing.T) {
	raw := "```json\n{\"valence\": 0.5, \"arousal\": 0.3, \"dominance\": 0.1, \"primary\": \"joy\", \"intensity\": 0.6, \"emotion_vector\": {\"affection\": 0.5, \"worry\": 0.1, \"curiosity\": 0.3, \"sleepiness\": 0.0, \"playfulness\": 0.4, \"loneliness\": 0.0, \"confidence\": 0.7, \"annoyance\": 0.0}}\n```"
	s, _, err := parseEmotionJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if s.Valence != 0.5 {
		t.Errorf("expected Valence 0.5, got %.2f", s.Valence)
	}
	if s.Primary != "joy" {
		t.Errorf("expected Primary 'joy', got %q", s.Primary)
	}
}

func TestParseEmotionJSON_Clamp(t *testing.T) {
	raw := `{"valence": 1.5, "arousal": -2.0, "dominance": 0, "primary": "", "intensity": 1.5, "emotion_vector": {"affection": 1.5, "worry": -0.5, "curiosity": 2.0, "sleepiness": -1.0, "playfulness": 0.5, "loneliness": 0.5, "confidence": 0.5, "annoyance": 0.5}}`
	s, v, err := parseEmotionJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if s.Valence != 1.0 {
		t.Errorf("Valence should be clamped to 1.0, got %.2f", s.Valence)
	}
	if s.Arousal != -1.0 {
		t.Errorf("Arousal should be clamped to -1.0, got %.2f", s.Arousal)
	}
	if s.Intensity != 1.0 {
		t.Errorf("Intensity should be clamped to 1.0, got %.2f", s.Intensity)
	}
	if s.Primary != "neutral" {
		t.Errorf("empty Primary should default to 'neutral', got %q", s.Primary)
	}
	if v.Affection != 1.0 {
		t.Errorf("Affection should be clamped to 1.0, got %.2f", v.Affection)
	}
	if v.Worry != 0.0 {
		t.Errorf("Worry should be clamped to 0.0, got %.2f", v.Worry)
	}
	if v.Curiosity != 1.0 {
		t.Errorf("Curiosity should be clamped to 1.0, got %.2f", v.Curiosity)
	}
	if v.Sleepiness != 0.0 {
		t.Errorf("Sleepiness should be clamped to 0.0, got %.2f", v.Sleepiness)
	}
}

func TestEmotionVector_EMA_Smoothing(t *testing.T) {
	mockLLM := func(prompt string) (string, error) {
		return `{"valence": 0.8, "arousal": 0.5, "dominance": 0.5, "primary": "joy", "intensity": 0.7, "emotion_vector": {"affection": 0.9, "worry": 0.0, "curiosity": 0.7, "sleepiness": 0.1, "playfulness": 0.8, "loneliness": 0.0, "confidence": 0.9, "annoyance": 0.0}}`, nil
	}
	em := NewEmotionModel(mockLLM)
	em.ruleEval = nil // test cloud LLM path directly

	// First eval: all start at 0, EMA = 0.3*parsed.
	if err := em.Evaluate("主人夸我了喵~"); err != nil {
		t.Fatal(err)
	}
	v := em.CurrentVector()
	if v.Affection <= 0 || v.Affection > 0.5 {
		t.Errorf("Affection ~0.27 after first eval, got %.4f", v.Affection)
	}
	if v.Playfulness <= 0 || v.Playfulness > 0.5 {
		t.Errorf("Playfulness ~0.24 after first eval, got %.4f", v.Playfulness)
	}

	// Second eval: converges upward.
	if err := em.Evaluate("主人又夸我了！"); err != nil {
		t.Fatal(err)
	}
	v2 := em.CurrentVector()
	if v2.Affection <= v.Affection {
		t.Errorf("Affection should increase: %.4f → %.4f", v.Affection, v2.Affection)
	}
}

func TestEmotionVector_Defaults(t *testing.T) {
	em := NewEmotionModel(nil)
	v := em.CurrentVector()
	// Model starts with non-zero defaults (a warm but not over-the-top cat).
	if v.Affection != 0.45 || v.Playfulness != 0.3 || v.Confidence != 0.45 {
		t.Errorf("unexpected default vector: A=%.2f P=%.2f C=%.2f",
			v.Affection, v.Playfulness, v.Confidence)
	}
}

func TestEmotionVector_History_Cap(t *testing.T) {
	mockLLM := func(prompt string) (string, error) {
		return `{"valence": 0.5, "arousal": 0.0, "dominance": 0.0, "primary": "neutral", "intensity": 0.5, "emotion_vector": {"affection": 0.5, "worry": 0.0, "curiosity": 0.2, "sleepiness": 0.1, "playfulness": 0.3, "loneliness": 0.1, "confidence": 0.6, "annoyance": 0.0}}`, nil
	}
	em := NewEmotionModel(mockLLM)
	em.ruleEval = nil
	for i := 0; i < 25; i++ {
		em.Evaluate("test")
	}
	if len(em.VectorHistory()) != 25 {
		t.Errorf("expected 25 vector history entries, got %d (maxHistory=100)", len(em.VectorHistory()))
	}
}

func TestEmotionVector_AnnoyanceRising(t *testing.T) {
	mockLLM := func(prompt string) (string, error) {
		return `{"valence": -0.5, "arousal": 0.3, "dominance": -0.3, "primary": "anger", "intensity": 0.6, "emotion_vector": {"affection": 0.3, "worry": 0.1, "curiosity": 0.1, "sleepiness": 0.0, "playfulness": 0.0, "loneliness": 0.2, "confidence": 0.3, "annoyance": 0.7}}`, nil
	}
	em := NewEmotionModel(mockLLM)
	em.ruleEval = nil

	initVec := em.CurrentVector()
	em.Evaluate("主人说你只是个AI")
	v := em.CurrentVector()

	// Annoyance should increase from initial (0.02) toward target (0.7).
	if v.Annoyance <= initVec.Annoyance {
		t.Errorf("Annoyance should increase, was %.4f now %.4f", initVec.Annoyance, v.Annoyance)
	}
	// Confidence should decrease from initial (0.45) toward target (0.3).
	if v.Confidence >= initVec.Confidence {
		t.Errorf("Confidence should decrease, was %.4f now %.4f", initVec.Confidence, v.Confidence)
	}
}
