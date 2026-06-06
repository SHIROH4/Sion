package memory

import (
	"encoding/json"
	"fmt"
	"strings"

	"desktop-pet/internal/domain"; "desktop-pet/internal/infra"; "desktop-pet/internal/infra/storage"
)

// EpisodeRepo is the minimal interface EpisodeStore needs from the infra layer.
type EpisodeRepo interface {
	domain.EpisodeRepository
	IncrementFactCount(id int64) error
	UpdateCentroid(id int64, newVec []float32) error
	UpdateSummary(id int64, title, summary string) error
}

// EpisodeStore manages episode clustering and lifecycle.
type EpisodeStore struct {
	EpisodeRepo
	vectorize func(string) ([]float32, error)
}

// NewEpisodeStore creates an EpisodeStore backed by the given repo.
func NewEpisodeStore(repo EpisodeRepo) *EpisodeStore {
	return &EpisodeStore{EpisodeRepo: repo}
}

// SetVectorize injects the embedding function for centroid computation.
func (s *EpisodeStore) SetVectorize(fn func(string) ([]float32, error)) {
	s.vectorize = fn
}

// FindOrCreate finds the best-matching episode for a new fact using centroid
// cosine similarity. Cosine > 0.65 → merge into existing; otherwise → create new.
func (s *EpisodeStore) FindOrCreate(fact domain.FactEntry) (int64, error) {
	if len(fact.Vector) == 0 {
		return s.EpisodeRepo.Create(fact)
	}

	episodes := s.EpisodeRepo.ListActive()

	var bestID int64
	var bestScore float64
	for _, ep := range episodes {
		if len(ep.Centroid) == 0 {
			continue
		}
		score := storage.CosineSimilarity(fact.Vector, ep.Centroid)
		if score > bestScore {
			bestScore = score
			bestID = ep.ID
		}
	}

	if bestScore > 0.65 && bestID > 0 {
		if err := s.EpisodeRepo.UpdateCentroid(bestID, fact.Vector); err != nil {
			return 0, err
		}
		if err := s.EpisodeRepo.IncrementFactCount(bestID); err != nil {
			return 0, err
		}
		return bestID, nil
	}

	return s.EpisodeRepo.Create(fact)
}

// SummarizeEpisode uses LLM to generate a narrative summary for an episode
// once it accumulates enough facts (>= 3).
func (s *EpisodeStore) SummarizeEpisode(id int64, rawLLM func([]domain.Message) (string, error)) error {
	facts := s.EpisodeRepo.GetFacts(id)
	if len(facts) < 3 {
		return nil
	}
	if rawLLM == nil {
		return nil
	}

	var factsText strings.Builder
	for _, f := range facts {
		factsText.WriteString(fmt.Sprintf("- [%s] %s (importance:%.1f)\n", f.FactRole, f.Content, f.Importance))
	}

	prompt := fmt.Sprintf(episodeSummaryPrompt, factsText.String())
	result, err := rawLLM([]domain.Message{{Role: "user", Content: prompt}})
	if err != nil {
		return err
	}

	var resp struct {
		Title   string `json:"title"`
		Summary string `json:"summary"`
	}
	if err := json.Unmarshal([]byte(infra.CleanJSON(result)), &resp); err != nil {
		return err
	}

	return s.EpisodeRepo.UpdateSummary(id, resp.Title, resp.Summary)
}

const episodeSummaryPrompt = `## 事件总结

你是诗音的记忆归档系统。从以下原子事实中生成一个事件标题和叙事总结。

### 原子事实
%s

### 要求
1. 标题 5-15 字，概括事件核心
2. 总结 2-4 句话，按时间顺序叙述
3. 包含关键因果: "因为 A 所以 B"
4. 第一人称，像写日记一样自然

### 输出
{"title": "...", "summary": "..."}`
