package native

import (
	"os"
	"testing"
)

func TestOCRImage_NoFile(t *testing.T) {
	_, err := OCRImage("/nonexistent/path.png")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestOCRImage_EmptyFile(t *testing.T) {
	tmp, err := os.CreateTemp("", "ocr-test-*.png")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmp.Name())
	tmp.Close()

	_, err = OCRImage(tmp.Name())
	if err != nil {
		t.Log("OCR returned error for empty file (expected):", err)
	}
}

func TestGetActiveWindowDetail(t *testing.T) {
	appName, windowTitle := GetActiveWindowDetail()
	if appName == "" {
		t.Skip("no active window detected (headless CI?)")
	}
	t.Logf("Active window: %s — %s", appName, windowTitle)
}

func TestCaptureScreenToBase64(t *testing.T) {
	b64, err := CaptureScreenToBase64()
	if err != nil {
		t.Skip("screen capture failed (headless CI?):", err)
	}
	if len(b64) < 100 {
		t.Error("base64 screenshot too short, likely invalid")
	}
}

func TestFriendlyAppName(t *testing.T) {
	tests := []struct{ raw, want string }{
		{"Code", "VS Code"},
		{"com.apple.Terminal", "Terminal"},
		{"Google Chrome", "Chrome"},
		{"Xcode", "Xcode"},
		{"unknown-app", "unknown-app"},
	}
	for _, tt := range tests {
		got := FriendlyAppName(tt.raw)
		if got != tt.want {
			t.Errorf("FriendlyAppName(%q) = %q, want %q", tt.raw, got, tt.want)
		}
	}
}

func TestIsSelfApp(t *testing.T) {
	if IsSelfApp("com.github.wails.诗音") {
		t.Log("self-app detected correctly")
	}
	if IsSelfApp("com.google.Chrome") {
		t.Error("Chrome should not be detected as self-app")
	}
}
