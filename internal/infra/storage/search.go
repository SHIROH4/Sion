package storage

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"desktop-pet/internal/domain"
)

// ---- Keyword search ----

func (s *Store) SearchArchives(keyword string, limit int) ([]domain.SearchResult, error) {
	if limit <= 0 {
		limit = 5
	}
	pattern := "%" + keyword + "%"
	rows, err := s.db.Query(`
		SELECT name, level, summary, 'archive' as source
		FROM memory_archive
		WHERE summary LIKE ? OR name LIKE ?
		UNION ALL
		SELECT CAST(id AS TEXT), 99, content, 'fact' as source
		FROM facts
		WHERE content LIKE ?
		ORDER BY level DESC
		LIMIT ?`,
		pattern, pattern, pattern, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []domain.SearchResult
	for rows.Next() {
		var r domain.SearchResult
		if err := rows.Scan(&r.Name, &r.Level, &r.Summary, &r.Source); err != nil {
			continue
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// ---- Time helpers ----

func relativeTime(ts int64) string {
	d := time.Since(time.Unix(ts, 0))
	switch {
	case d < time.Minute:
		return "刚刚"
	case d < time.Hour:
		return fmt.Sprintf("%d分钟前", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("今天%02d:%02d", time.Unix(ts, 0).Hour(), time.Unix(ts, 0).Minute())
	case d < 48*time.Hour:
		return "昨天"
	default:
		return fmt.Sprintf("%d天前", int(d.Hours()/24))
	}
}

// ---- Tokenization ----

func isCJK(s string) bool {
	for _, r := range s {
		if r >= 0x4E00 && r <= 0x9FFF {
			return true
		}
	}
	return false
}

func Tokenize(text string) []string {
	var tokens []string
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return r == '，' || r == '。' || r == '！' || r == '？' || r == ' ' ||
			r == ',' || r == '.' || r == '!' || r == '?' || r == '\n' || r == '\t'
	})
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		if isCJK(f) {
			runes := []rune(f)
			tokens = append(tokens, f)
			for i := 0; i < len(runes)-1; i++ {
				tokens = append(tokens, string(runes[i:i+2]))
			}
		} else {
			tokens = append(tokens, f)
		}
	}
	return tokens
}

func keywordHitBonus(content, query string) float64 {
	keywords := Tokenize(query)
	hits := 0
	for _, kw := range keywords {
		if len([]rune(kw)) >= 2 && strings.Contains(content, kw) {
			hits++
		}
	}
	bonus := float64(hits) * 0.3
	if bonus > 1.0 {
		bonus = 1.0
	}
	return bonus
}

func normalizedDecay(f domain.FactEntry) float64 {
	w := DecayWeight(f.Importance, f.LastRecalledAt, f.RecallCount, 30, 0.15)
	maxW := DecayWeight(1.0, time.Now().Unix(), 100, 30, 0.15)
	if maxW == 0 {
		return 0
	}
	normalized := w / maxW
	if normalized > 1.0 {
		normalized = 1.0
	}
	return normalized
}

// ---- Unified Search ----

func (s *Store) UnifiedSearch(queryVector []float32, queryText string, topK int) ([]domain.UnifiedResult, error) {
	if topK <= 0 {
		topK = 10
	}
	// Sub-stores are temporarily unavailable during migration — use flat search only.
	all := s.flatFactSearch(queryVector, queryText)

	sort.Slice(all, func(i, j int) bool { return all[i].Score > all[j].Score })
	if len(all) > topK {
		all = all[:topK]
	}
	return all, nil
}

func (s *Store) flatFactSearch(queryVector []float32, queryText string) []domain.UnifiedResult {
	var all []domain.UnifiedResult

	facts := s.ListActiveFacts(0)
	for _, f := range facts {
		if len(f.Vector) == 0 {
			continue
		}
		cosSim := cosSim(queryVector, f.Vector)
		kwBonus := keywordHitBonus(f.Content, queryText)
		decay := normalizedDecay(f)
		score := cosSim*0.6 + kwBonus*0.2 + decay*0.2
		all = append(all, domain.UnifiedResult{
			Source: "fact", ID: f.ID, Content: f.Content,
			Score: score, DecayW: decay, CreatedAt: f.CreatedAt,
		})
	}

	return all
}
