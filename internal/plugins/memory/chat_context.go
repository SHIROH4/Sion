package memory

import (
	"desktop-pet/internal/app/chat"
	"desktop-pet/internal/app/plugin"
	"desktop-pet/internal/domain"
)

// FilterMessage prepends a timestamp to the user message.
func (p *MemoryPlugin) FilterMessage(msg *plugin.Message) error {
	return chat.FilterMessage(msg)
}

// OnBeforeChat injects compact context by delegating to the standalone processor.
func (p *MemoryPlugin) OnBeforeChat(ctx *plugin.ChatContext) error {
	proc := &chat.Processor{
		Store:     p.store,
		EmbSvc:    p.embSvc,
		SelfModel: func() string { return p.selfModel.Current() },
		EmotionCurrent: func() (domain.EmotionState, domain.EmotionVector) {
			return p.emotionModel.Current(), p.emotionModel.CurrentVector()
		},
		Profile:       p.profile,
		IdentityGraph: p.identityGraph,
		CareSnapshot:  func() domain.UserCareState { return p.careEngine.State().Snapshot() },
		SessionBuf:    p.sessionBuf,
		RerankFn:      p.rerankWithLLM,
		TimeTag:       relativeTimeTag,
		TurnCount:     &p.turnCount,
		WG:            &p.wg,
		ScreenSummary: p.GetScreenSummary,
	}
	return proc.OnBeforeChat(ctx)
}
