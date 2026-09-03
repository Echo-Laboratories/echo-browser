package chrome

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Find locates a Google Chrome binary. ECHO_CHROME_PATH wins if set.
func Find() (string, error) {
	if p := os.Getenv("ECHO_CHROME_PATH"); p != "" {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
		return "", fmt.Errorf("chrome: ECHO_CHROME_PATH not a file: %s", p)
	}
	for _, c := range candidates() {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c, nil
		}
	}
	for _, name := range pathNames() {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("chrome: Google Chrome not found on %s; set ChromePath or ECHO_CHROME_PATH", runtime.GOOS)
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
		return []string{"chrome.exe", "chrome"}
	}
	return []string{"google-chrome-stable", "google-chrome", "chrome"}
}
