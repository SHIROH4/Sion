package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"desktop-pet/internal/api"
	"desktop-pet/internal/app/plugin"
	"desktop-pet/internal/domain"
	"desktop-pet/internal/service/cognition"
	infracfg "desktop-pet/internal/infra/config"
	infrallm "desktop-pet/internal/infra/llm"
	"desktop-pet/internal/infra/native"
	infrastorage "desktop-pet/internal/infra/storage"
	"desktop-pet/internal/plugins/chat"
	"desktop-pet/internal/plugins/memory"
	"desktop-pet/internal/plugins/qq"
	"desktop-pet/internal/plugins/search"
	"desktop-pet/internal/plugins/vision"
	care "desktop-pet/internal/service/care"
	"desktop-pet/internal/service/diary"
	emotion "desktop-pet/internal/service/emotion"
	"desktop-pet/internal/service/identity"
	svcmemory "desktop-pet/internal/service/memory"
	"desktop-pet/internal/service/scheduler"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx      context.Context
	manager  *plugin.Manager
	lastChat time.Time

	// DI: domain interfaces (populated in Startup).
	store         domain.MemoryStore
	LLMGW         *infrallm.Gateway
	visGW         *infrallm.Gateway
	diaryRepo     domain.DiaryRepository
	selfModel     *identity.SelfModel
	identityRepo  domain.IdentityRepository
	identityGraph *identity.IdentityGraph
	emotionModel  *emotion.EmotionModel
	careEngine    *care.CareEngine
	startTime     time.Time
	sessionBuf    *svcmemory.SessionBuffer
	todayTokens   int
	MemPlugin     *memory.MemoryPlugin
}

// emitSafe wraps runtime.EventsEmit with a context validity check, preventing
// panics when the Wails window context has been invalidated (e.g., window closed).
func (a *App) emitSafe(eventName string, data interface{}) {
	defer func() { recover() }()
	if a.ctx == nil || a.ctx.Err() != nil {
		return
	}
	runtime.EventsEmit(a.ctx, eventName, data)
}


// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// Startup is called when the pet window starts.
// The pet is a thin renderer — all AI logic runs in the settings process.
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx

	// ---- Wails UI setup ----
	runtime.WindowSetBackgroundColour(ctx, 0, 0, 0, 0)
	go a.trackMouse(ctx)
}

// domainReady assembles all dependencies using the layered DI pattern.
//
//	Layer 1: infra/  — concrete implementations
//	Layer 2: service/ — domain interfaces
//	Layer 3: app/ — orchestration
func (a *App) domainReady(ctx context.Context) error {
	// ---- Layer 1: Infrastructure ----
	cfg := infracfg.Load()
	a.LLMGW = infrallm.NewGateway(cfg)
	a.visGW = infrallm.NewVisionGateway(cfg)

	dbPath := filepath.Join(getPluginDir(), "memory.db")
	db, err := infrastorage.OpenDB(dbPath)
	if err != nil {
		return fmt.Errorf("无法打开数据库 (%s): %w — 请检查磁盘空间和目录权限", dbPath, err)
	}
	store := infrastorage.NewStore(db)
	a.store = store

	// ---- Layer 2: Services ----
	// Build all service objects before plugins start, so they can be injected.
	ollamaURL := os.Getenv("OLLAMA_BASE_URL")
	if ollamaURL == "" {
		ollamaURL = "http://localhost:11434"
	}

	embModel := cfg.EmbeddingModel
	if embModel == "" {
		embModel = "hf.co/CompendiumLabs/bge-small-zh-v1.5-gguf"
	}
	embSvc := svcmemory.NewEmbeddingService(ollamaURL, embModel)

	sessionBuf := svcmemory.NewSessionBuffer(20)

	// Backfill L0 working memory from persisted chat history on startup
	// so session context survives process restarts.
	if history, err := store.LoadHistory(20); err == nil && len(history) > 0 {
		for _, m := range history {
			sessionBuf.Append(m)
		}
		fmt.Printf("[startup] L0 session: restored %d messages from DB\n", len(history))
	}

	a.sessionBuf = sessionBuf
	diaryRepo := infrastorage.NewDiaryRepo(db)
	a.diaryRepo = diaryRepo
	diaryStore := diary.NewDiaryStore(diaryRepo)
	diaryStore.SetVectorize(embSvc.Vectorize)
	store.SetEmbedSvc(embSvc)

	selfModel := identity.NewSelfModel(store)
	a.selfModel = selfModel
	selfModel.Load()

	memLayers := svcmemory.NewMemoryLayer(sessionBuf, diaryStore, store, selfModel, identity.BuildSelfUpdatePrompt)

	emotionModel := emotion.NewEmotionModel(nil)
	a.emotionModel = emotionModel
	emotionModel.SetStore(store)
	_ = emotionModel.Load() // restore persisted emotion state
	// Wire emotion cloud LLM (only fires when rules don't match AND config flag is on).
	emotionModel.SetLLMEval(func(prompt string) (string, error) {
		cfg := infracfg.Load()
		if cfg.GetPluginConfig("memory")["emotion_cloud_enabled"] != true {
			return "", fmt.Errorf("emotion cloud disabled")
		}
		// Use dedicated emotion API if configured, otherwise fall back to chat LLM.
		gw := a.LLMGW
		if cfg.EmotionModel != "" || cfg.EmotionBaseURL != "" || cfg.EmotionAPIKey != "" {
			model, baseURL, apiKey := cfg.EmotionModel, cfg.EmotionBaseURL, cfg.EmotionAPIKey
			if model == "" {
				model = cfg.LLMModel
			}
			if baseURL == "" {
				baseURL = cfg.LLMBaseURL
			}
			if apiKey == "" {
				apiKey = cfg.LLMAPIKey
			}
			gw = infrallm.NewGateway(&infracfg.GlobalConfig{
				LLMModel: model, LLMAPIKey: apiKey, LLMBaseURL: baseURL,
			})
		}
		return gw.ChatSync(ctx, []plugin.Message{{Role: "user", Content: prompt}})
	})

	// CareEngine with EventBus callback.
	careEngine := care.NewCareEngine(
		domain.NewUserCareState(),
		func(action domain.CareAction) error {
			if a.manager != nil {
				a.manager.EventBus().Emit("care:action", action)
			}
			return nil
		},
		func() domain.EmotionState { return emotionModel.Current() },
	)
	a.careEngine = careEngine
	a.startTime = time.Now()

	// Warm start: apply pre-configured personality and known facts.
	a.applyWarmStart(cfg, emotionModel, store)

	merger := diary.NewDiaryMerger(diaryStore)
	compressor := svcmemory.NewCompressor(nil, svcmemory.CompressConfig{Level0Threshold: 20})

	episodeStore := svcmemory.NewEpisodeStore(infrastorage.NewEpisodeRepo(db))
	episodeStore.SetVectorize(embSvc.Vectorize)
	topicRepo := infrastorage.NewTopicRepo(db)
	topicStore := svcmemory.NewTopicService(topicRepo)
	topicStore.SetVectorize(embSvc.Vectorize)
	topicStore.Initialize()
	topicStore.SeedCentroids()
	topicStore.BackfillTopicAssignments(episodeStore)

	var vecFn func(string) ([]float32, error)
	if embSvc != nil {
		vecFn = embSvc.Vectorize
	}
	identityRepo := infrastorage.NewIdentityRepo(db)
	a.identityRepo = identityRepo
	identityGraph := identity.NewIdentityGraph(identityRepo, vecFn)
	a.identityGraph = identityGraph
	_ = identityGraph.Load()

	schedulerInst := scheduler.NewScheduler()

	profile := store.LoadProfile()

	// ---- Layer 3: Plugins with DI ----
	// Build LLM wrapper functions from gateways (avoids cross-plugin type assertions).
	llmSync := func(msgs []plugin.Message) (string, error) {
		return a.LLMGW.ChatSync(context.Background(), msgs)
	}

	// llmWithTools wraps the Gateway's ChatSyncWithTools for the System 2 decision
	// engine. It passes all 16 action tools with tool_choice="required" so the LLM
	// must pick exactly one action. Returns (toolName, toolArgsJSON, error).
	llmWithTools := func(messages []domain.Message, tools []cognition.DecisionToolSpec) (string, string, error) {
		// Convert domain.Message → plugin.Message (inline to avoid forward ref).
		pluginMsgs := make([]plugin.Message, len(messages))
		for i, m := range messages {
			pluginMsgs[i] = plugin.Message{Role: m.Role, Content: m.Content}
		}
		// Convert DecisionToolSpec → llm.Tool.
		llmTools := make([]infrallm.Tool, len(tools))
		for i, t := range tools {
			llmTools[i] = infrallm.Tool{
				Type: "function",
				Function: infrallm.ToolFunction{
					Name:        t.Name,
					Description: t.Description,
					Parameters:  t.Parameters,
				},
			}
		}

		var chosenName, chosenArgs string
		onTool := func(name, argsJSON string) string {
			chosenName = name
			chosenArgs = argsJSON
			return `{"status":"recorded"}`
		}

		_, err := a.LLMGW.ChatSyncWithTools(
			context.Background(),
			pluginMsgs,
			llmTools,
			onTool,
			1,          // maxRounds: decision is single-round
			"required", // must pick exactly one action
		)
		return chosenName, chosenArgs, err
	}
	var visLLM func([]plugin.Message) (string, error)
	if cfg.VisionModel != "" {
		visGW := infrallm.NewVisionGateway(cfg)
		visLLM = func(msgs []plugin.Message) (string, error) {
			return visGW.ChatSync(context.Background(), msgs)
		}
	}

	a.manager = plugin.NewManager(cfg)

	// Chat plugin.
	a.manager.Register(chat.NewPlugin())

	// Vision plugin — LLM injected before registration.
	visionPlugin := vision.NewPlugin().(*vision.VisionPlugin)
	visionPlugin.SetLLMSync(llmSync)
	a.manager.Register(visionPlugin)

	// Search plugin — web search via Bocha API.
	a.manager.Register(search.NewPlugin(cfg.BochaAPIKey))

	// QQ plugin — LLM and memory injected before registration.
	qqPlugin := qq.NewPlugin().(*qq.QQPlugin)
	qqPlugin.SetLLM(llmSync)

	// Memory plugin — all services injected before registration.
	memPlugin := memory.NewPlugin().(*memory.MemoryPlugin)
	memPlugin.SetStore(store)
	memPlugin.SetServices(&memory.Services{
		DB:           db,
		Store:        store,
		Profile:      profile,
		SelfModel:    selfModel,
		SessionBuf:   sessionBuf,
		Emotion:      emotionModel,
		Care:         careEngine,
		EmbSvc:       embSvc,
		MemLayers:    memLayers,
		Compressor:   compressor,
		DiaryStore:   diaryStore,
		Merger:       merger,
		EpisodeStore: episodeStore,
		TopicStore:   topicStore,
		Identity:     identityGraph,
		LLMSync:      llmSync,
		LLMWithTools: llmWithTools,
		VisionLLM:    visLLM,
		Scheduler:    schedulerInst,
	})

	// QQ needs memory plugin reference for ChatProcessor.
	qqPlugin.SetMemory(memPlugin)
	a.manager.Register(qqPlugin)
	a.manager.Register(memPlugin)
	a.MemPlugin = memPlugin
	memPlugin.SetBingSearchAPIKey(cfg.BingSearchAPIKey)
	memPlugin.SetBochaAPIKey(cfg.BochaAPIKey)

	// Proactive care messages → store for pet window polling.
	a.manager.EventBus().On("care:action", func(payload any) {
		action, ok := payload.(domain.CareAction)
		if !ok || action.Message == "" {
			return
		}
		source := string(action.Source)
		if source == "" {
			source = "care"
		}
		// Store for pet window polling (pet is a separate Wails process).
		proactiveMsgMu.Lock()
		proactiveMsg = action.Message
		proactiveMsgMu.Unlock()
		// Also emit to settings window chat panel (safe emit, ignores invalid ctx).
		a.emitSafe("chat:assistant", map[string]interface{}{
			"content": action.Message, "source": source, "observed": action.Observed,
		})
	})

	if err := a.manager.InitAll(ctx); err != nil {
		return fmt.Errorf("插件初始化失败: %w", err)
	}

	return nil
}

// ---- Plugin directory ----

func getPluginDir() string {
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".desktop-pet")
	}
	return ""
}

func apiBaseURL() string {
	if url := os.Getenv("PET_API_BASE_URL"); url != "" {
		return url
	}
	return "http://127.0.0.1:19840"
}

// ---- Wails-bound IPC methods (callable from frontend via window.go.main.App) ----

// Shutdown is called when the app is closing.
func (a *App) Shutdown(ctx context.Context) {
	if a.manager != nil {
		a.manager.Shutdown()
	}
	if a.store != nil {
		a.store.Close()
	}
}

// LearningOverview returns aggregated learning data for the settings dashboard.
func (a *App) LearningOverview() api.LearningOverview {
	lo := api.LearningOverview{}
	if a.MemPlugin == nil {
		return lo
	}
	acceptRate, totalToday, _, bySource, principles, threads, inquiries, _ := a.MemPlugin.LearningSnapshot()
	lo.Metrics = api.LearningMetrics{
		AcceptRatePct: acceptRate,
		TotalToday:    totalToday,
		TotalWeek:     totalToday,
		BySource:      bySource,
	}
	lo.PrinciplesCount = principles
	lo.ActiveThreads = threads
	lo.ActiveInquiries = inquiries
	if a.emotionModel != nil {
		p := a.emotionModel.Personality()
		lo.Personality = api.PersonalityModel{
			AnnoyanceSensitivity: p.AnnoyanceSensitivity,
			AffectionWarmth:      p.AffectionWarmth,
			WorryTendency:        p.WorryTendency,
		}
	}
	return lo
}

// applyWarmStart pre-loads personality and facts from config for a better first-run experience.
func (a *App) applyWarmStart(cfg *infracfg.GlobalConfig, em *emotion.EmotionModel, store domain.MemoryStore) {
	ws := cfg.WarmStart

	// Personality.
	if ws.Personality.AnnoyanceSensitivity > 0 || ws.Personality.AffectionWarmth > 0 {
		p := emotion.PersonalityScale{
			AnnoyanceSensitivity: 0.5,
			AffectionWarmth:      0.5,
			WorryTendency:        0.5,
		}
		if ws.Personality.AnnoyanceSensitivity > 0 {
			p.AnnoyanceSensitivity = ws.Personality.AnnoyanceSensitivity
		}
		if ws.Personality.AffectionWarmth > 0 {
			p.AffectionWarmth = ws.Personality.AffectionWarmth
		}
		if ws.Personality.WorryTendency > 0 {
			p.WorryTendency = ws.Personality.WorryTendency
		}
		em.SetPersonality(p)
	}

	// Known facts — pre-seed the memory system.
	for _, fact := range ws.KnownFacts {
		_ = store.SaveFact(fact, "warm_start")
	}
}
func (a *App) trackMouse(ctx context.Context) {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	activityTicker := time.NewTicker(60 * time.Second)
	defer activityTicker.Stop()
	var lastX, lastY float64
	var moved bool
	for {
		select {
		case <-ctx.Done():
			return
		case <-activityTicker.C:
			if moved {
				moved = false
				go func() {
					http.Post(apiBaseURL() + "/api/activity", "application/json", nil)
				}()
			}
		case <-ticker.C:
			gx, gy := native.GetGlobalMousePos()
			wx, wy := runtime.WindowGetPosition(ctx)
			_, wh := runtime.WindowGetSize(ctx)
			rx := gx - float64(wx)
			ry := float64(wy+wh) - gy
			if rx == lastX && ry == lastY {
				continue
			}
			lastX, lastY = rx, ry
			moved = true
			runtime.EventsEmit(ctx, "mouse:move", map[string]interface{}{
				"x": rx, "y": ry,
			})
		}
	}
}

// SendMessage proxies chat to the settings API so all chat context stays in one process.
func (a *App) SendMessage(text string) {
	a.lastChat = time.Now()
	go func() {
		payload, _ := json.Marshal(struct {
			Text string `json:"text"`
		}{Text: text})
		resp, err := http.Post(apiBaseURL() + "/api/chat/send", "application/json", bytes.NewReader(payload))
		if err != nil {
			runtime.EventsEmit(a.ctx, "chat:stream", "[错误] 无法连接到主进程")
			runtime.EventsEmit(a.ctx, "chat:sent", map[string]interface{}{})
			return
		}
		defer resp.Body.Close()
		var result struct {
			Content string `json:"content"`
			Source  string `json:"source,omitempty"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			runtime.EventsEmit(a.ctx, "chat:stream", "[错误] 响应解析失败")
			runtime.EventsEmit(a.ctx, "chat:sent", map[string]interface{}{})
			return
		}
		runtime.EventsEmit(a.ctx, "chat:stream", result.Content)
		runtime.EventsEmit(a.ctx, "chat:sent", map[string]interface{}{})
	}()
}

// Poke handles poke events from the frontend.


// DragWindow initiates a native macOS window drag (frameless window movement).
func (a *App) DragWindow() {
	native.PerformWindowDrag()
}

// ResizeWindow changes the window size.
func (a *App) ResizeWindow(w, h int) {
	runtime.WindowSetSize(a.ctx, w, h)
}

// Poke handles poke events from the frontend.
func (a *App) Poke(areas []string) {
	go func() {
		payload, _ := json.Marshal(struct {
			Text string `json:"text"`
		}{Text: "[戳了 " + strings.Join(areas, ",") + "]"})
		resp, err := http.Post(apiBaseURL() + "/api/poke", "application/json", bytes.NewReader(payload))
		if err != nil {
			return
		}
		if resp != nil {
			resp.Body.Close()
		}
	}()
}

// PlayExpression emits an event to switch the pet's facial expression.
func (a *App) PlayExpression(expressionId string) {
	runtime.EventsEmit(a.ctx, "pet:expression", expressionId)
}

// PlayMotion emits an event to play a motion animation on the pet.
func (a *App) PlayMotion(group string, index int) {
	runtime.EventsEmit(a.ctx, "pet:motion", map[string]interface{}{"group": group, "index": index})
}

// ShowBubble emits an event to display a chat bubble above the pet.
func (a *App) ShowBubble(text string, duration int) {
	runtime.EventsEmit(a.ctx, "pet:bubble", map[string]interface{}{"text": text, "duration": duration})
}

// HideBubble emits an event to hide the chat bubble.
func (a *App) HideBubble() {
	runtime.EventsEmit(a.ctx, "pet:hide_bubble", nil)
}

// ReportActivity receives user activity status from the frontend.
func (a *App) ReportActivity(isActive bool, mouseX int, mouseY int) {
	runtime.EventsEmit(a.ctx, "user:activity", map[string]interface{}{
		"active": isActive, "mouseX": mouseX, "mouseY": mouseY,
	})
}

// EmitChatHistory sends recent chat messages to the frontend.

// AnalyzeScreenshot receives a base64 screenshot from the frontend.
func (a *App) AnalyzeScreenshot(base64Data string, format string, question string) string {
	if a.manager == nil {
		return "[错误] 系统未初始化"
	}
	visionP, ok := a.manager.Plugin("vision").(domain.ScreenshotAnalyzer)
	if !ok || !visionP.IsRunning() {
		return "[错误] Vision插件未就绪"
	}
	if question == "" {
		question = "请分析这张截图"
	}
	imgBytes, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return "[错误] 图片解码失败: " + err.Error()
	}
	tmpDir := os.TempDir()
	tmpFile, err := os.CreateTemp(tmpDir, "sion-screenshot-*."+format)
	if err != nil {
		return "[错误] 创建临时文件失败: " + err.Error()
	}
	defer os.Remove(tmpFile.Name())
	if _, err := tmpFile.Write(imgBytes); err != nil {
		tmpFile.Close()
		return "[错误] 写入临时文件失败: " + err.Error()
	}
	tmpFile.Close()
	ocrText, ocrErr := native.OCRImage(tmpFile.Name())
	var result string
	if ocrErr == nil && strings.TrimSpace(ocrText) != "" {
		result, err = visionP.AnalyzeScreenshotText(ocrText, question)
		if err == nil {
			runtime.EventsEmit(a.ctx, "chat:sent", map[string]interface{}{
				"input": "[截图] " + question, "output": result,
			})
			return result
		}
	}
	result, err = visionP.AnalyzeScreenshotWithImage(base64Data, format, question)
	if err != nil {
		return "[错误] " + err.Error()
	}
	runtime.EventsEmit(a.ctx, "chat:sent", map[string]interface{}{
		"input": "[截图] " + question, "output": result,
	})
	return result
}
