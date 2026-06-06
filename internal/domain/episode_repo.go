package domain

// EpisodeRepository manages episode clustering and lifecycle.
type EpisodeRepository interface {
	Create(fact FactEntry) (int64, error)
	ListActive() []EpisodeEntry
	GetFacts(episodeID int64) []FactEntry
	SummarizeEpisode(id int64, rawLLM func([]Message) (string, error)) error
	ListByTopic(topicID int64) []EpisodeEntry
}
