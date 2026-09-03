package chrome

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestVersionFromInstallDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "152.0.7977.65"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "PlatformExperienceHelper"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := versionFromInstallDir(filepath.Join(dir, "chrome.exe"))
	if got != "152.0.7977.65" {
		t.Fatalf("got %q", got)
	}
}

func TestParseProductVersionIgnoresSessionNoise(t *testing.T) {
	got := ParseProductVersion("Opening in existing browser session.\r\n152.0.7977.65\r\n")
	if got != "152.0.7977.65" {
		t.Fatalf("got %q", got)
	}
	if ParseProductVersion("no version here") != "" {
		t.Fatal("expected empty")
	}
}

func TestDesktopUAReducedAndNoHeadless(t *testing.T) {
	ua := DesktopUA("152.0.7977.65")
	if strings.Contains(ua, "HeadlessChrome") {
		t.Fatal(ua)
	}
	if !strings.Contains(ua, "Chrome/152.0.0.0") {
		t.Fatal(ua)
	}
	switch runtime.GOOS {
	case "windows":
		if !strings.Contains(ua, "Windows NT 10.0") {
			t.Fatal(ua)
		}
	}
}
