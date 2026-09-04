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

func TestVersionFromInstallDirPicksNumericLatest(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"99.0.4844.84", "152.0.7977.65", "9.0.0.0"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	got := versionFromInstallDir(filepath.Join(dir, "chrome.exe"))
	if got != "152.0.7977.65" {
		t.Fatalf("got %q", got)
	}
}

func TestCompareProductVersion(t *testing.T) {
	if CompareProductVersion("152.0.7977.65", "99.0.4844.84") <= 0 {
		t.Fatal("152 should be newer than 99")
	}
	if CompareProductVersion("153.0.1.2", "152.0.7977.65") <= 0 {
		t.Fatal("153 should be newer than 152")
	}
	if CompareProductVersion("152.0.7977.65", "152.0.7977.65") != 0 {
		t.Fatal("equal")
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
	ua153 := DesktopUA("153.0.1.2")
	if !strings.Contains(ua153, "Chrome/153.0.0.0") {
		t.Fatal(ua153)
	}
	if strings.Contains(ua153, "HeadlessChrome") {
		t.Fatal(ua153)
	}
	switch runtime.GOOS {
	case "windows":
		if !strings.Contains(ua, "Windows NT 10.0") {
			t.Fatal(ua)
		}
	}
}
