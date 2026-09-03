package chrome

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

var productVersionRe = regexp.MustCompile(`\b(\d+\.\d+\.\d+\.\d+)\b`)

// ProductVersion is the installed Chrome version (e.g. "152.0.7977.65").
// Prefers the versioned folder next to chrome.exe so a running Chrome does not
// swallow stdout with "Opening in existing browser session."
func ProductVersion(bin string) (string, error) {
	if v := versionFromInstallDir(bin); v != "" {
		return v, nil
	}
	out, err := exec.Command(bin, "--product-version").Output()
	if err != nil {
		return "", fmt.Errorf("chrome: product-version: %w", err)
	}
	v := ParseProductVersion(string(out))
	if v == "" {
		return "", fmt.Errorf("chrome: empty product-version in %q", strings.TrimSpace(string(out)))
	}
	return v, nil
}

func versionFromInstallDir(bin string) string {
	dir := filepath.Dir(bin)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var best string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if productVersionRe.MatchString(e.Name()) {
			if e.Name() > best {
				best = e.Name()
			}
		}
	}
	return best
}

// ParseProductVersion pulls a.b.c.d out of noisy chrome stdout.
func ParseProductVersion(stdout string) string {
	m := productVersionRe.FindStringSubmatch(stdout)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

// DesktopUA is the reduced UA Chrome itself sends in headed mode.
// Headless still injects "HeadlessChrome" unless this is passed as --user-agent.
func DesktopUA(productVersion string) string {
	major := productVersion
	if i := strings.IndexByte(productVersion, '.'); i > 0 {
		major = productVersion[:i]
	}
	reduced := major + ".0.0.0"
	switch runtime.GOOS {
	case "windows":
		return fmt.Sprintf("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%s Safari/537.36", reduced)
	case "darwin":
		return fmt.Sprintf("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%s Safari/537.36", reduced)
	default:
		return fmt.Sprintf("Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%s Safari/537.36", reduced)
	}
}
