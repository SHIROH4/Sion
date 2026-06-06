package storage

import (
	"encoding/json"
	"time"

	"desktop-pet/internal/domain"
)

// ---- Chat History ----

func (s *Store) SaveHistory(messages []domain.Message, metaLevel int) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(
		`INSERT INTO chat_history (role, content, meta_level, created_at) VALUES (?, ?, ?, ?)`,
	)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now().Unix()
	for _, msg := range messages {
		level := metaLevel
		if ml, ok := msg.Meta.(metaLeveler); ok {
			level = ml.MetaLevel()
		}
		if _, err := stmt.Exec(msg.Role, msg.Content, level, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) CleanOldHistory(retentionDays int) error {
	cutoff := time.Now().AddDate(0, 0, -retentionDays).Unix()
	_, err := s.db.Exec(`DELETE FROM chat_history WHERE created_at < ?`, cutoff)
	return err
}

func (s *Store) LoadHistory(limit int) ([]domain.Message, error) {
	rows, err := s.db.Query(
		`SELECT role, content, meta_level FROM chat_history ORDER BY id DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []domain.Message
	for rows.Next() {
		var role, content string
		var metaLevel int
		if err := rows.Scan(&role, &content, &metaLevel); err != nil {
			return nil, err
		}
		msg := domain.Message{Role: role, Content: content}
		if metaLevel > 0 {
			msg.Meta = memoryMetaStub{level: metaLevel}
		}
		messages = append(messages, msg)
	}
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
	return messages, rows.Err()
}

// ---- Archives ----

func (s *Store) SaveArchive(name string, level int, original, summary string) error {
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO memory_archive (name, level, original, summary, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		name, level, original, summary, time.Now().Unix(),
	)
	return err
}

func (s *Store) FindArchiveByName(name string) (string, error) {
	var original string
	err := s.db.QueryRow(
		`SELECT original FROM memory_archive WHERE name = ?`, name,
	).Scan(&original)
	return original, err
}

// ---- Meta stub ----

type memoryMetaStub struct {
	level int
}

func (m memoryMetaStub) MetaLevel() int { return m.level }

func (m memoryMetaStub) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]int{"level": m.level})
}
