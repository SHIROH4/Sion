//go:build !darwin

package native

import "fmt"

func getGlobalMousePos() (float64, float64)      { return 0, 0 }
func setWindowIgnoresMouseEvents(ignore bool)    {}
func performWindowDrag()                         {}
func getActiveWindowTitle() string               { return "" }
func getActiveWindowDetail() (string, string)    { return "", "" }
func getActiveWindowFrame() (int, int, int, int) { return 0, 0, 0, 0 }
func captureScreenToBase64() (string, error) {
	return "", fmt.Errorf("capture: not supported on this platform")
}
