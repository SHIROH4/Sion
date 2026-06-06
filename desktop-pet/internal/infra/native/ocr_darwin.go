//go:build darwin

package native

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func ocrImage(imagePath string) (string, error) {
	helperPath := findOCRHelper()
	if helperPath == "" {
		return "", fmt.Errorf("ocr: helper not found")
	}

	cmd := exec.Command(helperPath, imagePath)
	cmd.Stderr = nil
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("ocr: %w", err)
	}
	return string(out), nil
}

func findOCRHelper() string {
	exe, err := os.Executable()
	candidates := []string{}
	if err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(exeDir, "ocr_helper"),
			filepath.Join(exeDir, "..", "..", "..", "..", "build", "ocr_helper"),
		)
	}
	candidates = append(candidates, "build/ocr_helper")

	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func ocrActiveScreen() (ScreenObservation, error) {
	appName, windowTitle := getActiveWindowDetail()
	x, y, w, h := getActiveWindowFrame()

	obs := ScreenObservation{
		AppName:     appName,
		WindowTitle: windowTitle,
		CapturedAt:  time.Now(),
		IsWorking:   ClassifyActivity(appName, windowTitle, ""),
	}

	if w == 0 || h == 0 {
		return obs, fmt.Errorf("ocr: invalid window frame (%d,%d,%d,%d)", x, y, w, h)
	}

	tmpFile, err := os.CreateTemp("", "sion-screen-ocr-*.png")
	if err != nil {
		return obs, fmt.Errorf("ocr: temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	frame := fmt.Sprintf("%d,%d,%d,%d", x, y, w, h)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, "screencapture", "-x", "-R"+frame, tmpPath).Run(); err != nil {
		return obs, fmt.Errorf("ocr: screencapture: %w", err)
	}

	text, err := ocrImage(tmpPath)
	if err != nil {
		return obs, fmt.Errorf("ocr: %w", err)
	}

	obs.OCRText = truncateText(strings.TrimSpace(text), 200)
	obs.IsWorking = ClassifyActivity(appName, windowTitle, obs.OCRText)
	return obs, nil
}

func truncateText(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen])
}
