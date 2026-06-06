package storage

import (
	"database/sql"
	"time"

	"desktop-pet/internal/domain"
)

// Compile-time check.
var _ domain.TopicRepository = (*TopicRepo)(nil)

// TopicRepo implements domain.TopicRepository with SQLite.
type TopicRepo struct {
	db *sql.DB
}

// NewTopicRepo creates a TopicRepo backed by the given DB.
func NewTopicRepo(db *sql.DB) *TopicRepo {
	return &TopicRepo{db: db}
}

// DB returns the underlying database handle for service-layer use.
func (r *TopicRepo) DB() *sql.DB { return r.db }

// Initialize creates predefined topics that don't exist yet (idempotent).
func (r *TopicRepo) Initialize(topics []string) error {
	for _, name := range topics {
		var count int
		r.db.QueryRow(`SELECT COUNT(*) FROM topics WHERE name = ?`, name).Scan(&count)
		if count == 0 {
			now := time.Now().Unix()
			r.db.Exec(`INSERT INTO topics (name, created_at, updated_at) VALUES (?, ?, ?)`,
				name, now, now)
		}
	}
	return nil
}

// ListTopics returns all topics ordered by episode_count descending.
func (r *TopicRepo) ListTopics() []domain.TopicEntry {
	rows, err := r.db.Query(
		`SELECT id, name, centroid, description, episode_count, created_at, updated_at
		 FROM topics ORDER BY episode_count DESC`,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()
	return scanTopicRows(rows)
}

// FindBestTopic returns the best-matching topic for an episode's centroid.
func (r *TopicRepo) FindBestTopic(episodeCentroid []float32) (int64, float64) {
	rows, err := r.db.Query(`SELECT id, centroid FROM topics WHERE centroid IS NOT NULL`)
	if err != nil {
		return 0, 0
	}
	defer rows.Close()

	var bestID int64
	var bestScore float64
	for rows.Next() {
		var id int64
		var data []byte
		if rows.Scan(&id, &data) != nil {
			continue
		}
		vec := DecodeVector(data)
		score := cosSim(episodeCentroid, vec)
		if score > bestScore {
			bestScore = score
			bestID = id
		}
	}
	return bestID, bestScore
}

// AssignEpisodeToTopic links an episode to a topic.
func (r *TopicRepo) AssignEpisodeToTopic(episodeID, topicID int64) error {
	_, err := r.db.Exec(`UPDATE episodes SET topic_id = ? WHERE id = ?`, topicID, episodeID)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(`UPDATE topics SET episode_count = episode_count + 1 WHERE id = ?`, topicID)
	return err
}

// UpdateCentroid recalculates a topic's centroid from all contained episode centroids.
func (r *TopicRepo) UpdateCentroid(topicID int64) error {
	rows, err := r.db.Query(
		`SELECT centroid FROM episodes WHERE topic_id = ? AND centroid IS NOT NULL`,
		topicID,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	var allVectors [][]float32
	for rows.Next() {
		var data []byte
		if rows.Scan(&data) != nil || len(data) == 0 {
			continue
		}
		allVectors = append(allVectors, DecodeVector(data))
	}
	if len(allVectors) == 0 {
		return nil
	}

	dim := len(allVectors[0])
	avg := make([]float32, dim)
	for _, v := range allVectors {
		for i := range v {
			avg[i] += v[i]
		}
	}
	n := float32(len(allVectors))
	for i := range avg {
		avg[i] /= n
	}

	_, err = r.db.Exec(
		`UPDATE topics SET centroid = ?, updated_at = ? WHERE id = ?`,
		EncodeVector(avg), time.Now().Unix(), topicID,
	)
	return err
}

// FindTopicIDByName returns the topic ID for a given name, or 0 if not found.
func (r *TopicRepo) FindTopicIDByName(name string) int64 {
	var id int64
	r.db.QueryRow(`SELECT id FROM topics WHERE name = ?`, name).Scan(&id)
	return id
}

// SetCentroidRaw stores a pre-computed centroid vector for a topic.
func (r *TopicRepo) SetCentroidRaw(topicID int64, vec []float32) {
	r.db.Exec(
		`UPDATE topics SET centroid = ?, updated_at = ? WHERE id = ?`,
		EncodeVector(vec), time.Now().Unix(), topicID,
	)
}

// scanTopicRows and scanTopicEntry are local scan helpers.

func scanTopicRows(rows *sql.Rows) []domain.TopicEntry {
	var entries []domain.TopicEntry
	for rows.Next() {
		e := scanTopicEntry(rows)
		if e != nil {
			entries = append(entries, *e)
		}
	}
	return entries
}

func scanTopicEntry(scanner interface {
	Scan(dest ...interface{}) error
}) *domain.TopicEntry {
	var e domain.TopicEntry
	var centroidData []byte
	err := scanner.Scan(
		&e.ID, &e.Name, &centroidData, &e.Description,
		&e.EpisodeCount, &e.CreatedAt, &e.UpdatedAt,
	)
	if err != nil {
		return nil
	}
	e.Centroid = DecodeVector(centroidData)
	return &e
}
