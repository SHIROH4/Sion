package domain

import "time"

// ThreadType classifies the purpose of a conversation thread.
type ThreadType string

const (
	ThreadFollowUp      ThreadType = "follow_up"
	ThreadExploration   ThreadType = "exploration"
	ThreadCare          ThreadType = "care"
	ThreadEntertainment ThreadType = "entertainment"
)

// ThreadStatus tracks the lifecycle of a conversation thread.
type ThreadStatus string

const (
	ThreadActive   ThreadStatus = "active"
	ThreadResolved ThreadStatus = "resolved"
	ThreadStale    ThreadStatus = "stale"
)

// ConversationThread is a persistent, cross-session conversation context.
// It gives proactive dialogue continuity — the AI remembers what it was
// talking about and can follow up naturally.
type ConversationThread struct {
	ID       int64        `json:"id"`
	Type     ThreadType   `json:"type"`
	Goal     string       `json:"goal"` // "跟进Rust学习进展"
	Status   ThreadStatus `json:"status"`
	Priority float64      `json:"priority"` // 0~1

	CreatedAt     int64 `json:"created_at"`
	LastTouchedAt int64 `json:"last_touched_at"`
	ResolvedAt    int64 `json:"resolved_at"`

	// Context linking.
	KeyMessages  string `json:"key_messages"`  // JSON: []string of key exchange summaries
	ContextRef   string `json:"context_ref"`   // comma-separated fact/inquiry IDs
	BestApproach string `json:"best_approach"` // how to naturally bring this up
	AvoidTopics  string `json:"avoid_topics"`  // comma-separated

	// Learning.
	Outcome   string `json:"outcome"`   // resolved / user_rejected / naturally_ended
	Learnings string `json:"learnings"` // what the AI learned from this thread
}

// ThreadRepository persists conversation threads.
type ThreadRepository interface {
	SaveThread(t ConversationThread) (int64, error)
	UpdateThread(t ConversationThread) error
	ListActive() ([]ConversationThread, error)
	ListByType(threadType ThreadType) ([]ConversationThread, error)
	TouchThread(id int64) error
	ResolveThread(id int64, outcome, learnings string) error
	MarkStale(id int64) error
	CleanResolved(days int) int
}

// ThreadSummary is a lightweight view of a thread for prompt injection.
type ThreadSummary struct {
	Goal         string
	BestApproach string
	LastTouched  time.Time
	Priority     float64
}

// SummarizeThreads extracts the most relevant info from active threads
// for prompt injection (limits total length).
func SummarizeThreads(threads []ConversationThread, limit int) []ThreadSummary {
	if len(threads) == 0 {
		return nil
	}
	summaries := make([]ThreadSummary, 0, len(threads))
	for i := range threads {
		t := &threads[i]
		if t.Status != ThreadActive {
			continue
		}
		summaries = append(summaries, ThreadSummary{
			Goal:         t.Goal,
			BestApproach: t.BestApproach,
			LastTouched:  time.Unix(t.LastTouchedAt, 0),
			Priority:     t.Priority,
		})
		if len(summaries) >= limit {
			break
		}
	}
	return summaries
}
