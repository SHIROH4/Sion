//go:build darwin

package native

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>

static void nativeGetGlobalMousePos(double *x, double *y) {
	NSPoint loc = [NSEvent mouseLocation];
	*x = (double)loc.x;
	*y = (double)loc.y;
}

static void nativeSetIgnoresMouseEvents(int ignore) {
	dispatch_async(dispatch_get_main_queue(), ^{
		for (NSWindow *window in [NSApp windows]) {
			window.ignoresMouseEvents = (BOOL)ignore;
		}
	});
}

static void nativePerformWindowDrag(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		NSWindow *window = [[NSApp windows] firstObject];
		if (window) {
			[window performWindowDragWithEvent:[NSApp currentEvent]];
		}
	});
}
*/
import "C"

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

func getActiveWindowTitle() string {
	cmd := exec.Command("osascript", "-e",
		`tell application "System Events" to get name of first application process whose frontmost is true`)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func getActiveWindowDetail() (string, string) {
	cmd := exec.Command("osascript", "-e",
		`tell application "System Events"
set frontApp to first application process whose frontmost is true
return name of frontApp & "|||" & name of front window of frontApp
end tell`)
	out, err := cmd.Output()
	if err != nil {
		return "", ""
	}
	parts := strings.SplitN(strings.TrimSpace(string(out)), "|||", 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	if len(parts) == 1 {
		return strings.TrimSpace(parts[0]), ""
	}
	return "", ""
}

func getActiveWindowFrame() (int, int, int, int) {
	cmd := exec.Command("osascript",
		"-e", `tell application "System Events"`,
		"-e", `set fp to first application process whose frontmost is true`,
		"-e", `set fw to front window of fp`,
		"-e", `set wp to position of fw`,
		"-e", `set ws to size of fw`,
		"-e", `return ((item 1 of wp) as string) & "," & ((item 2 of wp) as string) & "," & ((item 1 of ws) as string) & "," & ((item 2 of ws) as string)`,
		"-e", `end tell`,
	)
	out, err := cmd.Output()
	if err != nil {
		return 0, 0, 0, 0
	}
	parts := strings.Split(strings.TrimSpace(string(out)), ",")
	if len(parts) != 4 {
		return 0, 0, 0, 0
	}
	x := parseCoord(parts[0])
	y := parseCoord(parts[1])
	w := parseCoord(parts[2])
	h := parseCoord(parts[3])
	return x, y, w, h
}

func parseCoord(s string) int {
	var n int
	fmt.Sscanf(strings.TrimSpace(s), "%d", &n)
	return n
}

func getGlobalMousePos() (float64, float64) {
	var x, y C.double
	C.nativeGetGlobalMousePos(&x, &y)
	return float64(x), float64(y)
}

func setWindowIgnoresMouseEvents(ignore bool) {
	if ignore {
		C.nativeSetIgnoresMouseEvents(1)
	} else {
		C.nativeSetIgnoresMouseEvents(0)
	}
}

func performWindowDrag() {
	C.nativePerformWindowDrag()
}

func captureScreenToBase64() (string, error) {
	tmpFile, err := os.CreateTemp("", "sion-screen-*.png")
	if err != nil {
		return "", fmt.Errorf("capture: temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "screencapture", "-x", "-t", "png", tmpPath)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("capture: screencapture: %w", err)
	}

	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return "", fmt.Errorf("capture: read: %w", err)
	}

	return base64.StdEncoding.EncodeToString(data), nil
}
