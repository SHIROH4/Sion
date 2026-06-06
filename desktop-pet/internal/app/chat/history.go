package chat

import (
	"regexp"
	"strings"

	"desktop-pet/internal/domain"
)

var filterMsgTimestampRe = regexp.MustCompile(`^\[\d{2}:\d{2}\]\s*`)

// LoadRecentHistory returns the most recent chat messages, filtering out
// archive markers and stripping timestamps.
func LoadRecentHistory(store domain.MemoryStore, limit int) []domain.Message {
	if store == nil {
		return nil
	}
	msgs, _ := store.LoadHistory(limit)
	result := make([]domain.Message, 0, len(msgs))
	for _, m := range msgs {
		if strings.HasPrefix(m.Content, "[记忆存档") {
			continue
		}
		m.Content = filterMsgTimestampRe.ReplaceAllString(m.Content, "")
		m.Content = strings.TrimSpace(m.Content)
		if m.Content == "" {
			continue
		}
		result = append(result, m)
	}
	return result
}

// FilterTimestamp removes the [HH:MM] prefix from a message content.
func FilterTimestamp(content string) string {
	s := filterMsgTimestampRe.ReplaceAllString(content, "")
	return strings.TrimSpace(s)
}
