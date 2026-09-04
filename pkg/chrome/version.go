package chrome

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
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
	if runtime.GOOS == "darwin" {
		if v := versionFromMacFramework(bin); v != "" {
			return v, nil
		}
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
	return maxNumericVersionDir(filepath.Dir(bin))
}

func versionFromMacFramework(bin string) string {
	// .../Google Chrome.app/Contents/MacOS/Google Chrome
	contents := filepath.Dir(filepath.Dir(bin))
	dir := filepath.Join(contents, "Frameworks", "Google Chrome Framework.framework", "Versions")
	return maxNumericVersionDir(dir)
}

func maxNumericVersionDir(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var best string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if name == "Current" {
			if target, err := os.Readlink(filepath.Join(dir, name)); err == nil {
				name = filepath.Base(target)
			} else {
				continue
			}
		}
		if !productVersionRe.MatchString(name) {
			continue
		}
		if best == "" || CompareProductVersion(name, best) > 0 {
			best = name
		}
	}
	return best
}

// CompareProductVersion compares a.b.c.d strings. Higher is newer.
// Non-numeric segments compare as 0. Empty is less than any version.
func CompareProductVersion(a, b string) int {
	as, bs := splitProductVersion(a), splitProductVersion(b)
	n := len(as)
	if len(bs) > n {
		n = len(bs)
	}
	for i := 0; i < n; i++ {
		var ai, bi int
		if i < len(as) {
			ai = as[i]
		}
		if i < len(bs) {
			bi = bs[i]
		}
		if ai < bi {
			return -1
		}
		if ai > bi {
			return 1
		}
	}
	return 0
}

func splitProductVersion(v string) []int {
	v = strings.TrimSpace(v)
	if i := strings.IndexByte(v, ' '); i > 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			out = append(out, 0)
			continue
		}
		out = append(out, n)
	}
	return out
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
