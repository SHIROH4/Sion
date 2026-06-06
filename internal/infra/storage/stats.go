package storage

import "time"

// CountTodayMessages returns the number of chat messages created today.
func (s *Store) CountTodayMessages() int {
	todayStart := time.Now().Truncate(24 * time.Hour).Unix()
	var count int
	s.db.QueryRow(
		`SELECT COUNT(*) FROM chat_history WHERE created_at >= ?`,
		todayStart,
	).Scan(&count)
	return count
}
