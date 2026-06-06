package storage

import (
	"database/sql"
	"time"

	"desktop-pet/internal/domain"
)

var _ domain.CuriosityRepository = (*CuriosityRepo)(nil)

// CuriosityRepo implements domain.CuriosityRepository with SQLite.
type CuriosityRepo struct {
	db *sql.DB
}

// NewCuriosityRepo creates a CuriosityRepo backed by the given DB.
func NewCuriosityRepo(db *sql.DB) *CuriosityRepo {
	return &CuriosityRepo{db: db}
}

func (r *CuriosityRepo) Save(item domain.CuriosityItem) (int64, error) {
	now := time.Now().Unix()
	if item.CreatedAt == 0 {
		item.CreatedAt = now
	}
	item.UpdatedAt = now

	var vec []byte
	if len(item.Embedding) > 0 {
		vec = EncodeVector(item.Embedding)
	}

	if item.ID > 0 {
		_, err := r.db.Exec(
			`UPDATE curiosity_items SET content=?, confidence=?, priority=?, status=?,
				source=?, evidence=?, tags=?, embedding=?, asked_at=?, resolved_at=?, updated_at=?
			WHERE id=?`,
			item.Content, item.Confidence, item.Priority, item.Status,
			item.Source, item.Evidence, item.Tags, vec, item.AskedAt, item.ResolvedAt, item.UpdatedAt, item.ID,
		)
		return item.ID, err
	}

	result, err := r.db.Exec(
		`INSERT INTO curiosity_items (item_type, content, confidence, priority, status,
			source, evidence, tags, embedding, asked_at, resolved_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		string(item.ItemType), item.Content, item.Confidence, item.Priority, item.Status,
		item.Source, item.Evidence, item.Tags, vec, item.AskedAt, item.ResolvedAt, item.CreatedAt, item.UpdatedAt,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (r *CuriosityRepo) UpdateConfidence(id int64, delta float64) error {
	_, err := r.db.Exec(
		`UPDATE curiosity_items SET confidence = MIN(1.0, MAX(0.0, confidence + ?)), updated_at = ? WHERE id = ?`,
		delta, time.Now().Unix(), id,
	)
	return err
}

func (r *CuriosityRepo) List(itemType domain.CuriosityItemType, status string, limit int) ([]domain.CuriosityItem, error) {
	if limit <= 0 {
		limit = 20
	}
	query := `SELECT id, item_type, content, confidence, priority, status,
		source, evidence, tags, embedding, asked_at, resolved_at, created_at, updated_at
		FROM curiosity_items WHERE 1=1`
	var args []interface{}

	if itemType != "" {
		query += ` AND item_type = ?`
		args = append(args, string(itemType))
	}
	if status != "" {
		query += ` AND status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY priority DESC, created_at DESC LIMIT ?`
	args = append(args, limit)

	return r.query(query, args...)
}

func (r *CuriosityRepo) FindSimilar(content string) (*domain.CuriosityItem, error) {
	prefix := content
	if len([]rune(prefix)) > 40 {
		prefix = string([]rune(prefix)[:40])
	}
	row := r.db.QueryRow(
		`SELECT id, item_type, content, confidence, priority, status,
			source, evidence, tags, embedding, asked_at, resolved_at, created_at, updated_at
		FROM curiosity_items WHERE item_type = 'entry' AND content LIKE ? LIMIT 1`,
		prefix+"%",
	)
	return r.scan(row)
}

func (r *CuriosityRepo) MarkStatus(id int64, status string) error {
	now := time.Now().Unix()
	query := `UPDATE curiosity_items SET status = ?, updated_at = ?`
	args := []interface{}{status, now, id}

	switch status {
	case "asked":
		query += `, asked_at = ?`
		args = append([]interface{}{status, now, now}, id)
	case "resolved", "abandoned":
		query += `, resolved_at = ?`
		args = append([]interface{}{status, now, now}, id)
	}
	query += ` WHERE id = ?`
	_, err := r.db.Exec(query, args...)
	return err
}

func (r *CuriosityRepo) Cleanup(days int) int {
	cutoff := time.Now().AddDate(0, 0, -days).Unix()
	result, _ := r.db.Exec(
		`DELETE FROM curiosity_items WHERE status IN ('resolved','abandoned') AND resolved_at < ?`, cutoff,
	)
	n, _ := result.RowsAffected()
	return int(n)
}

func (r *CuriosityRepo) scan(row *sql.Row) (*domain.CuriosityItem, error) {
	var item domain.CuriosityItem
	var itemType string
	var vec []byte
	err := row.Scan(&item.ID, &itemType, &item.Content, &item.Confidence, &item.Priority,
		&item.Status, &item.Source, &item.Evidence, &item.Tags, &vec,
		&item.AskedAt, &item.ResolvedAt, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return nil, err
	}
	item.ItemType = domain.CuriosityItemType(itemType)
	if len(vec) > 0 {
		item.Embedding = DecodeVector(vec)
	}
	return &item, nil
}

func (r *CuriosityRepo) query(q string, args ...interface{}) ([]domain.CuriosityItem, error) {
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []domain.CuriosityItem
	for rows.Next() {
		var item domain.CuriosityItem
		var itemType string
		var vec []byte
		if err := rows.Scan(&item.ID, &itemType, &item.Content, &item.Confidence, &item.Priority,
			&item.Status, &item.Source, &item.Evidence, &item.Tags, &vec,
			&item.AskedAt, &item.ResolvedAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
			continue
		}
		item.ItemType = domain.CuriosityItemType(itemType)
		if len(vec) > 0 {
			item.Embedding = DecodeVector(vec)
		}
		items = append(items, item)
	}
	return items, nil
}
