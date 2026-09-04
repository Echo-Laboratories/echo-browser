package chrome

import (
	"fmt"
	"runtime"
)

// LaunchArgs is the typed input to BuildArgs.
type LaunchArgs struct {
	Port        int
	UserDataDir string
	Headless    bool
	// Hidden is headed Chrome parked off-screen (more like a real window than --headless).
	Hidden      bool
	Width       int
	Height      int
	UserAgent   string // headless: Chrome UA without HeadlessChrome
	ScreenInfo  string // headless: --screen-info={WxH}, avoids 800x600
	ProxyServer string // already Chrome-ready, e.g. "http://127.0.0.1:1234"
	ProxyBypass string // --proxy-bypass-list, e.g. "<-loopback>"
	Extra       []string
	StartURL    string
	AllowOrigin string
}

// BuildArgs returns Chrome argv (no binary). Stock Chrome plus debugging attach
// and profile. Headless uses Chrome's new headless (real browser process), not
// the old HeadlessChrome binary mode.
func BuildArgs(a LaunchArgs) []string {
	start := a.StartURL
	if start == "" {
		start = "about:blank"
	}
	origin := a.AllowOrigin
	if origin == "" && a.Port > 0 {
		origin = fmt.Sprintf("http://127.0.0.1:%d", a.Port)
	}
	args := []string{
		fmt.Sprintf("--remote-debugging-port=%d", a.Port),
		"--remote-debugging-address=127.0.0.1",
		"--user-data-dir=" + a.UserDataDir,
		"--no-first-run",
		"--no-default-browser-check",
	}
	if origin != "" {
		args = append(args, "--remote-allow-origins="+origin)
	}
	w, h := a.Width, a.Height
	if a.Headless {
		if w <= 0 {
			w = 1920
		}
		if h <= 0 {
			h = 1080
		}
		args = append(args, "--headless=new")
		args = append(args, fmt.Sprintf("--window-size=%d,%d", w, h))
		if a.ScreenInfo != "" {
			args = append(args, "--screen-info="+a.ScreenInfo)
		} else {
			args = append(args, fmt.Sprintf("--screen-info={%dx%d}", w, h))
		}
		if a.UserAgent != "" {
			args = append(args, "--user-agent="+a.UserAgent)
		}
	} else {
		if w > 0 && h > 0 {
			args = append(args, fmt.Sprintf("--window-size=%d,%d", w, h))
		}
		if a.Hidden {
			args = append(args, "--window-position=-2400,-2400")
			if runtime.GOOS == "windows" {
				args = append(args, "--start-minimized")
			}
		}
	}
	if a.ProxyServer != "" {
		args = append(args, "--proxy-server="+a.ProxyServer)
		args = append(args, "--force-webrtc-ip-handling-policy=disable_non_proxied_udp")
		if a.ProxyBypass != "" {
			args = append(args, "--proxy-bypass-list="+a.ProxyBypass)
		}
	}
	if runtime.GOOS == "linux" {
		args = append(args, "--password-store=basic")
	}
	args = append(args, a.Extra...)
	args = append(args, start)
	return args
}
