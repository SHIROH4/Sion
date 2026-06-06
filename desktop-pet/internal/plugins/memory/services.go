package memory

import (
	"database/sql"
	"encoding/json"

	"desktop-pet/internal/app/plugin"
	"desktop-pet/internal/domain"
)

// Services bundles all pre-built service objects that MemoryPlugin needs.
// Constructed in app.go and injected via SetServices before Start.
type Services struct {
	DB        *sql.DB
	Store     *Store
	Profile   *UserProfile
	SelfModel *SelfModel

	SessionBuf *SessionBuffer
	Emotion    *EmotionModel
	Care       *CareEngine
	EmbSvc     Vectorizer
	MemLayers  *MemoryLayer
	Compressor *Compressor

	DiaryStore   *DiaryStore
	Merger       *DiaryMerger
	EpisodeStore *EpisodeStore
	TopicStore   *TopicStore
	Identity     *IdentityGraph
	LLMSync      func([]plugin.Message) (string, error)
	VisionLLM    func([]plugin.Message) (string, error)
	Scheduler    Scheduler
}

// SetServices injects pre-built service objects into MemoryPlugin.
// Must be called after Awake and before Start.
func (p *MemoryPlugin) SetServices(svc *Services) {
	p.db = svc.DB
	p.store = svc.Store
	p.profile = svc.Profile
	p.selfModel = svc.SelfModel
	p.sessionBuf = svc.SessionBuf
	p.emotionModel = svc.Emotion
	p.careEngine = svc.Care
	p.diaryStore = svc.DiaryStore
	p.merger = svc.Merger
	p.memLayers = svc.MemLayers
	p.compressor = svc.Compressor
	p.episodeStore = svc.EpisodeStore
	p.topicStoreInst = svc.TopicStore
	p.identityGraph = svc.Identity
	p.rawLLM = svc.LLMSync
	p.scheduler = svc.Scheduler
	p.embSvc = svc.EmbSvc

	// LLM wiring (previously in SetLLMSync, now part of DI).
	if p.emotionModel != nil && p.rawLLM != nil {
		p.emotionModel.SetLLMEval(p.rawLLMWrapper())
	}
	if p.merger != nil && p.rawLLM != nil {
		p.merger.SetLLM(p.rawLLMWrapper())
	}
	if p.compressor != nil && p.rawLLM != nil {
		rawLLM := p.rawLLM
		p.compressor.SetLLMSync(func(msgs []plugin.Message) (string, error) {
			sysMsg := plugin.Message{Role: "system", Content: buildCompressionPrompt()}
			return rawLLM(append([]plugin.Message{sysMsg}, msgs...))
		})
		p.compressor.OnArchive = func(name string, level int, original []plugin.Message, summary string) {
			if p.store == nil {
				return
			}
			raw, err := json.Marshal(original)
			if err != nil {
				return
			}
			if err := p.store.SaveArchive(name, level, string(raw), summary); err != nil {
				if p.pctx.Logger != nil {
					p.pctx.Logger.Error("memory: save archive failed", "name", name, "err", err)
				}
			}
		}
	}
	if svc.VisionLLM != nil {
		p.screenAnalyzer = NewScreenAnalyzer(svc.VisionLLM)
		p.visLLM = svc.VisionLLM
	}
	// Inject vision LLM into curiosity engine for screenshot-driven gap detection.
	if p.curiosityEngine != nil && p.visLLM != nil {
		p.curiosityEngine.SetVisionLLM(p.visLLM)
	}
}

// CareProvider returns the care engine as a domain.CareProvider.
func (p *MemoryPlugin) CareProvider() domain.CareProvider { return p.careEngine }

// EmotionEvaluator returns the emotion model as a domain.EmotionEvaluator.
func (p *MemoryPlugin) EmotionEvaluator() domain.EmotionEvaluator { return p.emotionModel }

// SetStore injects a pre-built store.
func (p *MemoryPlugin) SetStore(s *Store) { p.store = s }
