package domain

// MemoryStore is the composed persistence interface for the core memory system.
// It bundles the three repository interfaces that the SQLite Store implements
// directly (facts + chat history + profile), plus lifecycle methods.
//
// For diary, episode, topics, and identity use
// the individual repository interfaces (DiaryRepository, EpisodeRepository,
// TopicRepository, IdentityRepository, QAQuestionRepository).
type MemoryStore interface {
	FactRepository
	MemCellRepository
	ArchiveRepository
	SearchRepository
	ProfileRepository
	ChatHistoryRepository

	// Lifecycle.
	Close() error

	// EmbedSvc returns the embedding service for vector operations.
	EmbedSvc() Vectorizer

	// SetEmbedSvc injects the embedding service.
	SetEmbedSvc(svc Vectorizer)
}

// IdentityNode is a single node in the identity knowledge graph.
type IdentityNode struct {
	ID          int64            `json:"id"`
	Type        IdentityNodeType `json:"type"`
	Content     string           `json:"content"`
	Confidence  float64          `json:"confidence"`
	Embedding   []float32        `json:"-"`
	CreatedAt   int64            `json:"created_at"`
	UpdatedAt   int64            `json:"updated_at"`
	LastMatched int64            `json:"last_matched"`
	MatchCount  int              `json:"match_count"`
	Active      bool             `json:"active"`
}

// IdentityNodeType defines the category of an identity graph node.
type IdentityNodeType string

const (
	NodeCoreValue    IdentityNodeType = "core_value"
	NodePreference   IdentityNodeType = "preference"
	NodeBelief       IdentityNodeType = "belief"
	NodeRelationship IdentityNodeType = "relationship"
	NodeBehaviorRule IdentityNodeType = "behavior_rule"
	NodeGoal         IdentityNodeType = "goal"
	NodeFear         IdentityNodeType = "fear"
)
