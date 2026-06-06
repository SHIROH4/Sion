package storage

import (
	"database/sql"
	"time"

	"desktop-pet/internal/domain"
)

var _ domain.ThreadRepository = (*ThreadRepo)(nil)

// ThreadRepo implements domain.ThreadRepository with SQLite.
type ThreadRepo struct {
	db *sql.DB
}

// NewThreadRepo creates a ThreadRepo backed by the given DB.
func NewThreadRepo(db *sql.DB) *ThreadRepo {
	return &ThreadRepo{db: db}
}

func (r *ThreadRepo) SaveThread(t domain.ConversationThread) (int64, error) {
	now := time.Now().Unix()
	if t.CreatedAt == 0 {
		t.CreatedAt = now
	}
	if t.LastTouchedAt == 0 {
		t.LastTouchedAt = now
	}
	result, err := r.db.Exec(
		`INSERT INTO conversation_threads (type, goal, status, priority, created_at, last_touched_at,
			resolved_at, key_messages, context_ref, best_approach, avoid_topics, outcome, learnings)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		string(t.Type), t.Goal, string(t.Status), t.Priority, t.CreatedAt, t.LastTouchedAt,
		t.ResolvedAt, t.KeyMessages, t.ContextRef, t.BestApproach, t.AvoidTopics, t.Outcome, t.Learnings,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (r *ThreadRepo) UpdateThread(t domain.ConversationThread) error {
	t.LastTouchedAt = time.Now().Unix()
	_, err := r.db.Exec(
		`UPDATE conversation_threads SET goal=?, status=?, priority=?, last_touched_at=?,
			resolved_at=?, key_messages=?, context_ref=?, best_approach=?, avoid_topics=?,
			outcome=?, learnings=? WHERE id=?`,
		t.Goal, string(t.Status), t.Priority, t.LastTouchedAt,
		t.ResolvedAt, t.KeyMessages, t.ContextRef, t.BestApproach, t.AvoidTopics,
		t.Outcome, t.Learnings, t.ID,
	)
	return err
}

func (r *ThreadRepo) ListActive() ([]domain.ConversationThread, error) {
	return r.query(
		`SELECT id, type, goal, status, priority, created_at, last_touched_at,
			resolved_at, key_messages, context_ref, best_approach, avoid_topics, outcome, learnings
		FROM conversation_threads WHERE status = 'active' ORDER BY priority DESC, last_touched_at DESC`,
	)
}

func (r *ThreadRepo) ListByType(threadType domain.ThreadType) ([]domain.ConversationThread, error) {
	return r.query(
		`SELECT id, type, goal, status, priority, created_at, last_touched_at,
			resolved_at, key_messages, context_ref, best_approach, avoid_topics, outcome, learnings
		FROM conversation_threads WHERE status = 'active' AND type = ? ORDER BY priority DESC`,
		string(threadType),
	)
}

func (r *ThreadRepo) TouchThread(id int64) error {
	_, err := r.db.Exec(
		`UPDATE conversation_threads SET last_touched_at = ? WHERE id = ?`,
		time.Now().Unix(), id,
	)
	return err
}

func (r *ThreadRepo) ResolveThread(id int64, outcome, learnings string) error {
	now := time.Now().Unix()
	_, err := r.db.Exec(
		`UPDATE conversation_threads SET status='resolved', resolved_at=?, outcome=?, learnings=? WHERE id=?`,
		now, outcome, learnings, id,
	)
	return err
}

func (r *ThreadRepo) MarkStale(id int64) error {
	_, err := r.db.Exec(
		`UPDATE conversation_threads SET status='stale', resolved_at=? WHERE id=?`,
		time.Now().Unix(), id,
	)
	return err
}

func (r *ThreadRepo) CleanResolved(days int) int {
	cutoff := time.Now().AddDate(0, 0, -days).Unix()
	result, _ := r.db.Exec(
		`DELETE FROM conversation_threads WHERE status != 'active' AND resolved_at < ?`, cutoff,
	)
	n, _ := result.RowsAffected()
	return int(n)
}

func (r *ThreadRepo) query(q string, args ...interface{}) ([]domain.ConversationThread, error) {
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var threads []domain.ConversationThread
	for rows.Next() {
		var t domain.ConversationThread
		var typ, status string
		if err := rows.Scan(&t.ID, &typ, &t.Goal, &status, &t.Priority,
			&t.CreatedAt, &t.LastTouchedAt, &t.ResolvedAt,
			&t.KeyMessages, &t.ContextRef, &t.BestApproach, &t.AvoidTopics,
			&t.Outcome, &t.Learnings); err != nil {
			continue
		}
		t.Type = domain.ThreadType(typ)
		t.Status = domain.ThreadStatus(status)
		threads = append(threads, t)
	}
	return threads, nil
}
