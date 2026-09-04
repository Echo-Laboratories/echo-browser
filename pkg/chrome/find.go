package chrome

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Find locates a Google Chrome binary. ECHO_CHROME_PATH wins if set.
func Find() (string, error) {
	if p := os.Getenv("ECHO_CHROME_PATH"); p != "" {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
		return "", fmt.Errorf("chrome: ECHO_CHROME_PATH not a file: %s", p)
	}
	var best string
	var bestVer string
	for _, c := range candidates() {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			if rejectNonChrome(c) != "" {
				continue
			}
			ver, _ := ProductVersion(c)
			if best == "" || CompareProductVersion(ver, bestVer) > 0 {
				best, bestVer = c, ver
			}
		}
	}
	if best != "" {
		return best, nil
	}
	for _, name := range pathNames() {
		p, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		if rejectNonChrome(p) != "" {
			continue
		}
		return p, nil
	}
	return "", fmt.Errorf("chrome: Google Chrome not found on %s; set ChromePath or ECHO_CHROME_PATH", runtime.GOOS)
}

func rejectNonChrome(p string) string {
	low := strings.ToLower(filepath.ToSlash(p))
	switch {
	case strings.Contains(low, "headless-shell"):
		return "chrome-headless-shell"
	case strings.Contains(low, "chromium"):
		return "chromium"
	default:
		return ""
	}
}

func candidates() []string {
	switch runtime.GOOS {
	case "windows":
		return []string{
			filepath.Join(os.Getenv("ProgramFiles"), "Google", "Chrome", "Application", "chrome.exe"),
			filepath.Join(os.Getenv("ProgramFiles(x86)"), "Google", "Chrome", "Application", "chrome.exe"),
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Google", "Chrome", "Application", "chrome.exe"),
		}
	case "darwin":
		return []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			filepath.Join(os.Getenv("HOME"), "Applications", "Google Chrome.app", "Contents", "MacOS", "Google Chrome"),
		}
	default:
		return []string{
			"/usr/bin/google-chrome-stable",
			"/usr/bin/google-chrome",
			"/usr/bin/chrome",
			"/snap/bin/google-chrome",
		}
	}
}

func pathNames() []string {
	if runtime.GOOS == "windows" {
		return []string{"chrome.exe"}
	}
	return []string{"google-chrome-stable", "google-chrome"}
}
