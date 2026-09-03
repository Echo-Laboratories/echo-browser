//go:build windows

package chrome

import "syscall"

var getSystemMetrics = syscall.NewLazyDLL("user32.dll").NewProc("GetSystemMetrics")

const (
	smCXScreen = 0
	smCYScreen = 1
)

// DisplaySize is the primary display in pixels. 0,0 if unknown.
func DisplaySize() (int, int) {
	cx, _, _ := getSystemMetrics.Call(smCXScreen)
	cy, _, _ := getSystemMetrics.Call(smCYScreen)
	if cx == 0 || cy == 0 {
		return 0, 0
	}
	return int(cx), int(cy)
}
