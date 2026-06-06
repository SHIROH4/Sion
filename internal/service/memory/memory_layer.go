package memory

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"desktop-pet/internal/domain"; "desktop-pet/internal/infra"
)

// SelfModel is the minimal interface for the AI's evolving self-image.
type SelfModel interface {
	Current() string
	Save(s string) error
}

// RetrievalContext combines results from all memory layers for LLM injection.
type RetrievalContext struct {
	CoreMemories []string            // L3 — core self-image
	Facts        []domain.FactEntry  // L2 — semantic facts
	Diaries      []domain.DiaryEntry // L1 — episodic diary entries
}

// MemoryLayer manages the 4-layer memory hierarchy and drives consolidation.
//
//	L0  working memory — SessionBuffer (short-term conversation buffer)
//	L1  episodic     — DiaryRepository (timestamped diary entries with vectors)
//	L2  semantic     — Store.facts + profile (learned facts about the user)
//	L3  core         — SelfModel (evolving self-image)
type MemoryLayer struct {
	mu          sync.Mutex
	Working     *SessionBuffer
	Episodic    domain.DiaryRepository
	Semantic    domain.MemoryStore
	Core        SelfModel
	buildPrompt func(string, string, string) string

	// Consolidation tracking (in-memory; reset on restart is acceptable —
	// consolidation is designed to be idempotent).
	lastAbstractDiaryCount int
	lastSelfUpdateAt       time.Time
	selfUpdateCount        int // progressive: 1d, 3d, 7d, then 30d
}

// NewMemoryLayer creates a MemoryLayer wrapping the four memory tiers.
func NewMemoryLayer(
	working *SessionBuffer,
	episodic domain.DiaryRepository,
	semantic domain.MemoryStore,
	core SelfModel,
	buildPrompt func(string, string, string) string,
) *MemoryLayer {
	return &MemoryLayer{
		Working:          working,
		Episodic:         episodic,
		Semantic:         semantic,
		Core:             core,
		buildPrompt:      buildPrompt,
		lastSelfUpdateAt: time.Now(), // prevent immediate L2→L3 trigger
	}
}

// Consolidate runs the upward abstraction pipeline:
//
//	L0→L1: diary generation (triggered by plugin's existing OnAfterChat path).
//	L1→L2: every 20 new diaries, LLM extracts/updates semantic facts.
//	L2→L3: every 30 days, LLM reflects on self-model evolution.
func (m *MemoryLayer) Consolidate(rawLLM func([]domain.Message) (string, error)) error {
	if rawLLM == nil {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Phase 2: L1 → L2 — abstract facts from recent diary entries.
	if err := m.consolidateDiariesToFacts(rawLLM); err != nil {
		return err
	}

	// Phase 3: L2 → L3 — reflect on self-model.
	if err := m.ConsolidateFactsToSelf(rawLLM); err != nil {
		return err
	}

	return nil
}

// Retrieve gathers memories from all layers. When Store has embedSvc and a query
// vector is provided, L2 uses UnifiedSearch; otherwise it falls back to the
// DecayWeight ranking. OnBeforeChat now uses its own UnifiedSearch path directly;
// this method remains for backward compatibility (e.g. background cognition).
func (m *MemoryLayer) Retrieve(queryVector []float32) *RetrievalContext {
	ctx := &RetrievalContext{}

	// L3: core self-image (always included).
	if m.Core != nil {
		self := m.Core.Current()
		if self != "" {
			ctx.CoreMemories = []string{self}
		}
	}

	// L2: semantic facts — UnifiedSearch when embedding is available; else DecayWeight.
	if m.Semantic != nil {
		if queryVector != nil {
			results, _ := m.Semantic.UnifiedSearch(queryVector, "", 5)
			for _, r := range results {
				if r.Source == "fact" {
					ctx.Facts = append(ctx.Facts, domain.FactEntry{
						ID: r.ID, Content: r.Content,
					})
				}
			}
		} else {
			facts := m.Semantic.ListActiveFacts(ActiveThreshold)
			type weighted struct {
				f domain.FactEntry
				w float64
			}
			var ranked []weighted
			for _, f := range facts {
				w := DecayWeight(f.Importance, f.LastRecalledAt, f.RecallCount, 30, 0.15)
				if w >= ActiveThreshold {
					ranked = append(ranked, weighted{f, w})
				}
			}
			sort.Slice(ranked, func(i, j int) bool { return ranked[i].w > ranked[j].w })
			limit := len(ranked)
			if limit > 5 {
				limit = 5
			}
			for i := 0; i < limit; i++ {
				ctx.Facts = append(ctx.Facts, ranked[i].f)
			}
		}
	}

	// L1: episodic diary entries — vector search when query is available,
	// fall back to recent diaries otherwise.
	if m.Episodic != nil {
		if queryVector != nil {
			diaries, _ := m.Episodic.Search(queryVector, 5)
			ctx.Diaries = diaries
		} else {
			ctx.Diaries = m.Episodic.ListRecent(10)
		}
	}

	return ctx
}

// Forget scans all layers and archives entries whose decay weight has fallen
// below the retention threshold.
func (m *MemoryLayer) Forget() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// L0 cleanup + L2 facts: clean chat_history, archive decayed facts.
	if m.Semantic != nil {
		// Clean chat_history older than 30 days.
		m.Semantic.CleanOldHistory(30)
		// Archive facts whose decay weight has fallen below threshold.
		facts := m.Semantic.ListActiveFacts(0)
		for _, f := range facts {
			w := DecayWeight(f.Importance, f.LastRecalledAt, f.RecallCount, 30, 0.15)
			if w < 0.05 {
				m.Semantic.ArchiveFact(f.ID)
			}
		}
	}
	if m.Episodic != nil {
		cutoff := time.Now().Add(-90 * 24 * time.Hour).Unix()
		recent := m.Episodic.ListRecent(200)
		for _, d := range recent {
			if d.CreatedAt < cutoff {
				intensity := (math.Abs(d.EmotionValence) + math.Abs(d.EmotionArousal)) / 2
				if intensity < 0.2 {
					m.Episodic.ArchiveDiary(d.ID) // soft-delete, preserves data
				}
			}
		}
	}
}

// ---- internal consolidation steps ----

// consolidateDiariesToFacts triggers when >= 20 new diaries have accumulated.
// It feeds the 20 most recent diaries plus existing facts to the LLM and saves
// any newly extracted facts.
func (m *MemoryLayer) consolidateDiariesToFacts(rawLLM func([]domain.Message) (string, error)) error {
	if m.Episodic == nil || m.Semantic == nil {
		return nil
	}

	totalDiaries := m.Episodic.Count()
	newDiaries := totalDiaries - m.lastAbstractDiaryCount
	if newDiaries < 20 {
		return nil
	}

	diaries := m.Episodic.ListRecent(20)
	if len(diaries) == 0 {
		return nil
	}

	facts := m.Semantic.LoadFacts()

	prompt := buildL1ToL2Prompt(diaries, facts)
	result, err := rawLLM([]domain.Message{{Role: "user", Content: prompt}})
	if err != nil {
		return nil
	}

	newFacts := parseFactList(result)
	for _, f := range newFacts {
		f = strings.TrimSpace(f)
		if f != "" {
			m.Semantic.SaveFact(f, "chat")
		}
	}

	m.lastAbstractDiaryCount = totalDiaries
	return nil
}

// shouldSelfUpdate uses progressive intervals: 1d, 3d, 7d, then 30d thereafter.
func (m *MemoryLayer) shouldSelfUpdate() bool {
	intervals := []time.Duration{24 * time.Hour, 3 * 24 * time.Hour, 7 * 24 * time.Hour}
	if m.selfUpdateCount < len(intervals) {
		return time.Since(m.lastSelfUpdateAt) > intervals[m.selfUpdateCount]
	}
	return time.Since(m.lastSelfUpdateAt) > 30*24*time.Hour
}

// ConsolidateFactsToSelf feeds the current self-model, important recent facts,
// and recent diaries to the LLM for introspective self-model update.
func (m *MemoryLayer) ConsolidateFactsToSelf(rawLLM func([]domain.Message) (string, error)) error {
	if !m.shouldSelfUpdate() {
		return nil
	}
	if m.Core == nil || m.Episodic == nil || m.Semantic == nil {
		return nil
	}

	oldSelf := m.Core.Current()
	if oldSelf == "" {
		return nil
	}

	facts := m.Semantic.ListActiveFacts(CoreThreshold)
	diaries := m.Episodic.ListRecent(10)

	var factLines []string
	for _, f := range facts {
		factLines = append(factLines, "- "+f.Content)
	}

	var diaryLines []string
	for _, d := range diaries {
		diaryLines = append(diaryLines, fmt.Sprintf("[%s] %s", d.Title, d.Summary))
	}

	prompt := m.buildPrompt(
		strings.Join(diaryLines, "\n"),
		oldSelf,
		strings.Join(factLines, "\n"),
	)

	result, err := rawLLM([]domain.Message{{Role: "user", Content: prompt}})
	if err != nil {
		return nil
	}

	newSelf := strings.TrimSpace(result)
	if newSelf != "" && newSelf != oldSelf {
		m.Core.Save(newSelf)
	}

	m.lastSelfUpdateAt = time.Now()
	m.selfUpdateCount++
	return nil
}

// ---- prompt builders ----

// buildL1ToL2Prompt constructs the LLM prompt for extracting semantic facts
// from recent diary entries.
func buildL1ToL2Prompt(diaries []domain.DiaryEntry, existingFacts []string) string {
	var diaryText strings.Builder
	for i, d := range diaries {
		diaryText.WriteString(fmt.Sprintf("%d. [%s] %s\n   %s\n", i+1, d.Title, EmotionTag(d.EmotionValence, d.EmotionArousal), d.Summary))
	}

	var factsText string
	if len(existingFacts) > 0 {
		factsText = strings.Join(existingFacts, "\n")
	} else {
		factsText = "(暂无已记录的事实)"
	}

	return fmt.Sprintf(l1ToL2PromptTemplate, diaryText.String(), factsText)
}

// parseFactList extracts a list of fact strings from the LLM's JSON response.
func parseFactList(raw string) []string {
	raw = infra.CleanJSON(raw)
	var facts []string
	if err := json.Unmarshal([]byte(raw), &facts); err != nil {
		// Fallback: treat each non-empty line as a fact.
		for _, line := range strings.Split(raw, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				facts = append(facts, line)
			}
		}
	}
	return facts
}

const l1ToL2PromptTemplate = `## 语义抽象：日记 → 事实

从以下日记条目中提取关于用户的具体事实知识。
如果信息与已有事实冲突，用新信息覆盖旧信息（不要重复已有事实）。

### 最近的日记
%s

### 已有的事实
%s

### 提取规则
1. 提取具体的、可验证的事实（"主人用Go语言"、"主人生日是6月15日"）
2. 偏好信息也很重要（"主人喜欢深色主题"、"主人讨厌开会"）
3. 不要提取情绪波动或一时的心情（日记里记录了情绪即可）
4. 不要提取纯技术问题讨论（"讨论了Rust trait"不是用户信息）
5. 如果日记中没有新的用户信息，输出空数组 []

### 输出格式
以 JSON 字符串数组输出，每条一个事实：
["主人的生日是6月15日", "主人使用Neovim编辑器", "主人最近在学分布式系统"]

只输出 JSON 数组。`
