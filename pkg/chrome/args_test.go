package chrome

import (
	"strings"
	"testing"

	"github.com/Echo-Laboratories/echo-browser/pkg/stealth"
)

func TestBuildArgsStealthDefault(t *testing.T) {
	args := BuildArgs(LaunchArgs{
		Port:        54321,
		UserDataDir: `C:\profiles\default`,
		AllowOrigin: "http://127.0.0.1:54321",
	})
	issues := stealth.AuditLaunchArgs(args, false)
	if len(issues) != 0 {
		t.Fatalf("stealth issues: %v\n%s", issues, strings.Join(args, "\n"))
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"--remote-debugging-port=54321",
		"--remote-debugging-address=127.0.0.1",
		"--no-first-run",
		"--remote-allow-origins=http://127.0.0.1:54321",
		"about:blank",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in %v", want, args)
		}
	}
	for _, bad := range []string{"--enable-automation", "--disable-blink-features", "--headless", "9222", "--remote-allow-origins=*"} {
		if strings.Contains(joined, bad) {
			t.Fatalf("unexpected %q in %v", bad, args)
		}
	}
}

func TestBuildArgsHeadlessOptIn(t *testing.T) {
	args := BuildArgs(LaunchArgs{Port: 9, UserDataDir: "/tmp/p", Headless: true})
	issues := stealth.AuditLaunchArgs(args, true)
	if len(issues) != 0 {
		t.Fatalf("%v", issues)
	}
	found := false
	size := false
	for _, a := range args {
		if a == "--headless=new" {
			found = true
		}
		if a == "--window-size=1920,1080" {
			size = true
		}
	}
	if !found {
		t.Fatal("expected --headless=new")
	}
	if !size {
		t.Fatal("expected realistic headless window-size")
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--screen-info={1920x1080}") {
		t.Fatalf("missing screen-info: %s", joined)
	}
}

func TestBuildArgsHeadlessUA(t *testing.T) {
	ua := DesktopUA("152.0.7977.65")
	args := BuildArgs(LaunchArgs{
		Port:        9,
		UserDataDir: "/tmp/p",
		Headless:    true,
		UserAgent:   ua,
		ScreenInfo:  "{2560x1440}",
	})
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "HeadlessChrome") {
		t.Fatal(joined)
	}
	if !strings.Contains(joined, "--user-agent="+ua) {
		t.Fatal(joined)
	}
	if !strings.Contains(joined, "--screen-info={2560x1440}") {
		t.Fatal(joined)
	}
}

func TestBuildArgsHiddenHeaded(t *testing.T) {
	args := BuildArgs(LaunchArgs{Port: 9, UserDataDir: "/tmp/p", Hidden: true})
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "--headless") {
		t.Fatal(joined)
	}
	if !strings.Contains(joined, "--window-position=-2400,-2400") {
		t.Fatal(joined)
	}
}

func TestExtraAutomationFlagRejected(t *testing.T) {
	args := BuildArgs(LaunchArgs{
		Port:        9,
		UserDataDir: "/tmp/p",
		Extra:       []string{"--enable-automation"},
	})
	issues := stealth.AuditLaunchArgs(args, false)
	if len(issues) == 0 {
		t.Fatal("expected --enable-automation to be flagged")
	}
}

func TestBuildArgsProxy(t *testing.T) {
	args := BuildArgs(LaunchArgs{
		Port:        1,
		UserDataDir: "/tmp/p",
		ProxyServer: "http://127.0.0.1:8080",
	})
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--proxy-server=http://127.0.0.1:8080") {
		t.Fatal(joined)
	}
	if !strings.Contains(joined, "--force-webrtc-ip-handling-policy=disable_non_proxied_udp") {
		t.Fatal(joined)
	}
}
