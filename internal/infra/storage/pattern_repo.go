package storage

import (
	"database/sql"
	"time"

	"desktop-pet/internal/domain"
)

// ---- ActivityEventRepo ----

var _ domain.ActivityEventRepository = (*ActivityEventRepo)(nil)

type ActivityEventRepo struct{ db *sql.DB }

func NewActivityEventRepo(db *sql.DB) *ActivityEventRepo { return &ActivityEventRepo{db: db} }

func (r *ActivityEventRepo) RecordSession(s domain.ActivityEvent) (int64, error) {
	result, err := r.db.Exec(
		`INSERT INTO activity_sessions (app_name, window_title, is_working, start_time, end_time)
		VALUES (?, ?, ?, ?, ?)`,
		s.AppName, s.WindowTitle, boolToInt(s.IsWorking), s.StartTime, s.EndTime,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (r *ActivityEventRepo) UpdateSessionEnd(id int64, endTime int64) error {
	_, err := r.db.Exec(`UPDATE activity_sessions SET end_time = ? WHERE id = ?`, endTime, id)
	return err
}

func (r *ActivityEventRepo) ListToday() ([]domain.ActivityEvent, error) {
	today := time.Now().Truncate(24 * time.Hour).Unix()
	return r.query(`SELECT id, app_name, window_title, is_working, start_time, end_time
		FROM activity_sessions WHERE start_time >= ? ORDER BY start_time`, today)
}

func (r *ActivityEventRepo) ListRange(since, until int64) ([]domain.ActivityEvent, error) {
	return r.query(`SELECT id, app_name, window_title, is_working, start_time, end_time
		FROM activity_sessions WHERE start_time >= ? AND start_time < ? ORDER BY start_time`, since, until)
}

func (r *ActivityEventRepo) CleanOld(days int) int {
	cutoff := time.Now().AddDate(0, 0, -days).Unix()
	result, _ := r.db.Exec(`DELETE FROM activity_sessions WHERE end_time < ?`, cutoff)
	n, _ := result.RowsAffected()
	return int(n)
}

func (r *ActivityEventRepo) query(q string, args ...interface{}) ([]domain.ActivityEvent, error) {
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []domain.ActivityEvent
	for rows.Next() {
		var e domain.ActivityEvent
		var work int
		if rows.Scan(&e.ID, &e.AppName, &e.WindowTitle, &work, &e.StartTime, &e.EndTime) == nil {
			e.IsWorking = work != 0
			events = append(events, e)
		}
	}
	return events, nil
}

// ---- PatternRepo ----

var _ domain.PatternRepository = (*PatternRepo)(nil)

type PatternRepo struct{ db *sql.DB }

func NewPatternRepo(db *sql.DB) *PatternRepo { return &PatternRepo{db: db} }

func (r *PatternRepo) SavePattern(p domain.BehaviorPattern) (int64, error) {
	now := time.Now().Unix()
	if p.CreatedAt == 0 {
		p.CreatedAt = now
	}
	p.UpdatedAt = now
	result, err := r.db.Exec(
		`INSERT INTO behavior_patterns (pattern, type, evidence, confidence, implication, active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		p.Pattern, p.Type, p.Evidence, p.Confidence, p.Implication, boolToInt(p.Active), p.CreatedAt, p.UpdatedAt,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (r *PatternRepo) ListActive() ([]domain.BehaviorPattern, error) {
	return r.queryPatterns(`SELECT id, pattern, type, evidence, confidence, implication, active, created_at, updated_at
		FROM behavior_patterns WHERE active = 1 ORDER BY confidence DESC`)
}

func (r *PatternRepo) ListByType(patternType string) ([]domain.BehaviorPattern, error) {
	return r.queryPatterns(`SELECT id, pattern, type, evidence, confidence, implication, active, created_at, updated_at
		FROM behavior_patterns WHERE active = 1 AND type = ? ORDER BY confidence DESC`, patternType)
}

func (r *PatternRepo) UpdateConfidence(id int64, delta float64) error {
	_, err := r.db.Exec(
		`UPDATE behavior_patterns SET confidence = MIN(1.0, MAX(0.0, confidence + ?)), updated_at = ? WHERE id = ?`,
		delta, time.Now().Unix(), id,
	)
	return err
}

func (r *PatternRepo) Deactivate(id int64) error {
	_, err := r.db.Exec(`UPDATE behavior_patterns SET active = 0, updated_at = ? WHERE id = ?`, time.Now().Unix(), id)
	return err
}

func (r *PatternRepo) CleanInactive(days int) int {
	cutoff := time.Now().AddDate(0, 0, -days).Unix()
	result, _ := r.db.Exec(`DELETE FROM behavior_patterns WHERE active = 0 AND updated_at < ?`, cutoff)
	n, _ := result.RowsAffected()
	return int(n)
}

func (r *PatternRepo) queryPatterns(q string, args ...interface{}) ([]domain.BehaviorPattern, error) {
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var patterns []domain.BehaviorPattern
	for rows.Next() {
		var p domain.BehaviorPattern
		var active int
		if rows.Scan(&p.ID, &p.Pattern, &p.Type, &p.Evidence, &p.Confidence, &p.Implication, &active, &p.CreatedAt, &p.UpdatedAt) == nil {
			p.Active = active != 0
			patterns = append(patterns, p)
		}
	}
	return patterns, nil
}
