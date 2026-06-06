package memory

import (
	"desktop-pet/internal/domain"; "desktop-pet/internal/infra"; "desktop-pet/internal/infra/storage"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// FactConsolidator periodically merges semantically similar facts, archives
// expired temporal facts, and resolves contradictions. The similarity threshold
// self-tunes based on merge quality (useful merges vs noise).
type FactConsolidator struct {
	store     domain.MemoryStore
	rawLLM    func([]domain.Message) (string, error)
	vectorize func(string) ([]float32, error)

	lastRun        time.Time
	simThreshold   float64
	minClusterSize int

	// Adaptive threshold tracking.
	totalRuns      int
	totalMerged    int
	totalDiscarded int
}

// NewFactConsolidator creates a FactConsolidator.
func NewFactConsolidator(
	store domain.MemoryStore,
	rawLLM func([]domain.Message) (string, error),
	vectorize func(string) ([]float32, error),
) *FactConsolidator {
	return &FactConsolidator{
		store:          store,
		rawLLM:         rawLLM,
		vectorize:      vectorize,
		simThreshold:   0.85,
		minClusterSize: 2,
	}
}

// ShouldRun returns true if at least 12 hours have passed since the last run.
func (c *FactConsolidator) ShouldRun() bool {
	return time.Since(c.lastRun) > 12*time.Hour
}

// adaptiveTune adjusts the similarity threshold based on merge quality.
// Too few merges (clusters found but LLM says keep_all) → lower threshold.
// Too many discarded (LLM says discard_all) → raise threshold (too noisy).
func (c *FactConsolidator) adaptiveTune(clustersFound, merged, discarded int) {
	c.totalRuns++
	c.totalMerged += merged
	c.totalDiscarded += discarded

	if c.totalRuns < 3 {
		return // need enough data
	}

	avgClusters := float64(clustersFound)
	mergeRate := 0.0
	discardRate := 0.0
	if avgClusters > 0 {
		mergeRate = float64(merged) / avgClusters
		discardRate = float64(discarded) / avgClusters
	}

	// Merge rate < 0.3: threshold too strict, lower it.
	if mergeRate < 0.3 && c.simThreshold > 0.70 {
		c.simThreshold -= 0.03
	}
	// Discard rate > 0.5: too much noise, raise threshold.
	if discardRate > 0.5 && c.simThreshold < 0.95 {
		c.simThreshold += 0.03
	}
}

// LastRun returns the timestamp of the last consolidation.
func (c *FactConsolidator) LastRun() time.Time { return c.lastRun }

// ============================================================
// Phase 1: Semantic clustering (local, no LLM)
// ============================================================

type factCluster struct {
	Facts      []domain.FactEntry
	Centroid   []float32
	Similarity float64 // average intra-cluster similarity
}

// clusterFacts loads active facts with vectors and groups them by cosine similarity.
func (c *FactConsolidator) clusterFacts() []factCluster {
	all := c.store.ListActiveFacts(0) // 0 = all active
	if len(all) < 2 {
		return nil
	}

	// Build adjacency: pair (i,j) where similarity > threshold.
	type edge struct{ i, j int }
	var edges []edge
	for i := 0; i < len(all); i++ {
		if len(all[i].Vector) == 0 {
			continue
		}
		for j := i + 1; j < len(all); j++ {
			if len(all[j].Vector) == 0 {
				continue
			}
			sim := storage.CosineSimilarity(all[i].Vector, all[j].Vector)
			if sim >= c.simThreshold {
				edges = append(edges, edge{i, j})
			}
		}
	}

	if len(edges) == 0 {
		return nil
	}

	// Union-Find to build connected components.
	parent := make([]int, len(all))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(x int) int {
		if parent[x] != x {
			parent[x] = find(parent[x])
		}
		return parent[x]
	}
	union := func(a, b int) {
		parent[find(a)] = find(b)
	}
	for _, e := range edges {
		union(e.i, e.j)
	}

	// Collect clusters.
	groups := make(map[int][]int)
	for i := range all {
		root := find(i)
		groups[root] = append(groups[root], i)
	}

	var clusters []factCluster
	for _, indices := range groups {
		if len(indices) < c.minClusterSize {
			continue
		}
		facts := make([]domain.FactEntry, len(indices))
		for k, idx := range indices {
			facts[k] = all[idx]
		}
		clusters = append(clusters, factCluster{
			Facts:      facts,
			Similarity: c.avgClusterSim(facts),
		})
	}

	return clusters
}

func (c *FactConsolidator) avgClusterSim(facts []domain.FactEntry) float64 {
	if len(facts) < 2 {
		return 1.0
	}
	total := 0.0
	count := 0
	for i := 0; i < len(facts); i++ {
		if len(facts[i].Vector) == 0 {
			continue
		}
		for j := i + 1; j < len(facts); j++ {
			if len(facts[j].Vector) == 0 {
				continue
			}
			total += storage.CosineSimilarity(facts[i].Vector, facts[j].Vector)
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return total / float64(count)
}

// ============================================================
// Phase 2: LLM merge (one LLM call per cluster, or batch)
// ============================================================

type mergeDecision struct {
	Action     string  `json:"action"`    // "merge" | "keep_all" | "discard_all"
	Merged     string  `json:"merged"`    // merged fact content (when action=merge)
	Role       string  `json:"fact_role"` // core | temporal | detail | context
	Importance float64 `json:"importance"`
	Reason     string  `json:"reason"`
}

const mergePromptTemplate = `## 事实合并

以下是语义相似的一组事实，请判断应该如何处理。

### 事实列表
%s

### 指令
1. 如果多条事实表达的是同一件事，用 ` + "`" + `merge` + "`" + ` 合并为一条简洁的陈述（第三人称，以"主人"为主语）
2. 如果事实不同但相关，用 ` + "`" + `keep_all` + "`" + `
3. 如果都是临时/过时信息（如"现在时间是晚上"、"主人说了你好"），用 ` + "`" + `discard_all` + "`" + `
4. importance: merge时给新事实的重要性，keep_all时填0（忽略）
5. fact_role: core=稳定偏好, temporal=时间相关, detail=补充信息

### 输出格式
{"action":"merge","merged":"合并后的事实","fact_role":"core","importance":0.8,"reason":"三条表达同一偏好"}
或
{"action":"keep_all","merged":"","fact_role":"","importance":0,"reason":"事实相关但不同"}
或
{"action":"discard_all","merged":"","fact_role":"","importance":0,"reason":"临时信息已过期"}

只输出一个JSON对象，不要附加其他文字。`

func (c *FactConsolidator) buildMergePrompt(facts []domain.FactEntry) string {
	var sb strings.Builder
	for i, f := range facts {
		sb.WriteString(fmt.Sprintf("%d. %s [importance:%.1f, role:%s]\n",
			i+1, f.Content, f.Importance, f.FactRole))
	}
	return fmt.Sprintf(mergePromptTemplate, sb.String())
}

func (c *FactConsolidator) callLLMForMerge(facts []domain.FactEntry) (*mergeDecision, error) {
	if c.rawLLM == nil {
		return nil, fmt.Errorf("no LLM available")
	}
	prompt := c.buildMergePrompt(facts)
	reply, err := c.rawLLM([]domain.Message{{Role: "user", Content: prompt}})
	if err != nil {
		return nil, err
	}

	reply = infra.CleanJSON(strings.TrimSpace(reply))
	var decision mergeDecision
	if err := json.Unmarshal([]byte(reply), &decision); err != nil {
		short := reply
		if len([]rune(short)) > 200 {
			short = string([]rune(short)[:200])
		}
		return nil, fmt.Errorf("parse merge decision: %w (reply: %s)", err, short)
	}
	return &decision, nil
}

// ============================================================
// Phase 3: Execute
// ============================================================

// consolidateResult tracks the outcome of one consolidation run.
type consolidateResult struct {
	ClustersFound int
	Merged        int // clusters where merge was applied
	Discarded     int // clusters where facts were discarded
	Kept          int // clusters where all facts were kept
	FactsArchived int // individual facts archived
}

// Run executes a full consolidation cycle.
func (c *FactConsolidator) Run() (*consolidateResult, error) {
	result := &consolidateResult{}

	clusters := c.clusterFacts()
	result.ClustersFound = len(clusters)
	if len(clusters) == 0 {
		c.lastRun = time.Now()
		return result, nil
	}

	for _, cl := range clusters {
		decision, err := c.callLLMForMerge(cl.Facts)
		if err != nil {
			// Skip this cluster on error, try the next one.
			continue
		}

		switch decision.Action {
		case "merge":
			if decision.Merged == "" {
				continue
			}
			if err := c.applyMerge(cl.Facts, decision); err != nil {
				continue
			}
			result.Merged++
			result.FactsArchived += len(cl.Facts)

		case "discard_all":
			for _, f := range cl.Facts {
				_ = c.store.ArchiveFact(f.ID)
			}
			result.Discarded++
			result.FactsArchived += len(cl.Facts)

		case "keep_all":
			result.Kept++
		}
	}

	// Also clean up expired temporal facts.
	n := c.cleanExpiredTemporal()
	result.FactsArchived += n

	// Self-tune similarity threshold based on merge quality.
	c.adaptiveTune(result.ClustersFound, result.Merged, result.Discarded)

	c.lastRun = time.Now()
	return result, nil
}

// applyMerge archives the old facts and saves the merged one with vector.
func (c *FactConsolidator) applyMerge(oldFacts []domain.FactEntry, decision *mergeDecision) error {
	role := domain.FactRole(decision.Role)
	if role == "" {
		role = domain.RoleCore
	}
	if decision.Importance <= 0 {
		decision.Importance = 0.6
	}

	input := domain.AtomicFactInput{
		Content:    decision.Merged,
		Importance: decision.Importance,
		FactRole:   role,
		Source:     "consolidation",
	}

	// Vectorize the merged fact.
	if c.vectorize != nil {
		vec, err := c.vectorize(decision.Merged)
		if err == nil && len(vec) > 0 {
			// Save manually to include the vector.
			newID, err := c.store.SaveFactWithVector(input, vec)
			if err != nil {
				return err
			}
			// Archive old facts, linking to the new one.
			for _, f := range oldFacts {
				_ = c.store.ReplaceFact(f.ID, newID)
				_ = c.store.ArchiveFact(f.ID)
			}
			return nil
		}
	}

	// Fallback: save without pre-computed vector (will be vectorized later).
	if err := c.store.SaveAtomicFact(input); err != nil {
		return err
	}
	for _, f := range oldFacts {
		_ = c.store.ArchiveFact(f.ID)
	}
	return nil
}

// cleanExpiredTemporal archives temporal facts whose end_time has passed.
func (c *FactConsolidator) cleanExpiredTemporal() int {
	now := time.Now().Unix()
	facts := c.store.ListActiveFacts(0)
	var ids []int64
	for _, f := range facts {
		if f.FactRole == domain.RoleTemporal && f.EndTime > 0 && f.EndTime < now {
			ids = append(ids, f.ID)
		}
	}
	for _, id := range ids {
		_ = c.store.ArchiveFact(id)
	}
	return len(ids)
}
