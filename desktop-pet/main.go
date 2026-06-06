package main

import (
	"context"
	"embed"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sync"

	"desktop-pet/internal/api"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	mode := "settings"
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}

	if mode == "pet" {
		startPetMode()
	} else {
		startSettingsMode()
	}
}

// ---- Settings Mode (main process) ----

func startSettingsMode() {
	app := NewSettingsApp()

	// Start HTTP API server early. Handlers are nil-safe — they return
	// empty/not-ready responses until Startup initialises the services.
	addr := os.Getenv("PET_API_LISTEN_ADDR")
	if addr == "" {
		addr = "127.0.0.1:19840"
	}
	go func() {
		log.Printf("API server listening on %s", addr)
		if err := api.StartServer(addr, buildHandlers(app)); err != nil {
			log.Println("API server error:", err)
		}
	}()

	err := wails.Run(&options.App{
		Title:     "诗音 · 设置",
		Width:     960,
		Height:    680,
		Frameless: false,
		AssetServer: &assetserver.Options{
			Assets: assets,
			Middleware: func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/" || r.URL.Path == "/index.html" {
						r.URL.Path = "/settings.html"
					}
					next.ServeHTTP(w, r)
				})
			},
		},
		BackgroundColour: &options.RGBA{R: 255, G: 255, B: 255, A: 255},
		DisableResize:    false,
		OnStartup:        app.startup,
		OnShutdown:       app.Shutdown,
		Mac: &mac.Options{
			WebviewIsTransparent: false,
		},
		Bind: []interface{}{app},
	})

	if err != nil {
		log.Println("Error:", err.Error())
	}
}

// proactiveMsg stores the latest proactive message for pet window polling.
var (
	proactiveMsg   string
	proactiveMsgMu sync.Mutex
)

// SettingsApp is the main process — hosts services, API server, and controls the pet window.
type SettingsApp struct {
	petCmd *exec.Cmd
	petMu  sync.Mutex
	petApp *App // pet's App for service access (set after domainReady)
	ready  bool
}

func NewSettingsApp() *SettingsApp {
	return &SettingsApp{}
}

// startup is called by Wails when the settings window is ready.
func (s *SettingsApp) startup(ctx context.Context) {
	s.petApp = NewApp()
	if err := s.petApp.domainReady(ctx); err != nil {
		log.Println("SettingsApp: domain init failed:", err)
		s.ready = false // API returns 503 until the user fixes config and restarts
		return
	}
	s.ready = true
	log.Println("SettingsApp: services ready")
}

// Shutdown cleans up.
func (s *SettingsApp) Shutdown(ctx context.Context) {
	s.StopPet()
	if s.petApp != nil && s.petApp.manager != nil {
		s.petApp.manager.Shutdown()
	}
}

// OpenPet spawns the desktop pet window as a child process.
func (s *SettingsApp) OpenPet() {
	s.petMu.Lock()
	defer s.petMu.Unlock()

	if s.petCmd != nil {
		return // already running
	}

	exe, err := os.Executable()
	if err != nil {
		log.Println("OpenPet: cannot find executable:", err)
		return
	}
	s.petCmd = exec.Command(exe, "pet")
	if err := s.petCmd.Start(); err != nil {
		log.Println("OpenPet: failed to start:", err)
		s.petCmd = nil
	}
}

// StopPet kills the pet child process.
func (s *SettingsApp) StopPet() {
	s.petMu.Lock()
	defer s.petMu.Unlock()

	if s.petCmd == nil || s.petCmd.Process == nil {
		return
	}
	s.petCmd.Process.Kill()
	s.petCmd.Wait()
	s.petCmd = nil
}

// IsPetRunning returns whether the pet window is currently open.
func (s *SettingsApp) IsPetRunning() bool {
	s.petMu.Lock()
	defer s.petMu.Unlock()
	return s.petCmd != nil && s.petCmd.Process != nil
}

// ---- Pet Mode (child process) ----

func startPetMode() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:     "桌宠",
		Width:     400,
		Height:    500,
		Frameless: true,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		AlwaysOnTop:       true,
		DisableResize:     true,
		BackgroundColour:  &options.RGBA{R: 0, G: 0, B: 0, A: 0},
		HideWindowOnClose: true,
		OnStartup:         app.Startup,
		OnShutdown:        app.Shutdown,
		Windows: &windows.Options{
			WebviewIsTransparent: true,
			WindowIsTranslucent:  true,
		},
		Mac: &mac.Options{
			WebviewIsTransparent: true,
		},
		Bind: []interface{}{app},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}

// ---- App: legacy pet methods (keep backward compat) ----

// OpenSettings spawns the settings window — used when pet is launched standalone.
func (a *App) OpenSettings() {
	exe, err := os.Executable()
	if err != nil {
		log.Println("OpenSettings: cannot find executable:", err)
		return
	}
	cmd := exec.Command(exe, "settings")
	if err := cmd.Start(); err != nil {
		log.Println("OpenSettings: failed to start:", err)
	}
}
