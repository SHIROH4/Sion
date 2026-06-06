package domain

// SearchRepository manages keyword and unified (vector + keyword) search.
type SearchRepository interface {
	SearchArchives(keyword string, limit int) ([]SearchResult, error)
	UnifiedSearch(queryVector []float32, queryText string, topK int) ([]UnifiedResult, error)
}
