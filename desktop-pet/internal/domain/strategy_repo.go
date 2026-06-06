package domain

// StrategyPrincipleRepository persists and retrieves reusable behavioral principles
// learned from interaction outcomes.
type StrategyPrincipleRepository interface {
	// SavePrinciple inserts or updates a strategy principle.
	SavePrinciple(p StrategyPrinciple) (int64, error)

	// ListActive returns all active principles.
	ListActive() ([]StrategyPrinciple, error)

	// FindByTags returns active principles matching any of the given tags.
	FindByTags(tags []string, limit int) ([]StrategyPrinciple, error)

	// SearchSimilar returns principles whose situation embedding is close to the query.
	SearchSimilar(queryVec []float32, limit int) ([]StrategyPrinciple, error)

	// Deactivate marks a principle as inactive.
	Deactivate(id int64) error

	// CleanInactive removes inactive principles older than the given number of days.
	CleanInactive(days int) (removed int)

	// Count returns the count of active principles.
	Count() int
}
