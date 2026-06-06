package storage

import (
	"database/sql"
	"time"

	"desktop-pet/internal/domain"
)

// Compile-time check.
var _ domain.EpisodeRepository = (*EpisodeRepo)(nil)

// EpisodeRepo implements domain.EpisodeRepository with SQLite.
type EpisodeRepo struct {
	db *sql.DB
}

// NewEpisodeRepo creates an EpisodeRepo backed by the given DB.
func NewEpisodeRepo(db *sql.DB) *EpisodeRepo {
	return &EpisodeRepo{db: db}
}

func (r *EpisodeRepo) Create(fact domain.FactEntry) (int64, error) {
	now := time.Now().Unix()
	centroid := EncodeVector(fact.Vector)
	title := fact.Content
	if len([]rune(title)) > 20 {
		title = string([]rune(title)[:20])
	}
	startTime := fact.StartTime
	if startTime == 0 {
		startTime = now
	}
	endTime := fact.EndTime
	if endTime == 0 {
		endTime = now
	}
	res, err := r.db.Exec(
		`INSERT INTO episodes (title, summary, centroid, topic_id, importance, fact_count, start_time, end_time, created_at, updated_at)
		 VALUES (?, ?, ?, 0, ?, 1, ?, ?, ?, ?)`,
		title, "", centroid, fact.Importance, startTime, endTime, now, now,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *EpisodeRepo) ListActive() []domain.EpisodeEntry {
	rows, err := r.db.Query(
		`SELECT id, title, summary, centroid, topic_id, importance, fact_count,
		        start_time, end_time, created_at, updated_at
		 FROM episodes WHERE fact_count > 0 ORDER BY importance DESC`,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()
	return scanEpisodeRows(rows)
}

func (r *EpisodeRepo) GetFacts(episodeID int64) []domain.FactEntry {
	rows, err := r.db.Query(
		`SELECT id, content, importance, fact_role, start_time, end_time,
		        last_recalled_at, recall_count, vector, episode_id, source, created_at, updated_at
		 FROM facts WHERE episode_id = ? AND archived = 0`,
		episodeID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()
	return ScanFactRows(rows)
}

func (r *EpisodeRepo) SummarizeEpisode(id int64, rawLLM func([]domain.Message) (string, error)) error {
	// LLM summarisation is handled by the service layer.
	return nil
}

func (r *EpisodeRepo) ListByTopic(topicID int64) []domain.EpisodeEntry {
	rows, err := r.db.Query(
		`SELECT id, title, summary, centroid, topic_id, importance, fact_count,
		        start_time, end_time, created_at, updated_at
		 FROM episodes WHERE topic_id = ? AND fact_count > 0 ORDER BY importance DESC`,
		topicID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()
	return scanEpisodeRows(rows)
}

// IncrementFactCount bumps the fact count for an episode.
func (r *EpisodeRepo) IncrementFactCount(id int64) error {
	_, err := r.db.Exec(
		`UPDATE episodes SET fact_count = fact_count + 1, updated_at = ? WHERE id = ?`,
		time.Now().Unix(), id,
	)
	return err
}

// UpdateCentroid recalculates the episode centroid with a new fact vector.
func (r *EpisodeRepo) UpdateCentroid(id int64, newVec []float32) error {
	row := r.db.QueryRow(`SELECT centroid, fact_count FROM episodes WHERE id = ?`, id)
	var oldData []byte
	var count int
	if err := row.Scan(&oldData, &count); err != nil {
		return err
	}
	oldVec := DecodeVector(oldData)
	if len(oldVec) != len(newVec) {
		return nil
	}
	n := float32(count)
	merged := make([]float32, len(oldVec))
	for i := range oldVec {
		merged[i] = (oldVec[i]*n + newVec[i]) / (n + 1)
	}
	_, err := r.db.Exec(
		`UPDATE episodes SET centroid = ?, updated_at = ? WHERE id = ?`,
		EncodeVector(merged), time.Now().Unix(), id,
	)
	return err
}

// UpdateSummary sets the title and summary for an episode.
func (r *EpisodeRepo) UpdateSummary(id int64, title, summary string) error {
	_, err := r.db.Exec(
		`UPDATE episodes SET title = ?, summary = ?, updated_at = ? WHERE id = ?`,
		title, summary, time.Now().Unix(), id,
	)
	return err
}

// scanEpisodeRows and scanEpisodeEntry helpers.

func scanEpisodeRows(rows *sql.Rows) []domain.EpisodeEntry {
	var entries []domain.EpisodeEntry
	for rows.Next() {
		e := scanEpisodeEntry(rows)
		if e != nil {
			entries = append(entries, *e)
		}
	}
	return entries
}

func scanEpisodeEntry(scanner interface {
	Scan(dest ...interface{}) error
}) *domain.EpisodeEntry {
	var e domain.EpisodeEntry
	var centroidData []byte
	err := scanner.Scan(
		&e.ID, &e.Title, &e.Summary, &centroidData,
		&e.TopicID, &e.Importance, &e.FactCount,
		&e.StartTime, &e.EndTime, &e.CreatedAt, &e.UpdatedAt,
	)
	if err != nil {
		return nil
	}
	e.Centroid = DecodeVector(centroidData)
	return &e
}
