//go:build linux

package chrome

import (
	"os/exec"
	"regexp"
	"strconv"
)

var x11Dimensions = regexp.MustCompile(`dimensions:\s+(\d+)x(\d+)`)
var xrandrCurrent = regexp.MustCompile(`current\s+(\d+)\s+x\s+(\d+)`)

// DisplaySize is the primary display in pixels. 0,0 if unknown.
func DisplaySize() (int, int) {
	if w, h := parseDisplayCmd("xdpyinfo", x11Dimensions); w > 0 {
		return w, h
	}
	return parseDisplayCmd("xrandr", xrandrCurrent)
}

func parseDisplayCmd(name string, re *regexp.Regexp) (int, int) {
	out, err := exec.Command(name).Output()
	if err != nil {
		return 0, 0
	}
	m := re.FindSubmatch(out)
	if len(m) < 3 {
		return 0, 0
	}
	w, err1 := strconv.Atoi(string(m[1]))
	h, err2 := strconv.Atoi(string(m[2]))
	if err1 != nil || err2 != nil || w <= 0 || h <= 0 {
		return 0, 0
	}
	return w, h
}
