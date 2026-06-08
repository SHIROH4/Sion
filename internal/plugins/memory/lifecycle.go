package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"

	"desktop-pet/internal/app/plugin"
	"desktop-pet/internal/domain"
	"desktop-pet/internal/infra/native"
	infrastorage "desktop-pet/internal/infra/storage"
	care "desktop-pet/internal/service/care"
	"desktop-pet/internal/service/cognition"
	"desktop-pet/internal/service/diary"
	"desktop-pet/internal/api"
	"desktop-pet/internal/service/tools"
	emotion "desktop-pet/internal/service/emotion"
	"desktop-pet/internal/service/identity"
	svcmemory "desktop-pet/internal/service/memory"
	"desktop-pet/internal/service/scheduler"
)

// Local type aliases (bridge.go removed).
type (
	Store            = infrastorage.Store
	UserProfile      = domain.UserProfile
	Compressor       = svcmemory.Compressor
	CompressConfig   = svcmemory.CompressConfig
	SelfModel        = identity.SelfModel
	SessionBuffer    = svcmemory.SessionBuffer
	DiaryStore       = diary.DiaryStore
	EmotionModel     = emotion.EmotionModel
	DiaryMerger      = diary.DiaryMerger
	MemoryLayer      = svcmemory.MemoryLayer
	ProactiveLearner = svcmemory.ProactiveLearner
	CareEngine       = care.CareEngine
	EpisodeStore     = svcmemory.EpisodeStore
	TopicStore       = svcmemory.TopicService
	IdentityGraph    = identity.IdentityGraph
	Scheduler        = domain.Scheduler
	Vectorizer       = domain.Vectorizer
	EmbeddingService = svcmemory.EmbeddingService
	ScreenAnalyzer   = scheduler.ScreenAnalyzer
	Observation      = domain.Observation
	MemoryStore      = domain.MemoryStore
	EmotionVector    = domain.EmotionVector
	UnifiedResult    = domain.UnifiedResult
	SchedulerResult  = domain.SchedulerResult
	EpisodeEntry     = domain.EpisodeEntry
	FactEntry        = domain.FactEntry
	AtomicFactInput  = domain.AtomicFactInput
	MemCellType      = domain.MemCellType
	DiaryEntry       = domain.DiaryEntry
	CareAction       = domain.CareAction
	Message          = domain.Message
)

var (
	NewObservation          = care.NewObservation
	NewCareEngine           = care.NewCareEngine
	NewEmotionModel         = emotion.NewEmotionModel
	NewSelfModel            = identity.NewSelfModel
	NewDiaryStore           = diary.NewDiaryStore
	NewDiaryMerger          = diary.NewDiaryMerger
	NewMemoryLayer          = svcmemory.NewMemoryLayer
	NewProactiveLearner     = svcmemory.NewProactiveLearner
	NewCompressor           = svcmemory.NewCompressor
	NewScheduler            = scheduler.NewScheduler
	NewScreenAnalyzer       = scheduler.NewScreenAnalyzer
	NewFactConsolidator     = svcmemory.NewFactConsolidator
	NewEpisodeStore         = svcmemory.NewEpisodeStore
	NewIdentityGraph        = identity.NewIdentityGraph
	NewTopicService         = svcmemory.NewTopicService
	NewSessionBuffer        = svcmemory.NewSessionBuffer
	NewEmbeddingService     = svcmemory.NewEmbeddingService
	RelativeTime            = svcmemory.RelativeTime
	BuildSelfUpdatePrompt   = identity.BuildSelfUpdatePrompt
	ExtractAtomicFacts      = svcmemory.ExtractAtomicFacts
	DeterministicImportance = svcmemory.DeterministicImportance
	IsNoiseFact             = svcmemory.IsNoiseFact
	EmotionTag              = svcmemory.EmotionTag
	MemCellTypeTag          = svcmemory.MemCellTypeTag
	LookupFactByContent     = infrastorage.LookupFactByContent
	ScanFactRows            = infrastorage.ScanFactRows
	BuildDiaryPrompt        = diary.BuildDiaryPrompt
	InferCareAcceptance     = svcmemory.InferCareAcceptance
)

const (
	ActiveThreshold    = svcmemory.ActiveThreshold
	ObsChat            = domain.ObsChat
	ObsQQ              = domain.ObsQQ
	TriggerSocial      = domain.TriggerSocial
	SourceKnowledgeGap = domain.SourceKnowledgeGap
)

const ObsScreen = domain.ObsScreen

// MemoryPlugin implements the Spark+Ember memory system: self-model, session
// buffer, forgetting decay, demoted compression, diary, and emotion.
type MemoryPlugin struct {
	pctx       plugin.PluginContext
	running    bool
	db         *sql.DB
	store      *Store
	profile    *UserProfile
	compressor *Compressor
	mu         sync.Mutex
	wg         sync.WaitGroup
	rawLLM     func([]plugin.Message) (string, error)
	toolLLM    func(messages []domain.Message, tools []cognition.DecisionToolSpec) (string, string, error)

	selfModel  *SelfModel
	sessionBuf *SessionBuffer

	diaryStore   *DiaryStore
	emotionModel *EmotionModel
	merger          *DiaryMerger
	lastDiaryAt         time.Time
	diaryCountToday     int
	lastSystem2Tick     time.Time
	decisionWarmupDone  bool   // first tick computes features but skips action
	bochaAPIKey         string // Bocha Web Search API key (optional, preferred)
	bingSearchAPIKey    string // Bing Web Search API key (optional)
	conversationSummary string              // compressed summary of current conversation
	rejectedActions     map[string]time.Time // action→when rejected, cleared on idle

	memLayers  *MemoryLayer
	background *BackgroundCognition
	proactive  *ProactiveLearner
	turnCount  int

	careEngine *CareEngine

	embSvc Vectorizer

	episodeStore            *EpisodeStore
	topicStoreInst          *TopicStore

	lastReflectAt time.Time

	identityGraph *IdentityGraph

	scheduler           Scheduler
	lastChatTime        time.Time
	pendingProactiveID     int64
	pendingProactiveSrc    domain.ProactiveSource
	pendingProactiveAt     time.Time
	pendingDriveID         int // learner drive record index for reward attribution
	consecutiveUnanswered  int // consecutive proactive messages with no user reply

	screenAnalyzer    *ScreenAnalyzer
	lastScreenSummary string
	lastScreenMu      sync.RWMutex // protects lastScreenSummary

	outcomeRepo     domain.ActionOutcomeRepository
	strategyAgent   *cognition.StrategicAgent
	principleRepo   domain.StrategyPrincipleRepository
	curiosityEngine *cognition.Engine
	cogRepo         domain.CuriosityRepository
	patternAnalyzer *cognition.Analyzer
	threadRepo      domain.ThreadRepository
	visLLM         func([]plugin.Message) (string, error)
	screenCapture  func() (string, error) // base64 PNG screenshot
	decisionEngine    *cognition.DecisionEngine
	motivator         *cognition.Motivator
	learner           *cognition.Learner
	toolRegistry      *tools.Registry
	lastScoredAction  string
	consecutiveAction int
	featureComputer   *cognition.FeatureComputer // 量化因子计算器
	needModel         *cognition.NeedModel       // 内源需求模型
	lastFeatures      *domain.QuantifiedFeatures // 最新量化因子快照 (API暴露)
	lastNeedsSnapshot *domain.IntrinsicNeeds     // 最新需求快照 (API暴露)

	// v0.4 decision pipeline (shadow mode — runs alongside existing, logs comparisons)
	ruleEngine        *cognition.S1RuleEngine
	metaReasoner      *cognition.MetaReasoner
	feedbackProcessor *cognition.UnifiedFeedbackProcessor

	pendingOutcomeCtx    domain.ActionContext
	pendingEscalationLvl int
	pendingOutcomeAt     time.Time
}

// NewPlugin returns the memory plugin as a plugin.Plugin interface.
func NewPlugin() plugin.Plugin {
	return &MemoryPlugin{}
}

// Info returns plugin metadata.
func (p *MemoryPlugin) Info() plugin.PluginInfo {
	return plugin.PluginInfo{
		Name:        "memory",
		Version:     "0.4.0",
		Description: "Ember级记忆系统：自我画像+遗忘衰减+精简上下文+情感日记+向量检索",
		Priority:    20,
		Requires:    []string{"chat"},
	}
}

// Awake stores the plugin context. All dependencies must already be injected
// via SetServices; no fallback construction happens here.
func (p *MemoryPlugin) Awake(pctx plugin.PluginContext) error {
	p.pctx = pctx
	return nil
}

// Start wires callbacks and starts background tasks. All service objects
// must be pre-injected via SetServices — no fallback construction happens here.
func (p *MemoryPlugin) Start() error {
	p.mu.Lock()
	p.running = true
	p.mu.Unlock()

	// Background cognition loop.
	p.proactive = NewProactiveLearner()
	p.background = NewBackgroundCognition(p.memLayers,
		func(msgs []plugin.Message) (string, error) {
			if p.rawLLM == nil {
				return "", nil
			}
			return p.rawLLM(msgs)
		},
		p.emotionModel,
	)
	p.background.SetProactiveLearner(p.proactive)

	// Dynamic interval: compute optimal decision tick from current state.
	p.background.SetIntervalFunc(func() time.Duration {
		if p.emotionModel == nil {
			return 5 * time.Minute // fallback to baseline
		}
		now := time.Now()
		timeSinceChatMin := time.Since(p.lastChatTime).Minutes()
		isNight := now.Hour() >= 22 || now.Hour() < 8

		isWorking := false
		continuousWorkMin := 0.0
		if p.background != nil {
			obs := p.background.LastScreenObs()
			isWorking = obs.IsWorking
			continuousWorkMin = float64(p.careEngine.State().ContinuousWork)
		}

		// Get rejection severity and daily quota from lastFeatures if available.
		rejectionSev := 0.0
		quotaRemaining := 20.0
		if p.lastFeatures != nil {
			rejectionSev = p.lastFeatures.R4_RejectionSeverity
			quotaRemaining = p.lastFeatures.E4_QuotaRemaining
		}

		// Compute drives from current emotion (lightweight, no feature computer needed).
		emoVec := p.emotionModel.CurrentVector()
		social := emoVec.Loneliness*0.3 + emoVec.Playfulness*0.2 + (1.0-emoVec.Annoyance)*0.1
		care := emoVec.Worry*0.35 + emoVec.Affection*0.2
		curious := emoVec.Curiosity * 0.3

		interval := cognition.DynamicInterval(
			timeSinceChatMin,
			isWorking,
			continuousWorkMin,
			isNight,
			rejectionSev,
			quotaRemaining,
			social, care, curious,
		)
		return interval
	})
	go p.background.Start()
	if p.pctx.Logger != nil {
		p.pctx.Logger.Info("memory: background loop started (dynamic interval)")
	}

	// Care engine callbacks.
	p.careEngine.SetGenerateMessage(p.buildCareMessage)
	p.careEngine.SetEmotionVector(func() EmotionVector {
		return p.emotionModel.CurrentVector()
	})

	p.background.SetCare(p.careEngine)
	p.background.SetTopicStore(p.topicStoreInst)
	p.background.SetOnReflect(p.ReflectAndForget)
	p.background.SetOnDetectGaps(func() {
		if p.curiosityEngine == nil {
			return
		}
		// Gap scanning (LLM-driven, runs every ~50 min with 2h cooldown).
		if p.curiosityEngine.ShouldScanGaps() {
			facts := p.store.ListActiveFacts(0)
			var factStrs []string
			for _, f := range facts {
				factStrs = append(factStrs, f.Content)
			}
			profile := p.store.LoadProfile()
			api.StatusBusInstance().EmitStart("curiosity", "知识缺口扫描...")
			n := p.curiosityEngine.ScanGaps(factStrs, profile.Name, profile.TechStack)
			if n > 0 && p.pctx.Logger != nil {
				api.StatusBusInstance().EmitOK("curiosity", fmt.Sprintf("发现 %d 个知识缺口", n))
				p.pctx.Logger.Info("memory: curiosity gap scan", "new_gaps", n)
			}
		}
		// Generate inquiries from pending gaps (runs independently each cycle).
		iqs := p.curiosityEngine.GenerateInquiries(3)
		if len(iqs) > 0 && p.pctx.Logger != nil {
			p.pctx.Logger.Info("memory: curiosity inquiries generated", "count", len(iqs))
		}
	})
	p.background.SetOnSystem2Decision(func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("memory: OnSystem2Decision panicked", "panic", r)
			}
		}()
		// Hard cooldown: minimum 2 minutes between decisions regardless of ShouldRun.
		if time.Since(p.lastSystem2Tick) < 2*time.Minute {
			return
		}
		if p.decisionEngine == nil || !p.decisionEngine.ShouldRun() {
			return
		}
		p.lastSystem2Tick = time.Now()
		api.StatusBusInstance().EmitInfo("system", "System 2 决策 tick")
		go func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("memory: runSystem2Decision goroutine panicked", "panic", r)
				}
				p.decisionEngine.FinishDecision()
			}()
			p.runSystem2Decision()
		}()
	})
	p.background.SetOnCleanup(func() {
		n := p.store.CleanArchivedFacts(7)
		if n > 0 && p.pctx.Logger != nil {
			p.pctx.Logger.Info("memory: cleaned archived facts", "count", n)
		}
	})


	// Screen observation.
	p.background.SetOnScreenObserve(func(obs native.ScreenObservation) {
		if native.IsSelfApp(obs.AppName) {
			return
		}
		app := native.FriendlyAppName(obs.AppName)
		summary := fmt.Sprintf("软件: %s", app)
		if obs.WindowTitle != "" {
			summary += fmt.Sprintf(" | 窗口: %s", obs.WindowTitle)
		}
		if obs.IsWorking {
			summary += " | 正在工作"
		} else {
			summary += " | 休闲中"
		}
		p.lastScreenMu.Lock()
		p.lastScreenSummary = summary
		p.lastScreenMu.Unlock()

		if obs.IsWorking {
			p.careEngine.IncrementWork(60)
		} else {
			p.careEngine.ResetWork()
		}
		content := fmt.Sprintf("%s %s %s", obs.AppName, obs.WindowTitle, obs.OCRText)
		observation := NewObservation(ObsScreen, content)
		p.careEngine.UpdateState(observation)
		if p.proactive != nil {
			p.proactive.Ingest(observation)
		}
		// Record activity event for pattern mining.
		if p.patternAnalyzer != nil {
			p.patternAnalyzer.RecordActivity(obs.AppName, obs.WindowTitle, obs.IsWorking)
		}
	})

	// Eager initial observation.
	go p.eagerScreenObserve()

	// Post-injection wiring.
	p.store.SetEmbedSvc(p.embSvc)
	p.diaryStore.SetVectorize(p.embSvc.Vectorize)
	p.episodeStore.SetVectorize(p.embSvc.Vectorize)
	p.topicStoreInst.SetVectorize(p.embSvc.Vectorize)

	// Adaptive outcome repo.
	p.outcomeRepo = infrastorage.NewActionOutcomeRepo(p.db)
	if sched, ok := p.scheduler.(*scheduler.Scheduler); ok {
		sched.SetOutcomeRepo(p.outcomeRepo)
		sched.SetCuriosityRepo(p.cogRepo)
	}
	p.emotionModel.SetOutcomeRepo(p.outcomeRepo)

	// Strategic agent (System 3 — daily reflection).
	p.principleRepo = infrastorage.NewStrategyPrincipleRepo(p.db)
	p.threadRepo = infrastorage.NewThreadRepo(p.db)
	p.strategyAgent = cognition.NewStrategicAgent(
		p.rawLLM,
		p.principleRepo,
		p.outcomeRepo,
		p.threadRepo,
		p.embSvc.Vectorize,
	)
	p.strategyAgent.SetSelfModel(
		func() string { return p.selfModel.Current() },
		func(s string) error { return p.selfModel.Save(s) },
	)
	p.strategyAgent.SetDiaryList(func(n int) []domain.DiaryEntry {
		return p.diaryStore.ListRecent(n)
	})
	p.strategyAgent.SetFactList(func() []domain.FactEntry {
		return p.store.ListActiveFacts(0)
	})
	p.background.SetStrategyAgent(func() {
		if !p.strategyAgent.ShouldRun() {
			return
		}
		api.StatusBusInstance().EmitStart("strategy", "战略反思...")
		output, err := p.strategyAgent.Run()
		if err == nil && p.featureComputer != nil {
			p.featureComputer.NoteReflection(time.Now())
		}
		if err == nil {
			api.StatusBusInstance().EmitOK("strategy", fmt.Sprintf("反思完成: %d 新策略, %d 指令", len(output.NewPrinciples), len(output.TacticalDirectives)))
			p.emotionModel.LearnPersonality()
			if sched, ok := p.scheduler.(*scheduler.Scheduler); ok {
				sched.LearnParams()
			}
		}
		if err != nil {
			if p.pctx.Logger != nil {
				p.pctx.Logger.Warn("memory: strategic agent failed", "err", err)
			}
			return
		}
		p.background.loop.TacticalDirectives = output.TacticalDirectives
		if p.pctx.Logger != nil {
			accepts, total := p.outcomeRepo.SuccessRate(domain.ActionContext{}, 1)
			rate := 0.0
			if total > 0 {
				rate = float64(accepts) / float64(total) * 100
			}
			p.pctx.Logger.Info("memory: daily metrics",
				"accept_rate_pct", fmt.Sprintf("%.0f", rate),
				"actions_today", total,
				"new_principles", len(output.NewPrinciples),
				"directives", len(output.TacticalDirectives),
			)
		}
	})

	// Load state from DB to survive restarts correctly.
	p.lastChatTime = time.Now()
	if p.store != nil {
		// lastChatTime: use the actual last user message timestamp.
		var lastUserMsg int64
		if err := p.store.DB().QueryRow(
			`SELECT COALESCE(MAX(created_at), 0) FROM chat_history WHERE role = 'user'`,
		).Scan(&lastUserMsg); err == nil && lastUserMsg > 0 {
			p.lastChatTime = time.Unix(lastUserMsg, 0)
		}
		// turnCount: estimate from today's chat messages.
		todayStart := time.Now().Truncate(24 * time.Hour).Unix()
		var msgCount int
		if err := p.store.DB().QueryRow(
			`SELECT COUNT(*) FROM chat_history WHERE created_at > ?`, todayStart,
		).Scan(&msgCount); err == nil {
			p.turnCount = msgCount / 2 // rough: each turn = user+assistant pair
		}
	}
	// diaryCountToday: count actual diaries from today in DB.
	p.diaryCountToday = 0
	if p.diaryStore != nil {
		recent := p.diaryStore.ListRecent(1)
		if len(recent) > 0 {
			p.lastDiaryAt = time.Unix(recent[0].CreatedAt, 0)
		}
		todayStart := time.Now().Truncate(24 * time.Hour).Unix()
		for _, d := range p.diaryStore.ListRecent(10) {
			if d.CreatedAt > todayStart {
				p.diaryCountToday++
			}
		}
	} else {
		p.lastDiaryAt = time.Now()
	}

	// Curiosity engine.
	p.cogRepo = infrastorage.NewCuriosityRepo(p.db)
	p.curiosityEngine = cognition.NewEngine(p.rawLLM, p.cogRepo, p.embSvc.Vectorize)
	p.screenCapture = native.CaptureScreenToBase64

	// System 2 decision engine + tool registry.
	p.decisionEngine = cognition.NewDecisionEngine(p.rawLLM)
	p.decisionEngine.SetStoragePath(os.Getenv("HOME") + "/.desktop-pet/reflexion.json")
	if p.toolLLM != nil {
		p.decisionEngine.SetToolLLM(p.toolLLM)
	}
	p.motivator = cognition.NewMotivator()
	p.motivator.SetStoragePath(os.Getenv("HOME") + "/.desktop-pet/motivator_weights.json")
	p.featureComputer = cognition.NewFeatureComputer(p.db, p.outcomeRepo)
	p.featureComputer.SetLLMAvailable(p.rawLLM != nil)
	p.featureComputer.SetVisionAvailable(p.visLLM != nil)
	// Wire LLM for U1/U2 classification of unknown apps/window titles.
	if p.rawLLM != nil {
		p.featureComputer.SetLLM(func(prompt string) (string, error) {
			return p.rawLLM([]plugin.Message{{Role: "user", Content: prompt}})
		})
	}
	// Wire vectorizer for U15 embedding-based fatigue search.
	if p.embSvc != nil {
		p.featureComputer.SetVectorizer(p.embSvc)
	}
	// Persist emotion history ring buffer for A4 trend continuity across restarts.
	p.featureComputer.SetEmotionHistoryPath(os.Getenv("HOME") + "/.desktop-pet/emotion_history.json")

	// Intrinsic need model.
	p.needModel = cognition.NewNeedModel()
	p.needModel.LoadFrom(p.store.LoadProfileValue)

	// ---- v0.4 decision pipeline (shadow mode — logs comparisons, no behavior change) ----
	p.ruleEngine = cognition.NewS1RuleEngine()
	p.metaReasoner = cognition.DefaultMetaReasoner()
	p.feedbackProcessor = cognition.NewUnifiedFeedbackProcessor(p.ruleEngine, 200)
	p.feedbackProcessor.OnStrategicDistill = func(exps []cognition.ExperienceRecord, rules []cognition.StrategyRule) error {
		if p.strategyAgent != nil && p.strategyAgent.ShouldRun() {
			_, err := p.strategyAgent.Run()
			return err
		}
		return nil
	}
	p.learner = cognition.NewLearner(p.motivator)
	p.learner.SetOutcomeRepo(p.outcomeRepo)
	p.toolRegistry = tools.NewRegistry()
	p.toolRegistry.Register(&tools.SpeakTool{
		SpeakFunc: func(source domain.ProactiveSource, mood, reason string) error {
			result := domain.SchedulerResult{
				ShouldAct:      true,
				Source:         source,
				Reason:         reason,
				EmotionContext: mood,
			}
			p.proactiveAction(SchedulerResult(result))
			return nil
		},
	})
	p.toolRegistry.Register(&tools.ObserveTool{
		ObserveFunc: func() (int, string, error) {
			if p.curiosityEngine == nil || p.screenCapture == nil {
				return 0, "", fmt.Errorf("curiosity engine not ready")
			}
			b64, err := p.screenCapture()
			if err != nil {
				return 0, "", err
			}
			appName, windowTitle := native.GetActiveWindowDetail()
			facts := p.store.ListActiveFacts(0)
			var factStrs []string
			for _, f := range facts {
				factStrs = append(factStrs, f.Content)
			}
			profile := p.store.LoadProfile()
			n := p.curiosityEngine.AnalyzeScreenshot(b64, appName, windowTitle, factStrs, profile.Name, profile.TechStack)
			return n, appName, nil
		},
	})
	p.toolRegistry.Register(&tools.ReflectTool{
		ReflectFunc: func() (*domain.DailyReflectionOutput, error) {
			if p.strategyAgent == nil {
				return nil, fmt.Errorf("strategy agent not ready")
			}
			return p.strategyAgent.Run()
		},
	})
	p.toolRegistry.Register(&tools.BrowseTool{
		OnExtract: func(url, rawText string) (string, []string) {
			return p.extractFactsFromBrowse(url, rawText)
		},
	})
	p.toolRegistry.Register(&tools.SearchTool{
		BingAPIKey: p.bingSearchAPIKey,
			BochaAPIKey: p.bochaAPIKey,
		OnResults: func(query string, results []tools.SearchResult) string {
			return p.extractFactsFromSearch(query, results)
		},
	})
	// Seed initial curiosity topics synchronously so they're ready for the first tick.
	if p.curiosityEngine != nil {
		profile := p.store.LoadProfile()
		p.curiosityEngine.SeedCuriosityTopics(profile.Name, profile.TechStack)
		p.curiosityEngine.ForceGapScan()
	}

	if p.visLLM != nil {
		p.curiosityEngine.SetVisionLLM(p.visLLM)
	}
	// Pattern analyzer.
	p.patternAnalyzer = cognition.NewAnalyzer(
		p.rawLLM,
		infrastorage.NewActivityEventRepo(p.db),
		infrastorage.NewPatternRepo(p.db),
	)

	p.background.SetScheduler(p.scheduler)
	p.background.SetFactConsolidator(NewFactConsolidator(p.store, p.rawLLM, p.embSvc.Vectorize))

	if p.screenAnalyzer == nil {
		p.screenAnalyzer = NewScreenAnalyzer(p.rawLLM)
	}

	// Proactive outcome timeout detection.
	go p.proactiveTimeoutLoop()

	// Health checks (best-effort, non-blocking).
	go p.checkEmbeddingHealth()
	go p.setupLocalEmotion()
	go p.backfillDiaryVectors()

	// Deferred backfill and state restoration.
	go p.migrateOldFacts()

	if data := p.store.LoadProfileValue("care_state"); data != "" {
		p.careEngine.LoadState([]byte(data))
	}

	return nil
}


// backfillDiaryVectors fills in missing vectors for existing diary entries.
func (p *MemoryPlugin) backfillDiaryVectors() {
	if p.diaryStore == nil || p.embSvc == nil {
		return
	}
	count := 0
	diaries := p.diaryStore.ListRecent(200)
	for _, d := range diaries {
		if len(d.Vector) > 0 {
			continue
		}
		vec, err := p.embSvc.Vectorize(d.Title + " " + d.Summary)
		if err != nil {
			if p.pctx.Logger != nil {
				p.pctx.Logger.Warn("memory: diary vector backfill failed", "id", d.ID, "err", err)
			}
			continue
		}
		if err := p.diaryStore.VectorizeDiary(d.ID, vec); err != nil {
			if p.pctx.Logger != nil {
				p.pctx.Logger.Warn("memory: diary vector save failed", "id", d.ID, "err", err)
			}
			continue
		}
		count++
	}
	if count > 0 && p.pctx.Logger != nil {
		p.pctx.Logger.Info("memory: diary vector backfill done", "count", count)
	}
}

// proactiveTimeoutLoop checks every 2 minutes for proactive messages that were
// never replied to, recording them as ignored for adaptive learning.
func (p *MemoryPlugin) proactiveTimeoutLoop() {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		if !p.IsRunning() {
			return
		}
		if p.pendingProactiveID == 0 {
			continue
		}
		if time.Since(p.pendingProactiveAt) < 5*time.Minute {
			continue
		}
		// User didn't reply within 5 minutes — record as ignored.
		if p.outcomeRepo != nil {
			o := domain.ActionOutcome{
				ActionSource:  p.pendingOutcomeCtx.Source,
				ActionType:    p.pendingOutcomeCtx.Type,
				HourOfDay:     p.pendingOutcomeCtx.HourOfDay,
				DayOfWeek:     p.pendingOutcomeCtx.DayOfWeek,
				AppContext:    p.pendingOutcomeCtx.AppContext,
				EmotionBucket: p.pendingOutcomeCtx.EmotionBucket,
				EscalationLvl: p.pendingEscalationLvl,
				Outcome:       domain.OutcomeIgnored,
			}
			if err := p.outcomeRepo.SaveOutcome(o); err != nil && p.pctx.Logger != nil {
				p.pctx.Logger.Warn("memory: failed to save outcome (timeout)", "err", err)
			}
			if p.decisionEngine != nil {
				p.decisionEngine.RecordOutcome(
					string(p.pendingOutcomeCtx.Source),
					domain.DecisionOutput{Action: "speak", Source: string(p.pendingOutcomeCtx.Source)},
					"ignored",
				)
				if p.learner != nil {
					p.learner.UpdateLastReward(p.pendingDriveID, 0) // ignored = neutral
				}
			}
		}
		p.pendingProactiveID = 0
		p.pendingDriveID = 0
		p.consecutiveUnanswered++
		if p.pctx.Logger != nil && p.consecutiveUnanswered >= 2 {
			p.pctx.Logger.Info("memory: consecutive unanswered", "count", p.consecutiveUnanswered)
		}
	}
	// Garbage-collect expired rejection entries every cycle.
	if p.rejectedActions != nil {
		cutoff := time.Now().Add(-30 * time.Minute)
		for k, t := range p.rejectedActions {
			if t.Before(cutoff) {
				delete(p.rejectedActions, k)
			}
		}
	}
}

func (p *MemoryPlugin) eagerScreenObserve() {
	time.Sleep(2 * time.Second)
	obs, err := native.OCRActiveScreen()
	if err != nil || obs.AppName == "" || native.IsSelfApp(obs.AppName) {
		return
	}
	app := native.FriendlyAppName(obs.AppName)
	summary := fmt.Sprintf("软件: %s", app)
	if obs.WindowTitle != "" {
		summary += fmt.Sprintf(" | 窗口: %s", obs.WindowTitle)
	}
	if obs.IsWorking {
		summary += " | 正在工作"
	} else {
		summary += " | 休闲中"
	}
	p.lastScreenMu.Lock()
	p.lastScreenSummary = summary
	p.lastScreenMu.Unlock()
}

// GetScreenSummary returns the last screen observation summary, safe for concurrent reads.
func (p *MemoryPlugin) GetScreenSummary() string {
	p.lastScreenMu.RLock()
	defer p.lastScreenMu.RUnlock()
	return p.lastScreenSummary
}

func (p *MemoryPlugin) checkEmbeddingHealth() {
	if es, ok := p.embSvc.(*EmbeddingService); ok {
		if err := es.Health(); err != nil && p.pctx.Logger != nil {
			p.pctx.Logger.Warn("memory: embedding service unreachable", "err", err)
		}
	}
}

func (p *MemoryPlugin) setupLocalEmotion() {
	if p.pctx.Logger != nil {
		p.emotionModel.SetLogger(func(msg string, args ...any) {
			p.pctx.Logger.Info(msg, args...)
		})
	}
}

// Stop deactivates the plugin.
func (p *MemoryPlugin) Stop() error {
	p.mu.Lock()
	p.running = false
	bg := p.background
	p.mu.Unlock()

	if bg != nil {
		bg.Stop()
	}
	p.wg.Wait()

	if p.careEngine != nil && p.store != nil {
		if data, err := p.careEngine.SaveState(); err == nil && len(data) > 0 {
			p.store.SaveProfileValue("care_state", string(data))
		}
	}
	return nil
}

// IsRunning reports whether the plugin is active.
func (p *MemoryPlugin) IsRunning() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.running
}

// Store returns the underlying domain.MemoryStore.
func (p *MemoryPlugin) Store() MemoryStore {
	return p.store
}

// SetLLMSync wires the LLM summarisation function into the compressor and emotion model.
func (p *MemoryPlugin) SetLLMSync(fn func([]plugin.Message) (string, error)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.rawLLM = fn

	if p.emotionModel != nil {
		p.emotionModel.SetLLMEval(p.rawLLMWrapper())
	}
	if p.merger != nil {
		p.merger.SetLLM(p.rawLLMWrapper())
	}
	if p.compressor == nil {
		return
	}
	p.compressor.SetLLMSync(func(msgs []plugin.Message) (string, error) {
		sysMsg := plugin.Message{Role: "system", Content: buildCompressionPrompt()}
		return fn(append([]plugin.Message{sysMsg}, msgs...))
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

// SetVisionLLM replaces the screen analyzer's LLM backend with a vision-specific
// model (e.g. Qwen-VL via SiliconFlow). Call from app.go after plugin wiring.
func (p *MemoryPlugin) SetVisionLLM(fn func([]plugin.Message) (string, error)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.screenAnalyzer = NewScreenAnalyzer(fn)
}

// SetVectorizeFunc wires the embedding function into the diary store.
func (p *MemoryPlugin) SetVectorizeFunc(fn func(string) ([]float32, error)) {
	if p.diaryStore != nil {
		p.diaryStore.SetVectorize(fn)
	}
}


// triggerVisualAnalysis captures the screen and sends it to the vision model
// for curiosity-driven gap detection. Runs in a goroutine.
func (p *MemoryPlugin) triggerVisualAnalysis() {
	if p.screenCapture == nil || p.curiosityEngine == nil {
		return
	}
	b64, err := p.screenCapture()
	if err != nil || b64 == "" {
		return
	}
	appName, windowTitle := native.GetActiveWindowDetail()
	facts := p.store.ListActiveFacts(0)
	var factStrs []string
	for _, f := range facts {
		factStrs = append(factStrs, f.Content)
	}
	profile := p.store.LoadProfile()
	n := p.curiosityEngine.AnalyzeScreenshot(b64, appName, windowTitle, factStrs, profile.Name, profile.TechStack)
	if n > 0 && p.pctx.Logger != nil {
		p.pctx.Logger.Info("memory: visual analysis gaps found", "count", n)
	}
}


// runSystem2Decision assembles context for the System 2 LLM and executes
// the resulting decision through the tool registry.
func (p *MemoryPlugin) runSystem2Decision() {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("memory: runSystem2Decision panicked", "panic", r)
		}
	}()
	if p.decisionEngine == nil {
		return
	}

	now := time.Now()
	ctx := domain.DecisionContext{
		Now:               now,
		TimeSinceLastChat: time.Since(p.lastChatTime),
	}

	// Emotion.
	if p.emotionModel != nil {
		ctx.EmotionVec = p.emotionModel.CurrentVector()
		ctx.EmotionState = p.emotionModel.Current()
	}

	// Screen context.
	if p.background != nil {
		obs := ConvertScreenObs(p.background.LastScreenObs())
		if obs.AppName != "" {
			ctx.ScreenSummary = obs.AppName
		}
	}

	// Recent user message.
	if p.sessionBuf != nil {
		recent := p.sessionBuf.Recent(1)
		if len(recent) > 0 && recent[0].Role == "user" {
			ctx.RecentUserMsg = recent[0].Content
		}
	}

	// Active patterns — use GetPatternTriggers as proxy.
	if p.patternAnalyzer != nil {
		triggers := p.patternAnalyzer.GetPatternTriggers(now)
		for _, t := range triggers {
			ctx.ActivePatterns = append(ctx.ActivePatterns, *t.Pattern)
		}
	}

	// Active strategies.
	if p.principleRepo != nil {
		ctx.ActivePrinciples, _ = p.principleRepo.ListActive()
	}

	// Curiosity state.
	if p.cogRepo != nil {
		inqs, _ := p.cogRepo.List(domain.CuriosityInquiry, "active", 5)
		ctx.ActiveInquiries = len(inqs)
		gaps, _ := p.cogRepo.List(domain.CuriosityGap, "active", 5)
		ctx.KnowledgeGaps = len(gaps)
	}

	// Recent outcomes.
	if p.outcomeRepo != nil {
		ctx.RecentOutcomes, _ = p.outcomeRepo.RecentOutcomes("", 5)
	}

	// Known facts sample.
	if p.store != nil {
		facts := p.store.ListActiveFacts(0.3)
		for i, f := range facts {
			if i >= 5 {
				break
			}
			ctx.RecentFactSample = append(ctx.RecentFactSample, f.Content)
		}
	}

	// Tactical directives.
	if p.background != nil {
		ctx.TacticalDirectives = p.background.TacticalDirectives()
	}

	// ---- Intrinsic Needs Growth ----
	if p.needModel != nil {
		isWorking := false
		if p.background != nil {
			obs := ConvertScreenObs(p.background.LastScreenObs())
			isWorking = obs.IsWorking
		}
		p.needModel.Grow(now, isWorking, now.Hour(), ctx.TimeSinceLastChat)
		p.needModel.SaveTo(p.store.SaveProfileValue)
		// Push need modulation into the emotion model.
		mod := p.needModel.Modulation()
		p.emotionModel.SetNeedModulation(&mod)
	}

	// ---- Quantified Features + Needs Snapshot ----
	var feats *domain.QuantifiedFeatures
	var needsSnapshot *domain.IntrinsicNeeds

	if p.featureComputer != nil && p.emotionModel != nil {
		// Push emotion snapshot for trend computation (A4).
		p.featureComputer.PushEmotionSnapshot(ctx.EmotionState.Valence, ctx.EmotionVec)

		// Determine is_working from last screen observation.
		isWorking := false
		if p.background != nil {
			obs := ConvertScreenObs(p.background.LastScreenObs())
			isWorking = obs.IsWorking
		}

		// Collect personality parameters.
		pers := p.emotionModel.Personality()
		annoySens, affectWarm, worryTend := pers.AnnoyanceSensitivity, pers.AffectionWarmth, pers.WorryTendency

		// Compute all 46 quantified features (Tier 1 + Tier 2).
		feats = p.featureComputer.ComputeFull(
			now,
			ctx.EmotionVec,
			ctx.EmotionState,
			ctx.TimeSinceLastChat,
			isWorking,
			ctx.DailyActionCount,
			20, // maxDaily
			p.consecutiveAction,
			ctx.ActiveInquiries,
			ctx.KnowledgeGaps,
			len(ctx.ActivePrinciples),
			len(ctx.ActivePatterns),
			p.decisionEngine.ReflexionLogCount(),
			annoySens, affectWarm, worryTend,
		)

		// Debug: log key feature values for observability.
		if p.pctx.Logger != nil {
			p.pctx.Logger.Info("memory: features",
				"app", feats.U1_AppCategory,
				"win", feats.U2_WindowSubtype,
				"work", feats.U3_IsWorking,
				"contWorkMin", fmt.Sprintf("%.0f", feats.U4_ContinuousWorkMins),
				"switch", fmt.Sprintf("%.0f", feats.U5_AppSwitchCount),
				"lengthTrend", fmt.Sprintf("%.2f", feats.U7_LengthTrend),
				"engage", fmt.Sprintf("%.2f", feats.U8_EngagementNorm),
				"meal", feats.U11_MealTime,
				"night", feats.U12_NightTime,
				"valenceTrend", fmt.Sprintf("%.2f", feats.A4_ValenceTrend),
				"cooldown", fmt.Sprintf("%.2f", feats.E3_CooldownNorm),
				"quota", feats.E4_QuotaRemaining,
				"accept", fmt.Sprintf("%.2f", feats.R1_OverallAcceptRate),
				"rejectSev", fmt.Sprintf("%.2f", feats.R4_RejectionSeverity),
				"neglectHrs", fmt.Sprintf("%.1f", feats.R5_NeglectHours),
			)
		}
	}

	// Snapshot needs for LLM prompt + API exposure.
	if p.needModel != nil {
		snap := p.needModel.Snapshot()
		needsSnapshot = &snap
		p.lastNeedsSnapshot = needsSnapshot
	}
	// Store latest features for API exposure.
	p.lastFeatures = feats

	// Compute drives from quantified features (kept for S1 fallback and debugging).
	var dec *domain.DecisionOutput
	s, c, cur, q, e := cognition.ComputeDrives(feats, needsSnapshot)

	// ---- v0.4 decision pipeline ----
	// Safety gates (hard rules — LLM cannot override).
	if p.consecutiveUnanswered >= 2 {
		if p.pctx.Logger != nil {
			p.pctx.Logger.Info("memory: suppressing all due to consecutive unanswered", "count", p.consecutiveUnanswered)
		}
		return
	}
	if p.recentChatSaysGoodnight() {
		return
	}
	if p.consecutiveUnanswered >= 1 && time.Since(p.pendingProactiveAt) > 30*time.Minute {
		p.consecutiveUnanswered--
	}

	// Immediate correction: check for recently-suppressed actions.
	immediateSuppress := make(map[string]bool)
	if p.feedbackProcessor != nil && p.feedbackProcessor.Immediate != nil {
		for _, a := range p.feedbackProcessor.Immediate.SuppressedActions() {
			immediateSuppress[a] = true
		}
	}

	// 1. Try S1 rule engine.
	drives := cognition.DriveScores{Social: s, Care: c, Curious: cur, Quiet: q, Explore: e}
	ruleResult, hasRule := p.ruleEngine.Decide(feats, drives)
	if hasRule && immediateSuppress[ruleResult.Action] {
		hasRule = false
	}

	// 2. Meta-reasoner decides the path.
	hasConflict := hasRule && ruleResult != nil && ruleResult.MatchedCount > 1
	hasExtreme := ctx.EmotionState.Primary == "anger" || ctx.EmotionState.Primary == "fear"
	route := p.metaReasoner.Route(feats, ruleResult, hasConflict, hasExtreme)

	// 3. Execute on the chosen path.
	var err error
	switch route {
	case cognition.PathNone:
		return
	case cognition.PathS1:
		if ruleResult == nil {
			return
		}
		dec = &domain.DecisionOutput{
			ShouldAct: ruleResult.Action != "none",
			Action:    ruleResult.Action,
			Source:    ruleResult.RuleSource,
			Reason:    "S1 rule engine (conf=" + fmt.Sprintf("%.2f", ruleResult.Confidence) + ")",
			Priority:  ruleResult.Confidence,
		}
	case cognition.PathS2Lite:
		dec, err = p.decisionEngine.DecideLite(ctx, feats, needsSnapshot)
	case cognition.PathS2Full:
		dec, err = p.decisionEngine.DecideFull(ctx, feats, needsSnapshot)
	}
	if err != nil || dec == nil || !dec.ShouldAct || dec.Action == "none" {
		return
	}
	if immediateSuppress[dec.Action] {
		return
	}

	// Track consecutive actions for repetition detection.
	if dec.Action == p.lastScoredAction {
		p.consecutiveAction++
	} else {
		p.consecutiveAction = 1
	}
	p.lastScoredAction = dec.Action

	// Record drive snapshot for feedback processing.
	if p.learner != nil {
		p.pendingDriveID = p.learner.RecordDrive(dec.Action, s, c, cur, q, e, 0)
		if p.learner.ShouldLearn() {
			p.learner.BatchLearn()
			if p.principleRepo != nil {
				p.learner.DistillStrategies(p.principleRepo)
			}
		}
		if stuck, drift := p.learner.Audit(); len(stuck) > 0 || drift {
			if drift && p.pctx.Logger != nil {
				p.pctx.Logger.Warn("learner: drift detected", "stuck", stuck)
			}
		}
	}

	// Execute via tool registry or direct action.
	// First tick after startup: prefer opening a curiosity topic over care_rest.
	if !p.decisionWarmupDone {
		p.decisionWarmupDone = true
		if ctx.ActiveInquiries > 0 && dec.Action != "speak_inquiry" {
			dec = &domain.DecisionOutput{
				ShouldAct: true, Action: "speak_inquiry", Source: "knowledge_gap",
				Reason: "启动后首次搭话——优先选择好奇心话题打开对话", Priority: 0.8,
			}
		}
	}
		// Unified dispatch via ActionDef — scorer and LLM share the same action space.
		def := cognition.ActionByName(dec.Action)
		switch {
		case def == nil:
			if p.pctx.Logger != nil {
				p.pctx.Logger.Warn("memory: unknown action", "action", dec.Action)
			}
		case def.NeedsTool:
			// Tool-based actions: search, observe, reflect, analyze_patterns, browse.
			switch def.Name {
			case "analyze_patterns":
				// Pattern analysis uses its own pipeline, not the tool registry.
				if p.patternAnalyzer != nil {
					api.StatusBusInstance().EmitStart("pattern", "行为模式分析...")
					patterns, err := p.patternAnalyzer.Analyze()
					if err != nil {
						api.StatusBusInstance().EmitFail("pattern", err.Error())
					} else {
						api.StatusBusInstance().EmitOK("pattern", fmt.Sprintf("发现 %d 个行为模式", len(patterns)))
					}
				}
			case "observe":
				// Screen observation runs asynchronously.
				go p.triggerVisualAnalysis()
			default:
				// search, reflect, browse — use the tool registry.
				if p.toolRegistry != nil {
					toolInput := dec.ToolInput
					if toolInput == "" {
						toolInput = dec.Reason
					}
					api.StatusBusInstance().EmitStart(dec.Action, fmt.Sprintf("%s: %s...",
						def.DisplayName, toolInput[:min(len(toolInput), 40)]))
					result, err := p.toolRegistry.Execute(context.Background(), def.ToolName, toolInput)
					if err != nil {
						api.StatusBusInstance().EmitFail(dec.Action, err.Error())
						if p.pctx.Logger != nil {
							p.pctx.Logger.Warn("memory: tool execution failed", "tool", def.ToolName, "err", err)
						}
					} else if !result.Success {
						api.StatusBusInstance().EmitFail(dec.Action, result.Error)
					} else {
						api.StatusBusInstance().EmitOK(dec.Action, result.Output[:min(len(result.Output), 80)])
					}
				}
			}
			if p.featureComputer != nil {
				p.featureComputer.NoteAction()
			}
			if p.needModel != nil {
				p.needModel.Satisfy(dec.Action, domain.OutcomeIgnored)
			}
		case def.Category == "social" || def.Category == "care":
			// Speak-based actions: talk to the user.
			toolName := "speak"
			toolInput := dec.Source
			if dec.Reason != "" {
				toolInput += "|" + dec.Reason + "|" + dec.Mood
			}
			result, err := p.toolRegistry.Execute(context.Background(), toolName, toolInput)
			if err != nil && p.pctx.Logger != nil {
				p.pctx.Logger.Warn("memory: tool execution failed", "tool", toolName, "err", err)
			}
			if !result.Success && p.pctx.Logger != nil {
				p.pctx.Logger.Warn("memory: tool returned failure", "tool", toolName, "error", result.Error)
			}
			if p.featureComputer != nil {
				p.featureComputer.NoteAction()
			}
			if p.needModel != nil {
				p.needModel.Satisfy(dec.Action, domain.OutcomeIgnored)
			}
		}

	// Note the decision tick for E5 (cooldown tracking).
	if p.featureComputer != nil {
		p.featureComputer.NoteDecision()
	}

	if p.pctx.Logger != nil {
		p.pctx.Logger.Info("memory: System 2 decision", "action", dec.Action, "source", dec.Source, "reason", dec.Reason)
	}
}

func (p *MemoryPlugin) rawLLMWrapper() func(string) (string, error) {
	return func(prompt string) (string, error) {
		return p.rawLLM([]plugin.Message{{Role: "user", Content: prompt}})
	}
}

// RunSelfUpdate triggers the memory layer's self-model consolidation using the LLM.
// Called from the identity self-update API endpoint.
func (p *MemoryPlugin) RunSelfUpdate() error {
	if p.memLayers == nil || p.rawLLM == nil {
		return nil
	}
	llmFunc := func(msgs []plugin.Message) (string, error) {
		return p.rawLLM(msgs)
	}
	// Pass through the layer's rawLLM wrapper.
	if err := p.memLayers.ConsolidateFactsToSelf(func(msgs []domain.Message) (string, error) {
		pluginMsgs := make([]plugin.Message, len(msgs))
		for i, m := range msgs {
			pluginMsgs[i] = plugin.Message{Role: m.Role, Content: m.Content}
		}
		return llmFunc(pluginMsgs)
	}); err != nil {
		return err
	}
	return nil
}

func relativeTimeTag(r UnifiedResult) string {
	if r.CreatedAt == 0 {
		return ""
	}
	return fmt.Sprintf("(%s) ", RelativeTime(r.CreatedAt))
}

func joinTechStack(stack []string) string {
	if len(stack) == 0 {
		return ""
	}
	result := stack[0]
	for _, s := range stack[1:] {
		result += "、" + s
	}
	return result
}

func parseStack(raw string) []string {
	parts := regexp.MustCompile(`[、,，和]\s*`).Split(raw, -1)
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// LearningSnapshot returns aggregated learning data for the frontend dashboard.
func (p *MemoryPlugin) LearningSnapshot() (acceptRate float64, totalToday int, totalWeek int, bySource map[string]int, principlesCount int, threadsCount int, inquiriesCount int, patternsCount int) {
	bySource = map[string]int{}
	if p.outcomeRepo != nil {
		// Single batch: one query for today+week accepts/totals, then per-source totals.
		ctx := domain.ActionContext{}
		accepts, total := p.outcomeRepo.SuccessRate(ctx, 1)
		totalToday = total
		if total > 0 {
			acceptRate = float64(accepts) / float64(total) * 100
		}
		_, totalWeek = p.outcomeRepo.SuccessRate(ctx, 7)
		// Per-source: two queries each, but only for 3 sources (6 total, not 10).
		for _, src := range []domain.ProactiveSource{domain.SourceCare, domain.SourceCasual, domain.SourceKnowledgeGap} {
			_, t := p.outcomeRepo.SuccessRate(domain.ActionContext{Source: src}, 7)
			bySource[string(src)] = t
		}
	}
	if p.principleRepo != nil {
		principlesCount = p.principleRepo.Count()
	}
	if p.threadRepo != nil {
		threads, _ := p.threadRepo.ListActive()
		threadsCount = len(threads)
	}
	if p.cogRepo != nil {
		inqs, _ := p.cogRepo.List(domain.CuriosityInquiry, "active", 100)
		inquiriesCount = len(inqs)
	}
	// Pattern count from the analyzer (approximate via active patterns).
	// The PatternAnalyzer doesn't expose a count directly; we skip for now.
	_ = patternsCount
	return
}

func actionToSource(action string) string {
	switch action {
	case "speak_care", "care_rest", "care_meal", "care_hydration", "care_health", "care_encourage", "care_social":
		return "care"
	case "speak_inquiry":
		return "knowledge_gap"
	case "speak_casual":
		return "casual"
	default:
		return ""
	}
}

func messagesToText(msgs []plugin.Message) string {
	var sb strings.Builder
	for _, m := range msgs {
		sb.WriteString(m.Role)
		sb.WriteString(": ")
		sb.WriteString(m.Content)
		sb.WriteString("\n")
	}
	return sb.String()
}

func recoverGuard(name string) {
	if r := recover(); r != nil {
		slog.Warn("memory: goroutine panic recovered", "name", name, "panic", r)
	}
}

// extractFactsFromSearch uses the LLM to extract and judge facts from search results.
// Quality scoring: reliability (source authority), relevance (to query), novelty (not already known).
// Only facts scoring ≥0.5 are saved to the memory system.
func (p *MemoryPlugin) extractFactsFromSearch(query string, results []tools.SearchResult) string {
	if p.rawLLM == nil {
		return fmt.Sprintf("search: %q → LLM not available", query)
	}

	// Collect known facts for novelty check (avoid re-learning the same thing).
	knownSample := ""
	if p.store != nil {
		facts := p.store.ListActiveFacts(0)
		var parts []string
		for i, f := range facts {
			if i >= 10 {
				break
			}
			parts = append(parts, f.Content)
		}
		knownSample = strings.Join(parts, "; ")
	}

	if len(results) > 0 {
		var sb strings.Builder
		for i, r := range results {
			if i >= 5 {
				break
			}
			sb.WriteString(fmt.Sprintf("%d. %s\n   %s\n   URL: %s\n", i+1, r.Title, r.Snippet, r.URL))
		}
		prompt := fmt.Sprintf(`从以下搜索结果中提取知识，并进行质量评判。

查询: %s

搜索结果:
%s

已知事实（避免重复学习）: %s

请用JSON格式回复：
{
  "facts": [{"content": "提取的事实", "source_url": "来源URL"}],
  "quality": {
    "reliability": 0.8,   // 来源可信度 (0-1): 官方文档>技术博客>论坛
    "relevance": 0.9,      // 与查询相关度 (0-1)
    "novelty": 0.7,        // 新颖性 (0-1): 与已知事实的差异程度
    "overall": 0.8         // 综合评分 (0-1): 0.5以上才值得记忆
  }
}
只输出JSON。`, query, sb.String(), knownSample)

		answer, err := p.rawLLM([]plugin.Message{{Role: "user", Content: prompt}})
		if err != nil || answer == "" {
			return fmt.Sprintf("search: %q → no answer", query)
		}
		return p.saveJudgedFacts(query, answer, results)
	}

	// No search results — LLM answers directly, but mark as lower confidence.
	prompt := fmt.Sprintf(`请用1-2句中文回答以下问题。同时进行自我评判。

问题: %s

用JSON格式回复：
{
  "facts": [{"content": "回答内容", "source_url": ""}],
  "quality": {"reliability": 0.4, "relevance": 0.8, "novelty": 0.5, "overall": 0.4}
}
只输出JSON。`, query)

	answer, err := p.rawLLM([]plugin.Message{{Role: "user", Content: prompt}})
	if err != nil || answer == "" {
		return fmt.Sprintf("search: %q → no answer", query)
	}
	return p.saveJudgedFacts(query, answer, nil)
}

// saveJudgedFacts parses the LLM's JSON output and saves high-quality facts.
func (p *MemoryPlugin) saveJudgedFacts(query, jsonResp string, results []tools.SearchResult) string {
	raw := CleanJSON(jsonResp)

	var output struct {
		Facts   []struct {
			Content   string `json:"content"`
			SourceURL string `json:"source_url"`
		} `json:"facts"`
		Quality struct {
			Reliability float64 `json:"reliability"`
			Relevance   float64 `json:"relevance"`
			Novelty     float64 `json:"novelty"`
			Overall     float64 `json:"overall"`
		} `json:"quality"`
	}
	if err := json.Unmarshal([]byte(raw), &output); err != nil {
		return fmt.Sprintf("search: %q → parse error: %v", query, err)
	}

	// Fallback: if no facts parsed, treat the raw response as a fact.
	if len(output.Facts) == 0 && output.Quality.Overall <= 0 {
		output.Facts = []struct {
			Content   string `json:"content"`
			SourceURL string `json:"source_url"`
		}{{Content: raw}}
		output.Quality.Overall = 0.4
	}
	if output.Quality.Overall <= 0 {
		output.Quality.Overall = (output.Quality.Reliability*0.3 +
			output.Quality.Relevance*0.3 + output.Quality.Novelty*0.2 + 0.5*0.2)
	}

	saved := 0
	for _, f := range output.Facts {
		if strings.TrimSpace(f.Content) == "" {
			continue
		}
		content := strings.TrimSpace(f.Content)

		// Quality gate: only save facts scoring ≥ 0.5 overall.
		if output.Quality.Overall >= 0.5 {
			if p.store != nil {
				_ = p.store.SaveFact(content, "web_search")
			}
			if p.cogRepo != nil {
				evidence := f.SourceURL
				if evidence == "" && len(results) > 0 {
					evidence = results[0].URL
				}
				inquiry := domain.CuriosityItem{
					ItemType: domain.CuriosityInquiry, Content: content,
					Confidence: output.Quality.Overall, Priority: output.Quality.Overall,
					Status: "active", Source: "web_search", Evidence: evidence,
				}
				_, _ = p.cogRepo.Save(inquiry)
			}
			saved++
		}
	}

	if p.pctx.Logger != nil {
		p.pctx.Logger.Info("memory: search facts judged",
			"query", query[:min(len(query), 40)],
			"extracted", len(output.Facts),
			"saved", saved,
			"score", fmt.Sprintf("%.2f", output.Quality.Overall),
		)
	}

	summary := fmt.Sprintf("search: %q → %d facts (score:%.2f, saved:%d)",
		query, len(output.Facts), output.Quality.Overall, saved)
	for _, f := range output.Facts {
		summary += "\n  - " + f.Content
	}
	return summary
}

// extractFactsFromBrowse uses the LLM to extract knowledge from a web page.
func (p *MemoryPlugin) extractFactsFromBrowse(url, rawText string) (string, []string) {
	if p.rawLLM == nil || len(rawText) < 200 {
		return fmt.Sprintf("browse: %s", url), nil
	}
	// Truncate to ~2000 chars for the LLM.
	text := rawText
	if len(text) > 2000 {
		text = text[:2000]
	}
	prompt := fmt.Sprintf(
		"从以下网页内容中提取1-2个关键事实，用中文简要总结。只输出事实，不要加前缀。\nURL: %s\n\n%s",
		url, text,
	)
	result, err := p.rawLLM([]plugin.Message{{Role: "user", Content: prompt}})
	if err != nil || result == "" {
		return fmt.Sprintf("browse: %s", url), nil
	}
	result = strings.TrimSpace(result)
	var facts []string
	for _, line := range strings.Split(result, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			facts = append(facts, line)
			if p.store != nil {
				_ = p.store.SaveFact(line, "web_browse")
			}
		}
	}
	if p.cogRepo != nil && len(facts) > 0 {
		inquiry := domain.CuriosityItem{
			ItemType: domain.CuriosityInquiry, Content: facts[0],
			Confidence: 0.5, Priority: 0.5, Status: "active", Source: "web_browse",
		}
		_, _ = p.cogRepo.Save(inquiry)
	}
	if p.pctx.Logger != nil {
		p.pctx.Logger.Info("memory: browse facts extracted", "url", url, "count", len(facts))
	}
	return result, facts
}

// CurrentFeatures returns the latest quantified features snapshot (for API exposure).
func (p *MemoryPlugin) CurrentFeatures() *domain.QuantifiedFeatures {
	return p.lastFeatures
}

// CurrentNeeds returns the latest intrinsic needs snapshot (for API exposure).
func (p *MemoryPlugin) CurrentNeeds() *domain.IntrinsicNeeds {
	return p.lastNeedsSnapshot
}

// buildPersonaSummary returns the same persona context used in normal chat.
func (p *MemoryPlugin) buildPersonaSummary() string {
	var parts []string
	if p.selfModel != nil {
		if s := p.selfModel.Current(); s != "" {
			parts = append(parts, s)
		}
	}
	if p.emotionModel != nil {
		e, v := p.emotionModel.Current(), p.emotionModel.CurrentVector()
		parts = append(parts, fmt.Sprintf(
			"当前情绪: %s (强度%.0f%%), 亲密度%.0f%%, 担忧%.0f%%, 好奇%.0f%%, 困倦%.0f%%, 贪玩%.0f%%, 寂寞%.0f%%, 烦躁%.0f%%",
			e.Primary, e.Intensity*100, v.Affection*100, v.Worry*100, v.Curiosity*100,
			v.Sleepiness*100, v.Playfulness*100, v.Loneliness*100, v.Annoyance*100,
		))
	}
	return strings.Join(parts, "\n")
}

// SetBingSearchAPIKey configures the Bing Web Search API key.
func (p *MemoryPlugin) SetBingSearchAPIKey(key string) { p.bingSearchAPIKey = key }

func (p *MemoryPlugin) SetBochaAPIKey(key string) { p.bochaAPIKey = key }

// ListStrategies returns all active strategy principles.
func (p *MemoryPlugin) ListStrategies() []domain.StrategyPrinciple {
	if p.principleRepo == nil {
		return nil
	}
	list, _ := p.principleRepo.ListActive()
	return list
}

// SummarizeConversation compresses the recent conversation into a summary via LLM.
// Called every ~10 user turns to keep proactive messages context-aware.
func (p *MemoryPlugin) SummarizeConversation() {
	if p.sessionBuf == nil || p.rawLLM == nil {
		return
	}
	recent := p.sessionBuf.Recent(15)
	if len(recent) < 8 {
		return
	}
	var sb strings.Builder
	for _, m := range recent {
		sb.WriteString(m.Role)
		sb.WriteString(": ")
		sb.WriteString(m.Content)
		sb.WriteString("\n")
	}
	prompt := fmt.Sprintf(
		"用1-2句中文简要总结以下对话。只写摘要，不要加前缀。关注：用户在做什么、情绪如何、有什么重要话题。\n\n%s",
		sb.String(),
	)
	summary, err := p.rawLLM([]plugin.Message{{Role: "user", Content: prompt}})
	if err == nil && summary != "" {
		p.conversationSummary = strings.TrimSpace(summary)
	}
}

// recentChatSaysGoodnight checks if the last few chat messages contain sleep/goodbye keywords.
func (p *MemoryPlugin) recentChatSaysGoodnight() bool {
	if p.sessionBuf == nil {
		return false
	}
	return recentChatContains(p, []string{"晚安", "睡了", "睡觉", "梦里见", "好梦", "明天见", "bye", "goodnight", "night", "困了", "先睡"})
}

// recentChatAlreadyCovers returns true if the recent conversation already covers
// the same topic that a proactive action is about to address.
// Checks both in-memory session buffer AND DB chat_history to survive restarts.
func (p *MemoryPlugin) recentChatAlreadyCovers(action string) bool {
	text := ""
	if p.sessionBuf != nil {
		recent := p.sessionBuf.Recent(6)
		for _, m := range recent {
			text += strings.ToLower(m.Content) + " "
		}
	}
	// Also load from DB to cover messages before restart.
	if text == "" && p.store != nil {
		history, err := p.store.LoadHistory(10)
		if err == nil {
			for _, m := range history {
				text += strings.ToLower(m.Content) + " "
			}
		}
	}
	if text == "" {
		return false
	}

	switch {
	case action == "care_meal":
		// Already talked about food/lunch/dinner — don't need both 吃+饭.
		foodWords := []string{"饭", "午饭", "晚餐", "煲仔", "外卖", "菜", "饿", "吃了", "吃完", "恰饭", "食堂", "点餐", "点了饭", "吃饱", "不饿", "不吃了", "吃过了", "吃饱了", "点过了", "已经点了"}
		for _, w := range foodWords {
			if strings.Contains(text, w) {
				return true
			}
		}
		return false
	case action == "speak_care", action == "care_rest":
		// Already talked about sleep/rest.
		return recentChatContains(p, []string{"晚安", "睡了", "睡觉", "休息", "困了", "先睡", "梦里见"})
	case action == "care_health":
		return strings.Contains(text, "健康") || strings.Contains(text, "运动") || strings.Contains(text, "锻炼")
	case action == "speak_casual":
		// Don't repeat casual topics from recent conversation.
		return recentChatContains(p, []string{"哈哈", "搞笑", "有趣", "好玩", "笑死", "nb", "牛", "厉害"})
	case action == "speak_inquiry":
		// Don't ask about things already discussed.
		return recentChatContains(p, []string{"知道", "了解", "请问", "查一下", "告诉我", "怎么", "什么是", "如何"})
	}
	return false
}

func recentChatContains(p *MemoryPlugin, keywords []string) bool {
	recent := p.sessionBuf.Recent(5)
	for _, m := range recent {
		lower := strings.ToLower(m.Content)
		for _, kw := range keywords {
			if strings.Contains(lower, kw) {
				return true
			}
		}
	}
	return false
}

// RejectAction marks an action type as rejected within the current conversation session.
func (p *MemoryPlugin) RejectAction(action string) {
	if p.rejectedActions == nil {
		p.rejectedActions = make(map[string]time.Time)
	}
	p.rejectedActions[action] = time.Now()
}

// IsActionRejected returns true if the action was rejected in the current session.
func (p *MemoryPlugin) IsActionRejected(action string) bool {
	if p.rejectedActions == nil {
		return false
	}
	t, ok := p.rejectedActions[action]
	if !ok {
		return false
	}
	// Rejection expires after 30 minutes of idle.
	return time.Since(t) < 30*time.Minute
}
