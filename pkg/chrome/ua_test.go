package chrome

import (
	"runtime"
	"testing"
)

func TestUAOverrideKeepsNavigatorPlatform(t *testing.T) {
	p := UAOverride("Mozilla/5.0", "152.0.7977.65")
	if runtime.GOOS == "windows" && p["platform"] != "Win32" {
		t.Fatalf("navigator.platform=%v", p["platform"])
	}
	meta := p["userAgentMetadata"].(map[string]any)
	if runtime.GOOS == "windows" && meta["platform"] != "Windows" {
		t.Fatalf("UA-CH platform=%v", meta["platform"])
	}
}
