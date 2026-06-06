// Package native provides platform-specific helpers (mouse tracking, window control)
// via CGo/Cocoa on Darwin.
package native

// GetGlobalMousePos returns the current mouse position in screen coordinates (bottom-left origin).
func GetGlobalMousePos() (float64, float64) {
	return getGlobalMousePos()
}

// SetWindowIgnoresMouseEvents makes the window ignore or accept mouse events.
func SetWindowIgnoresMouseEvents(ignore bool) {
	setWindowIgnoresMouseEvents(ignore)
}

// PerformWindowDrag initiates a native macOS window drag for frameless window movement.
func PerformWindowDrag() {
	performWindowDrag()
}

// GetActiveWindowTitle returns the title of the currently active window, or the
// frontmost application name if the window title is unavailable.
func GetActiveWindowTitle() string {
	return getActiveWindowTitle()
}

// GetActiveWindowDetail returns the frontmost application name and its active window title.
func GetActiveWindowDetail() (appName string, windowTitle string) {
	return getActiveWindowDetail()
}

// GetActiveWindowFrame returns the frame (x, y, width, height) of the frontmost window
// in screen coordinates. Returns zeros on failure.
func GetActiveWindowFrame() (x, y, w, h int) {
	return getActiveWindowFrame()
}

// CaptureScreenToBase64 captures the full screen as a PNG image and returns it
// as a base64-encoded string. Requires Screen Recording permission on macOS.
func CaptureScreenToBase64() (string, error) {
	return captureScreenToBase64()
}
