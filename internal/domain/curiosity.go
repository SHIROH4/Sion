package domain

import "time"

// CuriosityItemType classifies a curiosity item.
type CuriosityItemType string

const (
	CuriosityEntry   CuriosityItemType = "entry"
	CuriosityGap     CuriosityItemType = "gap"
	CuriosityInquiry CuriosityItemType = "inquiry"
)

// CuriosityItem unifies CognitiveEntry, KnowledgeGap, and Inquiry into a single table.
// Different item_type values use different subsets of fields:
//
//	entry:    content, confidence, evidence, source, tags
//	gap:      content (the question), priority, status
//	inquiry:  content (the goal), confidence, priority, status, asked_at, resolved_at
type CuriosityItem struct {
	ID         int64             `json:"id"`
	ItemType   CuriosityItemType `json:"item_type"`
	Content    string            `json:"content"`
	Confidence float64           `json:"confidence"` // for entry/inquiry
	Priority   float64           `json:"priority"`   // for gap/inquiry
	Status     string            `json:"status"`     // active/asked/resolved/abandoned
	Source     string            `json:"source"`
	Evidence   string            `json:"evidence"`
	Tags       string            `json:"tags"`
	Embedding  []float32         `json:"-"`
	AskedAt    int64             `json:"asked_at"`
	ResolvedAt int64             `json:"resolved_at"`
	CreatedAt  int64             `json:"created_at"`
	UpdatedAt  int64             `json:"updated_at"`
}

// CuriosityRepository is the unified persistence interface for curiosity items.
type CuriosityRepository interface {
	// Save creates or updates a curiosity item. Returns the ID.
	Save(item CuriosityItem) (int64, error)

	// UpdateConfidence adjusts an entry/inquiry's confidence by delta.
	UpdateConfidence(id int64, delta float64) error

	// List returns items filtered by type and status, ordered by priority desc.
	List(itemType CuriosityItemType, status string, limit int) ([]CuriosityItem, error)

	// FindSimilar finds an entry with similar content (LIKE prefix match).
	FindSimilar(content string) (*CuriosityItem, error)

	// MarkStatus updates an item's status and timestamps.
	MarkStatus(id int64, status string) error

	// Cleanup removes old resolved/abandoned items.
	Cleanup(days int) int
}

// --- Helpers ---

// ConfidenceFromObservations computes initial confidence from evidence count.
func ConfidenceFromObservations(count int) float64 {
	switch {
	case count >= 5:
		return 0.8
	case count >= 3:
		return 0.65
	case count >= 2:
		return 0.55
	default:
		return 0.4
	}
}

// ConfidenceStatus returns the status label for a given confidence level.
func ConfidenceStatus(c float64) string {
	switch {
	case c >= 0.8:
		return "known"
	case c >= 0.3:
		return "hypothesis"
	default:
		return "outdated"
	}
}

// HasRecentActivity returns true if the given timestamp is within the window.
func HasRecentActivity(ts int64, window time.Duration) bool {
	return time.Since(time.Unix(ts, 0)) < window
}
