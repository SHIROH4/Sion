package storage

import (
	"database/sql"
	"strings"
	"time"

	"desktop-pet/internal/domain"
)

var _ domain.StrategyPrincipleRepository = (*StrategyPrincipleRepo)(nil)

// StrategyPrincipleRepo implements domain.StrategyPrincipleRepository with SQLite.
type StrategyPrincipleRepo struct {
	db *sql.DB
}

// NewStrategyPrincipleRepo creates a StrategyPrincipleRepo backed by the given DB.
func NewStrategyPrincipleRepo(db *sql.DB) *StrategyPrincipleRepo {
	return &StrategyPrincipleRepo{db: db}
}

func (r *StrategyPrincipleRepo) SavePrinciple(p domain.StrategyPrinciple) (int64, error) {
	now := time.Now().Unix()
	var vec []byte
	if len(p.Embedding) > 0 {
		vec = EncodeVector(p.Embedding)
	}
	if p.CreatedAt == 0 {
		p.CreatedAt = now
	}
	p.UpdatedAt = now

	_, err := r.db.Exec(
		`INSERT INTO strategy_principles (situation, good_strategy, bad_strategy, reason,
			confidence, source, tags, embedding, active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.Situation, p.GoodStrategy, p.BadStrategy, p.Reason,
		p.Confidence, p.Source, p.Tags, vec, boolToInt(p.Active),
		p.CreatedAt, p.UpdatedAt,
	)
	if err != nil {
		return 0, err
	}

	var id int64
	r.db.QueryRow(`SELECT last_insert_rowid()`).Scan(&id)
	return id, nil
}

func (r *StrategyPrincipleRepo) ListActive() ([]domain.StrategyPrinciple, error) {
	return r.query(`SELECT id, situation, good_strategy, bad_strategy, reason,
		confidence, source, tags, embedding, active, created_at, updated_at
		FROM strategy_principles WHERE active = 1 ORDER BY confidence DESC, created_at DESC`)
}

func (r *StrategyPrincipleRepo) FindByTags(tags []string, limit int) ([]domain.StrategyPrinciple, error) {
	if len(tags) == 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 10
	}
	var conditions []string
	var args []interface{}
	for _, t := range tags {
		conditions = append(conditions, "tags LIKE ?")
		args = append(args, "%"+t+"%")
	}
	where := "active = 1 AND (" + strings.Join(conditions, " OR ") + ")"
	query := `SELECT id, situation, good_strategy, bad_strategy, reason,
		confidence, source, tags, embedding, active, created_at, updated_at
		FROM strategy_principles WHERE ` + where + ` ORDER BY confidence DESC LIMIT ?`
	args = append(args, limit)
	return r.query(query, args...)
}

func (r *StrategyPrincipleRepo) SearchSimilar(queryVec []float32, limit int) ([]domain.StrategyPrinciple, error) {
	if len(queryVec) == 0 || limit <= 0 {
		return nil, nil
	}
	all, err := r.ListActive()
	if err != nil {
		return nil, err
	}
	type scored struct {
		p     domain.StrategyPrinciple
		score float64
	}
	var ranked []scored
	for _, p := range all {
		if len(p.Embedding) == 0 {
			continue
		}
		sim := cosSim(queryVec, p.Embedding)
		ranked = append(ranked, scored{p, sim})
	}
	// Sort by similarity descending (simple bubble for small N).
	for i := 0; i < len(ranked); i++ {
		for j := i + 1; j < len(ranked); j++ {
			if ranked[j].score > ranked[i].score {
				ranked[i], ranked[j] = ranked[j], ranked[i]
			}
		}
	}
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	result := make([]domain.StrategyPrinciple, len(ranked))
	for i, s := range ranked {
		result[i] = s.p
	}
	return result, nil
}

func (r *StrategyPrincipleRepo) Deactivate(id int64) error {
	_, err := r.db.Exec(
		`UPDATE strategy_principles SET active = 0, updated_at = ? WHERE id = ?`,
		time.Now().Unix(), id,
	)
	return err
}

func (r *StrategyPrincipleRepo) CleanInactive(days int) (removed int) {
	cutoff := time.Now().AddDate(0, 0, -days).Unix()
	result, err := r.db.Exec(
		`DELETE FROM strategy_principles WHERE active = 0 AND updated_at < ?`, cutoff,
	)
	if err != nil {
		return 0
	}
	n, _ := result.RowsAffected()
	return int(n)
}

func (r *StrategyPrincipleRepo) Count() int {
	var n int
	r.db.QueryRow(`SELECT COUNT(*) FROM strategy_principles WHERE active = 1`).Scan(&n)
	return n
}

func (r *StrategyPrincipleRepo) query(q string, args ...interface{}) ([]domain.StrategyPrinciple, error) {
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var principles []domain.StrategyPrinciple
	for rows.Next() {
		var p domain.StrategyPrinciple
		var vec []byte
		var active int
		if err := rows.Scan(&p.ID, &p.Situation, &p.GoodStrategy, &p.BadStrategy,
			&p.Reason, &p.Confidence, &p.Source, &p.Tags, &vec,
			&active, &p.CreatedAt, &p.UpdatedAt); err != nil {
			continue
		}
		p.Active = active != 0
		if len(vec) > 0 {
			p.Embedding = DecodeVector(vec)
		}
		principles = append(principles, p)
	}
	return principles, nil
}
