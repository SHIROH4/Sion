package memory

import (
	"time"

	"desktop-pet/internal/app/chat"
	"desktop-pet/internal/domain"
	"desktop-pet/internal/infra/native"
)

// proactiveAction delegates to the standalone proactive generator.
func (p *MemoryPlugin) proactiveAction(result SchedulerResult) {
	gen := p.generatorConfig()
	var obs domain.ScreenObservation
	if p.background != nil {
		obs = ConvertScreenObs(p.background.LastScreenObs())
	}
	// Store pending context before generating, so outcome recording has proper data.
	now := time.Now()
	actionType := domain.TriggerSocial
	if result.Source == domain.SourceCare {
		actionType = domain.TriggerEncourage
	}
	p.pendingOutcomeCtx = domain.ActionContext{
		Source:    result.Source,
		Type:      actionType,
		HourOfDay: now.Hour(),
		DayOfWeek: int(now.Weekday()),
	}
	if obs.AppName != "" {
		p.pendingOutcomeCtx.AppContext = obs.AppName
	}
	if p.emotionModel != nil {
		vec := p.emotionModel.CurrentVector()
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
		p.pendingOutcomeCtx.EmotionBucket = b
	}
	p.pendingEscalationLvl = result.Escalation
	p.pendingOutcomeAt = now
	p.pendingProactiveID = now.UnixNano() // non-zero ID for timeout detection
	gen.GenerateProactive(result, obs)
}

// buildCareMessage delegates to the standalone care message builder.
func (p *MemoryPlugin) buildCareMessage(
	careType domain.CareTriggerType,
	state *domain.UserCareState,
	emotion *domain.EmotionState,
	emotionVec *domain.EmotionVector,
	customContext string,
) string {
	return p.generatorConfig().BuildCareMessage(careType, state, emotion, emotionVec, customContext)
}

func (p *MemoryPlugin) generatorConfig() *chat.Generator {
	return &chat.Generator{
		RawLLM: func(msgs []domain.Message) (string, error) {
			return p.rawLLM(msgs)
		},
		Store:          p.store,
		SelfModel:      func() string { return p.selfModel.Current() },
		EmotionFunc:    func() domain.EmotionState { return p.emotionModel.Current() },
		ScreenAnalyzer: p.screenAnalyzer,
		CaptureScreen:  native.CaptureScreenToBase64,
		ActiveWindow:   native.GetActiveWindowDetail,
		SessionRecent:  func(n int) []domain.Message { return p.sessionBuf.Recent(n) },
		Emit:           func(event string, payload any) { p.pctx.EventBus.Emit(event, payload) },
		InfoLog:        func(msg string, args ...any) { p.pctx.Logger.Info(msg, args...) },
		WarnLog:        func(msg string, args ...any) { p.pctx.Logger.Warn(msg, args...) },
		MetricsLog:     func(msg string, args ...any) { p.pctx.Logger.Info(msg, args...) },
		PendingID:      &p.pendingProactiveID,
		PendingSource:  &p.pendingProactiveSrc,
		PendingAt:      &p.pendingProactiveAt,
		OutcomeRepo:    p.outcomeRepo,
		EmotionVector:  func() domain.EmotionVector { return p.emotionModel.CurrentVector() },
		PrincipleRepo:  p.principleRepo,
		PickInquiry: func(source domain.ProactiveSource, hour int) *domain.CuriosityItem {
			if p.curiosityEngine == nil {
				return nil
			}
			return p.curiosityEngine.PickBestInquiry(source, hour)
		},
		PatternTriggers: func(now time.Time) []domain.PatternTrigger {
			if p.patternAnalyzer == nil {
				return nil
			}
			return p.patternAnalyzer.GetPatternTriggers(now)
		},
		ActiveThreads: func() []domain.ConversationThread {
			if p.threadRepo == nil {
				return nil
			}
			threads, _ := p.threadRepo.ListActive()
			return threads
		},
		// RAG: search long-term memory for grounding facts.
		FactSearch: func(query string) string {
			if p.embSvc == nil || p.store == nil {
				return ""
			}
			vec, err := p.embSvc.Vectorize(query)
			if err != nil {
				return ""
			}
			results, _ := p.store.UnifiedSearch(vec, query, 1)
			if len(results) == 0 {
				return ""
			}
			return results[0].Content
		},
		ConversationSummary: p.conversationSummary,
		// Reuse the same persona context as normal chat.
		PersonaSummary: p.buildPersonaSummary(),
	}
}

// ---- Shared constant (used by background_tasks.go) ----
