package storage

import (
	"database/sql"
	"time"

	"desktop-pet/internal/domain"
)

// EnsureEmotionSchema creates the emotion_state table if it doesn't exist.
func EnsureEmotionSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS emotion_state (
			id            INTEGER PRIMARY KEY CHECK (id = 1),
			valence       REAL DEFAULT 0,
			arousal       REAL DEFAULT 0,
			dominance     REAL DEFAULT 0,
			primary_emo   TEXT DEFAULT 'neutral',
			intensity     REAL DEFAULT 0,
			affection     REAL DEFAULT 0.5,
			worry         REAL DEFAULT 0.15,
			curiosity     REAL DEFAULT 0.4,
			sleepiness    REAL DEFAULT 0.25,
			playfulness   REAL DEFAULT 0.35,
			loneliness    REAL DEFAULT 0.2,
			confidence    REAL DEFAULT 0.5,
			annoyance     REAL DEFAULT 0.05,
			last_interact INTEGER DEFAULT 0,
			updated_at    INTEGER
		)
	`)
	return err
}

// SaveEmotion upserts the current emotion state into SQLite.
func (s *Store) SaveEmotion(es domain.EmotionState, ev domain.EmotionVector, lastInteract time.Time) error {
	if err := EnsureEmotionSchema(s.db); err != nil {
		return err
	}
	_, err := s.db.Exec(`
		INSERT INTO emotion_state (id, valence, arousal, dominance, primary_emo, intensity,
			affection, worry, curiosity, sleepiness, playfulness, loneliness, confidence, annoyance,
			last_interact, updated_at)
		VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			valence=excluded.valence, arousal=excluded.arousal, dominance=excluded.dominance,
			primary_emo=excluded.primary_emo, intensity=excluded.intensity,
			affection=excluded.affection, worry=excluded.worry, curiosity=excluded.curiosity,
			sleepiness=excluded.sleepiness, playfulness=excluded.playfulness,
			loneliness=excluded.loneliness, confidence=excluded.confidence,
			annoyance=excluded.annoyance, last_interact=excluded.last_interact,
			updated_at=excluded.updated_at`,
		es.Valence, es.Arousal, es.Dominance, es.Primary, es.Intensity,
		ev.Affection, ev.Worry, ev.Curiosity, ev.Sleepiness, ev.Playfulness,
		ev.Loneliness, ev.Confidence, ev.Annoyance,
		lastInteract.Unix(), time.Now().Unix(),
	)
	return err
}

// LoadEmotion reads the persisted emotion state from SQLite.
// Returns false if no saved state exists.
func (s *Store) LoadEmotion() (domain.EmotionState, domain.EmotionVector, time.Time, bool) {
	if err := EnsureEmotionSchema(s.db); err != nil {
		return domain.EmotionState{}, domain.EmotionVector{}, time.Time{}, false
	}
	var (
		es domain.EmotionState
		ev domain.EmotionVector
		li int64
	)
	err := s.db.QueryRow(`
		SELECT valence, arousal, dominance, primary_emo, intensity,
			affection, worry, curiosity, sleepiness, playfulness, loneliness, confidence, annoyance,
			last_interact
		FROM emotion_state WHERE id = 1`,
	).Scan(&es.Valence, &es.Arousal, &es.Dominance, &es.Primary, &es.Intensity,
		&ev.Affection, &ev.Worry, &ev.Curiosity, &ev.Sleepiness, &ev.Playfulness,
		&ev.Loneliness, &ev.Confidence, &ev.Annoyance, &li)
	if err != nil {
		return domain.EmotionState{}, domain.EmotionVector{}, time.Time{}, false
	}
	return es, ev, time.Unix(li, 0), true
}
