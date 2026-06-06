package native

import (
	"strings"
	"time"
)

// ScreenObservation captures a snapshot of what the user is doing on screen.
type ScreenObservation struct {
	AppName     string
	WindowTitle string
	OCRText     string // first 200 chars
	CapturedAt  time.Time
	IsWorking   bool
}

// FriendlyAppName maps a process name to a human-readable app name.
func FriendlyAppName(processName string) string {
	m := map[string]string{
		"Code": "VS Code", "Google Chrome": "Chrome", "iTerm2": "iTerm2",
		"com.apple.Terminal": "Terminal", "Xcode": "Xcode", "GoLand": "GoLand",
		"IntelliJ IDEA": "IntelliJ", "Sublime Text": "Sublime Text", "Android Studio": "Android Studio",
	}
	if name, ok := m[processName]; ok {
		return name
	}
	return processName
}

// IsSelfApp returns true if the process name belongs to the desktop pet itself.
func IsSelfApp(processName string) bool {
	selfApps := []string{"desktop-pet", "desktop-pet.app", "wails", "Sion"}
	for _, s := range selfApps {
		if strings.EqualFold(processName, s) {
			return true
		}
	}
	return false
}

// ClassifyActivity determines whether the current screen activity represents work.
func ClassifyActivity(appName, windowTitle, ocrText string) bool {
	lowerTitle := strings.ToLower(windowTitle)

	// First check non-work signals from window title — overrides app name.
	// Shopping, gaming, video, social media in any app → not working.
	nonWorkSignals := []string{
		"购物", "taobao", "淘宝", "jd.com", "京东", "amazon",
		"bilibili", "youtube", "netflix", "视频", "播放",
		"游戏", "game", "steam", "原神",
		"微博", "twitter", "微信", "朋友圈",
	}
	for _, s := range nonWorkSignals {
		if strings.Contains(lowerTitle, s) {
			return false
		}
	}

	workApps := []string{
		"Visual Studio Code", "Xcode", "GoLand", "IntelliJ",
		"Terminal", "iTerm2", "Warp",
		"Android Studio", "Sublime Text", "Vim", "Neovim", "Code",
	}

	for _, w := range workApps {
		if strings.Contains(appName, w) {
			return true
		}
	}

	if strings.Contains(appName, "Chrome") || strings.Contains(appName, "Safari") ||
		strings.Contains(appName, "Firefox") || strings.Contains(appName, "Edge") {
		devSites := []string{"github", "stackoverflow", "docs", "localhost", "jira", "confluence", "notion", "figma", "linear", "gitlab"}
		for _, s := range devSites {
			if strings.Contains(lowerTitle, s) {
				return true
			}
		}
	}

	return false
}
