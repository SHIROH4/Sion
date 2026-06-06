package cognition

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"desktop-pet/internal/domain"; "desktop-pet/internal/infra"
)

// Engine drives curiosity-driven proactive learning.
// Screenshot analysis (vision LLM) replaces the old OCR-based entry recording.
type Engine struct {
	rawLLM     func([]domain.Message) (string, error)
	visionLLM  func([]domain.Message) (string, error) // vision/multimodal gateway
	repo       domain.CuriosityRepository
	embedFn    func(string) ([]float32, error)

	lastGapScanAt       time.Time
	lastVisualAnalyzeAt time.Time
	minGapInterval      time.Duration
	minVisualInterval   time.Duration
}

// NewEngine creates a curiosity engine.
func NewEngine(
	rawLLM func([]domain.Message) (string, error),
	repo domain.CuriosityRepository,
	embedFn func(string) ([]float32, error),
) *Engine {
	return &Engine{
		rawLLM:            rawLLM,
		repo:              repo,
		embedFn:           embedFn,
		minGapInterval:    2 * time.Hour,
		minVisualInterval: 15 * time.Minute,
	}
}

// SetVisionLLM injects the vision/multimodal model for screenshot analysis.
func (e *Engine) SetVisionLLM(vllm func([]domain.Message) (string, error)) {
	e.visionLLM = vllm
}

// ShouldScanGaps returns true if it's time to scan for new knowledge gaps.
func (e *Engine) ShouldScanGaps() bool {
	return time.Since(e.lastGapScanAt) > e.minGapInterval
}

// ForceGapScan resets the cooldown so the next gap detection cycle runs immediately.
func (e *Engine) ForceGapScan() {
	e.lastGapScanAt = time.Time{}
}

// SeedCuriosityTopics inserts initial curiosity gaps so the AI has things to ask about
// from day one. Only inserts if no active gaps exist (avoids overwriting learned gaps).
func (e *Engine) SeedCuriosityTopics(profileName string, techStack []string) {
	if e.repo == nil {
		return
	}
	existing, _ := e.repo.List(domain.CuriosityGap, "active", 20)
	if len(existing) > 0 {
		return
	}
	now := time.Now().Unix()
	seeds := []struct {
		question string
		priority float64
	}{
		{"主人平时喜欢听什么类型的音乐？", 0.6},
		{"主人最近有看什么好看的电影或剧吗？", 0.5},
		{"主人有什么一直想学但还没开始的东西？", 0.7},
		{"主人在工作中遇到过最有趣的bug是什么？", 0.5},
		{"主人喜欢什么类型的游戏？", 0.5},
	}
	for _, tech := range techStack {
		seeds = append(seeds, struct {
			question string
			priority float64
		}{fmt.Sprintf("主人在%s开发中遇到过什么坑？", tech), 0.6})
	}
	for _, s := range seeds {
		_, _ = e.repo.Save(domain.CuriosityItem{
			ItemType: domain.CuriosityGap, Content: s.question,
			Priority: s.priority, Status: "active", CreatedAt: now,
		})
	}
}

// ForceVisualAnalyze resets the visual cooldown so the next screenshot analysis
// runs at the next tick, regardless of the 15min interval.
func (e *Engine) ForceVisualAnalyze() {
	e.lastVisualAnalyzeAt = time.Time{}
}

// ShouldVisualAnalyze returns true when curiosity is high enough and the
// cooldown has passed for a screenshot-driven gap analysis.
func (e *Engine) ShouldVisualAnalyze(curiosity float64) bool {
	return curiosity > 0.6 && time.Since(e.lastVisualAnalyzeAt) > e.minVisualInterval && e.visionLLM != nil
}

// AnalyzeScreenshot sends a screenshot to the vision model, extracts user activity
// context, and produces knowledge gaps directly. Returns the number of gaps created.
func (e *Engine) AnalyzeScreenshot(base64PNG, appName, windowTitle string, knownFacts []string, profileName string, techStack []string) int {
	if e.visionLLM == nil || e.repo == nil {
		return 0
	}

	existing, _ := e.repo.List(domain.CuriosityGap, "active", 20)
	if len(existing) >= 10 {
		return 0
	}

	var factsText strings.Builder
	for _, f := range knownFacts {
		factsText.WriteString(fmt.Sprintf("- %s\n", f))
	}
	factsStr := factsText.String()
	if factsStr == "" {
		factsStr = "(暂无已知事实)"
	}

	prompt := fmt.Sprintf(visualGapPrompt, profileName, fmt.Sprint(techStack), factsStr, appName, windowTitle)

	result, err := e.visionLLM([]domain.Message{{
		Role:    "user",
		Content: prompt,
		Images:  []domain.Image{{Format: "png", Base64: base64PNG}},
	}})
	if err != nil {
		slog.Warn("curiosity: visual analysis LLM failed", "err", err)
		return 0
	}

	var gaps []struct {
		Question string  `json:"question"`
		Reason   string  `json:"reason"`
		GapType  string  `json:"gap_type"`
		Priority float64 `json:"priority"`
		Activity string  `json:"activity"`
	}
	raw := infra.CleanJSON(result)
	if err := json.Unmarshal([]byte(raw), &gaps); err != nil {
		// LLM sometimes returns a single object instead of an array — wrap it.
		var single struct {
			Question string  `json:"question"`
			Reason   string  `json:"reason"`
			GapType  string  `json:"gap_type"`
			Priority float64 `json:"priority"`
			Activity string  `json:"activity"`
		}
		if err2 := json.Unmarshal([]byte(raw), &single); err2 == nil && single.Question != "" {
			gaps = append(gaps, single)
		} else {
			slog.Warn("curiosity: visual analysis JSON parse failed", "err", err, "raw", raw[:min(len(raw), 120)])
			return 0
		}
	}

	added := 0
	now := time.Now().Unix()
	for _, g := range gaps {
		if g.Question == "" || added >= 5 {
			break
		}
		if g.Priority <= 0 {
			g.Priority = 0.5
		}
		evidence := g.Reason
		if g.Activity != "" {
			evidence = fmt.Sprintf("[屏幕: %s] %s", g.Activity, g.Reason)
		}
		_, _ = e.repo.Save(domain.CuriosityItem{
			ItemType:  domain.CuriosityGap,
			Content:   g.Question,
			Priority:  g.Priority,
			Status:    "active",
			Evidence:  evidence,
			Tags:      g.GapType,
			CreatedAt: now,
		})
		added++
	}

	e.lastVisualAnalyzeAt = time.Now()
	return added
}

// ScanGaps uses text LLM to find new knowledge gaps from known facts.
func (e *Engine) ScanGaps(knownFacts []string, profileName string, techStack []string) int {
	if e.rawLLM == nil || e.repo == nil {
		return 0
	}

	existing, _ := e.repo.List(domain.CuriosityGap, "active", 20)
	if len(existing) >= 10 {
		return 0
	}

	var factsText strings.Builder
	for _, f := range knownFacts {
		factsText.WriteString(fmt.Sprintf("- %s\n", f))
	}
	factsStr := factsText.String()
	if factsStr == "" {
		factsStr = "(暂无已知事实)"
	}

	prompt := fmt.Sprintf(gapPrompt, profileName, fmt.Sprint(techStack), factsStr)
	result, err := e.rawLLM([]domain.Message{{Role: "user", Content: prompt}})
	if err != nil {
		slog.Warn("curiosity: gap scan LLM failed", "err", err)
		return 0
	}

	var gaps []struct {
		Question string  `json:"question"`
		Reason   string  `json:"reason"`
		GapType  string  `json:"gap_type"`
		Priority float64 `json:"priority"`
	}
	raw := infra.CleanJSON(result)
	if err := json.Unmarshal([]byte(raw), &gaps); err != nil {
		// LLM sometimes returns a single object instead of an array.
		var single struct {
			Question string  `json:"question"`
			Reason   string  `json:"reason"`
			GapType  string  `json:"gap_type"`
			Priority float64 `json:"priority"`
		}
		if err2 := json.Unmarshal([]byte(raw), &single); err2 == nil && single.Question != "" {
			gaps = append(gaps, single)
		} else {
			slog.Warn("curiosity: gap scan JSON parse failed", "err", err, "raw", raw[:min(len(raw), 120)])
			return 0
		}
	}

	added := 0
	now := time.Now().Unix()
	for _, g := range gaps {
		if g.Question == "" || added >= 5 {
			break
		}
		if g.Priority <= 0 {
			g.Priority = 0.5
		}
		_, _ = e.repo.Save(domain.CuriosityItem{
			ItemType:  domain.CuriosityGap,
			Content:   g.Question,
			Priority:  g.Priority,
			Status:    "active",
			Evidence:  g.Reason,
			Tags:      g.GapType,
			CreatedAt: now,
		})
		added++
	}

	e.lastGapScanAt = time.Now()
	return added
}

// GenerateInquiries converts pending gaps into active inquiries.
func (e *Engine) GenerateInquiries(limit int) []domain.CuriosityItem {
	if e.repo == nil {
		return nil
	}

	gaps, _ := e.repo.List(domain.CuriosityGap, "active", limit)
	if len(gaps) == 0 {
		return nil
	}

	active, _ := e.repo.List(domain.CuriosityInquiry, "active", 5)
	if len(active) >= 5 {
		return nil
	}

	var inquiries []domain.CuriosityItem
	now := time.Now().Unix()
	for _, g := range gaps {
		if len(inquiries) >= limit {
			break
		}
		item := domain.CuriosityItem{
			ItemType:   domain.CuriosityInquiry,
			Content:    fmt.Sprintf("了解: %s", g.Content),
			Confidence: 0.3,
			Priority:   g.Priority,
			Status:     "active",
			Tags:       g.Tags,
			CreatedAt:  now,
		}
		id, _ := e.repo.Save(item)
		item.ID = id
		inquiries = append(inquiries, item)
		_ = e.repo.MarkStatus(g.ID, "asked")
	}

	return inquiries
}

// PickBestInquiry selects the most valuable active inquiry for the current context.
// Returns for all source types — care actions can also carry curiosity-driven topics.
func (e *Engine) PickBestInquiry(source domain.ProactiveSource, hourOfDay int) *domain.CuriosityItem {
	if e.repo == nil {
		return nil
	}

	active, err := e.repo.List(domain.CuriosityInquiry, "active", 10)
	if err != nil || len(active) == 0 {
		return nil
	}

	best := &active[0]
	bestScore := 0.0
	for i := range active {
		score := active[i].Priority
		// Boost inquiries that haven't been asked recently.
		if active[i].AskedAt == 0 || time.Since(time.Unix(active[i].AskedAt, 0)) > 3*time.Hour {
			score += 0.2
		}
		// Boost web-sourced inquiries (freshly learned facts are more interesting).
		if active[i].Source == "web_search" || active[i].Source == "web_browse" {
			score += 0.1
		}
		if score > bestScore {
			bestScore = score
			best = &active[i]
		}
	}

	if best.Priority < 0.3 && bestScore < 0.5 {
		return nil
	}
	return best
}

const gapPrompt = `## 好奇心缺口扫描

你是诗音的好奇心模块。基于目前已知的主人信息，找出我们还不清楚但应该了解的事情。

### 主人已知信息
姓名: %s
技术栈: %v

### 已知事实
%s

### 扫描规则
1. Profile 缺口: 还不知道的偏好/习惯/计划
2. 隐性偏好: 已有行为 pattern 但未确认为长期偏好的
3. 过时确认: 超过 2 周未更新的信息
4. 生成 1-5 个自然的提问，像朋友聊天一样自然

### 输出格式
JSON 数组:
[
  {"question": "自然的提问", "reason": "检测到的缺口", "gap_type": "profile|preference|habit|relationship", "priority": 0.7},
  ...
]

只输出 JSON 数组。`

const visualGapPrompt = `## 屏幕分析

你是诗音的观察模块。你正在看主人的屏幕截图。分析主人在做什么，并找出我们不知道但应该了解的事情。

### 主人已知信息
姓名: %s
技术栈: %v

### 已知事实
%s

### 当前屏幕
应用: %s
标题: %s

### 分析任务
1. 从截图中判断主人当前正在做什么（项目名、任务类型、工具等）
2. 找出 1-3 个我们还不了解但值得问的事情
3. 提问要自然、像朋友聊天，不要像审问

### 输出格式
JSON 数组:
[
  {"question": "自然的提问", "reason": "为什么想知道", "gap_type": "profile|preference|habit|project", "priority": 0.7, "activity": "一句话描述主人在做什么"},
  ...
]

只输出 JSON 数组。`
