package chat

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"desktop-pet/internal/domain"
	svcmemory "desktop-pet/internal/service/memory"
)

// Processor prepares chat context by injecting persona, emotion, memory, and
// screen information before each turn.
type Processor struct {
	Store          domain.MemoryStore
	EmbSvc         domain.Vectorizer
	SelfModel      func() string
	EmotionCurrent func() (domain.EmotionState, domain.EmotionVector)
	Profile        *domain.UserProfile
	IdentityGraph  interface{ Retrieve(string, int) []string }
	CareSnapshot   func() domain.UserCareState
	SessionBuf     interface {
		Recent(int) []domain.Message
		Append(domain.Message)
		Len() int
	}
	RerankFn      func([]domain.UnifiedResult, string, int) []domain.UnifiedResult
	TimeTag       func(domain.UnifiedResult) string
	TurnCount     *int
	WG            *sync.WaitGroup
	ScreenSummary func() string
}

// ---- FilterMessage ----

// FilterMessage prepends a timestamp to the user message.
func FilterMessage(msg *domain.Message) error {
	ts := time.Now().Format("15:04")
	msg.Content = fmt.Sprintf("[%s] %s", ts, msg.Content)
	return nil
}

// ---- OnBeforeChat ----

// OnBeforeChat injects a compact context with three-layer memory retrieval:
// L3 Self Model + Emotion + User Profile (always) → L2+L1 semantic search
// (vector path) or fallback (DecayWeight) → L0 SessionBuffer → cross-session.
func (p *Processor) OnBeforeChat(ctx *domain.ChatContext) error {
	queryText := ctx.Input

	// P0-3: Personality summary.
	if p.EmotionCurrent != nil {
		summary := p.BuildPersonaSummary()
		ctx.Messages = append([]domain.Message{
			{Role: "system", Content: summary},
		}, ctx.Messages...)
	}

	// L1: Screen context.
	if p.ScreenSummary != nil {
		if s := p.ScreenSummary(); s != "" {
			ctx.Messages = append([]domain.Message{
				{Role: "system", Content: fmt.Sprintf("[主人当前]\n%s\n", s)},
			}, ctx.Messages...)
		}
	}

	// P1-1: Identity graph retrieval.
	if p.IdentityGraph != nil {
		nodes := p.IdentityGraph.Retrieve(ctx.Input, 3)
		if len(nodes) > 0 {
			identityCtx := "## 关于你自己（来自身份图谱）\n"
			for _, n := range nodes {
				identityCtx += "- " + n + "\n"
			}
			ctx.Messages = append([]domain.Message{
				{Role: "system", Content: identityCtx},
			}, ctx.Messages...)
		}
	}

	// L3: Self Model.
	if p.SelfModel != nil {
		if self := p.SelfModel(); self != "" {
			ctx.Messages = append([]domain.Message{
				{Role: "system", Content: "## 你对自己的认知\n" + self},
			}, ctx.Messages...)
		}
	}

	// Emotion state.
	if p.EmotionCurrent != nil {
		e, v := p.EmotionCurrent()
		emoCtx := fmt.Sprintf(
			"## 你当前的状态\n"+
				"基础情绪：%s（强度%.0f%%）\n"+
				"亲密度：%.0f%% | 担忧：%.0f%% | 好奇：%.0f%% | 困倦：%.0f%% | "+
				"想玩：%.0f%% | 寂寞：%.0f%% | 自信：%.0f%% | 被惹恼：%.0f%%\n"+
				"（这些状态影响你说话的语气。困倦时打哈欠，寂寞时主动搭话，被惹恼时会毒舌）",
			e.Primary, e.Intensity*100,
			v.Affection*100, v.Worry*100, v.Curiosity*100, v.Sleepiness*100,
			v.Playfulness*100, v.Loneliness*100, v.Confidence*100, v.Annoyance*100,
		)
		ctx.Messages = append([]domain.Message{
			{Role: "system", Content: emoCtx},
		}, ctx.Messages...)
	}

	// User profile.
	if p.Profile != nil && (p.Profile.Name != "" || len(p.Profile.TechStack) > 0) {
		profileMsg := domain.Message{Role: "system", Content: p.BuildProfileContext()}
		ctx.Messages = append([]domain.Message{profileMsg}, ctx.Messages...)
	}

	// Time-filtered retrieval: if user asks about "yesterday" etc, inject time-bound facts.
	if timeSince := parseTimeQuery(queryText); timeSince > 0 {
		timeFacts := p.Store.GetRecentFacts(time.Now().Unix() - timeSince)
		injected := 0
		for _, f := range timeFacts {
			if injected >= 5 {
				break
			}
			ctx.Messages = append([]domain.Message{
				{Role: "system", Content: fmt.Sprintf("[%s] %s", relativeDayLabel(timeSince), f.Content)},
			}, ctx.Messages...)
			injected++
		}
	}

	// L2+L1: Semantic retrieval (vector path) or DecayWeight fallback.
	if p.EmbSvc != nil && p.Store != nil {
		queryVec, err := p.EmbSvc.Vectorize(queryText)
		if err == nil {
			candidates, _ := p.Store.UnifiedSearch(queryVec, queryText, 10)
			var topResults []domain.UnifiedResult
			if p.TurnCount != nil && *p.TurnCount%5 == 0 && p.RerankFn != nil {
				topResults = p.RerankFn(candidates, queryText, 3)
			} else {
				topResults = candidates
				if len(topResults) > 3 {
					topResults = topResults[:3]
				}
			}

			for i := len(topResults) - 1; i >= 0; i-- {
				tag := ""
				if p.TimeTag != nil {
					tag = p.TimeTag(topResults[i])
				}
				ctx.Messages = append([]domain.Message{
					{Role: "system", Content: fmt.Sprintf("[相关记忆] %s%s", tag, topResults[i].Content)},
				}, ctx.Messages...)
			}

			var recallIDs []int64
			for _, r := range topResults {
				if r.Source == "fact" {
					recallIDs = append(recallIDs, r.ID)
				}
			}
			if len(recallIDs) > 0 && p.WG != nil {
				p.WG.Add(1)
				go func(ids []int64) {
					defer p.WG.Done()
					p.Store.BatchUpdateFactRecall(ids)
				}(recallIDs)
			}
		}
	} else if p.Store != nil {
		p.InjectFactsFallback(ctx)
	}

	// L0: Recent conversation — merge in-memory buffer with persisted history
	// so that cross-process chats (settings ↔ pet) stay in sync.
	if p.SessionBuf != nil {
		// Load recent history from DB to catch messages from the other process.
		if p.Store != nil {
			history, err := p.Store.LoadHistory(10)
			if err == nil {
				for _, m := range history {
					p.SessionBuf.Append(m)
				}
			}
		}
		recent := p.SessionBuf.Recent(10)
		if len(recent) > 0 {
			ctx.Messages = append(recent, ctx.Messages...)
		}
	}

	return nil
}

// ---- Fact fallback (pre-Phase-F) ----

// InjectFactsFallback is the pre-Phase-F manual DecayWeight ranking path,
// used when EmbedSvc hasn't been initialised.
func (p *Processor) InjectFactsFallback(ctx *domain.ChatContext) {
	facts := p.Store.ListActiveFacts(svcmemory.ActiveThreshold)
	type weighted struct {
		content string
		w       float64
	}
	var ranked []weighted
	for _, f := range facts {
		w := svcmemory.DecayWeight(f.Importance, f.LastRecalledAt, f.RecallCount, 30, 0.15)
		if w >= svcmemory.ActiveThreshold {
			ranked = append(ranked, weighted{content: f.Content, w: w})
		}
	}
	sort.Slice(ranked, func(i, j int) bool { return ranked[i].w > ranked[j].w })
	limit := len(ranked)
	if limit > 5 {
		limit = 5
	}
	for i := limit - 1; i >= 0; i-- {
		tag := ""
		if ranked[i].w >= svcmemory.CoreThreshold {
			tag = "[核心记忆] "
		}
		ctx.Messages = append([]domain.Message{
			{Role: "system", Content: tag + ranked[i].content},
		}, ctx.Messages...)
	}
	for _, f := range facts {
		if f.Importance > svcmemory.ActiveThreshold && p.WG != nil {
			p.WG.Add(1)
			go func(id int64) {
				defer p.WG.Done()
				p.Store.UpdateFactRecall(id)
			}(f.ID)
		}
	}
}

// ---- Context builders ----

// BuildProfileContext formats user profile info for injection.
func (p *Processor) BuildProfileContext() string {
	s := "## 用户档案\n"
	if p.Profile != nil && p.Profile.Name != "" {
		s += fmt.Sprintf("- 称呼：%s\n", p.Profile.Name)
	}
	if p.Profile != nil && len(p.Profile.TechStack) > 0 {
		s += fmt.Sprintf("- 技术栈：%s\n", strings.Join(p.Profile.TechStack, "、"))
	}
	s += "\n以上是已存档的用户信息，对话中可适时引用。"
	return s
}

// BuildPersonaSummary builds a short personality status summary (~100-150 tokens)
// injected before every turn.
func (p *Processor) BuildPersonaSummary() string {
	self := ""
	if p.SelfModel != nil {
		self = p.SelfModel()
	}
	e, v := p.EmotionCurrent()
	identityLine := ExtractIdentityLine(self)
	emotionLine := DescribeTopEmotions(v, e)
	concernLine := BuildConcernLine(p.CareSnapshot)
	return fmt.Sprintf(
		"[此刻的诗音]\n%s\n%s\n%s\n",
		identityLine, emotionLine, concernLine,
	)
}

// ---- Standalone helpers ----

// ExtractIdentityLine returns the first sentence of self-model, or a default.
func ExtractIdentityLine(self string) string {
	lines := strings.SplitN(self, "。", 2)
	if len(lines) > 0 && len([]rune(lines[0])) > 10 {
		return "身份: " + strings.TrimSpace(lines[0]) + "。"
	}
	return "身份: 我是诗音，主人的猫娘伙伴。"
}

// DescribeTopEmotions returns a one-line description of the dominant emotions.
func DescribeTopEmotions(v domain.EmotionVector, e domain.EmotionState) string {
	type dim struct {
		name  string
		value float64
		desc  string
	}
	dims := []dim{
		{"affection", v.Affection, "对主人很亲近"},
		{"worry", v.Worry, "有点担心主人"},
		{"curiosity", v.Curiosity, "好奇心满满"},
		{"sleepiness", v.Sleepiness, "有点困了"},
		{"playfulness", v.Playfulness, "想和主人玩"},
		{"loneliness", v.Loneliness, "有点寂寞"},
		{"annoyance", v.Annoyance, "被惹得有点不开心"},
	}
	sort.Slice(dims, func(i, j int) bool { return dims[i].value > dims[j].value })
	top := []string{}
	for _, d := range dims {
		if d.value > 0.5 {
			top = append(top, d.desc)
		}
		if len(top) >= 2 {
			break
		}
	}
	if len(top) == 0 {
		top = append(top, "心情平稳")
	}
	return "状态: " + e.Primary + "情绪，" + strings.Join(top, "，") + "。"
}

// BuildConcernLine returns a concern description based on care state snapshot.
func BuildConcernLine(snapshot func() domain.UserCareState) string {
	if snapshot == nil {
		return ""
	}
	sn := snapshot()
	concerns := []string{}
	if sn.ContinuousWork > 120 {
		concerns = append(concerns, "主人工作很久了，需要休息")
	}
	if sn.StressLevel > 0.6 {
		concerns = append(concerns, "主人压力有点大")
	}
	if time.Now().Hour() >= 23 {
		concerns = append(concerns, "这么晚了主人还没睡")
	}
	if len(concerns) == 0 {
		return "关注: 陪着主人就好。"
	}
	return "关注: " + strings.Join(concerns, "；") + "。"
}

// parseTimeQuery detects temporal keywords and returns the approximate time window in seconds.
// Returns 0 if no temporal keyword is found (no time filtering needed).
func parseTimeQuery(query string) int64 {
	lower := strings.ToLower(query)
	now := time.Now()
	todayStart := now.Truncate(24 * time.Hour)

	// Granular time queries.
	switch {
	// Yesterday sub-periods.
	case strings.Contains(lower, "昨天早上") || strings.Contains(lower, "昨天上午"):
		return now.Unix() - (todayStart.AddDate(0, 0, -1).Add(6*time.Hour)).Unix()
	case strings.Contains(lower, "昨天中午"):
		return now.Unix() - (todayStart.AddDate(0, 0, -1).Add(11*time.Hour)).Unix()
	case strings.Contains(lower, "昨天下午"):
		return now.Unix() - (todayStart.AddDate(0, 0, -1).Add(13*time.Hour)).Unix()
	case strings.Contains(lower, "昨晚") || strings.Contains(lower, "昨天睡前") || strings.Contains(lower, "昨天夜里") || strings.Contains(lower, "昨夜"):
		return now.Unix() - (todayStart.AddDate(0, 0, -1).Add(20*time.Hour)).Unix()
	case strings.Contains(lower, "昨天"):
		return now.Unix() - todayStart.AddDate(0, 0, -1).Unix()

	// Today sub-periods.
	case strings.Contains(lower, "今早") || strings.Contains(lower, "今天早上") || strings.Contains(lower, "今天上午"):
		return now.Unix() - (todayStart.Add(6*time.Hour)).Unix()
	case strings.Contains(lower, "今天中午"):
		return now.Unix() - (todayStart.Add(11*time.Hour)).Unix()
	case strings.Contains(lower, "今天下午"):
		return now.Unix() - (todayStart.Add(13*time.Hour)).Unix()
	case strings.Contains(lower, "今晚") || strings.Contains(lower, "今天晚上"):
		return now.Unix() - (todayStart.Add(18*time.Hour)).Unix()
	case strings.Contains(lower, "今天") || strings.Contains(lower, "刚才") || strings.Contains(lower, "刚刚"):
		return now.Unix() - todayStart.Unix()

	// This week / 这周 / 本周.
	case strings.Contains(lower, "这周") || strings.Contains(lower, "本周") || strings.Contains(lower, "这个星期"):
		return 7 * 86400
	// Last week / 上周.
	case strings.Contains(lower, "上周") || strings.Contains(lower, "上个星期"):
		return 14 * 86400
	// 前几天 / 最近几天.
	case strings.Contains(lower, "前几天") || strings.Contains(lower, "最近几天"):
		return 3 * 86400
	}
	return 0
}

func relativeDayLabel(seconds int64) string {
	switch {
	case seconds <= 86400:
		return "今天"
	case seconds <= 2*86400:
		return "昨天"
	case seconds <= 3*86400:
		return "前天"
	default:
		return fmt.Sprintf("%d天前", seconds/86400)
	}
}
