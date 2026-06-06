package domain

// TopicRepository manages topic clustering and episode assignment.
type TopicRepository interface {
	ListTopics() []TopicEntry
	FindBestTopic(episodeCentroid []float32) (int64, float64)
	AssignEpisodeToTopic(episodeID, topicID int64) error
}
