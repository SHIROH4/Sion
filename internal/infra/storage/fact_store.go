package storage

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"desktop-pet/internal/domain"
)

// ---- Quality filters ----

var garbageFactPatterns = []string{
	"之类", "教教我", "帮我", "告诉我", "怎么办", "问一下",
	"的记录", "的信息", "别的称呼", "这个信息很有用",
}

var emojiRunes = []rune{'喵', '🐱', '😺', '😸', '😹', '😻', '😼', '😽', '🙀', '😿', '😾',
	'❤', '💙', '💚', '💛', '💜', '🖤', '✨', '🎉', '👍', '👎',
	'✧', '◕', '́', '•', '̀', 'ㅂ', '•', 'و', '✧', '｡', '•', '̀', 'ᴗ', '✧'}

func qualifyFactContent(content string) bool {
	content = strings.TrimSpace(content)
	if content == "" {
		return false
	}
	if len([]rune(content)) < 5 {
		return false
	}
	for _, p := range garbageFactPatterns {
		if strings.Contains(content, p) {
			return false
		}
	}
	for _, suffix := range []string{"就行", "好了", "算了", "上我", "给我"} {
		if strings.HasSuffix(content, suffix) {
			return false
		}
	}
	for _, q := range []string{"什么", "谁", "哪", "怎么", "吗", "呢", "吧", "嘛", "如何"} {
		if strings.HasPrefix(content, q) {
			return false
		}
	}
	for _, r := range emojiRunes {
		if strings.ContainsRune(content, r) {
			return false
		}
	}
	return true
}

// ---- Fact CRUD ----

func (s *Store) SaveFact(content, source string) error {
	if !qualifyFactContent(content) {
		return nil
	}
	var count int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM facts WHERE content = ?`, content).Scan(&count)
	if count > 0 {
		return nil
	}
	if s.embedSvc != nil {
		vec, err := s.embedSvc.Vectorize(content)
		if err == nil {
			activeFacts := s.factsWithVectors()
			for _, f := range activeFacts {
				if cosSim(vec, f.Vector) > 0.85 {
					return nil
				}
			}
			if len(vec) > 0 {
				vecData := EncodeVector(vec)
				_, err = s.db.Exec(
					`INSERT INTO facts (content, importance, vector, source, created_at, updated_at) VALUES (?, 0.5, ?, ?, ?, ?)`,
					content, vecData, source, time.Now().Unix(), time.Now().Unix(),
				)
				return err
			}
		}
	}
	_, err := s.db.Exec(
		`INSERT INTO facts (content, importance, source, created_at, updated_at) VALUES (?, 0.5, ?, ?, ?)`,
		content, source, time.Now().Unix(), time.Now().Unix(),
	)
	return err
}

func (s *Store) SaveAtomicFact(input domain.AtomicFactInput) error {
	input.Content = strings.TrimSpace(input.Content)
	if input.Content == "" || !qualifyFactContent(input.Content) {
		return nil
	}
	var count int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM facts WHERE content = ?`, input.Content).Scan(&count)
	if count > 0 {
		return nil
	}
	if input.FactRole == "" {
		input.FactRole = domain.RoleCore
	}
	if input.Source == "" {
		input.Source = "chat"
	}
	if input.Importance <= 0 {
		input.Importance = 0.5
	}
	if s.embedSvc != nil {
		vec, err := s.embedSvc.Vectorize(input.Content)
		if err == nil && len(vec) > 0 {
			activeFacts := s.factsWithVectors()
			for _, f := range activeFacts {
				if len(f.Vector) > 0 && cosSim(vec, f.Vector) > 0.85 {
					return nil
				}
			}
			vecData := EncodeVector(vec)
			_, err = s.db.Exec(
				`INSERT INTO facts (content, importance, fact_role, start_time, end_time, vector, source, created_at, updated_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				input.Content, input.Importance, string(input.FactRole),
				input.StartTime, input.EndTime, vecData, input.Source, time.Now().Unix(), time.Now().Unix(),
			)
			return err
		}
	}
	_, err := s.db.Exec(
		`INSERT INTO facts (content, importance, fact_role, start_time, end_time, source, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		input.Content, input.Importance, string(input.FactRole),
		input.StartTime, input.EndTime, input.Source, time.Now().Unix(), time.Now().Unix(),
	)
	return err
}

func (s *Store) SaveFactWithVector(input domain.AtomicFactInput, vec []float32) (int64, error) {
	input.Content = strings.TrimSpace(input.Content)
	if input.Content == "" || !qualifyFactContent(input.Content) {
		return 0, fmt.Errorf("fact content disqualified")
	}
	if input.FactRole == "" {
		input.FactRole = domain.RoleCore
	}
	if input.Source == "" {
		input.Source = "consolidation"
	}
	if input.Importance <= 0 {
		input.Importance = 0.5
	}
	vecData := EncodeVector(vec)
	res, err := s.db.Exec(
		`INSERT INTO facts (content, importance, fact_role, start_time, end_time, vector, source, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		input.Content, input.Importance, string(input.FactRole),
		input.StartTime, input.EndTime, vecData, input.Source, time.Now().Unix(), time.Now().Unix(),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// LookupFactByContent finds a fact by exact content match (exported for sub-stores).
func LookupFactByContent(db *sql.DB, content string) *domain.FactEntry {
	row := db.QueryRow(
		`SELECT id, content, importance, fact_role, start_time, end_time,
		        last_recalled_at, recall_count, vector, episode_id, source, created_at, updated_at
		 FROM facts WHERE content = ? AND archived = 0`, content,
	)
	return ScanFactEntry(row)
}

func (s *Store) AttachFactToEpisode(factID, episodeID int64) error {
	_, err := s.db.Exec(`UPDATE facts SET episode_id = ? WHERE id = ?`, episodeID, factID)
	return err
}

func (s *Store) factsWithVectors() []domain.FactEntry {
	rows, err := s.db.Query(
		`SELECT id, content, importance, fact_role, start_time, end_time,
		        last_recalled_at, recall_count, vector, episode_id, source, created_at, updated_at FROM facts WHERE archived = 0 AND vector IS NOT NULL ORDER BY created_at DESC LIMIT 100`,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()
	return ScanFactRows(rows)
}

func (s *Store) LoadFacts() []string {
	rows, err := s.db.Query(`SELECT content FROM facts ORDER BY id ASC`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var facts []string
	for rows.Next() {
		var c string
		if rows.Scan(&c) == nil {
			facts = append(facts, c)
		}
	}
	return facts
}

func (s *Store) UpdateFactRecall(id int64) error {
	_, err := s.db.Exec(
		`UPDATE facts SET recall_count = recall_count + 1, last_recalled_at = ? WHERE id = ?`,
		time.Now().Unix(), id,
	)
	return err
}

func (s *Store) ListActiveFacts(threshold float64) []domain.FactEntry {
	rows, err := s.db.Query(
		`SELECT id, content, importance, fact_role, start_time, end_time,
		        last_recalled_at, recall_count, vector, episode_id, source, created_at, updated_at FROM facts WHERE archived = 0 AND importance > ? ORDER BY importance DESC`,
		threshold,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()
	return ScanFactRows(rows)
}

func (s *Store) ArchiveFact(id int64) error {
	_, err := s.db.Exec(`UPDATE facts SET archived = 1 WHERE id = ?`, id)
	return err
}

func (s *Store) CleanArchivedFacts(retentionDays int) int {
	cutoff := time.Now().AddDate(0, 0, -retentionDays).Unix()
	res, err := s.db.Exec(`DELETE FROM facts WHERE archived = 1 AND created_at < ?`, cutoff)
	if err != nil {
		return 0
	}
	n, _ := res.RowsAffected()
	return int(n)
}

func (s *Store) BatchUpdateFactRecall(ids []int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().Unix()
	for _, id := range ids {
		if _, err := tx.Exec(
			`UPDATE facts SET recall_count = recall_count + 1, last_recalled_at = ? WHERE id = ?`,
			now, id,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) VectorizeFact(id int64, vec []float32) error {
	data := EncodeVector(vec)
	_, err := s.db.Exec(`UPDATE facts SET vector = ? WHERE id = ?`, data, id)
	return err
}

func (s *Store) FactsWithoutVectors() ([]domain.FactEntry, error) {
	rows, err := s.db.Query(
		`SELECT id, content, importance, fact_role, start_time, end_time,
		        last_recalled_at, recall_count, vector, episode_id, source, created_at, updated_at FROM facts WHERE (vector IS NULL OR vector = x'6E756C6C')`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return ScanFactRows(rows), nil
}

func (s *Store) UnlabeledFacts() ([]domain.FactEntry, error) {
	rows, err := s.db.Query(
		`SELECT id, content, importance, fact_role, start_time, end_time,
		        last_recalled_at, recall_count, vector, episode_id, source, created_at, updated_at FROM facts WHERE archived = 0 AND fact_role = 'core' AND start_time = 0`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return ScanFactRows(rows), nil
}

func (s *Store) UpdateFactAnnotations(id int64, role domain.FactRole, startTime, endTime int64) error {
	_, err := s.db.Exec(
		`UPDATE facts SET fact_role = ?, start_time = ?, end_time = ? WHERE id = ?`,
		string(role), startTime, endTime, id,
	)
	return err
}

func (s *Store) GetRecentFacts(since int64) []domain.FactEntry {
	rows, err := s.db.Query(
		`SELECT id, content, importance, fact_role, start_time, end_time,
		        last_recalled_at, recall_count, vector, episode_id, source, created_at, updated_at FROM facts WHERE archived = 0 AND created_at > ? ORDER BY id`,
		since,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()
	return ScanFactRows(rows)
}

func (s *Store) ReplaceFact(oldID, newID int64) error {
	_, err := s.db.Exec(`UPDATE facts SET archived = 1, replaced_by = ? WHERE id = ?`, newID, oldID)
	return err
}

func (s *Store) UpdateFactContent(id int64, content string) error {
	_, err := s.db.Exec(
		`UPDATE facts SET content = ?, version = version + 1, updated_at = ? WHERE id = ?`,
		content, time.Now().Unix(), id,
	)
	return err
}

// ---- MemCell CRUD ----

func (s *Store) SaveMemCell(t string, content string, importance float64, valence, arousal float64, sourceMsg string) error {
	_, err := s.db.Exec(
		`INSERT INTO memcell (type, content, importance, emotion_valence, emotion_arousal, source_msg, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		t, content, importance, valence, arousal, sourceMsg, time.Now().Unix(),
	)
	return err
}

func (s *Store) ListMemCells(cellType string, limit int) []domain.MemCell {
	if limit <= 0 {
		limit = 20
	}
	var rows *sql.Rows
	var err error
	if cellType == "" {
		rows, err = s.db.Query(
			`SELECT id, type, content, importance, emotion_valence, emotion_arousal, source_msg, created_at
			 FROM memcell ORDER BY created_at DESC LIMIT ?`, limit,
		)
	} else {
		rows, err = s.db.Query(
			`SELECT id, type, content, importance, emotion_valence, emotion_arousal, source_msg, created_at
			 FROM memcell WHERE type = ? ORDER BY created_at DESC LIMIT ?`, cellType, limit,
		)
	}
	if err != nil {
		return nil
	}
	defer rows.Close()
	var cells []domain.MemCell
	for rows.Next() {
		var c domain.MemCell
		if err := rows.Scan(&c.ID, &c.Type, &c.Content, &c.Importance,
			&c.Emotion.Valence, &c.Emotion.Arousal, &c.SourceMsg, &c.CreatedAt); err != nil {
			continue
		}
		cells = append(cells, c)
	}
	return cells
}

// ---- Scan helpers (exported for use by episode/topic stores during migration) ----

// ScanFactEntry scans a single FactEntry from a row scanner.
func ScanFactEntry(scanner interface {
	Scan(dest ...interface{}) error
}) *domain.FactEntry {
	var f domain.FactEntry
	var vecData []byte
	err := scanner.Scan(&f.ID, &f.Content, &f.Importance, &f.FactRole, &f.StartTime, &f.EndTime,
		&f.LastRecalledAt, &f.RecallCount, &vecData, &f.EpisodeID, &f.Source, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		return nil
	}
	f.Vector = DecodeVector(vecData)
	return &f
}

// ScanFactRows converts *sql.Rows into a slice of FactEntry.
func ScanFactRows(rows *sql.Rows) []domain.FactEntry {
	var facts []domain.FactEntry
	for rows.Next() {
		f := ScanFactEntry(rows)
		if f != nil {
			facts = append(facts, *f)
		}
	}
	return facts
}
