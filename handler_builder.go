package main

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"desktop-pet/internal/api"
	appchat "desktop-pet/internal/app/chat"
	"desktop-pet/internal/app/plugin"
	"desktop-pet/internal/domain"
	"desktop-pet/internal/infra"
	"desktop-pet/internal/service/cognition"
	infracfg "desktop-pet/internal/infra/config"
	infrallm "desktop-pet/internal/infra/llm"
	infrastorage "desktop-pet/internal/infra/storage"
)

// buildHandlers wires the SettingsApp's services into HTTP API handlers.
func buildHandlers(app *SettingsApp) *api.Handlers {
	sse := api.NewSSEBroker()
	bus := api.InitStatusBus(sse)
	bus.SetStoragePath(getPluginDir() + "/status_log.json")

	return &api.Handlers{
		IsReady: func() bool { return app.ready },
		SSE:     sse,

		GetConfig: func() *infracfg.GlobalConfig {
			return infracfg.Load()
		},
		SaveConfig: func(cfg *infracfg.GlobalConfig) error {
			return infracfg.Save(cfg)
		},

		ListPlugins: func() []api.PluginInfoViewModel {
			if app.petApp == nil || app.petApp.manager == nil {
				return nil
			}
			plugins := app.petApp.manager.Plugins()
			result := make([]api.PluginInfoViewModel, 0, len(plugins))
			for _, p := range plugins {
				info := p.Info()
				result = append(result, api.PluginInfoViewModel{
					Name: info.Name, Version: info.Version, Description: info.Description,
					Running: p.IsRunning(), Priority: info.Priority, Requires: info.Requires,
				})
			}
			return result
		},
		TogglePlugin: func(name string) error {
			if app.petApp == nil || app.petApp.manager == nil {
				return fmt.Errorf("plugin manager not ready")
			}
			p := app.petApp.manager.Plugin(name)
			if p == nil {
				return fmt.Errorf("plugin %q not found", name)
			}
			if p.IsRunning() {
				return p.Stop()
			}
			return p.Start()
		},
		ListPluginComponents: func() []api.PluginComponentViewModel {
			if app.petApp == nil || app.petApp.manager == nil {
				return nil
			}
			providers := app.petApp.manager.UIProviders()
			result := make([]api.PluginComponentViewModel, 0, len(providers))
			for _, p := range providers {
				// p is a plugin — get its name via the Plugin interface
				name := ""
				if pl, ok := p.(plugin.Plugin); ok {
					name = pl.Info().Name
				}
				result = append(result, api.PluginComponentViewModel{
					PluginName: name,
					Component:  p.SettingsComponent(),
					Defaults:   p.DefaultConfig(),
				})
			}
			return result
		},
		Memory: &api.MemoryHandlers{
			ListL0: func() []domain.Message {
				if app.petApp == nil || app.petApp.sessionBuf == nil {
					return nil
				}
				return app.petApp.sessionBuf.Recent(20)
			},
			ListFacts: func() []domain.FactEntry {
				if app.petApp == nil || app.petApp.store == nil {
					return nil
				}
				return app.petApp.store.ListActiveFacts(0)
			},
			ListDiaries: func() []domain.DiaryEntry {
				if app.petApp == nil || app.petApp.diaryRepo == nil {
					return nil
				}
				return app.petApp.diaryRepo.ListRecent(200)
			},
			DeleteFact: func(id int64) error {
				if app.petApp == nil || app.petApp.store == nil {
					return fmt.Errorf("store not available")
				}
				return app.petApp.store.ArchiveFact(id)
			},
			ListSelfProfiles: func() []string {
				if app.petApp == nil || app.petApp.store == nil {
					return nil
				}
				if s, ok := app.petApp.store.(*infrastorage.Store); ok {
					return s.ListSelfProfiles()
				}
				return nil
			},
			SelfModel: func() string {
				if app.petApp == nil || app.petApp.selfModel == nil {
					return ""
				}
				return app.petApp.selfModel.Current()
			},
		},
		Chat: &api.ChatHandlers{
			SendMessage: func(text string) (string, error) {
				if app.petApp == nil || app.petApp.manager == nil {
					return "", fmt.Errorf("not ready")
				}
				bus := api.StatusBusInstance()
				bus.EmitInfo("chat", "主人: "+infra.Truncate(text, 60))
				bus.EmitStart("chat", "生成回复中...")

				llmGW := app.petApp.LLMGW
				if llmGW == nil {
					return "", fmt.Errorf("llm gateway not available")
				}
				// Get all available tools from the chat plugin.
				var allTools []infrallm.Tool
				if cp, ok := app.petApp.manager.Plugin("chat").(interface{ FunctionTools() []infrallm.Tool }); ok {
					allTools = cp.FunctionTools()
				}

				// Execute a tool call via the chat plugin's function handlers.
				execTool := func(name, argsJSON string) string {
					if cp, ok := app.petApp.manager.Plugin("chat").(interface {
						ExecuteFunction(name, argsJSON string) string
					}); ok {
						return cp.ExecuteFunction(name, argsJSON)
					}
					return "工具不可用: " + name
				}

				// Pass all tools to the LLM with tool_choice="auto".
				// The LLM decides when to call which tool based on context.
				slog.Info("chat: tools", "count", len(allTools), "msg", text[:min(len(text), 40)])

				ctx, err := app.petApp.manager.ProcessChat(text, func(msgs []plugin.Message, onChunk func(string) error) error {
					if len(allTools) > 0 {
						result, e := llmGW.ChatSyncWithTools(
							context.Background(), msgs, allTools, execTool, 3, "auto",
						)
						if e != nil {
							return e
						}
						return onChunk(result)
					}
					result, e := llmGW.ChatSync(context.Background(), msgs)
					if e != nil {
						return e
					}
					return onChunk(result)
				})
				if err != nil {
					bus.EmitFail("chat", err.Error())
					return "", err
				}
				app.petApp.todayTokens += (len(text) + len(ctx.Output)) / 2
				sse.Publish("chat-message", `{"role":"user","content":"`+infra.EscapeJSON(text)+`"}`)
				sse.Publish("chat-message", `{"role":"assistant","content":"`+infra.EscapeJSON(ctx.Output)+`"}`)
				bus.EmitOK("chat", "诗音: "+infra.Truncate(ctx.Output, 60))
				return ctx.Output, nil
			},
			LoadHistory: func(limit int) []domain.Message {
				return appchat.LoadRecentHistory(app.petApp.store, limit)
			},
		},
		GetPluginConfig: func(name string) (map[string]interface{}, error) {
			cfg := infracfg.Load()
			return cfg.GetPluginConfig(name), nil
		},
		SavePluginConfig: func(name string, cfg map[string]interface{}) error {
			c := infracfg.Load()
			c.SetPluginConfig(name, cfg)
			return infracfg.Save(c)
		},
		GetStats: func() api.DashboardStats {
			stats := api.DashboardStats{}
			if app.petApp == nil {
				return stats
			}
			if app.petApp.store != nil {
				facts := app.petApp.store.ListActiveFacts(0)
				stats.L2FactCount = len(facts)
			}
			if s, ok := app.petApp.store.(*infrastorage.Store); ok {
				stats.TodayMessageCount = s.CountTodayMessages()
			}
			if app.petApp.diaryRepo != nil {
				stats.L1DiaryCount = app.petApp.diaryRepo.Count()
			}
			if app.petApp.manager != nil {
				for _, p := range app.petApp.manager.Plugins() {
					if p.IsRunning() {
						stats.ActivePlugins = append(stats.ActivePlugins, p.Info().Name)
					}
				}
			}
			if app.petApp.emotionModel != nil {
				es := app.petApp.emotionModel.Current()
				ev := app.petApp.emotionModel.CurrentVector()
				stats.Emotion = api.EmotionViewModel{
					Valence: es.Valence, Arousal: es.Arousal, Dominance: es.Dominance,
					Primary: es.Primary, Intensity: es.Intensity,
					Vector: api.EmotionVectorModel{
						Affection: ev.Affection, Worry: ev.Worry, Curiosity: ev.Curiosity,
						Sleepiness: ev.Sleepiness, Playfulness: ev.Playfulness,
						Loneliness: ev.Loneliness, Confidence: ev.Confidence, Annoyance: ev.Annoyance,
					},
				}
			}
			stats.ContinuousWorkMin = 0
			if app.petApp.store != nil {
				now := time.Now()
				today := now.Format("2006-01-02")
				saved := app.petApp.store.LoadProfileValue("work_date")
				if saved == today {
					prevMin, _ := strconv.Atoi(app.petApp.store.LoadProfileValue("work_min"))
					stats.ContinuousWorkMin = prevMin + int(now.Sub(app.petApp.startTime).Minutes())
				} else {
					stats.ContinuousWorkMin = int(now.Sub(app.petApp.startTime).Minutes())
				}
				app.petApp.store.SaveProfile("work_date", today)
				app.petApp.store.SaveProfile("work_min", strconv.Itoa(stats.ContinuousWorkMin))
			}
			stats.TodayTokens = app.petApp.todayTokens
			return stats
		},
		TestConnection: func(target string) error {
			cfg := infracfg.Load()
			var model, baseURL, apiKey string
			switch target {
			case "chat":
				model, baseURL, apiKey = cfg.LLMModel, cfg.LLMBaseURL, cfg.LLMAPIKey
			case "vision":
				model, baseURL, apiKey = cfg.VisionModel, cfg.VisionBaseURL, cfg.VisionAPIKey
				if baseURL == "" {
					baseURL = cfg.LLMBaseURL
				}
				if apiKey == "" {
					apiKey = cfg.LLMAPIKey
				}
			case "emotion":
				model, baseURL, apiKey = cfg.EmotionModel, cfg.EmotionBaseURL, cfg.EmotionAPIKey
				if model == "" {
					model = cfg.LLMModel
				}
				if baseURL == "" {
					baseURL = cfg.LLMBaseURL
				}
				if apiKey == "" {
					apiKey = cfg.LLMAPIKey
				}
			default:
				return fmt.Errorf("unknown target: %s", target)
			}
			gw := infrallm.NewGateway(&infracfg.GlobalConfig{
				LLMModel: model, LLMAPIKey: apiKey, LLMBaseURL: baseURL,
			})
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			_, err := gw.ChatSync(ctx, []plugin.Message{{Role: "user", Content: "hi"}})
			return err
		},
		NotifyActivity: func() {
			if app.petApp != nil && app.petApp.emotionModel != nil {
				app.petApp.emotionModel.NotifyActivity()
			}
		},
		PokeFunc: func(text string) {
			if app.petApp != nil && app.petApp.manager != nil {
				app.petApp.manager.Poke("settings", text)
			}
		},
		Identity: &api.IdentityHandlers{
			ListAll: func() []domain.IdentityNode {
				if app.petApp == nil || app.petApp.identityGraph == nil {
					return nil
				}
				return app.petApp.identityGraph.ListAll()
			},
			Upsert: func(node *domain.IdentityNode) error {
				if app.petApp == nil || app.petApp.identityGraph == nil {
					return fmt.Errorf("identity graph not available")
				}
				return app.petApp.identityGraph.Upsert(node)
			},
			SelfUpdate: func() error {
				if app.petApp == nil || app.petApp.MemPlugin == nil {
					return fmt.Errorf("memory plugin not available")
				}
				return app.petApp.MemPlugin.RunSelfUpdate()
			},
		},
		GetEmotion: func() api.EmotionViewModel {
			if app.petApp == nil || app.petApp.emotionModel == nil {
				return api.EmotionViewModel{}
			}
			es := app.petApp.emotionModel.Current()
			ev := app.petApp.emotionModel.CurrentVector()
			return api.EmotionViewModel{
				Valence: es.Valence, Arousal: es.Arousal, Dominance: es.Dominance,
				Primary: es.Primary, Intensity: es.Intensity,
				Vector: api.EmotionVectorModel{
					Affection: ev.Affection, Worry: ev.Worry, Curiosity: ev.Curiosity,
					Sleepiness: ev.Sleepiness, Playfulness: ev.Playfulness,
					Loneliness: ev.Loneliness, Confidence: ev.Confidence, Annoyance: ev.Annoyance,
				},
			}
		},
		GetLearningOverview: func() api.LearningOverview {
			lo := api.LearningOverview{}
			if app.petApp != nil {
				lo = app.petApp.LearningOverview()
			}
			return lo
		},
		GetFeatures: func() api.FeaturesViewModel {
			vm := api.FeaturesViewModel{}
			if app.petApp == nil || app.petApp.MemPlugin == nil {
				return vm
			}
			f := app.petApp.MemPlugin.CurrentFeatures()
			n := app.petApp.MemPlugin.CurrentNeeds()
			if f == nil {
				return vm
			}
			vm.ComputedAt = f.ComputedAt
			// User context.
			vm.User = api.UserContext{
				AppCategory: f.U1_AppCategory, WindowSubtype: f.U2_WindowSubtype,
				IsWorking: f.U3_IsWorking > 0, ContinuousWorkMin: f.U4_ContinuousWorkMins,
				AppSwitchCount: f.U5_AppSwitchCount, LengthTrend: f.U7_LengthTrend,
				EngagementNorm: f.U8_EngagementNorm, MealTime: f.U11_MealTime > 0,
				NightTime: f.U12_NightTime > 0, IsWeekend: f.U13_IsWeekend > 0,
				TimeSinceChatMin: f.U14_TimeSinceChatMins, FatigueMentionHrs: f.U15_FatigueMentionHrs,
				PrefDiversity: f.U16_PrefDiversity,
			}
			// Relationship.
			vm.Relationship = api.RelationshipContext{
				OverallAcceptRate: f.R1_OverallAcceptRate, SampleCount: int(f.R1_SampleCount),
				TimeWindowAccept: f.R2_TimeWindowAccept, SourceAcceptRate: f.R3_SourceAcceptRate,
				RecentRejections: int(f.R4_RecentRejections), RejectionSeverity: f.R4_RejectionSeverity,
				NeglectHours: f.R5_NeglectHours, DepthTrend: f.R6_DepthTrend,
				UserInitiative24h: f.R7_UserInitiative24h, IntimacyTrend: f.R8_IntimacyTrend,
			}
			// Needs.
			if n != nil {
				vm.Needs = api.NeedsContext{
					Companionship: n.Companionship, Rest: n.Rest, Play: n.Play,
					Curiosity: n.Curiosity, Care: n.Care, Autonomy: n.Autonomy,
				}
			}
			// Task.
			vm.Task = api.TaskContext{
				PrincipleCount: int(f.T1_PrincipleCount), PatternCount: int(f.T2_PatternCount),
				ReflexionLogSize: int(f.T3_ReflexionLogCount), TodayActivityCount: f.T5_TodayActivityCount,
				QuotaRemaining: f.E4_QuotaRemaining, CooldownNorm: f.E3_CooldownNorm,
				ReflectionDue: f.E7_ReflectionDue,
			}
			// Drives: compute from features + needs on the fly.
			var needsPtr *domain.IntrinsicNeeds
			if n != nil {
				needsCopy := *n
				needsPtr = &needsCopy
			}
			s, c, cur, q, e := cognition.ComputeDrives(f, needsPtr)
			vm.Drives = api.DriveScores{
				Social: s, Care: c, Curious: cur, Quiet: q, Explore: e,
			}
			return vm
		},
		ListStrategies: func() []api.StrategyViewModel {
			if app.petApp == nil || app.petApp.MemPlugin == nil {
				return nil
			}
			list := app.petApp.MemPlugin.ListStrategies()
			result := make([]api.StrategyViewModel, 0, len(list))
			for _, p := range list {
				result = append(result, api.StrategyViewModel{
					ID: p.ID, Situation: p.Situation,
					GoodStrategy: p.GoodStrategy, BadStrategy: p.BadStrategy,
					Reason: p.Reason, Confidence: p.Confidence,
					Source: p.Source, Active: p.Active,
				})
			}
			return result
		},
		ProactivePoll: func() string {
			proactiveMsgMu.Lock()
			msg := proactiveMsg
			proactiveMsg = ""
			proactiveMsgMu.Unlock()
			return msg
		},
	}
}

// detectChatIntent classifies a user message into: search, recall, memorize, chat.
