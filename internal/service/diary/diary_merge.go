package diary

import (
	"fmt"
	"strings"
	"time"

	"desktop-pet/internal/domain"; infrastorage "desktop-pet/internal/infra/storage"
)

// MergeConfig controls the thresholds for diary merging.
type MergeConfig struct {
	// SimilarityThreshold is the minimum cosine similarity to consider two
	// entries as candidates for merging. Default: 0.75.
	SimilarityThreshold float64
	// MaxAgeDays limits merging to entries within this age window.
	// Entries older than this are considered "settled" and won't be merged.
	// Default: 30.
	MaxAgeDays int
	// MaxPerRun caps the number of merges per invocation to avoid runaway
	// consolidation. Default: 5.
	MaxPerRun int
}

// DiaryMerger periodically scans diary entries and consolidates similar ones.
type DiaryMerger struct {
	store  *DiaryStore
	config MergeConfig
	llm    func(prompt string) (string, error) // optional LLM for summary rewrite
}

// NewDiaryMerger creates a DiaryMerger with defaults filled in.
func NewDiaryMerger(store *DiaryStore) *DiaryMerger {
	return &DiaryMerger{
		store: store,
		config: MergeConfig{
			SimilarityThreshold: 0.75,
			MaxAgeDays:          30,
			MaxPerRun:           5,
		},
	}
}

// SetLLM injects an optional LLM function for rewriting merged summaries.
// When nil, summaries are concatenated directly.
func (m *DiaryMerger) SetLLM(fn func(string) (string, error)) {
	m.llm = fn
}

// Run scans recent diary entries for similar pairs and merges them.
// Returns the number of merges performed.
func (m *DiaryMerger) Run() (int, error) {
	cutoff := time.Now().Add(-time.Duration(m.config.MaxAgeDays) * 24 * time.Hour).Unix()

	// Load recent entries with vectors.
	entries := m.loadRecentWithVectors(cutoff)
	if len(entries) < 2 {
		return 0, nil
	}

	merged := 0
	mergedIDs := make(map[int64]bool)

	for i := 0; i < len(entries) && merged < m.config.MaxPerRun; i++ {
		if mergedIDs[entries[i].ID] {
			continue
		}
		for j := i + 1; j < len(entries) && merged < m.config.MaxPerRun; j++ {
			if mergedIDs[entries[j].ID] {
				continue
			}
			sim := infrastorage.CosineSimilarity(entries[i].Vector, entries[j].Vector)
			if sim >= m.config.SimilarityThreshold {
				if err := m.mergePair(entries[i], entries[j]); err != nil {
					return merged, fmt.Errorf("merge pair %d+%d: %w", entries[i].ID, entries[j].ID, err)
				}
				mergedIDs[entries[i].ID] = true
				mergedIDs[entries[j].ID] = true
				merged++
				break // each entry can only be merged once per run
			}
		}
	}

	return merged, nil
}

// loadRecentWithVectors returns diary entries within the age window that
// have non-nil vectors.
func (m *DiaryMerger) loadRecentWithVectors(cutoff int64) []domain.DiaryEntry {
	all := m.store.ListRecent(200)
	var out []domain.DiaryEntry
	for _, e := range all {
		if e.CreatedAt >= cutoff && len(e.Vector) > 0 {
			out = append(out, e)
		}
	}
	return out
}

// mergePair combines two similar diary entries into one and deletes the originals.
func (m *DiaryMerger) mergePair(a, b domain.DiaryEntry) error {
	// Normalise order: earlier entry first.
	if a.CreatedAt > b.CreatedAt {
		a, b = b, a
	}

	mergedTitle := mergeTitles(a.Title, b.Title)
	mergedSummary := mergeSummaries(a, b, m.llm)

	// Average the emotion scores, weighted equally.
	mergedValence := (a.EmotionValence + b.EmotionValence) / 2
	mergedArousal := (a.EmotionArousal + b.EmotionArousal) / 2

	// Average the vectors for continuity.
	mergedVec := averageVectors(a.Vector, b.Vector)

	merged := &domain.DiaryEntry{
		Title:          mergedTitle,
		Summary:        mergedSummary,
		Vector:         mergedVec,
		EmotionValence: mergedValence,
		EmotionArousal: mergedArousal,
		StartTime:      a.StartTime,
		EndTime:        b.EndTime,
		CreatedAt:      b.CreatedAt, // use later timestamp
	}

	if err := m.store.Save(merged); err != nil {
		return err
	}
	if err := m.store.Delete(a.ID); err != nil {
		return err
	}
	return m.store.Delete(b.ID)
}

func mergeTitles(a, b string) string {
	if a == b {
		return a
	}
	// Prefer the shorter, more specific title.
	if len(a) <= len(b) {
		return a
	}
	return b
}

func mergeSummaries(a, b domain.DiaryEntry, llm func(string) (string, error)) string {
	if llm != nil {
		prompt := fmt.Sprintf(
			`合并以下两段日记，保留关键信息和情感色彩，输出一段连贯的日记（200字以内）：

日记1（%s）：%s
日记2（%s）：%s

合并后的日记：`,
			time.Unix(a.CreatedAt, 0).Format("01-02 15:04"), a.Summary,
			time.Unix(b.CreatedAt, 0).Format("01-02 15:04"), b.Summary,
		)
		if result, err := llm(prompt); err == nil && strings.TrimSpace(result) != "" {
			return strings.TrimSpace(result)
		}
	}
	// Fallback: concatenate.
	return fmt.Sprintf("%s；%s", a.Summary, b.Summary)
}

// averageVectors computes the element-wise mean of two vectors.
func averageVectors(a, b []float32) []float32 {
	if len(a) != len(b) {
		return a
	}
	out := make([]float32, len(a))
	for i := range a {
		out[i] = (a[i] + b[i]) / 2
	}
	return out
}

