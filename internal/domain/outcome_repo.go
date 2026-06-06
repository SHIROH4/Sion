package domain

// ActionOutcomeRepository persists proactive action outcomes for adaptive learning.
type ActionOutcomeRepository interface {
	// SaveOutcome records the result of a proactive action.
	SaveOutcome(o ActionOutcome) error

	// QuerySimilarContexts returns outcomes in similar contexts, ordered by recency.
	QuerySimilarContexts(ctx ActionContext, limit int) ([]ActionOutcome, error)

	// RecentOutcomes returns the last N outcomes for a given source.
	RecentOutcomes(source ProactiveSource, limit int) ([]ActionOutcome, error)

	// SuccessRate returns the acceptance rate for a given context bucket.
	SuccessRate(ctx ActionContext, windowDays int) (accepts int, total int)

	// CleanOldOutcomes removes outcomes older than the given number of days.
	CleanOldOutcomes(days int) (removed int)

	// SuccessRateByType returns acceptance rate per action type within the window.
	SuccessRateByType(windowDays int) (map[CareTriggerType]float64, error)

	// SuccessRateByTimeBlock returns acceptance rate per 4 time blocks:
	//   0 = late_night (0-5), 1 = morning (6-11), 2 = afternoon (12-17), 3 = evening (18-23).
	SuccessRateByTimeBlock(windowDays int) (map[int]float64, error)

	// SuccessRateBySource returns acceptance rate per proactive source within the window.
	SuccessRateBySource(windowDays int) (map[ProactiveSource]float64, error)
}
