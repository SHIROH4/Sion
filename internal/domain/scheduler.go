package domain

// Scheduler is the System 1 safety gate for autonomous decisions.
// System 2 (LLM) makes the decision; the Scheduler validates it against
// hard constraints (cooldown, daily cap, rejection silence).
type Scheduler interface {
	// ValidateDecision checks safety constraints and commits state if passed.
	// Returns (ok, reason).
	ValidateDecision(input SchedulerInput, dec *DecisionOutput) (bool, string)

	// MarkReplied resets the escalation counter when the user replies.
	MarkReplied(source ProactiveSource)

	// DailyCount returns how many actions have been taken today.
	DailyCount() int
}
