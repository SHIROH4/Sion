package memory

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"desktop-pet/internal/app/chat"
	"desktop-pet/internal/app/plugin"
	"desktop-pet/internal/api"
	"desktop-pet/internal/domain"
)

// OnAfterChat delegates the full after-chat pipeline to the standalone PostProcessor.
func (p *MemoryPlugin) OnAfterChat(ctx *plugin.ChatContext) error {
	pp := &chat.PostProcessor{
		Store:  p.store,
		RawLLM: p.rawLLM,

		SessionBuf: p.sessionBuf,

		EmotionCurrent:  p.emotionModel.Current,
		EmotionEvaluate: func(text string) { p.emotionModel.Evaluate(text) },

		SelfCurrent: p.selfModel.Current,
		SelfSave:    func(s string) { _ = p.selfModel.Save(s) },

		CareUpdateStress:   p.careEngine.UpdateStress,
		CareUpdateState:    p.careEngine.UpdateState,
		CareActionLog:      p.careEngine.ActionLog,
		CareRecordResponse: p.careEngine.RecordResponse,

		SchedulerMarkReplied: p.scheduler.MarkReplied,

		ShouldCompress: func(msgs []domain.Message) bool { return p.compressor.ShouldCompress(msgs) },
		Compress:       func(msgs []domain.Message, level int) []domain.Message { return p.compressor.Compress(msgs, level) },

		IdentityAudit: func(dialogue string, llmFn func(string) (string, error)) ([]domain.IdentityNode, error) {
			return p.identityGraph.Audit(dialogue, llmFn)
		},
		IdentityDeactivate: func(id int64) { _ = p.identityGraph.Deactivate(id) },
		IdentityUpsert:     func(node *domain.IdentityNode) { _ = p.identityGraph.Upsert(node) },

		EpisodeFindOrCreate:     p.episodeStore.FindOrCreate,
		AttachFactToEpisode:     p.store.AttachFactToEpisode,
		SummarizeAndAssignTopic: p.summarizeAndAssignTopic,

		DiarySave:      func(entry *domain.DiaryEntry) { _ = p.diaryStore.Save(entry) },
		DiaryVectorize: p.diaryStore.Vectorize,
		MergerRun:      func() { _, _ = p.merger.Run() },

		DB: p.db,

		MemorizeFn: func(c string) { p.Memorize(c) },
		RecallFn:   p.Recall,

		ExtractAtomicFacts:      ExtractAtomicFacts,
		DeterministicImportance: DeterministicImportance,
		IsNoiseFact:             IsNoiseFact,
		LookupFactByContent:     LookupFactByContent,

		NewObservation:      NewObservation,
		BuildDiaryPrompt:    BuildDiaryPrompt,
		InferCareAcceptance: InferCareAcceptance,

		CleanJSON:      CleanJSON,
		StatusReport:   api.StatusBusInstance().Emit,
		MessagesToText: messagesToText,
		RecoverGuard:   recoverGuard,

		TurnCount:           &p.turnCount,
		LastChatTime:        &p.lastChatTime,
		LastDiaryAt:         &p.lastDiaryAt,
	DiaryCountToday:     &p.diaryCountToday,
		PendingProactiveID:  &p.pendingProactiveID,
		PendingProactiveSrc: &p.pendingProactiveSrc,
		PendingProactiveAt:  &p.pendingProactiveAt,

		BackgroundSetMsg: func(msg string) {
			if p.background != nil {
				p.background.SetUserMsg(msg)
			}
		},
		BackgroundNotify: func() {
			if p.background != nil {
				p.background.NotifyActivity()
			}
		},

		RecordProactiveOutcome: func(outcome int, delaySec int) {
			if p.outcomeRepo == nil {
				return
			}
			o := domain.ActionOutcome{
				ActionSource:  p.pendingOutcomeCtx.Source,
				ActionType:    p.pendingOutcomeCtx.Type,
				HourOfDay:     p.pendingOutcomeCtx.HourOfDay,
				DayOfWeek:     p.pendingOutcomeCtx.DayOfWeek,
				AppContext:    p.pendingOutcomeCtx.AppContext,
				EmotionBucket: p.pendingOutcomeCtx.EmotionBucket,
				EscalationLvl: p.pendingEscalationLvl,
				Outcome:       domain.OutcomeResult(outcome),
				ResponseDelay: delaySec,
			}
			if err := p.outcomeRepo.SaveOutcome(o); err != nil && p.pctx.Logger != nil {
				p.pctx.Logger.Warn("memory: failed to save outcome (reply)", "err", err)
			}
			// Feed outcome back to NeedModel with the real outcome.
			if p.needModel != nil {
		if outcome != 0 { p.consecutiveUnanswered = 0 }
				p.needModel.Satisfy(p.lastScoredAction, domain.OutcomeResult(outcome))
			}
			// Feed outcome back to offline RL (action weight learning).
			// Uses pendingDriveID for correct reward attribution — no race with concurrent decisions.
			if p.learner != nil {
				reward := 0.0
				if outcome == 1 { reward = 1.0 }   // OutcomeReplied
				if outcome == 2 { reward = 1.0 }   // OutcomeEngaged
				if outcome == -1 { reward = -1.0 } // OutcomeRejected
				p.learner.UpdateLastReward(p.pendingDriveID, reward)
			}
			// Session-level rejection tracking.
			if outcome == -1 || outcome == 0 {
				p.RejectAction(string(p.pendingOutcomeCtx.Source) + "_" + string(p.pendingOutcomeCtx.Type))
			}
			// Feed outcome back to System 2 Reflexion loop.
			if p.decisionEngine != nil {
				label := "ignored"
				if outcome == 1 { label = "replied" }
				if outcome == -1 { label = "rejected" }
				p.decisionEngine.RecordOutcome(
					string(p.pendingOutcomeCtx.Source),
					domain.DecisionOutput{Action: "speak", Source: string(p.pendingOutcomeCtx.Source)},
					label,
				)
			}
		},

		WG: &p.wg,
		Mu: &p.mu,

		Profile: p.profile,

		ErrorLog: func(msg string, args ...any) {
			if p.pctx.Logger != nil {
				p.pctx.Logger.Error(msg, args...)
			}
		},
	}

	pp.Process(ctx)

	// Conversation summarization every ~10 user turns.
	p.turnCount++
	if p.turnCount%10 == 0 {
		go p.SummarizeConversation()
	}

	return nil
}

// ---- LLM Reranker (used by chat_context.go and memory_tags.go) ----

func (p *MemoryPlugin) rerankWithLLM(candidates []UnifiedResult, query string, topK int) []UnifiedResult {
	if len(candidates) <= topK || p.rawLLM == nil {
		return candidates
	}
	var sb strings.Builder
	sb.WriteString("## 当前用户问题\n")
	sb.WriteString(query)
	sb.WriteString("\n\n## 候选记忆列表\n")
	for i, c := range candidates {
		sourceTag := map[string]string{"fact": "[事实]", "diary": "[日记]"}[c.Source]
		sb.WriteString(fmt.Sprintf("[%d] %s %s\n", i+1, sourceTag, c.Content))
	}
	sb.WriteString(fmt.Sprintf("\n请选出和当前问题最相关的 %d 条记忆，只输出编号列表，如: 1,3,5\n", topK))
	response, err := p.rawLLM([]plugin.Message{
		{Role: "system", Content: "你是一个记忆排序助手。从候选记忆中选出和用户问题最相关的条目。只输出编号，格式如: 1,3,5"},
		{Role: "user", Content: sb.String()},
	})
	if err != nil {
		return candidates[:topK]
	}
	indices := parseRerankResponse(response, len(candidates))
	if len(indices) == 0 {
		return candidates[:topK]
	}
	result := make([]UnifiedResult, 0, len(indices))
	for _, idx := range indices {
		result = append(result, candidates[idx])
	}
	return result
}

// buildCompressionPrompt delegates to the standalone implementation in app/chat.
func buildCompressionPrompt() string {
	return chat.BuildCompressionPrompt()
}

func parseRerankResponse(response string, maxIdx int) []int {
	var indices []int
	seen := map[int]bool{}
	re := regexp.MustCompile(`\d+`)
	for _, m := range re.FindAllString(response, -1) {
		n, _ := strconv.Atoi(m)
		n--
		if n >= 0 && n < maxIdx && !seen[n] {
			seen[n] = true
			indices = append(indices, n)
		}
	}
	return indices
}
