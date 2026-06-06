package domain

// EmotionEvaluator manages the AI's emotional state using a multi-tier evaluation
// chain, tracking both a primary PAD emotion state and a multi-dimensional vector.
type EmotionEvaluator interface {
	// Current returns the EMA-smoothed current emotion state.
	Current() EmotionState

	// CurrentVector returns the EMA-smoothed current emotion vector.
	CurrentVector() EmotionVector

	// Evaluate runs the emotion evaluation chain on recent dialogue turns.
	Evaluate(recentTurns string) error

	// History returns a copy of recent emotion state history (up to 20 entries).
	History() []EmotionHistoryEntry
}
