package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"desktop-pet/internal/infra/config"
)

// Handlers holds all dependencies for HTTP API handlers.
type Handlers struct {
	IsReady func() bool // returns true when services are initialised
	SSE     *SSEBroker

	GetConfig  func() *config.GlobalConfig
	SaveConfig func(*config.GlobalConfig) error

	ListPlugins         func() []PluginInfoViewModel
	TogglePlugin        func(name string) error
	GetPluginConfig     func(name string) (map[string]interface{}, error)
	SavePluginConfig    func(name string, cfg map[string]interface{}) error
	ListPluginComponents func() []PluginComponentViewModel

	Chat     *ChatHandlers
	Memory   *MemoryHandlers
	Identity *IdentityHandlers

	PokeFunc       func(text string)
	NotifyActivity func()
	TestConnection func(target string) error

	GetStats            func() DashboardStats
	GetEmotion          func() EmotionViewModel
	GetLearningOverview func() LearningOverview
	GetFeatures         func() FeaturesViewModel
	ListStrategies      func() []StrategyViewModel
	ProactivePoll       func() string
}

// Register wires all routes onto the given mux.
func (h *Handlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/config", h.handleConfig)
	mux.HandleFunc("/api/plugins", h.handlePlugins)
	mux.HandleFunc("/api/plugins/components", h.handlePluginComponents)
	mux.HandleFunc("/api/plugins/", h.handlePluginDispatch)
	mux.HandleFunc("/api/test-connection", h.handleTestConnection)
	mux.HandleFunc("/api/events", h.SSE.SSEHandler())
	mux.HandleFunc("/api/activity", h.handleActivity)
	mux.HandleFunc("/api/poke", h.handlePoke)
	mux.HandleFunc("/api/chat/send", h.handleChatSend)
	mux.HandleFunc("/api/chat/history", h.handleChatHistory)
	mux.HandleFunc("/api/stats", h.handleStats)
	mux.HandleFunc("/api/learning/overview", h.handleLearningOverview)
	mux.HandleFunc("/api/models", h.handleModels)
	mux.HandleFunc("/api/proactive/poll", h.handleProactivePoll)
	mux.HandleFunc("/api/memories", h.handleMemories)
	mux.HandleFunc("/api/memories/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			h.handleMemoryDelete(w, r)
		} else {
			h.handleMemoryDetail(w, r)
		}
	})
	mux.HandleFunc("/api/diaries", h.handleDiaries)
	mux.HandleFunc("/api/identity/self-update", h.handleIdentitySelfUpdate)
	mux.HandleFunc("/api/identity/", h.handleIdentity)
	mux.HandleFunc("/api/identity", h.handleIdentity)
	mux.HandleFunc("/api/emotion", h.handleEmotion)
	mux.HandleFunc("/api/features/current", h.handleFeaturesCurrent)
	mux.HandleFunc("/api/strategies", h.handleStrategies)
	mux.HandleFunc("/api/status/recent", func(w http.ResponseWriter, r *http.Request) {
		bus := StatusBusInstance()
		if bus == nil {
			writeJSON(w, 200, []StatusEvent{})
			return
		}
		writeJSON(w, 200, bus.Recent(30))
	})
}

// StartServer listens on addr and serves the registered routes with CORS support.
func StartServer(addr string, h *Handlers) error {
	mux := http.NewServeMux()
	h.Register(mux)
	return http.ListenAndServe(addr, withMiddleware(mux, h))
}

func withMiddleware(next http.Handler, h *Handlers) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// CORS: restrict to local origins (desktop app + dev server).
		origin := r.Header.Get("Origin")
		if origin == "" || strings.HasPrefix(origin, "http://127.0.0.1") ||
			strings.HasPrefix(origin, "http://localhost") ||
			strings.HasPrefix(origin, "wails://") {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// Check if services are ready (skip for config endpoints).
		if h.IsReady != nil && !h.IsReady() &&
			r.URL.Path != "/api/config" {
			writeError(w, http.StatusServiceUnavailable, "services not ready")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// ---- helpers ----

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// maskKey returns "***" if the key is non-empty, otherwise "".
func maskKey(key string) string {
	if key != "" {
		return "***"
	}
	return ""
}

// ---- config ----

type configResponse struct {
	LLMProvider    string   `json:"llm_provider"`
	LLMAPIKey      string   `json:"llm_api_key"`
	LLMModel       string   `json:"llm_model"`
	LLMBaseURL     string   `json:"llm_base_url"`
	VisionModel    string   `json:"vision_model"`
	VisionAPIKey   string   `json:"vision_api_key"`
	VisionBaseURL  string   `json:"vision_base_url"`
	EmotionModel   string   `json:"emotion_model"`
	EmotionAPIKey  string   `json:"emotion_api_key"`
	EmotionBaseURL string   `json:"emotion_base_url"`
	UserName       string   `json:"user_name"`
	UserTechStack  []string `json:"user_tech_stack"`
}

func configToResponse(cfg *config.GlobalConfig) configResponse {
	return configResponse{
		LLMProvider:    cfg.LLMProvider,
		LLMAPIKey:      maskKey(cfg.LLMAPIKey),
		LLMModel:       cfg.LLMModel,
		LLMBaseURL:     cfg.LLMBaseURL,
		VisionModel:    cfg.VisionModel,
		VisionAPIKey:   maskKey(cfg.VisionAPIKey),
		VisionBaseURL:  cfg.VisionBaseURL,
		EmotionModel:   cfg.EmotionModel,
		EmotionAPIKey:  maskKey(cfg.EmotionAPIKey),
		EmotionBaseURL: cfg.EmotionBaseURL,
		UserName:       cfg.UserName,
		UserTechStack:  cfg.UserTechStack,
	}
}

func (h *Handlers) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if h.GetConfig == nil {
			writeError(w, http.StatusNotImplemented, "config not wired")
			return
		}
		writeJSON(w, http.StatusOK, configToResponse(h.GetConfig()))

	case http.MethodPost:
		if h.SaveConfig == nil {
			writeError(w, http.StatusNotImplemented, "config save not wired")
			return
		}
		var req configResponse
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}

		current := h.GetConfig()

		current.LLMProvider = req.LLMProvider
		if req.LLMAPIKey != "***" {
			current.LLMAPIKey = req.LLMAPIKey
		}
		current.LLMModel = req.LLMModel
		current.LLMBaseURL = req.LLMBaseURL
		current.VisionModel = req.VisionModel
		if req.VisionAPIKey != "***" {
			current.VisionAPIKey = req.VisionAPIKey
		}
		current.VisionBaseURL = req.VisionBaseURL
		current.EmotionModel = req.EmotionModel
		if req.EmotionAPIKey != "***" {
			current.EmotionAPIKey = req.EmotionAPIKey
		}
		current.EmotionBaseURL = req.EmotionBaseURL
		current.UserName = req.UserName
		current.UserTechStack = req.UserTechStack

		if err := h.SaveConfig(current); err != nil {
			writeError(w, http.StatusInternalServerError, "save failed: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, configToResponse(current))

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// ---- test-connection ----

type testConnectionRequest struct {
	Target string `json:"target"` // "chat", "vision", or "emotion"
}

func (h *Handlers) handleTestConnection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req testConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if h.TestConnection == nil {
		writeError(w, http.StatusNotImplemented, "test-connection not wired")
		return
	}
	if err := h.TestConnection(req.Target); err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

// ---- activity ----

func (h *Handlers) handleActivity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h.NotifyActivity != nil {
		h.NotifyActivity()
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ---- poke ----

type pokeRequest struct {
	Text string `json:"text"`
}

func (h *Handlers) handlePoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req pokeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if h.PokeFunc != nil {
		h.PokeFunc(req.Text)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ---- plugins ----

func (h *Handlers) handlePlugins(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h.ListPlugins == nil {
		writeError(w, http.StatusNotImplemented, "plugins not wired")
		return
	}
	writeJSON(w, http.StatusOK, h.ListPlugins())
}

func (h *Handlers) handlePluginToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/api/plugins/")
	name = strings.TrimSuffix(name, "/toggle")
	if name == "" {
		writeError(w, http.StatusBadRequest, "plugin name required")
		return
	}
	if h.TogglePlugin == nil {
		writeError(w, http.StatusNotImplemented, "plugin toggle not wired")
		return
	}
	if err := h.TogglePlugin(name); err != nil {
		writeError(w, http.StatusInternalServerError, "toggle failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handlers) handlePluginComponents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h.ListPluginComponents == nil {
		writeError(w, http.StatusNotImplemented, "plugin components not wired")
		return
	}
	writeJSON(w, http.StatusOK, h.ListPluginComponents())
}

func (h *Handlers) handlePluginDispatch(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/plugins/")

	// PATCH /api/plugins/{name}/toggle
	if strings.HasSuffix(path, "/toggle") {
		h.handlePluginToggle(w, r)
		return
	}

	// GET/POST /api/plugins/{name} (config)
	name := path
	if name == "" {
		writeError(w, http.StatusBadRequest, "plugin name required")
		return
	}

	switch r.Method {
	case http.MethodGet:
		if h.GetPluginConfig == nil {
			writeError(w, http.StatusNotImplemented, "plugin config not wired")
			return
		}
		cfg, err := h.GetPluginConfig(name)
		if err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, cfg)

	case http.MethodPost:
		if h.SavePluginConfig == nil {
			writeError(w, http.StatusNotImplemented, "plugin config save not wired")
			return
		}
		var cfg map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		if err := h.SavePluginConfig(name, cfg); err != nil {
			writeError(w, http.StatusInternalServerError, "save failed: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// ---- stats ----

func (h *Handlers) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h.GetStats == nil {
		writeError(w, http.StatusNotImplemented, "stats not wired")
		return
	}
	writeJSON(w, http.StatusOK, h.GetStats())
}

// ---- learning overview ----

func (h *Handlers) handleLearningOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h.GetLearningOverview == nil {
		writeError(w, http.StatusNotImplemented, "learning overview not wired")
		return
	}
	writeJSON(w, http.StatusOK, h.GetLearningOverview())
}

// ---- emotion ----

func (h *Handlers) handleEmotion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h.GetEmotion == nil {
		writeError(w, http.StatusNotImplemented, "emotion not wired")
		return
	}
	writeJSON(w, http.StatusOK, h.GetEmotion())
}

// ---- features ----

func (h *Handlers) handleFeaturesCurrent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h.GetFeatures == nil {
		writeError(w, http.StatusNotImplemented, "features not wired")
		return
	}
	writeJSON(w, http.StatusOK, h.GetFeatures())
}

// ---- strategies ----

func (h *Handlers) handleStrategies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h.ListStrategies == nil {
		writeError(w, http.StatusNotImplemented, "strategies not wired")
		return
	}
	writeJSON(w, http.StatusOK, h.ListStrategies())
}
