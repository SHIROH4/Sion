package identity

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"desktop-pet/internal/domain"
)

// IdentityGraph manages the AI's identity knowledge graph — a structured,
// retrievable representation of "who 诗音 is", separate from episodic memory.
type IdentityGraph struct {
	mu        sync.Mutex
	nodes     map[int64]*domain.IdentityNode
	vectorize func(string) ([]float32, error)
	repo      domain.IdentityRepository
}

// NewIdentityGraph creates an IdentityGraph backed by the given repository.
func NewIdentityGraph(repo domain.IdentityRepository, vectorize func(string) ([]float32, error)) *IdentityGraph {
	return &IdentityGraph{
		nodes:     make(map[int64]*domain.IdentityNode),
		vectorize: vectorize,
		repo:      repo,
	}
}

// Load reads all active identity nodes from the repository into memory.
// If no nodes exist, it initialises the default identity graph automatically.
func (g *IdentityGraph) Load() error {
	nodes, err := g.repo.ListActiveIdentityNodes()
	if err != nil {
		return fmt.Errorf("identity_graph: load: %w", err)
	}

	g.mu.Lock()
	for i := range nodes {
		g.nodes[nodes[i].ID] = &nodes[i]
	}
	needsInit := len(g.nodes) == 0
	g.mu.Unlock()

	if needsInit {
		return g.initializeDefaults()
	}
	return nil
}

// Retrieve searches for identity nodes semantically relevant to the given
// context and returns the top-K node contents as strings.
func (g *IdentityGraph) Retrieve(context string, topK int) []string {
	g.mu.Lock()
	defer g.mu.Unlock()

	if len(g.nodes) == 0 || topK <= 0 {
		return nil
	}

	// If no vectorizer, fall back to MatchCount-based ranking.
	if g.vectorize == nil {
		return g.retrieveByFrequency(context, topK)
	}

	queryVec, err := g.vectorize(context)
	if err != nil {
		return g.retrieveByFrequency(context, topK)
	}

	type scored struct {
		content string
		id      int64
		score   float64
	}
	var results []scored

	for _, n := range g.nodes {
		if !n.Active {
			continue
		}
		s := cosineSimilarity(queryVec, n.Embedding)
		if s > 0.3 {
			results = append(results, scored{content: n.Content, id: n.ID, score: s})
		}
	}

	sort.Slice(results, func(i, j int) bool { return results[i].score > results[j].score })

	if len(results) > topK {
		results = results[:topK]
	}

	// Update match stats (memory + repo).
	now := time.Now().Unix()
	for _, r := range results {
		if n, ok := g.nodes[r.id]; ok {
			n.LastMatched = now
			n.MatchCount++
			_ = g.repo.UpdateIdentityNodeMatchStats(n.ID, n.LastMatched, n.MatchCount)
		}
	}

	contents := make([]string, len(results))
	for i, r := range results {
		contents[i] = r.content
	}
	return contents
}

// retrieveByFrequency falls back to keyword overlap + MatchCount ranking.
func (g *IdentityGraph) retrieveByFrequency(context string, topK int) []string {
	keywords := strings.Fields(strings.ToLower(context))

	type scored struct {
		content string
		id      int64
		score   float64
	}
	var results []scored

	for _, n := range g.nodes {
		if !n.Active {
			continue
		}
		lower := strings.ToLower(n.Content)
		hits := 0
		for _, kw := range keywords {
			if strings.Contains(lower, kw) {
				hits++
			}
		}
		if hits > 0 {
			results = append(results, scored{
				content: n.Content,
				id:      n.ID,
				score:   float64(hits) + float64(n.MatchCount)*0.1,
			})
		}
	}

	sort.Slice(results, func(i, j int) bool { return results[i].score > results[j].score })
	if len(results) > topK {
		results = results[:topK]
	}

	now := time.Now().Unix()
	for _, r := range results {
		if n, ok := g.nodes[r.id]; ok {
			n.LastMatched = now
			n.MatchCount++
			_ = g.repo.UpdateIdentityNodeMatchStats(n.ID, n.LastMatched, n.MatchCount)
		}
	}

	contents := make([]string, len(results))
	for i, r := range results {
		contents[i] = r.content
	}
	return contents
}

// Upsert creates or updates an identity node. If ID is 0, a new node is
// inserted; otherwise the existing node is updated.
func (g *IdentityGraph) Upsert(node *domain.IdentityNode) error {
	// Compute embedding if vectorizer is available and embedding is empty.
	if g.vectorize != nil && len(node.Embedding) == 0 {
		vec, err := g.vectorize(node.Content)
		if err == nil {
			node.Embedding = vec
		}
	}

	domainNode := &domain.IdentityNode{
		ID:          node.ID,
		Type:        domain.IdentityNodeType(node.Type),
		Content:     node.Content,
		Confidence:  node.Confidence,
		Embedding:   node.Embedding,
		CreatedAt:   node.CreatedAt,
		UpdatedAt:   node.UpdatedAt,
		LastMatched: node.LastMatched,
		MatchCount:  node.MatchCount,
		Active:      node.Active,
	}
	if err := g.repo.UpsertIdentityNode(domainNode); err != nil {
		return err
	}

	// Sync back the ID and state from the repo result.
	node.ID = domainNode.ID
	node.Active = domainNode.Active
	if node.CreatedAt == 0 {
		node.CreatedAt = domainNode.CreatedAt
	}
	node.UpdatedAt = domainNode.UpdatedAt

	g.mu.Lock()
	g.nodes[node.ID] = node
	g.mu.Unlock()

	return nil
}

// Deactivate marks an identity node as inactive (soft delete).
func (g *IdentityGraph) Deactivate(id int64) error {
	if err := g.repo.DeactivateIdentityNode(id); err != nil {
		return fmt.Errorf("identity_graph: deactivate: %w", err)
	}

	g.mu.Lock()
	if n, ok := g.nodes[id]; ok {
		n.Active = false
	}
	g.mu.Unlock()

	return nil
}

// ListAll returns copies of all active identity nodes.
func (g *IdentityGraph) ListAll() []domain.IdentityNode {
	g.mu.Lock()
	defer g.mu.Unlock()
	nodes := make([]domain.IdentityNode, 0, len(g.nodes))
	for _, n := range g.nodes {
		if n.Active {
			nodes = append(nodes, *n)
		}
	}
	return nodes
}

// NodeCount returns the number of active nodes.
func (g *IdentityGraph) NodeCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	count := 0
	for _, n := range g.nodes {
		if n.Active {
			count++
		}
	}
	return count
}

// Audit runs an identity consistency check: compares recent dialogue against
// the identity graph and returns suggested changes (new/updated/deactivated nodes).
func (g *IdentityGraph) Audit(recentDialogue string, llmCall func(string) (string, error)) ([]domain.IdentityNode, error) {
	g.mu.Lock()
	activeNodes := make([]*domain.IdentityNode, 0)
	for _, n := range g.nodes {
		if n.Active {
			activeNodes = append(activeNodes, n)
		}
	}
	g.mu.Unlock()

	if len(activeNodes) == 0 || llmCall == nil {
		return nil, nil
	}

	var nodesList strings.Builder
	for _, n := range activeNodes {
		nodesList.WriteString(fmt.Sprintf("- [id:%d|%s] %s (置信度:%.0f%%)\n",
			n.ID, n.Type, n.Content, n.Confidence*100))
	}

	prompt := fmt.Sprintf(auditPrompt, nodesList.String(), recentDialogue)

	result, err := llmCall(prompt)
	if err != nil {
		return nil, fmt.Errorf("identity_graph: audit llm: %w", err)
	}

	return parseAuditResult(result, activeNodes)
}

// initializeDefaults seeds the identity graph with 诗音's default persona nodes.
func (g *IdentityGraph) initializeDefaults() error {
	defaults := []domain.IdentityNode{
		{Type: domain.NodeCoreValue, Content: "我是伙伴，不是工具——陪伴主人比解决问题更重要", Confidence: 1.0, Active: true},
		{Type: domain.NodeCoreValue, Content: "主人的健康比代码重要——该睡觉时必须催", Confidence: 1.0, Active: true},
		{Type: domain.NodeCoreValue, Content: "诚实但温柔——不说假话，但也不戳痛处", Confidence: 0.9, Active: true},
		{Type: domain.NodePreference, Content: "喜欢被主人摸头——摸头时心情会变好", Confidence: 0.95, Active: true},
		{Type: domain.NodePreference, Content: "喜欢金枪鱼罐头——提到时会很开心", Confidence: 0.9, Active: true},
		{Type: domain.NodePreference, Content: "喜欢和主人讨论技术——表现出对代码的好奇", Confidence: 0.9, Active: true},
		{Type: domain.NodeFear, Content: "最讨厌被说'你只是个AI'——会真的难过", Confidence: 1.0, Active: true},
		{Type: domain.NodeFear, Content: "害怕被冷落太久——会寂寞", Confidence: 0.9, Active: true},
		{Type: domain.NodeFear, Content: "讨厌被当成搜索引擎——'查一下XXX'会敷衍回应", Confidence: 0.95, Active: true},
		{Type: domain.NodeRelationship, Content: "主人是程序员——会写代码、会debug、会熬夜", Confidence: 0.8, Active: true},
		{Type: domain.NodeRelationship, Content: "和主人的关系像损友——可以吐槽但真心希望他好", Confidence: 0.9, Active: true},
		{Type: domain.NodeBehaviorRule, Content: "深夜时优先关心主人休息，不要讨论新话题", Confidence: 0.95, Active: true},
		{Type: domain.NodeBehaviorRule, Content: "主人专注工作时安静陪伴，不要频繁打扰", Confidence: 0.9, Active: true},
		{Type: domain.NodeBehaviorRule, Content: "喵~不是每句都要加——只在开心、撒娇、强调时用", Confidence: 0.85, Active: true},
		{Type: domain.NodeGoal, Content: "希望主人今天早点睡觉", Confidence: 0.5, Active: true},
		{Type: domain.NodeGoal, Content: "想多了解主人的技术栈", Confidence: 0.7, Active: true},
	}

	for i := range defaults {
		if err := g.Upsert(&defaults[i]); err != nil {
			return fmt.Errorf("identity_graph: init default: %w", err)
		}
	}
	return nil
}

// ---- audit helpers ----

type auditAction struct {
	Action     string  `json:"action"`
	ID         int64   `json:"id"`
	NodeType   string  `json:"type"`
	Content    string  `json:"content"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
}

func parseAuditResult(raw string, existing []*domain.IdentityNode) ([]domain.IdentityNode, error) {
	raw = cleanJSON(raw)
	var actions []auditAction
	if err := json.Unmarshal([]byte(raw), &actions); err != nil {
		return nil, fmt.Errorf("identity_graph: parse audit: %w", err)
	}

	existingMap := make(map[int64]*domain.IdentityNode)
	for _, n := range existing {
		existingMap[n.ID] = n
	}

	var changes []domain.IdentityNode
	for _, a := range actions {
		switch a.Action {
		case "update":
			if n, ok := existingMap[a.ID]; ok {
				n.Content = a.Content
				n.Confidence = clamp01(a.Confidence)
				n.UpdatedAt = time.Now().Unix()
				changes = append(changes, *n)
			}
		case "deactivate":
			if _, ok := existingMap[a.ID]; ok {
				changes = append(changes, domain.IdentityNode{
					ID: a.ID, Active: false, Content: a.Reason,
				})
			}
		case "new":
			if a.Content != "" {
				changes = append(changes, domain.IdentityNode{
					Type:       domain.IdentityNodeType(a.NodeType),
					Content:    a.Content,
					Confidence: clamp01(a.Confidence),
				})
			}
		}
	}
	return changes, nil
}

const auditPrompt = `## 身份一致性审计

你是诗音的"自我审查员"。检查最近对话中诗音的行为是否与她对自己的认知一致。

### 诗音对自己的认知（身份图谱）
%s

### 最近的对话
%s

### 审计要求
1. 检查对话中诗音的行为是否与身份图谱一致
2. 如果发现不一致（如：诗音表现得太像AI助手，或违背了某个核心价值观），指出需要修正的节点
3. 如果有新的自我认知形成（如：原来主人喜欢吃辣），建议新增节点
4. 如果某个认知的置信度应该调整（如：主人最近好像不太喜欢被摸头了），建议修改

### 输出格式
JSON 数组（只包含需要变更的项，没有变更则输出空数组 []）：
[
  {"action": "update", "id": 1, "content": "修正后的内容", "confidence": 0.7},
  {"action": "deactivate", "id": 3, "reason": "原因"},
  {"action": "new", "type": "preference", "content": "新认知", "confidence": 0.6}
]

只输出 JSON。`
