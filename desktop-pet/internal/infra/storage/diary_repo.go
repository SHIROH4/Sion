package storage

import (
	"database/sql"
	"time"

	"desktop-pet/internal/domain"
)

// Compile-time check.
var _ domain.DiaryRepository = (*DiaryRepo)(nil)

// DiaryRepo implements domain.DiaryRepository with SQLite.
type DiaryRepo struct {
	db        *sql.DB
	vectorize func(string) ([]float32, error)
}

// NewDiaryRepo creates a DiaryRepo backed by the given DB.
func NewDiaryRepo(db *sql.DB) *DiaryRepo {
	return &DiaryRepo{db: db}
}

// SetVectorize injects the embedding function.
func (r *DiaryRepo) SetVectorize(fn func(string) ([]float32, error)) {
	r.vectorize = fn
}

// Vectorize calls the injected embedding function.
func (r *DiaryRepo) Vectorize(text string) ([]float32, error) {
	if r.vectorize == nil {
		return nil, nil
	}
	return r.vectorize(text)
}

func (r *DiaryRepo) Save(entry *domain.DiaryEntry) error {
	now := time.Now().Unix()
	if entry.CreatedAt == 0 {
		entry.CreatedAt = now
	}
	if entry.EndTime == 0 {
		entry.EndTime = now
	}
	if entry.StartTime == 0 {
		entry.StartTime = now - 7200 // default 2h window
	}
	vecBlob := EncodeVector(entry.Vector)
	res, err := r.db.Exec(
		`INSERT INTO diary (title, summary, vector, emotion_valence, emotion_arousal, start_time, end_time, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.Title, entry.Summary, vecBlob,
		entry.EmotionValence, entry.EmotionArousal,
		entry.StartTime, entry.EndTime, entry.CreatedAt,
	)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	entry.ID = id
	return nil
}

func (r *DiaryRepo) Search(queryVector []float32, topK int) ([]domain.DiaryEntry, error) {
	if topK <= 0 {
		topK = 5
	}
	cutoff := time.Now().Add(-90 * 24 * time.Hour).Unix()
	rows, err := r.db.Query(
		`SELECT id, title, summary, vector, emotion_valence, emotion_arousal, start_time, end_time, created_at
		 FROM diary WHERE archived = 0 AND created_at > ? ORDER BY created_at DESC LIMIT 500`, cutoff,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type scored struct {
		entry domain.DiaryEntry
		score float64
	}
	var candidates []scored

	for rows.Next() {
		e := scanDiaryEntry(rows)
		if e == nil || len(e.Vector) == 0 {
			continue
		}
		sim := CosineSimilarity(queryVector, e.Vector)
		candidates = append(candidates, scored{entry: *e, score: sim})
	}

	for i := 1; i < len(candidates); i++ {
		for j := i; j > 0 && candidates[j].score > candidates[j-1].score; j-- {
			candidates[j], candidates[j-1] = candidates[j-1], candidates[j]
		}
	}

	if topK > len(candidates) {
		topK = len(candidates)
	}
	result := make([]domain.DiaryEntry, topK)
	for i := 0; i < topK; i++ {
		result[i] = candidates[i].entry
	}
	return result, nil
}

func (r *DiaryRepo) ListRecent(limit int) []domain.DiaryEntry {
	if limit <= 0 {
		limit = 10
	}
	rows, err := r.db.Query(
		`SELECT id, title, summary, vector, emotion_valence, emotion_arousal, start_time, end_time, created_at
		 FROM diary WHERE archived = 0 ORDER BY created_at DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()
	return scanDiaryRows(rows)
}

func (r *DiaryRepo) Delete(id int64) error {
	_, err := r.db.Exec(`DELETE FROM diary WHERE id = ?`, id)
	return err
}

func (r *DiaryRepo) ArchiveDiary(id int64) error {
	_, err := r.db.Exec(`UPDATE diary SET archived = 1 WHERE id = ?`, id)
	return err
}

func (r *DiaryRepo) Count() int {
	var n int
	r.db.QueryRow(`SELECT COUNT(*) FROM diary WHERE archived = 0`).Scan(&n)
	return n
}

func (r *DiaryRepo) VectorizeDiary(id int64, vec []float32) error {
	_, err := r.db.Exec(`UPDATE diary SET vector = ? WHERE id = ?`, EncodeVector(vec), id)
	return err
}

func scanDiaryRows(rows *sql.Rows) []domain.DiaryEntry {
	var entries []domain.DiaryEntry
	for rows.Next() {
		e := scanDiaryEntry(rows)
		if e != nil {
			entries = append(entries, *e)
		}
	}
	return entries
}

func scanDiaryEntry(scanner interface {
	Scan(dest ...interface{}) error
}) *domain.DiaryEntry {
	var e domain.DiaryEntry
	var vecBlob []byte
	err := scanner.Scan(
		&e.ID, &e.Title, &e.Summary, &vecBlob,
		&e.EmotionValence, &e.EmotionArousal,
		&e.StartTime, &e.EndTime, &e.CreatedAt,
	)
	if err != nil {
		return nil
	}
	e.Vector = DecodeVector(vecBlob)
	return &e
}
