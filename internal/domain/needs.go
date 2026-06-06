package domain

// IntrinsicNeeds represents the agent's 6-dimensional internal needs.
// Values range 0~1 and grow over time. They modulate emotion sensitivity
// rather than directly driving behavior — high needs make related emotions
// more responsive, which in turn affects drive computation.
type IntrinsicNeeds struct {
	// Companionship — desire for social connection with the user.
	// +0.05/30min naturally. Satisfied by user interaction (-0.3) and replies (-0.5).
	Companionship float64 `json:"companionship"`

	// Rest — need for quiet/downtime. Grows faster at night.
	// +0.08/h naturally. Satisfied by choosing "none" (-0.2).
	Rest float64 `json:"rest"`

	// Play — desire for fun/playful interaction.
	// +0.03/30min. Satisfied by speak_casual (-0.3). Resets to 0 at night.
	Play float64 `json:"play"`

	// Curiosity — drive to learn and explore.
	// +0.04/h. Satisfied by discovering info (-0.4) or observe (-0.3).
	Curiosity float64 `json:"curiosity"`

	// Care — urge to look after the user's wellbeing.
	// +0.06/h. Satisfied by speak_care (-0.5) and user positive reply (-0.2).
	Care float64 `json:"care"`

	// Autonomy — need for self-directed action and reflection.
	// +0.02/h. Satisfied by reflect (-0.6) or analyze_patterns (-0.5).
	Autonomy float64 `json:"autonomy"`

	// LastUpdated is the unix timestamp of the last growth tick.
	LastUpdated int64 `json:"last_updated"`
}

// NeedSatisfaction describes how much an action satisfies each need.
// Positive values reduce the need (satisfaction), negative values increase it.
type NeedSatisfaction struct {
	Companionship float64 // e.g. -0.3 (reduce need by 0.3)
	Rest          float64
	Play          float64
	Curiosity     float64
	Care          float64
	Autonomy      float64
}

// NeedSatisfactionForAction returns the satisfaction for a given action + outcome.
// Values scaled to overcome passive decay (~0.03/h) + active growth (~0.03/h).
// A single action should reduce the relevant need by ~0.20-0.40.
func NeedSatisfactionForAction(action string, outcome OutcomeResult) NeedSatisfaction {
	s := NeedSatisfaction{}

	switch action {
	case "speak_casual":
		s.Companionship = -0.25
		s.Rest = 0.05
		s.Play = -0.30

	case "speak_care":
		s.Care = -0.40
		s.Rest = 0.05
		s.Companionship = -0.15

	case "speak_inquiry":
		s.Curiosity = -0.30
		s.Rest = 0.03
		s.Companionship = -0.10

	case "observe":
		s.Curiosity = -0.35

	case "reflect":
		s.Autonomy = -0.50
		s.Curiosity = -0.15

	case "analyze_patterns":
		s.Autonomy = -0.40
		s.Curiosity = -0.10

	case "none":
		s.Rest = -0.20 // passive relaxation, slower decay
	}

	// Outcome bonus: stronger engagement = more satisfaction.
	switch outcome {
	case OutcomeEngaged:
		s.Companionship -= 0.20
		s.Care -= 0.15
	case OutcomeReplied:
		s.Companionship -= 0.10
	case OutcomeRejected:
		s.Companionship += 0.15 // stronger negative feedback
		s.Care += 0.10
	}

	return s
}

// NeedModulation computes how needs modulate emotion decay/growth rates.
// Returns multipliers: >1 means faster change, <1 means slower change.
func (n *IntrinsicNeeds) NeedModulation() NeedModulation {
	return NeedModulation{
		// High companionship → loneliness decays slower (loneliness persists).
		LonelinessDecayMul:  1.0 - clamp01(n.Companionship)*0.5,
		// High rest → sleepiness grows faster.
		SleepinessGrowthMul: 1.0 + clamp01(n.Rest)*0.5,
		// High play → playfulness decays slower.
		PlayfulnessDecayMul: 1.0 - clamp01(n.Play)*0.5,
		// High curiosity → curiosity decays slower.
		CuriosityDecayMul:   1.0 - clamp01(n.Curiosity)*0.4,
		// High care → worry decays slower.
		WorryDecayMul:       1.0 - clamp01(n.Care)*0.4,
		// High autonomy → confidence decays slightly faster (restlessness).
		ConfidenceDecayMul:  1.0 + clamp01(n.Autonomy)*0.3,
	}
}

// NeedModulation holds multipliers for emotion decay/growth rates.
type NeedModulation struct {
	LonelinessDecayMul   float64
	SleepinessGrowthMul  float64
	PlayfulnessDecayMul  float64
	CuriosityDecayMul    float64
	WorryDecayMul        float64
	ConfidenceDecayMul   float64
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
