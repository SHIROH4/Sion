package domain

import (
	"math"
	"testing"
)

func TestNeedSatisfactionForAction_SpeakCasual(t *testing.T) {
	s := NeedSatisfactionForAction("speak_casual", OutcomeIgnored)
	if s.Play >= 0 {
		t.Errorf("speak_casual should reduce play need: %.2f", s.Play)
	}
	if s.Companionship >= 0 {
		t.Errorf("speak_casual should reduce companionship: %.2f", s.Companionship)
	}
	if s.Rest <= 0 {
		t.Error("speak_casual should increase rest need (costs energy)")
	}
}

func TestNeedSatisfactionForAction_SpeakCare(t *testing.T) {
	s := NeedSatisfactionForAction("speak_care", OutcomeIgnored)
	if s.Care >= 0 {
		t.Errorf("speak_care should reduce care need: %.2f", s.Care)
	}
}

func TestNeedSatisfactionForAction_Reflect(t *testing.T) {
	s := NeedSatisfactionForAction("reflect", OutcomeIgnored)
	if s.Autonomy >= 0 {
		t.Errorf("reflect should reduce autonomy need: %.2f", s.Autonomy)
	}
}

func TestNeedSatisfactionForAction_None(t *testing.T) {
	s := NeedSatisfactionForAction("none", OutcomeIgnored)
	if s.Rest >= 0 {
		t.Errorf("none should reduce rest need: %.2f", s.Rest)
	}
}

func TestNeedSatisfactionForAction_OutcomeEngaged(t *testing.T) {
	base := NeedSatisfactionForAction("speak_casual", OutcomeIgnored)
	engaged := NeedSatisfactionForAction("speak_casual", OutcomeEngaged)
	if engaged.Companionship >= base.Companionship {
		t.Error("engaged should satisfy companionship more than ignored")
	}
}

func TestNeedSatisfactionForAction_OutcomeRejected(t *testing.T) {
	base := NeedSatisfactionForAction("speak_care", OutcomeIgnored)
	rejected := NeedSatisfactionForAction("speak_care", OutcomeRejected)
	if rejected.Companionship <= base.Companionship {
		t.Error("rejection should increase companionship need vs ignored")
	}
}

func TestNeedModulation_Defaults(t *testing.T) {
	n := IntrinsicNeeds{
		Companionship: 0.3, Rest: 0.2, Play: 0.3,
		Curiosity: 0.4, Care: 0.3, Autonomy: 0.3,
	}
	mod := n.NeedModulation()

	// Default needs → modulation near neutral.
	if mod.LonelinessDecayMul <= 0.5 || mod.LonelinessDecayMul > 1.0 {
		t.Errorf("default LonelinessDecayMul=%.3f", mod.LonelinessDecayMul)
	}
	if mod.SleepinessGrowthMul < 1.0 || mod.SleepinessGrowthMul > 1.5 {
		t.Errorf("default SleepinessGrowthMul=%.3f", mod.SleepinessGrowthMul)
	}
}

func TestNeedModulation_MaxNeeds(t *testing.T) {
	n := IntrinsicNeeds{
		Companionship: 1.0, Rest: 1.0, Play: 1.0,
		Curiosity: 1.0, Care: 1.0, Autonomy: 1.0,
	}
	mod := n.NeedModulation()

	if math.Abs(mod.LonelinessDecayMul-0.5) > 0.01 {
		t.Errorf("max comp: LonelinessDecayMul=%.3f, want 0.5", mod.LonelinessDecayMul)
	}
	if math.Abs(mod.SleepinessGrowthMul-1.5) > 0.01 {
		t.Errorf("max rest: SleepinessGrowthMul=%.3f, want 1.5", mod.SleepinessGrowthMul)
	}
	if math.Abs(mod.ConfidenceDecayMul-1.3) > 0.01 {
		t.Errorf("max autonomy: ConfidenceDecayMul=%.3f, want 1.3", mod.ConfidenceDecayMul)
	}
}

func TestNeedModulation_MinNeeds(t *testing.T) {
	n := IntrinsicNeeds{} // all zero
	mod := n.NeedModulation()

	if math.Abs(mod.LonelinessDecayMul-1.0) > 0.01 {
		t.Errorf("min comp: LonelinessDecayMul=%.3f, want 1.0", mod.LonelinessDecayMul)
	}
	if math.Abs(mod.SleepinessGrowthMul-1.0) > 0.01 {
		t.Errorf("min rest: SleepinessGrowthMul=%.3f, want 1.0", mod.SleepinessGrowthMul)
	}
}

func TestIntrinsicNeeds_AllFieldsInRange(t *testing.T) {
	n := IntrinsicNeeds{}
	if n.Companionship != 0 || n.Rest != 0 || n.Play != 0 ||
		n.Curiosity != 0 || n.Care != 0 || n.Autonomy != 0 {
		t.Error("zero-value needs should be all zero")
	}
}
