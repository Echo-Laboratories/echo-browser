//go:build darwin

package chrome

import (
	"os/exec"
	"strconv"
	"strings"
)

// DisplaySize is the primary display in CSS pixels. 0,0 if unknown.
func DisplaySize() (int, int) {
	out, err := exec.Command("osascript", "-e", `tell application "Finder" to get bounds of window of desktop`).Output()
	if err != nil {
		return 0, 0
	}
	parts := strings.Split(strings.TrimSpace(string(out)), ",")
	if len(parts) != 4 {
		return 0, 0
	}
	x0, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	y0, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	x1, err3 := strconv.Atoi(strings.TrimSpace(parts[2]))
	y1, err4 := strconv.Atoi(strings.TrimSpace(parts[3]))
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
		return 0, 0
	}
	w, h := x1-x0, y1-y0
	if w <= 0 || h <= 0 {
		return 0, 0
	}
	return w, h
}
