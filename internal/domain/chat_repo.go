package domain

// ChatHistoryRepository manages chat message persistence.
type ChatHistoryRepository interface {
	SaveHistory(messages []Message, metaLevel int) error
	CleanOldHistory(retentionDays int) error
	LoadHistory(limit int) ([]Message, error)
}
