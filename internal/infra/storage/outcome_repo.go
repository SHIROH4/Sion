package storage

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"desktop-pet/internal/domain"
)

// Compile-time check.
var _ domain.ActionOutcomeRepository = (*ActionOutcomeRepo)(nil)

// ActionOutcomeRepo implements domain.ActionOutcomeRepository with SQLite.
type ActionOutcomeRepo struct {
	db *sql.DB
}

// NewActionOutcomeRepo creates an ActionOutcomeRepo backed by the given DB.
func NewActionOutcomeRepo(db *sql.DB) *ActionOutcomeRepo {
	return &ActionOutcomeRepo{db: db}
}

func (r *ActionOutcomeRepo) SaveOutcome(o domain.ActionOutcome) error {
	_, err := r.db.Exec(
		`INSERT INTO action_outcomes (action_source, action_type, hour_of_day, day_of_week,
			app_context, emotion_bucket, escalation_lvl, outcome, response_delay, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		string(o.ActionSource), string(o.ActionType), o.HourOfDay, o.DayOfWeek,
		o.AppContext, o.EmotionBucket, o.EscalationLvl, int(o.Outcome),
		o.ResponseDelay, time.Now().Unix(),
	)
	return err
}

func (r *ActionOutcomeRepo) QuerySimilarContexts(ctx domain.ActionContext, limit int) ([]domain.ActionOutcome, error) {
	if limit <= 0 {
		limit = 20
	}

	var conditions []string
	var args []interface{}

	if ctx.Source != "" {
		conditions = append(conditions, "action_source = ?")
		args = append(args, string(ctx.Source))
	}
	if ctx.Type != "" {
		conditions = append(conditions, "action_type = ?")
		args = append(args, string(ctx.Type))
	}
	if ctx.HourOfDay >= 0 {
		// Match within ±1 hour window
		conditions = append(conditions, "ABS(hour_of_day - ?) <= 1")
		args = append(args, ctx.HourOfDay)
	}
	if ctx.DayOfWeek >= 0 {
		conditions = append(conditions, "day_of_week = ?")
		args = append(args, ctx.DayOfWeek)
	}
	if ctx.EmotionBucket != "" {
		conditions = append(conditions, "emotion_bucket = ?")
		args = append(args, ctx.EmotionBucket)
	}

	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	query := fmt.Sprintf(
		`SELECT id, action_source, action_type, hour_of_day, day_of_week,
			app_context, emotion_bucket, escalation_lvl, outcome, response_delay, created_at
		FROM action_outcomes %s ORDER BY created_at DESC LIMIT ?`, where,
	)
	args = append(args, limit)

	return r.query(query, args...)
}

func (r *ActionOutcomeRepo) RecentOutcomes(source domain.ProactiveSource, limit int) ([]domain.ActionOutcome, error) {
	if limit <= 0 {
		limit = 20
	}
	return r.query(
		`SELECT id, action_source, action_type, hour_of_day, day_of_week,
			app_context, emotion_bucket, escalation_lvl, outcome, response_delay, created_at
		FROM action_outcomes WHERE action_source = ? ORDER BY created_at DESC LIMIT ?`,
		string(source), limit,
	)
}

func (r *ActionOutcomeRepo) SuccessRate(ctx domain.ActionContext, windowDays int) (accepts int, total int) {
	since := time.Now().AddDate(0, 0, -windowDays).Unix()

	var conditions []string
	var args []interface{}
	conditions = append(conditions, "created_at > ?")
	args = append(args, since)

	if ctx.Source != "" {
		conditions = append(conditions, "action_source = ?")
		args = append(args, string(ctx.Source))
	}
	if ctx.HourOfDay > 0 || ctx.DayOfWeek > 0 || ctx.EmotionBucket != "" {
		if ctx.HourOfDay >= 0 && ctx.HourOfDay <= 23 {
			conditions = append(conditions, "ABS(hour_of_day - ?) <= 1")
			args = append(args, ctx.HourOfDay)
		}
		if ctx.DayOfWeek >= 0 && ctx.DayOfWeek <= 6 {
			conditions = append(conditions, "day_of_week = ?")
			args = append(args, ctx.DayOfWeek)
		}
	}
	if ctx.EmotionBucket != "" {
		conditions = append(conditions, "emotion_bucket = ?")
		args = append(args, ctx.EmotionBucket)
	}

	where := strings.Join(conditions, " AND ")
	r.db.QueryRow(
		fmt.Sprintf(`SELECT COUNT(*) FROM action_outcomes WHERE %s AND outcome > 0`, where),
		args...,
	).Scan(&accepts)
	r.db.QueryRow(
		fmt.Sprintf(`SELECT COUNT(*) FROM action_outcomes WHERE %s`, where),
		args...,
	).Scan(&total)
	return
}

func (r *ActionOutcomeRepo) CleanOldOutcomes(days int) (removed int) {
	cutoff := time.Now().AddDate(0, 0, -days).Unix()
	result, err := r.db.Exec(`DELETE FROM action_outcomes WHERE created_at < ?`, cutoff)
	if err != nil {
		return 0
	}
	n, _ := result.RowsAffected()
	return int(n)
}

// SuccessRateByType returns acceptance rate per action type within the window.
func (r *ActionOutcomeRepo) SuccessRateByType(windowDays int) (map[domain.CareTriggerType]float64, error) {
	since := time.Now().AddDate(0, 0, -windowDays).Unix()
	rows, err := r.db.Query(
		`SELECT action_type, AVG(CASE WHEN outcome > 0 THEN 1.0 ELSE 0.0 END) as rate
		FROM action_outcomes WHERE created_at > ? GROUP BY action_type`, since,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[domain.CareTriggerType]float64)
	for rows.Next() {
		var typ string
		var rate float64
		if rows.Scan(&typ, &rate) == nil {
			result[domain.CareTriggerType(typ)] = rate
		}
	}
	return result, nil
}

// SuccessRateByTimeBlock returns acceptance rate per 4 time blocks.
// 0 = late_night (0-5), 1 = morning (6-11), 2 = afternoon (12-17), 3 = evening (18-23).
func (r *ActionOutcomeRepo) SuccessRateByTimeBlock(windowDays int) (map[int]float64, error) {
	since := time.Now().AddDate(0, 0, -windowDays).Unix()
	rows, err := r.db.Query(
		`SELECT
			CASE
				WHEN hour_of_day BETWEEN 0 AND 5 THEN 0
				WHEN hour_of_day BETWEEN 6 AND 11 THEN 1
				WHEN hour_of_day BETWEEN 12 AND 17 THEN 2
				ELSE 3
			END as block,
			AVG(CASE WHEN outcome > 0 THEN 1.0 ELSE 0.0 END) as rate
		FROM action_outcomes WHERE created_at > ?
		GROUP BY block`, since,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[int]float64)
	for rows.Next() {
		var block int
		var rate float64
		if rows.Scan(&block, &rate) == nil {
			result[block] = rate
		}
	}
	return result, nil
}

// SuccessRateBySource returns acceptance rate per proactive source within the window.
func (r *ActionOutcomeRepo) SuccessRateBySource(windowDays int) (map[domain.ProactiveSource]float64, error) {
	since := time.Now().AddDate(0, 0, -windowDays).Unix()
	rows, err := r.db.Query(
		`SELECT action_source, AVG(CASE WHEN outcome > 0 THEN 1.0 ELSE 0.0 END) as rate
		FROM action_outcomes WHERE created_at > ? GROUP BY action_source`, since,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[domain.ProactiveSource]float64)
	for rows.Next() {
		var src string
		var rate float64
		if rows.Scan(&src, &rate) == nil {
			result[domain.ProactiveSource(src)] = rate
		}
	}
	return result, nil
}

func (r *ActionOutcomeRepo) query(q string, args ...interface{}) ([]domain.ActionOutcome, error) {
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var outcomes []domain.ActionOutcome
	for rows.Next() {
		var o domain.ActionOutcome
		var src, typ string
		var outcome int
		if err := rows.Scan(&o.ID, &src, &typ, &o.HourOfDay, &o.DayOfWeek,
			&o.AppContext, &o.EmotionBucket, &o.EscalationLvl,
			&outcome, &o.ResponseDelay, &o.CreatedAt); err != nil {
			continue
		}
		o.ActionSource = domain.ProactiveSource(src)
		o.ActionType = domain.CareTriggerType(typ)
		o.Outcome = domain.OutcomeResult(outcome)
		outcomes = append(outcomes, o)
	}
	return outcomes, nil
}
