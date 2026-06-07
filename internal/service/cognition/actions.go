package cognition

import "sync"

// ActionDef defines a single action available to both the scorer and the LLM fallback.
// It is the single source of truth — weights, skill cards, outcome tracking, and
// execution metadata all live here.
type ActionDef struct {
	Name        string       // "search", "speak_inquiry", etc.
	DisplayName string       // "搜索学习" — human-readable label for UI
	Category    string       // "social" | "care" | "learning" | "none"
	Description string       // one-line what this action does (for LLM menus)
	SkillCard   string       // when/how/output guidance for LLM decision prompts
	Weights     ActionWeight  // scorer drive weights
	NightSafe   bool         // allowed during 22:00-08:00
	NeedsTool   bool         // requires tool execution (not just speak)
	ToolName    string       // name in tool registry (set only when NeedsTool)
	ToolHint    string       // what the LLM should put in ToolInput
	OutcomeType string       // value stored in action_outcomes.action_type for R3/A7
	Source      string       // ProactiveSource: "care" | "casual" | "knowledge_gap"
}

// ---- cached action registry ----

var (
	actionsOnce sync.Once
	actionsList []ActionDef
)

// AllActions returns every action in the decision space, ordered by category.
// Result is cached after first call — safe for concurrent use.
func AllActions() []ActionDef {
	actionsOnce.Do(func() {
		actionsList = buildActions()
	})
	return actionsList
}

func buildActions() []ActionDef {
	return []ActionDef{
		// ---- Social ----
		{
			Name: "speak_casual", DisplayName: "轻松闲聊", Category: "social",
			Description: "和主人轻松闲聊，吐槽或逗乐",
			SkillCard: "when: 主人空闲、情绪好、互动频率适中\n" +
				"how: 不调工具，直接生成对话。mood: playful/tsundere\n" +
				"output: 1-2句，自然猫娘风格",
			Weights:     ActionWeight{Social: 0.80, Care: 0.15, Curious: 0.05, Quiet: -0.30, Explore: 0.00},
			NightSafe:   false,
			OutcomeType: "social", Source: "casual",
		},
		{
			Name: "speak_inquiry", DisplayName: "好奇搭话", Category: "social",
			Description: "用学到的知识或好奇心缺口打开话题",
			SkillCard: "when: 有活跃探索目标或新学到的知识，主人不太忙\n" +
				"how: 从 PickBestInquiry 获取话题。mood: curious/gentle\n" +
				"output: 自然引出话题，不要像考试提问",
			Weights:     ActionWeight{Social: 0.40, Care: 0.00, Curious: 0.60, Quiet: 0.00, Explore: 0.10},
			NightSafe:   false,
			OutcomeType: "social", Source: "knowledge_gap",
		},
		{
			Name: "speak_care", DisplayName: "关心搭话", Category: "social",
			Description: "关心主人的健康、作息、情绪",
			SkillCard: "when: 主人连续工作久、深夜、饭点、情绪低落\n" +
				"how: CareEngine 生成具体内容。mood: gentle/worried\n" +
				"output: 温暖的提醒，不要像说教",
			Weights:     ActionWeight{Social: 0.40, Care: 0.70, Curious: 0.00, Quiet: -0.20, Explore: 0.00},
			NightSafe:   false,
			OutcomeType: "encourage", Source: "care",
		},

		// ---- Care ----
		{
			Name: "care_rest", DisplayName: "提醒休息", Category: "care",
			Description: "提醒主人休息",
			SkillCard: "when: 连续工作超90分钟\nhow: CareEngine trigger。mood: gentle/firm\noutput: 温柔的休息提醒",
			Weights:     ActionWeight{Social: 0.10, Care: 0.75, Curious: 0.00, Quiet: 0.00, Explore: 0.00},
			NightSafe:   true,
			OutcomeType: "rest", Source: "care",
		},
		{
			Name: "care_meal", DisplayName: "饭点关心", Category: "care",
			Description: "关心主人是否按时吃饭",
			SkillCard: "when: 午间(11-13)或晚间(17-20)\nhow: CareEngine trigger。mood: gentle\noutput: 提醒吃饭，可以推荐食物",
			Weights:     ActionWeight{Social: 0.10, Care: 0.70, Curious: 0.00, Quiet: 0.00, Explore: 0.00},
			NightSafe:   false,
			OutcomeType: "meal", Source: "care",
		},
		{
			Name: "care_hydration", DisplayName: "喝水提醒", Category: "care",
			Description: "提醒主人喝水",
			SkillCard: "when: 定时触发(间隔45分钟)\nhow: CareEngine trigger。mood: gentle\noutput: 简短提醒喝水",
			Weights:     ActionWeight{Social: 0.05, Care: 0.65, Curious: 0.00, Quiet: 0.00, Explore: 0.00},
			NightSafe:   true,
			OutcomeType: "hydration", Source: "care",
		},
		{
			Name: "care_health", DisplayName: "健康关怀", Category: "care",
			Description: "关心主人的身体健康",
			SkillCard: "when: 深夜还在工作、或长时间未活动\nhow: CareEngine trigger。mood: worried/gentle\noutput: 温和的健康提醒",
			Weights:     ActionWeight{Social: 0.05, Care: 0.65, Curious: 0.00, Quiet: 0.00, Explore: 0.00},
			NightSafe:   true,
			OutcomeType: "health", Source: "care",
		},
		{
			Name: "care_encourage", DisplayName: "鼓励打气", Category: "care",
			Description: "给主人加油打气",
			SkillCard: "when: 主人情绪低落或遇到困难\nhow: CareEngine trigger。mood: gentle/playful\noutput: 暖心的鼓励，不空洞",
			Weights:     ActionWeight{Social: 0.20, Care: 0.55, Curious: 0.00, Quiet: 0.00, Explore: 0.00},
			NightSafe:   false,
			OutcomeType: "encourage", Source: "care",
		},
		{
			Name: "care_social", DisplayName: "社交关怀", Category: "care",
			Description: "关心主人的社交状态",
			SkillCard: "when: 主人长时间独处\nhow: CareEngine trigger。mood: gentle\noutput: 轻松的建议",
			Weights:     ActionWeight{Social: 0.30, Care: 0.40, Curious: 0.00, Quiet: 0.00, Explore: 0.00},
			NightSafe:   false,
			OutcomeType: "social", Source: "care",
		},

		// ---- Learning (background, tool-based) ----
		{
			Name: "search", DisplayName: "主动搜索学习", Category: "learning",
			Description: "搜索互联网获取新知识，填补好奇心缺口",
			SkillCard: "when: 好奇驱力>0.5 且有活跃探索目标或知识缺口，配额充足\n" +
				"how: 从 CuriosityItem 中选最佳目标作为搜索词，" +
				"填写 tool_input=搜索关键词\n" +
				"output: 搜索结果会由系统自动评判并存入记忆，" +
				"后续对话中自然体现\n" +
				"cooldown: 至少间隔30分钟，每天最多5次",
			Weights:     ActionWeight{Social: 0.05, Care: 0.05, Curious: 0.45, Quiet: -0.10, Explore: 0.30},
			NightSafe:   true, NeedsTool: true, ToolName: "search",
			ToolHint:    "搜索关键词 — 从 PickBestInquiry 获取或根据当前场景生成",
			OutcomeType: "search", Source: "knowledge_gap",
		},
		{
			Name: "observe", DisplayName: "屏幕观察", Category: "learning",
			Description: "观察主人屏幕，发现新的知识缺口",
			SkillCard: "when: 好奇驱力>0.6 且距上次观察>15分钟\n" +
				"how: 截屏 → Vision LLM 分析 → 生成知识缺口\n" +
				"output: 静默执行，不打扰主人",
			Weights:     ActionWeight{Social: 0.10, Care: 0.00, Curious: 0.30, Quiet: 0.00, Explore: 0.60},
			NightSafe:   true, NeedsTool: true, ToolName: "observe",
			OutcomeType: "observe", Source: "knowledge_gap",
		},
		{
			Name: "reflect", DisplayName: "策略反思", Category: "learning",
			Description: "反思最近的行动结果，提炼行为策略",
			SkillCard: "when: 积累足够反馈(≥10条) 且距上次反思>6小时\n" +
				"how: StrategicAgent.Run() → 生成策略原则\n" +
				"output: 静默执行，结果存入策略库",
			Weights:     ActionWeight{Social: 0.00, Care: 0.00, Curious: 0.00, Quiet: 0.20, Explore: 0.75},
			NightSafe:   true, NeedsTool: true, ToolName: "reflect",
			OutcomeType: "reflect", Source: "knowledge_gap",
		},
		{
			Name: "analyze_patterns", DisplayName: "模式分析", Category: "learning",
			Description: "从主人行为中挖掘习惯模式",
			SkillCard: "when: 积累了足够的行为数据(≥20条活动事件)\n" +
				"how: PatternAnalyzer.Analyze() → 发现行为模式\n" +
				"output: 静默执行",
			Weights:     ActionWeight{Social: 0.00, Care: 0.00, Curious: 0.20, Quiet: 0.00, Explore: 0.65},
			NightSafe:   true, NeedsTool: false, // handled by special case in lifecycle.go
			OutcomeType: "analyze", Source: "knowledge_gap",
		},

		// ---- None ----
		{
			Name: "none", DisplayName: "不行动", Category: "none",
			Description: "当前不适合采取任何行动",
			SkillCard: "when: 主人忙碌、刚被拒绝、配额耗尽、" +
				"深夜(除非care_rest/care_health)\noutput: 静默等待下一轮",
			Weights:     ActionWeight{Social: 0.00, Care: 0.00, Curious: 0.00, Quiet: 1.00, Explore: 0.00},
			NightSafe:   true,
		},
	}
}

// ---- Lookup helpers ----

// ActionByName returns the ActionDef for a given action name, or nil.
func ActionByName(name string) *ActionDef {
	for i := range AllActions() {
		if AllActions()[i].Name == name {
			return &AllActions()[i]
		}
	}
	return nil
}

// BuildWeightsMap returns the scorer-compatible map of action name → ActionWeight.
func BuildWeightsMap() map[string]ActionWeight {
	m := make(map[string]ActionWeight, 16)
	for _, a := range AllActions() {
		m[a.Name] = a.Weights
	}
	return m
}

// BuildNightActions returns the set of action names allowed during nighttime.
func BuildNightActions() map[string]bool {
	m := make(map[string]bool, 16)
	for _, a := range AllActions() {
		if a.NightSafe {
			m[a.Name] = true
		}
	}
	return m
}

// BuildDecisionSkills returns a formatted skill menu for the LLM decision prompt.
func BuildDecisionSkills() string {
	var sb string
	sb = "可选动作:\n\n"
	for _, a := range AllActions() {
		sb += "--- " + a.Name + " (" + a.DisplayName + ") ---\n"
		sb += a.SkillCard + "\n\n"
	}
	sb += "输出JSON: {\"should_act\":true,\"action\":\"动作名\",\"source\":\"来源(care/casual/knowledge_gap)\"," +
		"\"reason\":\"为什么选这个\",\"mood\":\"gentle|playful|firm|tsundere|worried\"," +
		"\"priority\":0.0-1.0,\"tool_input\":\"搜索词或URL(仅search/browse需要)\"}\n"
	return sb
}

// isSpeakAction returns true for actions that involve talking to the user.
func isSpeakAction(action string) bool {
	a := ActionByName(action)
	if a == nil {
		return false
	}
	return a.Category == "social" || a.Category == "care"
}
