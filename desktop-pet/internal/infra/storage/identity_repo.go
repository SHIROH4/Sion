package storage

import (
	"database/sql"
	"fmt"
	"time"

	"desktop-pet/internal/domain"
)

// Compile-time check.
var _ domain.IdentityRepository = (*IdentityRepo)(nil)

// IdentityRepo implements domain.IdentityRepository with SQLite.
type IdentityRepo struct {
	db *sql.DB
}

// NewIdentityRepo creates an IdentityRepo backed by the given DB.
func NewIdentityRepo(db *sql.DB) *IdentityRepo {
	return &IdentityRepo{db: db}
}

func (r *IdentityRepo) ListActiveIdentityNodes() ([]domain.IdentityNode, error) {
	rows, err := r.db.Query(
		`SELECT id, node_type, content, confidence, embedding,
		        created_at, updated_at, last_matched, match_count, active
		 FROM identity_nodes WHERE active = 1`)
	if err != nil {
		return nil, fmt.Errorf("identity_repo: list: %w", err)
	}
	defer rows.Close()

	var nodes []domain.IdentityNode
	for rows.Next() {
		var n domain.IdentityNode
		var embBytes []byte
		if err := rows.Scan(&n.ID, &n.Type, &n.Content, &n.Confidence,
			&embBytes, &n.CreatedAt, &n.UpdatedAt, &n.LastMatched,
			&n.MatchCount, &n.Active); err != nil {
			return nil, fmt.Errorf("identity_repo: scan: %w", err)
		}
		if len(embBytes) > 0 {
			n.Embedding = DecodeVector(embBytes)
		}
		nodes = append(nodes, n)
	}
	return nodes, nil
}

func (r *IdentityRepo) UpsertIdentityNode(node *domain.IdentityNode) error {
	now := time.Now().Unix()
	if node.CreatedAt == 0 {
		node.CreatedAt = now
	}
	node.UpdatedAt = now

	embBytes := EncodeVector(node.Embedding)

	if node.ID == 0 {
		res, err := r.db.Exec(
			`INSERT INTO identity_nodes (node_type, content, confidence, embedding,
			 created_at, updated_at, last_matched, match_count, active)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1)`,
			string(node.Type), node.Content, node.Confidence, embBytes,
			node.CreatedAt, node.UpdatedAt, node.LastMatched, node.MatchCount,
		)
		if err != nil {
			return fmt.Errorf("identity_repo: upsert insert: %w", err)
		}
		node.ID, _ = res.LastInsertId()
		node.Active = true
	} else {
		_, err := r.db.Exec(
			`UPDATE identity_nodes SET node_type=?, content=?, confidence=?,
			 embedding=?, updated_at=?, last_matched=?, match_count=?, active=?
			 WHERE id=?`,
			string(node.Type), node.Content, node.Confidence, embBytes,
			node.UpdatedAt, node.LastMatched, node.MatchCount,
			boolToInt(node.Active), node.ID,
		)
		if err != nil {
			return fmt.Errorf("identity_repo: upsert update: %w", err)
		}
	}
	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (r *IdentityRepo) DeactivateIdentityNode(id int64) error {
	_, err := r.db.Exec(
		`UPDATE identity_nodes SET active=0, updated_at=? WHERE id=?`,
		time.Now().Unix(), id,
	)
	if err != nil {
		return fmt.Errorf("identity_repo: deactivate: %w", err)
	}
	return nil
}

func (r *IdentityRepo) UpdateIdentityNodeMatchStats(id int64, lastMatched int64, matchCount int) error {
	_, err := r.db.Exec(
		`UPDATE identity_nodes SET last_matched=?, match_count=? WHERE id=?`,
		lastMatched, matchCount, id,
	)
	return err
}
