//go:build !darwin

package native

import (
	"fmt"
	"time"
)

func ocrImage(imagePath string) (string, error) {
	return "", fmt.Errorf("ocr: not supported on this platform")
}

func ocrActiveScreen() (ScreenObservation, error) {
	return ScreenObservation{
		CapturedAt: time.Now(),
	}, fmt.Errorf("ocr: not supported on this platform")
}
