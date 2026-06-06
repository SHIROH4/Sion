package domain

// IdentityRepository manages the identity knowledge graph.
type IdentityRepository interface {
	ListActiveIdentityNodes() ([]IdentityNode, error)
	UpsertIdentityNode(node *IdentityNode) error
	DeactivateIdentityNode(id int64) error
	UpdateIdentityNodeMatchStats(id int64, lastMatched int64, matchCount int) error
}
