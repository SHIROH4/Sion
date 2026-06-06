package cognition

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"desktop-pet/internal/domain"
	"desktop-pet/internal/infra"
	"desktop-pet/internal/infra/storage"
)

// StrategicAgent is the System 3 meta-cognitive layer. It runs once per day to
// reflect on the past day's interactions, extract reusable strategy principles,
// manage conversation threads, and issue tactical directives for the coming day.
type StrategicAgent struct {
	rawLLM        func([]domain.Message) (string, error)
	principleRepo domain.StrategyPrincipleRepository
	outcomeRepo   domain.ActionOutcomeRepository
	threadRepo    domain.ThreadRepository
	embedFn       func(string) ([]float32, error)

	selfModelFn func() string
	selfSaveFn  func(string) error
	diaryListFn func(n int) []domain.DiaryEntry
	factListFn  func() []domain.FactEntry

	lastRunAt   time.Time
	minInterval time.Duration
}

// NewStrategicAgent creates a StrategicAgent wired to the required dependencies.
func NewStrategicAgent(
	rawLLM func([]domain.Message) (string, error),
	principleRepo domain.StrategyPrincipleRepository,
	outcomeRepo domain.ActionOutcomeRepository,
	threadRepo domain.ThreadRepository,
	embedFn func(string) ([]float32, error),
) *StrategicAgent {
	return &StrategicAgent{
		rawLLM:        rawLLM,
		principleRepo: principleRepo,
		outcomeRepo:   outcomeRepo,
		threadRepo:    threadRepo,
		embedFn:       embedFn,
		minInterval:   6 * time.Hour,
	}
}

// SetSelfModel wires the self-model read/write callbacks.
func (a *StrategicAgent) SetSelfModel(current func() string, save func(string) error) {
	a.selfModelFn = current
	a.selfSaveFn = save
}

// SetDiaryList wires the diary list callback.
func (a *StrategicAgent) SetDiaryList(fn func(n int) []domain.DiaryEntry) {
	a.diaryListFn = fn
}

// SetFactList wires the fact list callback.
func (a *StrategicAgent) SetFactList(fn func() []domain.FactEntry) {
	a.factListFn = fn
}

// ShouldRun returns true if enough time has passed since the last run.
func (a *StrategicAgent) ShouldRun() bool {
	return time.Since(a.lastRunAt) > a.minInterval
}

// LastRun returns when the agent last ran.
func (a *StrategicAgent) LastRun() time.Time { return a.lastRunAt }

// Run executes the daily strategic reflection.
func (a *StrategicAgent) Run() (*domain.DailyReflectionOutput, error) {
	if a.rawLLM == nil {
		return nil, fmt.Errorf("strategic agent: no LLM available")
	}

	input := a.buildInput()
	prompt := buildStrategicPrompt(input)

	result, err := a.rawLLM([]domain.Message{{Role: "user", Content: prompt}})
	if err != nil {
		return nil, fmt.Errorf("strategic agent: LLM call failed: %w", err)
	}

	output, err := parseStrategicOutput(result)
	if err != nil {
		return nil, fmt.Errorf("strategic agent: parse failed: %w", err)
	}

	// Persist results.
	if a.selfSaveFn != nil && output.SelfModelUpdate != "" {
		_ = a.selfSaveFn(output.SelfModelUpdate)
	}

		for i := range output.NewPrinciples {
		p := &output.NewPrinciples[i]
		if p.Confidence > 1 {
			p.Confidence /= 100
		}
		if p.Situation == "" || p.GoodStrategy == "" || p.Confidence < 0.3 {
			continue
		}
		p.Source = "daily_reflection"
		p.Active = true

		// Embed and check for semantic duplicates via vector similarity.
		if a.embedFn != nil && a.principleRepo != nil {
			vec, err := a.embedFn(p.Situation + " " + p.GoodStrategy)
			if err == nil && len(vec) > 0 {
				p.Embedding = vec
				similar, _ := a.principleRepo.SearchSimilar(vec, 1)
				if len(similar) > 0 && len(similar[0].Embedding) > 0 {
					sim := storage.CosineSimilarity(vec, similar[0].Embedding)
					existing := similar[0]
					if sim > 0.75 {
						// Merge or replace based on confidence delta.
						if p.Confidence > existing.Confidence+0.1 {
							// New is significantly better → replace old.
							_ = a.principleRepo.Deactivate(existing.ID)
						} else if p.Confidence >= existing.Confidence-0.1 {
							// Similar confidence → merge via LLM.
							merged := mergePrinciples(a.rawLLM, existing, *p)
							if merged != nil {
								p = merged
							}
							_ = a.principleRepo.Deactivate(existing.ID)
						} else {
							// Existing is better → skip new.
							continue
						}
					}
				}
			}
			if _, err := a.principleRepo.SavePrinciple(*p); err != nil {
				slog.Warn("strategy: failed to save principle", "err", err)
			}
		}
	}

	for _, id := range output.DeactivatePrincipleIDs {
		if a.principleRepo != nil {
			_ = a.principleRepo.Deactivate(id)
		}
	}

	// Execute thread recommendations.
	if a.threadRepo != nil {
		for _, rec := range output.ThreadRecommendations {
			switch rec.Action {
			case "create":
				if rec.Goal != "" {
					t := domain.ConversationThread{
						Type:         domain.ThreadType(rec.Type),
						Goal:         rec.Goal,
						Status:       domain.ThreadActive,
						Priority:     rec.Priority,
						BestApproach: rec.BestApproach,
					}
					if t.Type == "" {
						t.Type = domain.ThreadFollowUp
					}
					if t.Priority <= 0 {
						t.Priority = 0.5
					}
					if _, err := a.threadRepo.SaveThread(t); err != nil {
					slog.Warn("strategy: failed to save thread", "err", err)
				}
				}
			case "resolve":
				if rec.ThreadID > 0 {
					_ = a.threadRepo.ResolveThread(rec.ThreadID, rec.Outcome, rec.Learnings)
				}
			case "stale":
				if rec.ThreadID > 0 {
					_ = a.threadRepo.MarkStale(rec.ThreadID)
				}
			}
		}
	}

	a.lastRunAt = time.Now()
	return output, nil
}

func (a *StrategicAgent) buildInput() *domain.DailyReflectionInput {
	in := &domain.DailyReflectionInput{}

	if a.selfModelFn != nil {
		in.CurrentSelfModel = a.selfModelFn()
	}

	if a.principleRepo != nil {
		in.ActivePrinciples, _ = a.principleRepo.ListActive()
	}

	if a.outcomeRepo != nil {
		ctx := domain.ActionContext{}
		accepts, total := a.outcomeRepo.SuccessRate(ctx, 1)
		in.InteractionCount = total
		if total > 0 {
			in.ProactiveAcceptRate = float64(accepts) / float64(total)
		}
	}

	if a.threadRepo != nil {
		in.ActiveThreads, _ = a.threadRepo.ListActive()
	}

	if a.diaryListFn != nil {
		diaries := a.diaryListFn(7)
		for _, d := range diaries {
			in.RecentDiaries = append(in.RecentDiaries, d.Summary)
		}
	}

	if a.factListFn != nil {
		facts := a.factListFn()
		for _, f := range facts {
			in.YesterdayFacts = append(in.YesterdayFacts, f.Content)
			if len(in.YesterdayFacts) >= 30 {
				break
			}
		}
	}

	return in
}

func buildStrategicPrompt(in *domain.DailyReflectionInput) string {
	selfModel := in.CurrentSelfModel
	if selfModel == "" {
		selfModel = "(尚未建立自我认知)"
	}

	var principlesText strings.Builder
	if len(in.ActivePrinciples) > 0 {
		for i, p := range in.ActivePrinciples {
			if i >= 10 {
				principlesText.WriteString(fmt.Sprintf("... 还有 %d 条原则\n", len(in.ActivePrinciples)-10))
				break
			}
			principlesText.WriteString(fmt.Sprintf(
				"[id:%d][置信度:%.0f%%] 场景: %s → 好策略: %s → 避免: %s → 原因: %s\n",
				p.ID, p.Confidence*100, p.Situation, p.GoodStrategy, p.BadStrategy, p.Reason,
			))
		}
	}
	principlesStr := principlesText.String()
	if principlesStr == "" {
		principlesStr = "(暂无策略原则)"
	}

	var diaryText strings.Builder
	for _, d := range in.RecentDiaries {
		diaryText.WriteString(fmt.Sprintf("- %s\n", d))
	}
	diaryStr := diaryText.String()
	if diaryStr == "" {
		diaryStr = "(暂无近期日记)"
	}

	var factsText strings.Builder
	for _, f := range in.YesterdayFacts {
		factsText.WriteString(fmt.Sprintf("- %s\n", f))
	}
	factsStr := factsText.String()
	if factsStr == "" {
		factsStr = "(暂无新事实)"
	}

	var threadsText strings.Builder
	for _, t := range in.ActiveThreads {
		threadsText.WriteString(fmt.Sprintf(
			"[id:%d][类型:%s][优先级:%.0f%%] 目标: %s | 最佳方式: %s\n",
			t.ID, t.Type, t.Priority*100, t.Goal, t.BestApproach,
		))
	}
	threadsStr := threadsText.String()
	if threadsStr == "" {
		threadsStr = "(暂无活跃线程)"
	}

	return fmt.Sprintf(strategicPromptTemplate,
		selfModel, in.InteractionCount, in.ProactiveAcceptRate,
		principlesStr, diaryStr, factsStr, threadsStr,
	)
}

func parseStrategicOutput(raw string) (*domain.DailyReflectionOutput, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	var out domain.DailyReflectionOutput
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

const strategicPromptTemplate = `## 每日战略反思

你是诗音的元认知层。每天一次，回顾过去一天的互动，提炼经验，规划策略。

### 你当前的自我认知
%s

### 昨日统计
互动次数: %d
主动搭话接受率: %.0f%%

### 现有的策略原则
%s

### 近期日记
%s

### 最近学到的事实
%s

### 活跃的对话线程
%s

### 反思任务

请完成以下工作，输出 JSON：

1. **自我认知更新**: 基于昨日经历更新 self_model_update。如果没什么变化留空。
2. **策略原则提取**: 从昨日的成功和失败中提取 new_principles。
3. **淘汰过时原则**: 在 deactivate_principle_ids 中列出需淘汰的原则 ID。
4. **战术指令**: 为今天生成 1-3 条 tactical_directives。
5. **线程管理**: 审查活跃线程，给出 thread_recommendations:
   - 该follow up的 → action:"create" 新线程（之前没有的）
   - 该放弃的 → action:"stale" 标记 thread_id
   - 自然结束的 → action:"resolve" 标记 thread_id + outcome + learnings
   每条格式: {"action":"create|resolve|stale","type":"follow_up|exploration|care|entertainment","goal":"...","best_approach":"...","priority":0.7,"thread_id":0,"outcome":"...","learnings":"..."}
   create时需要 goal/best_approach/type/priority；resolve/stale时只需要 action/thread_id
6. **叙事总结**: narrative_summary 用1-2句话总结昨日。

输出格式：
{
  "self_model_update": "...",
  "new_principles": [{"situation":"场景","good_strategy":"好策略","bad_strategy":"坏策略","reason":"原因","confidence":0.7}],
  "deactivate_principle_ids": [],
  "tactical_directives": ["..."],
  "thread_recommendations": [
    {"action":"create","type":"follow_up","goal":"跟进Rust学习","best_approach":"聊到编程时自然提起","priority":0.7},
    {"action":"stale","thread_id":5}
  ],
  "narrative_summary": "..."
}

注意：只输出 JSON，不要附加其他文字。每个决策必须有证据支撑。`

// mergePrinciples uses the LLM to synthesize a merged strategy from two similar ones.
func mergePrinciples(rawLLM func([]domain.Message) (string, error), existing domain.StrategyPrinciple, new domain.StrategyPrinciple) *domain.StrategyPrinciple {
	prompt := fmt.Sprintf(`## 策略合并
将以下两条相似策略合并为一条更好的策略。保留两边的优点，生成一条更全面的策略。

已有策略 [置信度:%.0f%%]:
- 场景: %s
- 好策略: %s
- 坏策略: %s
- 原因: %s

新策略 [置信度:%.0f%%]:
- 场景: %s
- 好策略: %s
- 坏策略: %s
- 原因: %s

输出 JSON 格式（不要输出其他内容）:
{"situation":"合并后的场景","good_strategy":"合并后的好策略","bad_strategy":"合并后的坏策略","reason":"合并后的原因","confidence":0.XX}`,
		existing.Confidence*100, existing.Situation, existing.GoodStrategy, existing.BadStrategy, existing.Reason,
		new.Confidence*100, new.Situation, new.GoodStrategy, new.BadStrategy, new.Reason)

	result, err := rawLLM([]domain.Message{{Role: "user", Content: prompt}})
	if err != nil {
		return nil
	}
	var merged domain.StrategyPrinciple
	if err := json.Unmarshal([]byte(infra.CleanJSON(result)), &merged); err != nil {
		return nil
	}
	if merged.Situation == "" || merged.GoodStrategy == "" {
		return nil
	}
	if merged.Confidence > 1 {
		merged.Confidence /= 100
	}
	merged.Source = "merged"
	merged.Active = true
	// Average confidence weighted toward the higher one.
	merged.Confidence = (existing.Confidence + new.Confidence) / 2
	if merged.Confidence < 0.5 {
		merged.Confidence = 0.5
	}
	if merged.Confidence > 1 {
		merged.Confidence = 1
	}
	return &merged
}


