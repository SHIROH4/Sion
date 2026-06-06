package domain

import "time"

// CareSuggestion is a non-destructive care trigger evaluation result,
// fed into the System 2 decision pipeline as an action score bonus.
type CareSuggestion struct {
	Type     CareTriggerType // rest, meal, hydration, health, encourage, social
	Priority int             // 1 (urgent) ~ 4 (optional)
	Reason   string          // human-readable trigger reason
}

// CareActionName returns the System 2 action name for a care trigger type.
func (s CareSuggestion) ActionName() string {
	switch s.Type {
	case TriggerRest:
		return "care_rest"
	case TriggerMeal:
		return "care_meal"
	case TriggerHydration:
		return "care_hydration"
	case TriggerHealth:
		return "care_health"
	case TriggerEncourage:
		return "care_encourage"
	case TriggerSocial:
		return "care_social"
	default:
		return ""
	}
}

// CareProvider manages proactive care: state tracking, trigger evaluation,
// action execution, and feedback adaptation.
type CareProvider interface {
	// State returns the underlying UserCareState pointer.
	State() *UserCareState

	// UpdateState updates the care state from an observation.
	UpdateState(obs Observation)

	// UpdateStress sets the user's stress level (0-1), derived from emotion model.
	UpdateStress(stress float64)

	// TickIsolation increments the user's isolation counter.
	TickIsolation(interval time.Duration)

	// ShouldPoke decides whether it's a good time to initiate care.
	ShouldPoke(now time.Time) bool

	// Poke evaluates triggers and, if any are ready, generates a message.
	// Returns the action that was fired, or nil if nothing to do.
	Poke(now time.Time) (*CareAction, error)

	// Suggestions evaluates all care triggers non-destructively and returns
	// triggered suggestions for the System 2 decision pipeline.
	Suggestions(now time.Time) []CareSuggestion

	// RecordResponse records the user's feedback to a care action.
	RecordResponse(actionID int64, accepted bool, response string)

	// SaveState serialises care state to JSON for persistence.
	SaveState() ([]byte, error)

	// LoadState restores care state from persisted JSON.
	LoadState(data []byte) error

	// ActionLog returns the most recent n care action records.
	ActionLog(n int) []CareAction

	// IncrementWork increments the user's continuous work counter by seconds.
	IncrementWork(seconds int)

	// ResetWork zeroes the user's continuous work counter.
	ResetWork()
}
