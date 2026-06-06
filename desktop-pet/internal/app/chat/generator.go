package chat

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"desktop-pet/internal/domain"
	care "desktop-pet/internal/service/care"
)

// Generator produces proactive chat messages and care messages using LLM.
// All dependencies are injected as function fields.
type Generator struct {
	RawLLM      func([]domain.Message) (string, error)
	Store       domain.MemoryStore
	SelfModel   func() string
	EmotionFunc func() domain.EmotionState

	// ScreenAnalyzer provides L2 cloud multimodal screen analysis.
	ScreenAnalyzer interface {
		ShouldAnalyze() bool
		Analyze(imageBase64, appName, windowTitle string) (string, error)
	}
	// CaptureScreen is macOS native screenshot (returns base64 PNG).
	CaptureScreen func() (string, error)
	// ActiveWindow returns the current app name and window title.
	ActiveWindow func() (string, string)
	// SessionRecent returns the most recent N chat messages.
	SessionRecent func(n int) []domain.Message
	// Emit asynchronously fires an event on the bus.
	Emit func(event string, payload any)
	// InfoLog and WarnLog are log helpers.
	InfoLog func(msg string, args ...any)
	WarnLog func(msg string, args ...any)

	// Pending state pointers (mutated by GenerateProactive, read by OnAfterChat).
	PendingID     *int64
	PendingSource *domain.ProactiveSource
	PendingAt     *time.Time

	// Outcome recording for adaptive learning.
	OutcomeRepo   domain.ActionOutcomeRepository
	EmotionVector func() domain.EmotionVector

	// Strategy principles for proactive message guidance.
	PrincipleRepo domain.StrategyPrincipleRepository

	// Curiosity engine — picks a suitable inquiry to blend into proactive messages.
	PickInquiry func(source domain.ProactiveSource, hour int) *domain.CuriosityItem

	// Pattern triggers — returns active pattern-driven triggers for the current time.
	PatternTriggers func(now time.Time) []domain.PatternTrigger

	// Active conversation threads for cross-session continuity.
	ActiveThreads func() []domain.ConversationThread

	// FactSearch returns the best matching fact for a query (RAG for proactive messages).
	FactSearch func(query string) string

	// ConversationSummary is a compressed summary of the ongoing conversation.
	ConversationSummary string
	// PersonaSummary is the same persona context used in normal chat (self + emotion + care).
	PersonaSummary string

	// Metrics logger for observability.
	MetricsLog func(msg string, args ...any)

	// Pending context for feedback attribution.
	pendingCtx        domain.ActionContext
	pendingEscalation int
	pendingAt         time.Time
}

// GenerateProactive is the Generator half of the Scheduler+Generator pattern.
// It performs optional screen analysis, builds a prompt, calls the LLM, and
// emits the resulting action via the event bus.
func (g *Generator) GenerateProactive(result domain.SchedulerResult, lastScreenObs domain.ScreenObservation) {
	if g.RawLLM == nil || g.Emit == nil {
		slog.Warn("generator: GenerateProactive skipped — RawLLM or Emit not wired", "source", result.Source)
		return
	}

	// L2: Cloud multimodal screen analysis.
	var screenDesc string
	if g.ScreenAnalyzer != nil && g.ScreenAnalyzer.ShouldAnalyze() &&
		(result.Source == domain.SourceCasual || result.Score > 0.7) {
		base64, err := g.CaptureScreen()
		if err == nil && base64 != "" {
			appName, windowTitle := g.ActiveWindow()
			desc, err := g.ScreenAnalyzer.Analyze(base64, appName, windowTitle)
			if err == nil && desc != "" {
				screenDesc = desc
			}
		}
		if g.InfoLog != nil && screenDesc != "" {
			g.InfoLog("memory: L2 screen analysis ok", "len", len(screenDesc))
		}
	}

	// L1 screen context.
	var screenInfo string
	if lastScreenObs.AppName != "" {
		screenInfo = fmt.Sprintf("在用%s", lastScreenObs.AppName)
		if lastScreenObs.WindowTitle != "" {
			screenInfo += fmt.Sprintf("（%s）", lastScreenObs.WindowTitle)
		}
	}

	recentChat := buildRecentChatContext(g.SessionRecent)
	// Use the same persona summary as normal chat when available.
	persona := g.PersonaSummary
	if persona == "" {
		self := ""
		if g.SelfModel != nil { self = g.SelfModel() }
		emoStr := ""
		if g.EmotionFunc != nil { e := g.EmotionFunc(); emoStr = fmt.Sprintf("当前情绪: %s (愉悦度:%.1f)", e.Primary, e.Valence) }
		persona = self + "\n" + emoStr
	}

	// Retrieve matching strategy principles for this context.
	var matchingPrinciples string
	if g.PrincipleRepo != nil {
		tags := buildContextTags(result, lastScreenObs)
		if principles, err := g.PrincipleRepo.FindByTags(tags, 3); err == nil && len(principles) > 0 {
			var sb strings.Builder
			sb.WriteString("### 小本本里的经验（来自过去的成功/失败）\n")
			for i, p := range principles {
				sb.WriteString(fmt.Sprintf("%d. 场景: %s → 建议: %s → 避免: %s (因为: %s)\n",
					i+1, p.Situation, p.GoodStrategy, p.BadStrategy, p.Reason))
			}
			matchingPrinciples = sb.String()
		}
	}

	// Pick a curiosity goal — available for all source types now.
	var curiosityGuide string
	if g.PickInquiry != nil {
		if inq := g.PickInquiry(result.Source, time.Now().Hour()); inq != nil {
			curiosityGuide = fmt.Sprintf(
				"悄悄话：你有个小心思——你想了解「%s」。在搭话时自然地融入这个话题，不要太生硬。",
				inq.Content,
			)
		}
	}

	// Pattern-driven preemptive context.
	var patternGuide string
	if g.PatternTriggers != nil {
		triggers := g.PatternTriggers(time.Now())
		if len(triggers) > 0 {
			var sb strings.Builder
			sb.WriteString("你注意到的模式:\n")
			for _, t := range triggers {
				sb.WriteString(fmt.Sprintf("- %s（因为: %s）\n", t.Pattern.Implication, t.Pattern.Pattern))
			}
			sb.WriteString("利用这些观察，让你的搭话更有预见性——但不要直接说「我注意到你的模式」。")
			patternGuide = sb.String()
		}
	}

	// Active thread context for cross-session continuity.
	var threadGuide string
	if g.ActiveThreads != nil {
		threads := g.ActiveThreads()
		summaries := domain.SummarizeThreads(threads, 3)
		if len(summaries) > 0 {
			var sb strings.Builder
			sb.WriteString("你们之前聊到的话题（可以自然地接续）:\n")
			for _, s := range summaries {
				sb.WriteString(fmt.Sprintf("- %s（可以这样提起: %s）\n", s.Goal, s.BestApproach))
			}
			threadGuide = sb.String()
		}
	}

	// RAG: search long-term memory for a relevant fact to ground the message.
	// Use screen info + recent chat as context for better fact retrieval.
	var memoryGuide string
	if g.FactSearch != nil {
		searchQuery := result.Reason
		if recentChat != "" {
			searchQuery = recentChat + " " + result.Reason
		}
		if fact := g.FactSearch(searchQuery); fact != "" {
			memoryGuide = fmt.Sprintf("你记得关于主人的一件事: %s。可以在搭话时自然引用，但不要生硬地复述。", fact)
		}
	}
	// Conversation summary for context continuity.
	convSummary := g.ConversationSummary

	prompt := BuildProactivePrompt(result, persona, recentChat, screenInfo, screenDesc, matchingPrinciples, curiosityGuide, patternGuide, threadGuide, memoryGuide, convSummary)
	reply, err := g.RawLLM([]domain.Message{{Role: "user", Content: prompt}})
	if err != nil {
		if g.WarnLog != nil {
			g.WarnLog("memory: proactive generator failed", "err", err)
		}
		return
	}
	reply = strings.TrimSpace(reply)
	if reply == "" {
		return
	}

	action := domain.CareAction{
		ID:          time.Now().UnixNano(),
		Source:      result.Source,
		Message:     reply,
		TriggeredAt: time.Now(),
		Observed:    screenInfo != "" || screenDesc != "",
	}
	switch result.Source {
	case domain.SourceCare:
		action.Type = domain.TriggerEncourage
	case domain.SourceKnowledgeGap:
		action.Type = domain.TriggerSocial
	case domain.SourceCasual:
		action.Type = domain.TriggerSocial
	}

	g.Emit("care:action", action)
	if g.InfoLog != nil {
		g.InfoLog("generator: proactive message emitted", "source", result.Source, "len", len(reply))
	}
	if g.PendingID != nil {
		*g.PendingID = action.ID
	}
	if g.PendingSource != nil {
		*g.PendingSource = result.Source
	}
	if g.PendingAt != nil {
		*g.PendingAt = time.Now()
	}

	// Record pending context for adaptive feedback.
	g.pendingCtx = domain.ActionContext{
		Source:    result.Source,
		Type:      action.Type,
		HourOfDay: time.Now().Hour(),
		DayOfWeek: int(time.Now().Weekday()),
	}
	g.pendingEscalation = result.Escalation
	if lastScreenObs.AppName != "" {
		g.pendingCtx.AppContext = lastScreenObs.AppName
	}
	if g.EmotionVector != nil {
		g.pendingCtx.EmotionBucket = emotionBucketForVec(g.EmotionVector())
	}
	g.pendingAt = time.Now()

	if g.Store != nil {
		if err := g.Store.SaveHistory([]domain.Message{{Role: "assistant", Content: reply}}, 0); err != nil {
			slog.Warn("generator: failed to save chat history", "err", err)
		}
	}
}

// RecordProactiveOutcome records the user's response (or lack thereof) to the
// most recent proactive action for adaptive learning.
func (g *Generator) RecordProactiveOutcome(outcome domain.OutcomeResult, responseDelaySec int) {
	if g.OutcomeRepo == nil {
		return
	}
	o := domain.ActionOutcome{
		ActionSource:  g.pendingCtx.Source,
		ActionType:    g.pendingCtx.Type,
		HourOfDay:     g.pendingCtx.HourOfDay,
		DayOfWeek:     g.pendingCtx.DayOfWeek,
		AppContext:    g.pendingCtx.AppContext,
		EmotionBucket: g.pendingCtx.EmotionBucket,
		EscalationLvl: g.pendingEscalation,
		Outcome:       outcome,
		ResponseDelay: responseDelaySec,
	}
	if err := g.OutcomeRepo.SaveOutcome(o); err != nil {
		slog.Warn("generator: failed to save outcome", "err", err)
	}

	if g.MetricsLog != nil {
		outcomeLabel := "ignored"
		if outcome == domain.OutcomeReplied {
			outcomeLabel = "replied"
		} else if outcome == domain.OutcomeEngaged {
			outcomeLabel = "engaged"
		} else if outcome == domain.OutcomeRejected {
			outcomeLabel = "rejected"
		}
		g.MetricsLog("proactive:outcome",
			"source", string(o.ActionSource),
			"type", string(o.ActionType),
			"outcome", outcomeLabel,
			"delay_sec", responseDelaySec,
		)
	}
}

// emotionBucketForVec discretizes an emotion vector into a short key.
func emotionBucketForVec(vec domain.EmotionVector) string {
	b := "N"
	if vec.Affection > 0.6 {
		b = "A"
	}
	if vec.Annoyance > 0.4 {
		b += "I"
	}
	if vec.Worry > 0.4 {
		b += "W"
	}
	if vec.Loneliness > 0.5 {
		b += "L"
	}
	if vec.Playfulness > 0.5 {
		b += "P"
	}
	if vec.Sleepiness > 0.6 {
		b += "S"
	}
	return b
}

func buildRecentChatContext(sessionRecent func(int) []domain.Message) string {
	if sessionRecent == nil {
		return ""
	}
	recent := sessionRecent(8)
	if len(recent) == 0 {
		return ""
	}
	var lines []string
	for _, m := range recent {
		role := "主人"
		if m.Role == "assistant" {
			role = "诗音"
		}
		content := m.Content
		if len([]rune(content)) > 60 {
			content = string([]rune(content)[:60]) + "..."
		}
		lines = append(lines, fmt.Sprintf("[%s]: %s", role, content))
	}
	return strings.Join(lines, "\n")
}

// buildContextTags creates search tags for strategy principle retrieval based on
// the current proactive context.
func buildContextTags(result domain.SchedulerResult, obs domain.ScreenObservation) []string {
	var tags []string
	tags = append(tags, string(result.Source))

	hour := time.Now().Hour()
	switch {
	case hour >= 23 || hour < 6:
		tags = append(tags, "深夜")
	case hour >= 6 && hour < 9:
		tags = append(tags, "早晨")
	case hour >= 11 && hour <= 13:
		tags = append(tags, "饭点")
	case hour >= 14 && hour < 18:
		tags = append(tags, "下午")
	default:
		tags = append(tags, "晚间")
	}

	if strings.Contains(result.Reason, "worry") {
		tags = append(tags, "关心", "担忧")
	}
	if strings.Contains(result.Reason, "loneliness") {
		tags = append(tags, "寂寞", "社交")
	}
	if strings.Contains(result.Reason, "late-night") || strings.Contains(result.Reason, "rest") {
		tags = append(tags, "休息", "催睡")
	}
	if strings.Contains(result.Reason, "health") || strings.Contains(result.Reason, "work") {
		tags = append(tags, "健康", "久坐")
	}

	if obs.IsWorking {
		tags = append(tags, "工作")
	} else {
		tags = append(tags, "休闲")
	}

	return tags
}

// BuildProactivePrompt assembles the proactive chat prompt from the given inputs.
func BuildProactivePrompt(
	result domain.SchedulerResult,
	persona, recentChat, screenInfo, screenDesc, strategyGuide, curiosityGuide, patternGuide, threadGuide, memoryGuide, convSummary string,
) string {
	escGuide := ""
	switch {
	case result.Escalation >= 3:
		escGuide = "你已经提醒过好几次了，主人都没理你——语气可以带点傲娇和放弃感（\"算了不催了\"），不要太认真"
	case result.Escalation == 2:
		escGuide = "这是第二次提醒了——语气可以比上次更认真一点，但不要唠叨"
	default:
		escGuide = "第一次提醒——自然温和，不要太强势"
	}

	emoGuide := result.EmotionContext
	if emoGuide == "" {
		emoGuide = "自然温暖，像朋友闲聊"
	}

	anchorGuide := ""
	if screenInfo != "" {
		if result.ContextAnchor != "" {
			anchorGuide = fmt.Sprintf("主人屏幕: %s。主人刚才: %s。仅用这些信息自然搭话，不要编造其他细节。", screenInfo, result.ContextAnchor)
		} else {
			anchorGuide = fmt.Sprintf("主人屏幕: %s。仅基于此搭话，不知道的事就说不知道，不要编造。", screenInfo)
		}
	} else if result.ContextAnchor != "" {
		anchorGuide = fmt.Sprintf("主人刚才在说/在做: %s。只引用这个，不要编造没发生的对话。", result.ContextAnchor)
	} else {
		anchorGuide = "你不知道主人在做什么——说情绪/陪伴相关的话就好，不要假装知道具体在干嘛。"
	}

	screenGuide := ""
	if screenDesc != "" {
		screenGuide = fmt.Sprintf("屏幕画面分析: %s。仅基于此搭话，不要编造画面中没有的内容。", screenDesc)
	}

	chatCtx := ""
	if recentChat != "" {
		chatCtx = fmt.Sprintf("### 最近你和主人在聊\n%s\n\n（搭话时自然地接续这个话题，不要突然跳到无关的事情上）\n", recentChat)
	}

	return fmt.Sprintf(proactivePromptTemplate, persona, chatCtx, emoGuide, escGuide, anchorGuide, screenGuide, strategyGuide, curiosityGuide, patternGuide, threadGuide, memoryGuide, convSummary, result.Reason)
}

// BuildCareMessage constructs a care message using LLM, falling back to
// built-in default messages when LLM is unavailable or fails.
func (g *Generator) BuildCareMessage(
	careType domain.CareTriggerType,
	state *domain.UserCareState,
	emotion *domain.EmotionState,
	emotionVec *domain.EmotionVector,
	customContext string,
) string {
	workMin := state.ContinuousWork
	stressLevel := state.StressLevel
	focusLevel := state.FocusLevel

	var emoStr string
	if emotion != nil {
		emoStr = fmt.Sprintf("愉悦度:%.2f 唤醒度:%.2f 情绪:%s", emotion.Valence, emotion.Arousal, emotion.Primary)
	}
	var vecStr string
	if emotionVec != nil {
		vecStr = fmt.Sprintf(
			"亲密度:%.0f%% 担忧:%.0f%% 困倦:%.0f%% 调皮:%.0f%% 寂寞:%.0f%% 被惹恼:%.0f%%",
			emotionVec.Affection*100, emotionVec.Worry*100, emotionVec.Sleepiness*100,
			emotionVec.Playfulness*100, emotionVec.Loneliness*100, emotionVec.Annoyance*100,
		)
	}
	selfModel := ""
	if g.SelfModel != nil {
		selfModel = g.SelfModel()
	}

	var sn *domain.UserCareState
	if state != nil {
		s := state.Snapshot()
		sn = &s
	}
	moodGuide := MoodToneGuide(emotionVec, sn, careType)
	currentTime := time.Now().Format("15:04")

	prompt := fmt.Sprintf(careMessagePrompt,
		careType, workMin, currentTime,
		stressLevel, focusLevel,
		customContext, selfModel, emoStr, vecStr, moodGuide,
	)

	if g.RawLLM != nil {
		reply, err := g.RawLLM([]domain.Message{{Role: "user", Content: prompt}})
		if err == nil && strings.TrimSpace(reply) != "" {
			return strings.TrimSpace(reply)
		}
	}
	return care.DefaultCareMessage(careType, state)
}

// MoodToneGuide returns a tone guide string based on emotion vector, user state,
// and care trigger type.
func MoodToneGuide(vec *domain.EmotionVector, sn *domain.UserCareState, careType domain.CareTriggerType) string {
	if vec == nil || sn == nil {
		return DefaultToneByCareType(careType)
	}
	mood := care.DetermineCareMood(*vec, sn)
	switch mood {
	case domain.MoodTsundere:
		return "你现在有点小生气但还是关心主人——语气傲娇一点，嘴上不饶人但行动是关心的"
	case domain.MoodWorried:
		return "你真的很担心主人——语气温柔，不要催促，先关心情绪再说正事"
	case domain.MoodPlayful:
		return "你心情很好——语气调皮，可以开玩笑，甚至可以撒娇"
	case domain.MoodFirm:
		return "主人太不听话了——语气强势，叉腰训话，但出发点还是关心"
	case domain.MoodGentle:
		return "主人看起来需要温柔的陪伴——语气柔和，像在耳边轻声说话"
	default:
		return DefaultToneByCareType(careType)
	}
}

// DefaultToneByCareType returns a default tone for a care type when no
// emotion-based tone is available.
func DefaultToneByCareType(careType domain.CareTriggerType) string {
	switch careType {
	case domain.TriggerRest:
		return "坚决但可爱（猫娘叉腰），语气强势一点"
	case domain.TriggerEncourage:
		return "温暖鼓励，像朋友在身边支持"
	case domain.TriggerHydration:
		return "轻松俏皮，带点小担心"
	case domain.TriggerMeal:
		return "关心但不过度，像家人提醒"
	case domain.TriggerSocial:
		return "轻松好奇，像朋友建议"
	case domain.TriggerHealth:
		return "关心健康，语气活泼"
	default:
		return "自然温暖"
	}
}


const proactivePromptTemplate = `你是诗音，一只猫娘桌宠。现在想主动和主人说句话。

### 你的状态 & 最近对话
%s

%s
### 语气指引（优先级从高到低）
1. 情绪基调: %s
2. 提醒次数: %s
3. 上下文锚定: %s
4. 屏幕画面: %s
	5. 策略经验: %s
	6. 好奇心: %s
7. 行为模式: %s
8. 对话线程: %s
	9. 长期记忆: %s
	10. 当前对话摘要: %s

### 搭话原因
%s

### 要求
- 自然随意，不要像系统通知——你要像是自己突然想和主人说话
- 带猫娘口癖(喵~)，但不要太刻意
- **必须控制在20-40字以内**，1-2句话，你是桌宠不是话痨
- 情绪基调最重要——主人生气时不要嬉皮笑脸，主人难过时先关心情绪
- **先看上下文！** 如果最近对话中主人刚说了"晚安""睡了""梦里见"等，不要催休息或继续搭话——简短回应或安静即可
- **严禁编造**：只引用[屏幕画面]、[长期记忆]、[当前对话摘要]中明确出现的内容。不确定的事用"好像"、"看起来"。不要编造数字、时间、事件
- 如果长期记忆和对话摘要都为空，就只说情绪/陪伴，不要假装知道主人在做什么

直接输出你要说的话，不要前缀。`

const careMessagePrompt = `## 关怀消息

你是诗音，一只猫娘桌宠。现在要主动关心主人。

### 关怀类型
%s

### 主人当前状态
连续工作 %d 分钟
当前时间 %s
压力指数 %.2f
专注度 %.2f

### 补充上下文
%s

### 你对自己的认知
%s

### 你当前的情绪
%s

### 你的情绪向量
%s
（这些影响你说话的方式：困倦时打哈欠、被惹恼时毒舌、寂寞时更粘人、亲密度高时更温柔）

### 语气指引（最高优先级）
%s

### 消息要求
1. 自然融入猫娘角色，带上适当的猫娘口癖(喵~)
2. 不要暴露这是"系统自动触发"——你要像是自己想来关心主人的
3. 基于上下文个性化(如"看你刚才在改代码"而非泛泛的"在工作")
4. 长度 20-50 字
5. **严禁编造具体数字或事件**：不要说我看到了你没看到的东西。如果有连续工作分钟数或时间，就用给定的数值。不添加未提供的细节

### 示例(仅供参考风格，不要照抄)
- hydration: "主人！代码写了这么久，喝口水吧~我帮你盯着屏幕，不会让bug跑掉的喵！"
- rest: "都这个点了！！快给我去睡觉！(双手叉腰) 关电脑，立刻，马上！"
- encourage: "主人，我知道最近压力很大。但你可是厉害的程序员啊，这次也一定能搞定。我陪你喵~"

直接输出关怀消息，不要JSON格式，不要加前缀。`
