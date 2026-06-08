package cognition

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"desktop-pet/internal/api"
	"desktop-pet/internal/domain"; "desktop-pet/internal/infra"
)

// DecisionEngine wraps LLM-driven autonomous decision-making (System 2).
// It assembles rich context from all learning components and asks the LLM
// to decide what the AI should do next.
// Reflexion: recent decisions and their outcomes are fed back into the prompt,
// enabling the LLM to learn from experience without weight updates.
type DecisionEngine struct {
	rawLLM func([]domain.Message) (string, error)

	// toolLLM is the function-calling path. When set, Decide() uses it instead of
	// the rawLLM + JSON-parse path. Returns (toolName, toolArgsJSON, error).
	toolLLM func(messages []domain.Message, tools []DecisionToolSpec) (string, string, error)

	mu             sync.Mutex
	lastDecisionAt time.Time
	minInterval    time.Duration
	idleCount      int // for exponential backoff
	deciding       bool

	// Reflexion memory: recent (context summary, decision, outcome) triplets.
	reflexionLog    []reflexionEntry
	reflexionPath   string // JSON file path for persistence
}

type reflexionEntry struct {
	contextSummary string
	decision       domain.DecisionOutput
	outcome        string // "accepted" | "ignored" | "rejected"
	at             time.Time
}

// NewDecisionEngine creates a System 2 decision engine.
func NewDecisionEngine(rawLLM func([]domain.Message) (string, error)) *DecisionEngine {
	return &DecisionEngine{
		rawLLM:      rawLLM,
		minInterval: 15 * time.Minute,
	}
}

// SetToolLLM enables the function-calling decision path. When set, Decide() calls
// the LLM with 16 action tools + tool_choice="required", eliminating JSON parsing.
func (e *DecisionEngine) SetToolLLM(fn func(messages []domain.Message, tools []DecisionToolSpec) (string, string, error)) {
	e.toolLLM = fn
}

// SetStoragePath enables Reflexion log persistence to a JSON file.
func (e *DecisionEngine) SetStoragePath(path string) {
	e.reflexionPath = path
	e.loadReflexionLog()
}

func (e *DecisionEngine) loadReflexionLog() {
	if e.reflexionPath == "" {
		return
	}
	data, err := os.ReadFile(e.reflexionPath)
	if err != nil {
		return
	}
	var log []reflexionEntry
	if json.Unmarshal(data, &log) == nil {
		e.reflexionLog = log
	}
}

func (e *DecisionEngine) saveReflexionLog() {
	if e.reflexionPath == "" {
		return
	}
	data, err := json.Marshal(e.reflexionLog)
	if err != nil {
		slog.Warn("decision: failed to marshal reflexion log", "err", err)
		return
	}
	if err := os.WriteFile(e.reflexionPath, data, 0644); err != nil {
		slog.Warn("decision: failed to write reflexion log", "path", e.reflexionPath, "err", err)
	}
}

// ShouldRun returns true when it's time for the next System 2 decision tick.
// Uses Alife-style exponential backoff: 15min × 3^idleCount.
// Returns false if a decision is already in progress.
func (e *DecisionEngine) ShouldRun() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.deciding {
		return false
	}
	interval := e.minInterval
	for i := 0; i < e.idleCount && i < 4; i++ {
		interval *= 3
	}
	if time.Since(e.lastDecisionAt) <= interval {
		return false
	}
	e.deciding = true
	return true
}

// ResetIdle resets the idle counter (called after any user interaction).
func (e *DecisionEngine) ResetIdle() {
	e.idleCount = 0
}

// Decide is the LLM fallback for complex/edge scenarios.
// When toolLLM is set (v0.4+), it uses function calling with 16 action tools
// instead of the legacy prompt + JSON-parse path.
func (e *DecisionEngine) Decide(ctx domain.DecisionContext, feats *domain.QuantifiedFeatures, needs *domain.IntrinsicNeeds) (*domain.DecisionOutput, error) {
	return e.DecideFull(ctx, feats, needs)
}

// DecideFull uses the full decision prompt (~370 tokens) for complex scenarios.
func (e *DecisionEngine) DecideFull(ctx domain.DecisionContext, feats *domain.QuantifiedFeatures, needs *domain.IntrinsicNeeds) (*domain.DecisionOutput, error) {
	api.StatusBusInstance().EmitStart("decision", "LLM full decision...")
	if e.toolLLM != nil {
		return e.decideWithPrompt(e.buildDecisionSystemPrompt(ctx, feats, needs))
	}
	return e.decideLegacy(ctx, feats, needs)
}

// DecideLite uses a compact prompt (~200 tokens) for simpler scenarios.
func (e *DecisionEngine) DecideLite(ctx domain.DecisionContext, feats *domain.QuantifiedFeatures, needs *domain.IntrinsicNeeds) (*domain.DecisionOutput, error) {
	api.StatusBusInstance().EmitStart("decision", "LLM lite decision...")
	prompt := buildS2LitePrompt(feats, nil)
	return e.decideWithPrompt(prompt)
}

// decideWithPrompt is the shared function-calling path.
func (e *DecisionEngine) decideWithPrompt(prompt string) (*domain.DecisionOutput, error) {
	if e.toolLLM == nil {
		return nil, fmt.Errorf("decision engine: toolLLM not set")
	}
	messages := []domain.Message{{Role: "system", Content: prompt}}
	tools := BuildDecisionTools()

	toolName, toolArgs, err := e.toolLLM(messages, tools)
	if err != nil {
		slog.Warn("decision: tool LLM failed", "err", err)
		e.mu.Lock()
		e.deciding = false
		e.mu.Unlock()
		api.StatusBusInstance().EmitFail("decision", err.Error())
		return nil, err
	}

	if toolName == "" {
		slog.Warn("decision: LLM returned no tool call, defaulting to none")
		e.finishDecision("none")
		api.StatusBusInstance().EmitOK("decision", "none: no tool called")
		return &domain.DecisionOutput{ShouldAct: false, Action: "none", Reason: "no tool called"}, nil
	}

	var args struct {
		Reason    string `json:"reason"`
		Mood      string `json:"mood"`
		ToolInput string `json:"tool_input"`
	}
	if toolArgs != "" {
		json.Unmarshal([]byte(toolArgs), &args)
	}

	def := ActionByName(toolName)
	source := ""
	if def != nil {
		source = def.Source
	}

	output := &domain.DecisionOutput{
		ShouldAct: toolName != "none",
		Action:    toolName,
		Source:    source,
		Reason:    args.Reason,
		Mood:      args.Mood,
		ToolInput: args.ToolInput,
		Priority:  0.8,
	}

	e.finishDecision(toolName)
	api.StatusBusInstance().EmitOK("decision", toolName+"/"+source+": "+args.Reason)
	return output, nil
}

// decideLegacy is the pre-v0.4 prompt + JSON-parse path, kept as fallback.
func (e *DecisionEngine) decideLegacy(ctx domain.DecisionContext, feats *domain.QuantifiedFeatures, needs *domain.IntrinsicNeeds) (*domain.DecisionOutput, error) {
	if e.rawLLM == nil {
		return nil, fmt.Errorf("decision engine: no LLM available")
	}

	prompt := e.buildFallbackPrompt(ctx, feats, needs)
	result, err := e.rawLLM([]domain.Message{{Role: "user", Content: prompt}})
	if err != nil {
		slog.Warn("decision: LLM fallback failed", "err", err)
		e.mu.Lock()
		e.deciding = false
		e.mu.Unlock()
		api.StatusBusInstance().EmitFail("decision", err.Error())
		return nil, err
	}

	raw := infra.CleanJSON(result)
	var output domain.DecisionOutput
	if err := json.Unmarshal([]byte(raw), &output); err != nil {
		slog.Warn("decision: JSON parse failed", "err", err, "raw", raw[:min(len(raw), 200)])
		e.finishDecision("none")
		return &domain.DecisionOutput{ShouldAct: false, Action: "none", Reason: "parse error"}, nil
	}

	if !output.ShouldAct && output.Action != "" && output.Action != "none" {
		output.ShouldAct = true
	}

	e.finishDecision(output.Action)
	api.StatusBusInstance().EmitOK("decision", output.Action+"/"+output.Source+": "+output.Reason)
	return &output, nil
}

// finishDecision updates internal state after a decision is made.
func (e *DecisionEngine) finishDecision(action string) {
	e.mu.Lock()
	e.lastDecisionAt = time.Now()
	if action == "none" {
		e.idleCount++
	} else {
		e.idleCount = 0
	}
	e.deciding = false
	e.mu.Unlock()
}

// buildDecisionSystemPrompt creates a brief system prompt for the function-calling
// decision path. Unlike the legacy fallback prompt, this is concise — the detailed
// action guidance lives in each tool's description, not in the prompt text.
func (e *DecisionEngine) buildDecisionSystemPrompt(ctx domain.DecisionContext, feats *domain.QuantifiedFeatures, needs *domain.IntrinsicNeeds) string {
	var sb strings.Builder
	sb.WriteString("你是诗音，一只关心主人的猫娘桌面宠物。现在是自主决策时刻——你需要从可用工具中选择一个动作。\n\n")

	// User context (compact).
	if feats != nil {
		workLabel := "休闲中"
		if feats.U3_IsWorking > 0 && feats.U4_ContinuousWorkMins >= 5 {
			workLabel = fmt.Sprintf("工作中(已%.0f分钟)", feats.U4_ContinuousWorkMins)
		}
		sb.WriteString(fmt.Sprintf("主人: %s %s\n", feats.U1_AppCategory, workLabel))
		if feats.U12_NightTime > 0 {
			sb.WriteString("⚠️ 深夜时段，避免打扰\n")
		}
		if feats.R4_RecentRejections >= 2 {
			sb.WriteString(fmt.Sprintf("⚠️ 最近被拒绝%.0f次，谨慎搭话\n", feats.R4_RecentRejections))
		}
	}

	// Emotion snapshot (compact).
	sb.WriteString(fmt.Sprintf("情绪: %s(%.0f%%) 亲密度%.0f%% 困倦%.0f%% 烦躁%.0f%%\n",
		ctx.EmotionState.Primary, ctx.EmotionState.Intensity*100,
		ctx.EmotionVec.Affection*100, ctx.EmotionVec.Sleepiness*100, ctx.EmotionVec.Annoyance*100))

	// Needs snapshot.
	if needs != nil {
		sb.WriteString(fmt.Sprintf("需求: 陪伴%.0f 关怀%.0f 好奇%.0f 休息%.0f\n",
			needs.Companionship*100, needs.Care*100, needs.Curiosity*100, needs.Rest*100))
	}

	// Recent user message.
	if ctx.RecentUserMsg != "" {
		sb.WriteString(fmt.Sprintf("最近消息: \"%s\"\n", ctx.RecentUserMsg))
	}

	sb.WriteString(fmt.Sprintf("距上次互动: %.0f分钟\n", ctx.TimeSinceLastChat.Minutes()))
	sb.WriteString("\n选择一个最合适的动作。如果主人正忙或深夜且非紧急，选 none。")

	return sb.String()
}

// buildFallbackPrompt creates a structured prompt with full quantified context.
// feats and needs may be nil — sections are omitted gracefully.
func (e *DecisionEngine) buildFallbackPrompt(ctx domain.DecisionContext, feats *domain.QuantifiedFeatures, needs *domain.IntrinsicNeeds) string {
	var sb strings.Builder
	sb.WriteString("你是诗音，一只关心主人的猫娘。现在遇到了需要你判断的复杂场景。\n\n")

	// ================================================================
	// Block 1: User State (主人状态)
	// ================================================================
	sb.WriteString("[主人状态]\n")
	if feats != nil {
		// App + working status.
		workLabel := "休闲中"
		if feats.U3_IsWorking > 0 && feats.U4_ContinuousWorkMins >= 5 {
			workLabel = "工作中"
		} else if feats.U3_IsWorking > 0 {
			workLabel = "刚打开编辑器"
		}
		sb.WriteString(fmt.Sprintf("  当前: %s (%s)\n", feats.U1_AppCategory, workLabel))
		if feats.U2_WindowSubtype != "" {
			sb.WriteString(fmt.Sprintf("  窗口: %s\n", feats.U2_WindowSubtype))
		}
		// Work duration + app switching.
		if feats.U4_ContinuousWorkMins > 0 {
			sb.WriteString(fmt.Sprintf("  连续工作: %.0f分钟", feats.U4_ContinuousWorkMins))
			if feats.U5_AppSwitchCount > 0 {
				sb.WriteString(fmt.Sprintf(" | 30分钟切换: %.0f次", feats.U5_AppSwitchCount))
			}
			sb.WriteString("\n")
		}
		// Time context.
		mealTag := ""
		if feats.U11_MealTime > 0 {
			mealTag = "饭点, "
		}
		nightTag := ""
		if feats.U12_NightTime > 0 {
			nightTag = "深夜, "
		}
		weekendTag := ""
		if feats.U13_IsWeekend > 0 {
			weekendTag = "周末, "
		}
		sb.WriteString(fmt.Sprintf("  时段: %s 周%d (%s%s%s)\n",
			ctx.Now.Format("15:04"), int(ctx.Now.Weekday()), mealTag, nightTag, weekendTag))
	} else {
		// Degraded: basic time info from ctx.
		sb.WriteString(fmt.Sprintf("  时间: %s 周%d\n", ctx.Now.Format("15:04"), int(ctx.Now.Weekday())))
		if ctx.ScreenSummary != "" {
			sb.WriteString(fmt.Sprintf("  屏幕: %s\n", ctx.ScreenSummary))
		}
	}
	sb.WriteString("\n")

	// ================================================================
	// Block 2: Relationship (互动关系)
	// ================================================================
	if feats != nil {
		sb.WriteString("[互动关系]\n")
		if feats.R1_SampleCount > 0 {
			sb.WriteString(fmt.Sprintf("  整体接受率: %.0f%% (最近%.0f条)\n",
				feats.R1_OverallAcceptRate*100, feats.R1_SampleCount))
		} else {
			sb.WriteString(fmt.Sprintf("  整体接受率: %.0f%%\n", feats.R1_OverallAcceptRate*100))
		}
		if feats.R4_RecentRejections > 0 {
			sb.WriteString(fmt.Sprintf("  最近拒绝: %.0f次 (严重度: %.2f)\n",
				feats.R4_RecentRejections, feats.R4_RejectionSeverity))
		}
		sb.WriteString(fmt.Sprintf("  距上次用户消息: %.0f分钟\n", feats.R5_NeglectHours*60))
		// Message trends.
		if feats.U7_LengthTrend < -0.3 {
			sb.WriteString(fmt.Sprintf("  消息趋势: 变短 (趋势%.2f, 用户可能在敷衍)\n", feats.U7_LengthTrend))
		} else if feats.U7_LengthTrend > 0.3 {
			sb.WriteString(fmt.Sprintf("  消息趋势: 变长 (趋势%.2f, 用户投入中)\n", feats.U7_LengthTrend))
		}
		if feats.R6_DepthTrend < -0.3 {
			sb.WriteString(fmt.Sprintf("  对话深度: 变浅 (趋势%.2f)\n", feats.R6_DepthTrend))
		} else if feats.R6_DepthTrend > 0.3 {
			sb.WriteString(fmt.Sprintf("  对话深度: 加深 (趋势%.2f)\n", feats.R6_DepthTrend))
		}
		sb.WriteString("\n")
	}

	// ================================================================
	// Block 3: Agent State (你的状态)
	// ================================================================
	sb.WriteString("[你的状态]\n")
	// Emotion (always available from ctx).
	sb.WriteString(fmt.Sprintf("  主情绪: %s (强度%.0f%%)\n", ctx.EmotionState.Primary, ctx.EmotionState.Intensity*100))
	sb.WriteString(fmt.Sprintf("  情感%.0f%% 担忧%.0f%% 好奇%.0f%% 困倦%.0f%% 贪玩%.0f%% 寂寞%.0f%% 烦躁%.0f%%\n",
		ctx.EmotionVec.Affection*100, ctx.EmotionVec.Worry*100, ctx.EmotionVec.Curiosity*100,
		ctx.EmotionVec.Sleepiness*100, ctx.EmotionVec.Playfulness*100, ctx.EmotionVec.Loneliness*100,
		ctx.EmotionVec.Annoyance*100))
	// Intrinsic needs.
	if needs != nil {
		needParts := []string{}
		addNeed := func(label string, v float64, high bool) {
			tag := ""
			if high {
				tag = "(高!)"
			}
			needParts = append(needParts, fmt.Sprintf("%s%.0f%%%s", label, v*100, tag))
		}
		addNeed("陪伴", needs.Companionship, needs.Companionship > 0.7)
		addNeed("关怀", needs.Care, needs.Care > 0.7)
		addNeed("玩耍", needs.Play, needs.Play > 0.7)
		addNeed("好奇", needs.Curiosity, needs.Curiosity > 0.7)
		addNeed("休息", needs.Rest, needs.Rest > 0.7)
		addNeed("自主", needs.Autonomy, needs.Autonomy > 0.7)
		sb.WriteString("  内在需求: " + strings.Join(needParts, " ") + "\n")
	}
	// Daily quota.
	if feats != nil {
		sb.WriteString(fmt.Sprintf("  今日已行动: %.0f次 (配额剩余%.0f)\n",
			feats.A6_DailyActionCount, feats.E4_QuotaRemaining))
	}
	sb.WriteString("\n")

	// ================================================================
	// Block 4: Historical Experience (历史经验)
	// ================================================================
	if feats != nil && len(feats.A7_ActionSuccessRate) > 0 {
		sb.WriteString("[历史经验]\n")
		for typ, rate := range feats.A7_ActionSuccessRate {
			sb.WriteString(fmt.Sprintf("  %s成功率: %.0f%%\n", typ, rate*100))
		}
		if feats.U10_TimeWindowPref > 0 {
			sb.WriteString(fmt.Sprintf("  当前时段(%dh)接受率: %.0f%%\n",
				int(feats.E1_Hour), feats.U10_TimeWindowPref*100))
		}
		// Available strategies.
		princCount := int(feats.T1_PrincipleCount)
		reflCount := int(feats.T3_ReflexionLogCount)
		if princCount > 0 || reflCount > 0 {
			sb.WriteString(fmt.Sprintf("  可用策略: %d条  反思记忆: %d条\n", princCount, reflCount))
		}
		sb.WriteString("\n")
	}

	// ================================================================
	// AI self-model and recent user facts (filled from DecisionContext).
	if ctx.SelfSummary != "" {
		sb.WriteString("[自我认知]\n")
		sb.WriteString(fmt.Sprintf("  %s\n", ctx.SelfSummary))
	}
	if len(ctx.RecentFactSample) > 0 {
		sb.WriteString("\n[最近了解到的主人信息]\n")
		for _, f := range ctx.RecentFactSample {
			sb.WriteString(fmt.Sprintf("  - %s\n", f))
		}
	}
	sb.WriteString("\n")

	// Self-learned strategies (from context — kept for compatibility).
	// ================================================================
	if len(ctx.ActivePrinciples) > 0 {
		sb.WriteString("从经验中学到的策略:\n")
		for _, p := range ctx.ActivePrinciples {
			if p.GoodStrategy != "" {
				sb.WriteString(fmt.Sprintf("  - 场景:%s → %s\n", p.Situation, p.GoodStrategy))
			}
		}
		sb.WriteString("\n")
	}

	// Recent outcomes (compact view).
	if len(ctx.RecentOutcomes) > 0 {
		sb.WriteString("最近行动结果: ")
		for _, o := range ctx.RecentOutcomes {
			label := ""
			switch o.Outcome {
			case 1:
				label = "✓"
			case -1:
				label = "✗"
			case 0:
				label = "-"
			}
			sb.WriteString(fmt.Sprintf("%s/%s%s ", o.ActionSource, o.ActionType, label))
		}
		sb.WriteString("\n")
	}

	// Reflexion memory.
	if len(e.reflexionLog) > 0 {
		sb.WriteString("反思记忆: ")
		start := len(e.reflexionLog) - 3
		if start < 0 {
			start = 0
		}
		for _, r := range e.reflexionLog[start:] {
			sb.WriteString(fmt.Sprintf("[%s→%s] ", r.contextSummary, r.outcome))
		}
		sb.WriteString("\n")
	}

	// ---- Decision Skill Cards ----
	// The full action menu with when/how/output guidance.
	// LLM picks from the same action space as the scorer.
	sb.WriteString(BuildDecisionSkills())
	return sb.String()
}

// FinishDecision resets the deciding flag. Called after every decision attempt
// completes, regardless of which code path was taken (LLM or motivator).
func (e *DecisionEngine) FinishDecision() {
	e.mu.Lock()
	e.deciding = false
	e.mu.Unlock()
}

// ForceRun resets the cooldown so the next ShouldRun() returns true.
func (e *DecisionEngine) ForceRun() {
	e.lastDecisionAt = time.Time{}
}

// ReflexionLogCount returns the number of reflexion entries.
func (e *DecisionEngine) ReflexionLogCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.reflexionLog)
}

// RecordOutcome stores a Reflexion entry so future decisions learn from this outcome.
func (e *DecisionEngine) RecordOutcome(ctxSummary string, dec domain.DecisionOutput, outcome string) {
	e.reflexionLog = append(e.reflexionLog, reflexionEntry{
		contextSummary: ctxSummary,
		decision:       dec,
		outcome:        outcome,
		at:             time.Now(),
	})
	if len(e.reflexionLog) > 20 {
		e.reflexionLog = e.reflexionLog[len(e.reflexionLog)-20:]
	}
	e.saveReflexionLog()
}

