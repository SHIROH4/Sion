package storage

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"desktop-pet/internal/domain"
)

// metaLeveler is an optional interface that Message.Meta can implement.
type metaLeveler interface {
	MetaLevel() int
}

// Compile-time check: Store implements domain.MemoryStore.
var _ domain.MemoryStore = (*Store)(nil)

// Store implements domain.MemoryStore with SQLite.
type Store struct {
	db       *sql.DB
	embedSvc domain.Vectorizer
	// Sub-stores are temporarily commented out during migration to service/.
	// diaryStore     *DiaryStore
	// episodeStore   *EpisodeStore
	// topicStore     *TopicStore
	// foresightStore *ForesightStore
}

// DB returns the underlying *sql.DB for direct queries.
func (s *Store) DB() *sql.DB { return s.db }

// Close shuts down the underlying database connection.
func (s *Store) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// SetEmbedSvc injects the embedding service.
func (s *Store) SetEmbedSvc(svc domain.Vectorizer) {
	s.embedSvc = svc
}

// EmbedSvc returns the embedding service.
func (s *Store) EmbedSvc() domain.Vectorizer {
	return s.embedSvc
}

// NewStore wraps an already-opened *sql.DB as a domain.MemoryStore.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func shouldRunMigration(db *sql.DB, key string) bool {
	var v string
	if err := db.QueryRow(`SELECT value FROM profile WHERE key = ?`, key).Scan(&v); err == nil && v == "1" {
		return false
	}
	return true
}

func markMigrationDone(db *sql.DB, key string) {
	db.Exec(`INSERT INTO profile (key, value, updated_at) VALUES (?, '1', ?)
		ON CONFLICT(key) DO UPDATE SET value = '1', updated_at = excluded.updated_at`,
		key, time.Now().Unix())
}


func createSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS profile (
			key        TEXT PRIMARY KEY,
			value      TEXT NOT NULL,
			updated_at INTEGER
		);
		CREATE TABLE IF NOT EXISTS chat_history (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			role       TEXT NOT NULL,
			content    TEXT NOT NULL,
			meta_level INTEGER DEFAULT 0,
			created_at INTEGER
		);
		CREATE INDEX IF NOT EXISTS idx_history_created ON chat_history(created_at);
		CREATE TABLE IF NOT EXISTS facts (
			id               INTEGER PRIMARY KEY AUTOINCREMENT,
			content          TEXT NOT NULL,
			importance       REAL DEFAULT 0.5,
			last_recalled_at INTEGER DEFAULT 0,
			recall_count     INTEGER DEFAULT 0,
			archived         INTEGER DEFAULT 0,
			created_at       INTEGER
		);
		CREATE TABLE IF NOT EXISTS self_profile (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			content    TEXT NOT NULL,
			source     TEXT DEFAULT '',
			created_at INTEGER
		);
		CREATE TABLE IF NOT EXISTS memory_archive (
			name       TEXT PRIMARY KEY,
			level      INTEGER NOT NULL,
			original   TEXT NOT NULL,
			summary    TEXT NOT NULL,
			created_at INTEGER
		);
		CREATE TABLE IF NOT EXISTS diary (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			title           TEXT NOT NULL,
			summary         TEXT NOT NULL,
			vector          BLOB,
			emotion_valence REAL DEFAULT 0,
			emotion_arousal REAL DEFAULT 0,
			start_time      INTEGER,
			end_time        INTEGER,
			created_at      INTEGER
		);
		CREATE INDEX IF NOT EXISTS idx_diary_created ON diary(created_at);
		CREATE TABLE IF NOT EXISTS memcell (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			type            TEXT NOT NULL,
			content         TEXT NOT NULL,
			importance      REAL DEFAULT 0.5,
			emotion_valence REAL DEFAULT 0,
			emotion_arousal REAL DEFAULT 0,
			source_msg      TEXT DEFAULT '',
			created_at      INTEGER
		);
		CREATE INDEX IF NOT EXISTS idx_memcell_type ON memcell(type);
		CREATE INDEX IF NOT EXISTS idx_memcell_created ON memcell(created_at);
		CREATE TABLE IF NOT EXISTS episodes (
			id               INTEGER PRIMARY KEY AUTOINCREMENT,
			title            TEXT NOT NULL,
			summary          TEXT NOT NULL DEFAULT '',
			centroid         BLOB,
			topic_id         INTEGER DEFAULT 0,
			importance       REAL DEFAULT 0.5,
			fact_count       INTEGER DEFAULT 0,
			start_time       INTEGER,
			end_time         INTEGER,
			created_at       INTEGER,
			updated_at       INTEGER
		);
		CREATE INDEX IF NOT EXISTS idx_episodes_topic ON episodes(topic_id);
		CREATE INDEX IF NOT EXISTS idx_episodes_created ON episodes(created_at);
		CREATE TABLE IF NOT EXISTS topics (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			name         TEXT NOT NULL,
			centroid     BLOB,
			description  TEXT DEFAULT '',
			episode_count INTEGER DEFAULT 0,
			created_at   INTEGER,
			updated_at   INTEGER
		);
		CREATE INDEX IF NOT EXISTS idx_topics_created ON topics(created_at);
	`)
	if err != nil {
		return err
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS identity_nodes (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			node_type     TEXT NOT NULL,
			content       TEXT NOT NULL,
			confidence    REAL DEFAULT 1.0,
			embedding     BLOB,
			created_at    INTEGER NOT NULL,
			updated_at    INTEGER NOT NULL,
			last_matched  INTEGER DEFAULT 0,
			match_count   INTEGER DEFAULT 0,
			active        INTEGER DEFAULT 1
		);
	`); err != nil {
		return err
	}

	// Migrations.
	if _, err := db.Exec(`ALTER TABLE facts ADD COLUMN archived INTEGER DEFAULT 0`); err != nil {
		slog.Warn("storage: migration failed", "sql", "ALTER TABLE facts ADD COLUMN archived", "err", err)
	}
	if _, err := db.Exec(`ALTER TABLE facts ADD COLUMN importance REAL DEFAULT 0.5`); err != nil {
		slog.Warn("storage: migration failed", "sql", "ALTER TABLE facts ADD COLUMN importance", "err", err)
	}
	if _, err := db.Exec(`ALTER TABLE facts ADD COLUMN last_recalled_at INTEGER DEFAULT 0`); err != nil {
		slog.Warn("storage: migration failed", "sql", "ALTER TABLE facts ADD COLUMN last_recalled_at", "err", err)
	}
	if _, err := db.Exec(`ALTER TABLE facts ADD COLUMN recall_count INTEGER DEFAULT 0`); err != nil {
		slog.Warn("storage: migration failed", "sql", "ALTER TABLE facts ADD COLUMN recall_count", "err", err)
	}
	if _, err := db.Exec(`ALTER TABLE diary ADD COLUMN archived INTEGER DEFAULT 0`); err != nil {
		slog.Warn("storage: migration failed", "sql", "ALTER TABLE diary ADD COLUMN archived", "err", err)
	}
	if _, err := db.Exec(`ALTER TABLE self_profile ADD COLUMN source TEXT DEFAULT ''`); err != nil {
		slog.Warn("storage: migration failed", "sql", "ALTER TABLE self_profile ADD COLUMN source", "err", err)
	}
	if _, err := db.Exec(`ALTER TABLE facts ADD COLUMN vector BLOB`); err != nil {
		slog.Warn("storage: migration failed", "sql", "ALTER TABLE facts ADD COLUMN vector", "err", err)
	}
	migrateVectorEncoding(db)
	if _, err := db.Exec(`ALTER TABLE facts ADD COLUMN fact_role TEXT DEFAULT 'core'`); err != nil {
		slog.Warn("storage: migration failed", "sql", "ALTER TABLE facts ADD COLUMN fact_role", "err", err)
	}
	if _, err := db.Exec(`ALTER TABLE facts ADD COLUMN start_time INTEGER DEFAULT 0`); err != nil {
		slog.Warn("storage: migration failed", "sql", "ALTER TABLE facts ADD COLUMN start_time", "err", err)
	}
	if _, err := db.Exec(`ALTER TABLE facts ADD COLUMN end_time INTEGER DEFAULT 0`); err != nil {
		slog.Warn("storage: migration failed", "sql", "ALTER TABLE facts ADD COLUMN end_time", "err", err)
	}
	if _, err := db.Exec(`ALTER TABLE facts ADD COLUMN episode_id INTEGER DEFAULT 0`); err != nil {
		slog.Warn("storage: migration failed", "sql", "ALTER TABLE facts ADD COLUMN episode_id", "err", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_facts_episode ON facts(episode_id)`); err != nil {
		slog.Warn("storage: migration failed", "sql", "CREATE INDEX idx_facts_episode", "err", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_facts_role ON facts(fact_role)`); err != nil {
		slog.Warn("storage: migration failed", "sql", "CREATE INDEX idx_facts_role", "err", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_facts_archived ON facts(archived, importance)`); err != nil {
		slog.Warn("storage: migration failed", "sql", "CREATE INDEX idx_facts_archived", "err", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_facts_created ON facts(created_at)`); err != nil {
		slog.Warn("storage: migration failed", "sql", "CREATE INDEX idx_facts_created", "err", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_diary_archived ON diary(archived, created_at)`); err != nil {
		slog.Warn("storage: migration failed", "sql", "CREATE INDEX idx_diary_archived", "err", err)
	}
	if _, err := db.Exec(`ALTER TABLE facts ADD COLUMN source TEXT DEFAULT 'chat'`); err != nil {
		slog.Warn("storage: migration failed", "sql", "ALTER TABLE facts ADD COLUMN source", "err", err)
	}
	if _, err := db.Exec(`ALTER TABLE facts ADD COLUMN replaced_by INTEGER DEFAULT 0`); err != nil {
		slog.Warn("storage: migration failed", "sql", "ALTER TABLE facts ADD COLUMN replaced_by", "err", err)
	}
	if _, err := db.Exec(`ALTER TABLE facts ADD COLUMN version INTEGER DEFAULT 1`); err != nil {
		slog.Warn("storage: migration failed", "sql", "ALTER TABLE facts ADD COLUMN version", "err", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_facts_source ON facts(source)`); err != nil {
		slog.Warn("storage: migration failed", "sql", "CREATE INDEX idx_facts_source", "err", err)
	}
	if _, err := db.Exec(`ALTER TABLE facts ADD COLUMN updated_at INTEGER DEFAULT 0`); err != nil {
		slog.Warn("storage: migration failed", "sql", "ALTER TABLE facts ADD COLUMN updated_at", "err", err)
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS action_outcomes (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			action_source   TEXT NOT NULL,
			action_type     TEXT NOT NULL,
			hour_of_day     INTEGER NOT NULL,
			day_of_week     INTEGER NOT NULL,
			app_context     TEXT DEFAULT '',
			emotion_bucket  TEXT NOT NULL,
			escalation_lvl  INTEGER DEFAULT 0,
			outcome         INTEGER DEFAULT 0,
			response_delay  INTEGER DEFAULT 0,
			created_at      INTEGER
		);
		CREATE INDEX IF NOT EXISTS idx_outcomes_context ON action_outcomes(action_source, hour_of_day, day_of_week);
		CREATE INDEX IF NOT EXISTS idx_outcomes_emotion ON action_outcomes(emotion_bucket);
		CREATE INDEX IF NOT EXISTS idx_outcomes_created ON action_outcomes(created_at);
	`); err != nil {
		slog.Warn("storage: migration failed", "sql", "CREATE TABLE action_outcomes", "err", err)
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS strategy_principles (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			situation     TEXT NOT NULL,
			good_strategy TEXT NOT NULL,
			bad_strategy  TEXT DEFAULT "",
			reason        TEXT DEFAULT "",
			confidence    REAL DEFAULT 0.5,
			source        TEXT DEFAULT "daily_reflection",
			tags          TEXT DEFAULT "",
			embedding     BLOB,
			active        INTEGER DEFAULT 1,
			created_at    INTEGER,
			updated_at    INTEGER
		);
		CREATE INDEX IF NOT EXISTS idx_principles_active ON strategy_principles(active);
		CREATE INDEX IF NOT EXISTS idx_principles_tags ON strategy_principles(tags);
		CREATE INDEX IF NOT EXISTS idx_principles_created ON strategy_principles(created_at);
	`); err != nil {
		slog.Warn("storage: migration failed", "sql", "CREATE TABLE strategy_principles", "err", err)
	}
	if _, err := db.Exec(`
			CREATE TABLE IF NOT EXISTS curiosity_items (
				id          INTEGER PRIMARY KEY AUTOINCREMENT,
				item_type   TEXT NOT NULL,
				content     TEXT NOT NULL,
				confidence  REAL DEFAULT 0.5,
				priority    REAL DEFAULT 0.5,
				status      TEXT DEFAULT 'active',
				source      TEXT DEFAULT '',
				evidence    TEXT DEFAULT '',
				tags        TEXT DEFAULT '',
				embedding   BLOB,
				asked_at    INTEGER DEFAULT 0,
				resolved_at INTEGER DEFAULT 0,
				created_at  INTEGER,
				updated_at  INTEGER
			);
			CREATE INDEX IF NOT EXISTS idx_curiosity_type ON curiosity_items(item_type, status);
			CREATE INDEX IF NOT EXISTS idx_curiosity_priority ON curiosity_items(priority DESC);
		`); err != nil {
		slog.Warn("storage: migration failed", "sql", "CREATE TABLE curiosity_items", "err", err)
	}
	if _, err := db.Exec(`
			CREATE TABLE IF NOT EXISTS behavior_patterns (
				id          INTEGER PRIMARY KEY AUTOINCREMENT,
				pattern     TEXT NOT NULL,
				type        TEXT NOT NULL,
				evidence    TEXT DEFAULT "",
				confidence  REAL DEFAULT 0.5,
				implication TEXT DEFAULT "",
				active      INTEGER DEFAULT 1,
				created_at  INTEGER,
				updated_at  INTEGER
			);
			CREATE INDEX IF NOT EXISTS idx_bp_active ON behavior_patterns(active);
		`); err != nil {
		slog.Warn("storage: migration failed", "sql", "CREATE TABLE pattern tables", "err", err)
	}
	if _, err := db.Exec(`
				CREATE TABLE IF NOT EXISTS activity_sessions (
					id           INTEGER PRIMARY KEY AUTOINCREMENT,
					app_name     TEXT NOT NULL,
					window_title TEXT DEFAULT "",
					is_working   INTEGER DEFAULT 0,
					start_time   INTEGER NOT NULL,
					end_time     INTEGER NOT NULL
				);
				CREATE INDEX IF NOT EXISTS idx_as_start ON activity_sessions(start_time);
				CREATE INDEX IF NOT EXISTS idx_as_end ON activity_sessions(end_time);
			`); err != nil {
			slog.Warn("storage: migration failed", "sql", "CREATE TABLE activity_sessions", "err", err)
		}
		if _, err := db.Exec(`
				CREATE TABLE IF NOT EXISTS feature_cache (
					feature_name TEXT PRIMARY KEY,
					value_json  TEXT NOT NULL,
					confidence  REAL DEFAULT 1.0,
					sample_count INTEGER DEFAULT 0,
					computed_at INTEGER NOT NULL,
					ttl_seconds INTEGER NOT NULL
				);
				CREATE INDEX IF NOT EXISTS idx_fc_computed ON feature_cache(computed_at);
			`); err != nil {
				slog.Warn("storage: migration failed", "sql", "CREATE TABLE feature_cache", "err", err)
			}
			if _, err := db.Exec(`
				CREATE TABLE IF NOT EXISTS conversation_threads (
				id              INTEGER PRIMARY KEY AUTOINCREMENT,
				type            TEXT NOT NULL,
				goal            TEXT NOT NULL,
				status          TEXT DEFAULT "active",
				priority        REAL DEFAULT 0.5,
				created_at      INTEGER,
				last_touched_at INTEGER,
				resolved_at     INTEGER DEFAULT 0,
				key_messages    TEXT DEFAULT "",
				context_ref     TEXT DEFAULT "",
				best_approach   TEXT DEFAULT "",
				avoid_topics    TEXT DEFAULT "",
				outcome         TEXT DEFAULT "",
				learnings       TEXT DEFAULT ""
			);
			CREATE INDEX IF NOT EXISTS idx_threads_status ON conversation_threads(status);
			CREATE INDEX IF NOT EXISTS idx_threads_type ON conversation_threads(type);
		`); err != nil {
		slog.Warn("storage: migration failed", "sql", "CREATE TABLE conversation_threads", "err", err)
	}
	// Data migration: rename AI from 真央 to 诗音 in all stored text.
	// Gated behind schema_version key — only runs once, avoids 20 full-table scans on every startup.
	if shouldRunMigration(db, "schema_rename_migration") {
		renameMigrations := []struct{ table, column string }{
			{"self_profile", "content"},
			{"identity_nodes", "content"},
			{"chat_history", "content"},
			{"diary", "title"},
			{"diary", "summary"},
			{"topics", "name"},
			{"topics", "description"},
			{"facts", "content"},
			{"episodes", "title"},
			{"episodes", "summary"},
			{"memcell", "content"},
			{"strategy_principles", "situation"},
			{"strategy_principles", "good_strategy"},
			{"strategy_principles", "bad_strategy"},
			{"strategy_principles", "reason"},
			{"curiosity_items", "content"},
			{"behavior_patterns", "pattern"},
			{"behavior_patterns", "evidence"},
			{"behavior_patterns", "implication"},
			{"conversation_threads", "goal"},
			{"conversation_threads", "best_approach"},
		}
		for _, m := range renameMigrations {
			db.Exec(`UPDATE ` + m.table + ` SET ` + m.column + ` = REPLACE(` + m.column + `, '真央', '诗音')`)
		}
		markMigrationDone(db, "schema_rename_migration")
	}
	// Drop legacy tables replaced by unified counterparts.
	dropTables := []string{
		"activity_events", "event_segments", "cognitive_entries",
		"foresights", "knowledge_gaps", "inquiries", "proactive_questions", "mistake_notes",
	}
	for _, t := range dropTables {
		db.Exec(`DROP TABLE IF EXISTS ` + t)
	}
	// Fix identity_nodes with bogus confidence=0.0 from old write path.
	db.Exec(`UPDATE identity_nodes SET confidence = 0.85 WHERE confidence < 0.1 AND active = 1`)
	// Remove corrupted strategy_principles.
	db.Exec(`DELETE FROM strategy_principles WHERE situation = '' AND confidence < 0.1`)
	// Fix LLM-generated confidence values stored as percentages (85 → 0.85).
	db.Exec(`UPDATE strategy_principles SET confidence = confidence / 100 WHERE confidence > 1`)
	// Merge duplicate topic 8 ("诗音-自我成长") into topic 6.
	db.Exec(`UPDATE episodes SET topic_id = 6 WHERE topic_id = 8`)
	db.Exec(`DELETE FROM topics WHERE id = 8`)
	// Remove topic 5 ("主人-社交关系") with 0 episodes.
	db.Exec(`DELETE FROM topics WHERE id = 5 AND episode_count = 0`)
	// Clean any remaining orphan topics (episode_count = 0).
	db.Exec(`DELETE FROM topics WHERE episode_count = 0`)
	// Round excessive float precision in emotion_state.
	db.Exec(`UPDATE emotion_state SET
		valence = ROUND(valence, 3), arousal = ROUND(arousal, 3),
		dominance = ROUND(dominance, 3), intensity = ROUND(intensity, 3),
		affection = ROUND(affection, 3), worry = ROUND(worry, 3),
		curiosity = ROUND(curiosity, 3), sleepiness = ROUND(sleepiness, 3),
		playfulness = ROUND(playfulness, 3), loneliness = ROUND(loneliness, 3),
		confidence = ROUND(confidence, 3), annoyance = ROUND(annoyance, 3)
		WHERE id = 1`)
	// Remove error episode and orphaned topics.
	db.Exec(`DELETE FROM episodes WHERE title LIKE '%无法提取%'`)
	db.Exec(`DELETE FROM topics WHERE episode_count = 0`)
	// Fix episodes with missing end_time or hallucinated dates.
	db.Exec(`UPDATE episodes SET end_time = start_time + 3600 WHERE end_time = 0 AND start_time > 0`)
	db.Exec(`UPDATE episodes SET start_time = strftime('%s', '2026-06-01 00:00:00'), end_time = strftime('%s', '2026-06-01 23:59:59') WHERE start_time < strftime('%s', '2025-01-01')`)
	// Remove legacy garbage: OCR entries, broken outcomes.
	db.Exec(`DELETE FROM curiosity_items WHERE item_type = 'entry'`)
	db.Exec(`DELETE FROM action_outcomes WHERE action_type = ''`)
	return nil
}

func migrateVectorEncoding(db *sql.DB) {
	rows, err := db.Query(`SELECT id, vector FROM facts WHERE vector IS NOT NULL`)
	if err != nil {
		return
	}
	defer rows.Close()

	type rec struct {
		id  int64
		raw []byte
	}
	var updates []rec
	for rows.Next() {
		var r rec
		if rows.Scan(&r.id, &r.raw) != nil || len(r.raw) == 0 {
			continue
		}
		if r.raw[0] != '[' {
			continue
		}
		var vec []float32
		if json.Unmarshal(r.raw, &vec) == nil && len(vec) > 0 {
			r.raw = EncodeVector(vec)
			updates = append(updates, r)
		}
	}
	if len(updates) == 0 {
		return
	}
	for _, u := range updates {
		db.Exec(`UPDATE facts SET vector = ? WHERE id = ?`, u.raw, u.id)
	}
	slog.Info("storage: migrated fact vectors JSON→binary", "count", len(updates))
}

// ---- Profile ----

func (s *Store) SaveProfile(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO profile (key, value, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`,
		key, value, time.Now().Unix(),
	)
	return err
}

func (s *Store) LoadProfile() *domain.UserProfile {
	profile := &domain.UserProfile{}
	rows, err := s.db.Query(`SELECT key, value FROM profile`)
	if err != nil {
		return profile
	}
	defer rows.Close()
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			continue
		}
		switch key {
		case "name":
			profile.Name = value
		case "tech_stack":
			profile.TechStack = strings.Split(value, ",")
		}
	}
	return profile
}

func (s *Store) SaveProfileValue(key, value string) {
	if err := s.SaveProfile(key, value); err != nil {
		slog.Warn("storage: SaveProfileValue failed", "key", key, "err", err)
	}
}

func (s *Store) LoadProfileValue(key string) string {
	var v string
	s.db.QueryRow(`SELECT value FROM profile WHERE key = ?`, key).Scan(&v)
	return v
}

func (s *Store) SaveSelfProfile(content string) error {
	return s.SaveSelfProfileWithSource(content, "")
}

func (s *Store) SaveSelfProfileWithSource(content, source string) error {
	prefix := content
	if len([]rune(prefix)) > 80 {
		prefix = string([]rune(prefix)[:80])
	}
	var lastContent string
	s.db.QueryRow(`SELECT content FROM self_profile ORDER BY id DESC LIMIT 1`).Scan(&lastContent)
	if lastContent != "" {
		lastPrefix := lastContent
		if len([]rune(lastPrefix)) > 80 {
			lastPrefix = string([]rune(lastPrefix)[:80])
		}
		if prefix == lastPrefix {
			return nil
		}
	}
	_, err := s.db.Exec(
		`INSERT INTO self_profile (content, source, created_at) VALUES (?, ?, ?)`,
		content, source, time.Now().Unix(),
	)
	if err != nil {
		return err
	}
	s.db.Exec(`DELETE FROM self_profile WHERE id NOT IN (
		SELECT id FROM self_profile ORDER BY id DESC LIMIT 5
	)`)
	return nil
}

func (s *Store) LoadSelfProfile() string {
	var content string
	err := s.db.QueryRow(`SELECT content FROM self_profile ORDER BY id DESC LIMIT 1`).Scan(&content)
	if err != nil {
		return ""
	}
	return content
}

// ListSelfProfiles returns the most recent self_profile entries (up to 5).
func (s *Store) ListSelfProfiles() []string {
	rows, err := s.db.Query(`SELECT content FROM self_profile ORDER BY id DESC LIMIT 5`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var result []string
	for rows.Next() {
		var content string
		if rows.Scan(&content) == nil {
			result = append(result, content)
		}
	}
	return result
}

// ---- Feature Cache ----

// SaveFeatureCache upserts a feature value into the cache.
func (s *Store) SaveFeatureCache(featureName, valueJSON string, confidence float64, sampleCount int, ttlSeconds int) error {
	_, err := s.db.Exec(
		`INSERT INTO feature_cache (feature_name, value_json, confidence, sample_count, computed_at, ttl_seconds)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(feature_name) DO UPDATE SET
			value_json=excluded.value_json,
			confidence=excluded.confidence,
			sample_count=excluded.sample_count,
			computed_at=excluded.computed_at,
			ttl_seconds=excluded.ttl_seconds`,
		featureName, valueJSON, confidence, sampleCount, time.Now().Unix(), ttlSeconds,
	)
	return err
}

// LoadFeatureCache returns a cached feature value if it exists and has not expired.
// Returns empty string if missing or expired.
func (s *Store) LoadFeatureCache(featureName string) (valueJSON string, ok bool) {
	var computedAt, ttlSeconds int64
	err := s.db.QueryRow(
		`SELECT value_json, computed_at, ttl_seconds FROM feature_cache WHERE feature_name = ?`,
		featureName,
	).Scan(&valueJSON, &computedAt, &ttlSeconds)
	if err != nil {
		return "", false
	}
	if time.Now().Unix()-computedAt > ttlSeconds {
		return "", false // expired
	}
	return valueJSON, true
}

// LoadFeatureCacheMulti loads multiple cached features at once.
// Returns only the non-expired entries.
func (s *Store) LoadFeatureCacheMulti(featureNames []string) map[string]string {
	if len(featureNames) == 0 {
		return nil
	}
	result := make(map[string]string, len(featureNames))
	now := time.Now().Unix()
	for _, name := range featureNames {
		var valueJSON string
		var computedAt, ttlSeconds int64
		err := s.db.QueryRow(
			`SELECT value_json, computed_at, ttl_seconds FROM feature_cache WHERE feature_name = ?`,
			name,
		).Scan(&valueJSON, &computedAt, &ttlSeconds)
		if err != nil {
			continue
		}
		if now-computedAt <= ttlSeconds {
			result[name] = valueJSON
		}
	}
	return result
}
