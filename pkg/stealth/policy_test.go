package stealth

import "testing"

func TestDenyRuntimeEnable(t *testing.T) {
	var p Policy
	if err := p.Allow("Runtime.enable", nil); err == nil {
		t.Fatal("expected deny")
	}
}

func TestEvaluateRequiresContextID(t *testing.T) {
	var p Policy
	if err := p.Allow("Runtime.evaluate", map[string]any{"expression": "1"}); err == nil {
		t.Fatal("expected deny without contextId")
	}
	if err := p.Allow("Runtime.evaluate", map[string]any{"expression": "1", "contextId": 0}); err == nil {
		t.Fatal("expected deny for contextId 0")
	}
	if err := p.Allow("Runtime.evaluate", map[string]any{"expression": "1", "contextId": 7}); err != nil {
		t.Fatal(err)
	}
}

func TestNetworkDenied(t *testing.T) {
	var p Policy
	if err := p.Allow("Network.enable", nil); err == nil {
		t.Fatal("expected deny")
	}
	if err := p.Allow("Network.getResponseBody", nil); err == nil {
		t.Fatal("expected deny")
	}
}

func TestUserAgentOverrideAllowed(t *testing.T) {
	var p Policy
	if err := p.Allow("Emulation.setUserAgentOverride", map[string]any{"userAgent": "x"}); err != nil {
		t.Fatal(err)
	}
	if err := p.Allow("Emulation.setDeviceMetricsOverride", nil); err == nil {
		t.Fatal("device metrics must stay denied")
	}
}

func TestAllowedDomains(t *testing.T) {
	var p Policy
	for _, m := range []string{
		"Target.setAutoAttach",
		"Page.enable",
		"Page.navigate",
		"Page.createIsolatedWorld",
		"Input.dispatchMouseEvent",
		"Input.dispatchKeyEvent",
		"Browser.close",
	} {
		if err := p.Allow(m, nil); err != nil {
			t.Fatalf("%s: %v", m, err)
		}
	}
}

func TestUnknownDenied(t *testing.T) {
	var p Policy
	if err := p.Allow("Accessibility.getFullAXTree", nil); err == nil {
		t.Fatal("expected deny")
	}
}

func TestInsertTextDenied(t *testing.T) {
	var p Policy
	if err := p.Allow("Input.insertText", map[string]any{"text": "x"}); err == nil {
		t.Fatal("expected deny")
	}
}

func TestPermissive(t *testing.T) {
	p := Policy{Permissive: true}
	if err := p.Allow("Runtime.enable", nil); err != nil {
		t.Fatal(err)
	}
}

func TestAuditLaunchArgs(t *testing.T) {
	issues := AuditLaunchArgs([]string{
		"--remote-debugging-port=9222",
		"--headless=new",
		"--enable-automation",
		"--remote-allow-origins=*",
	}, false)
	if len(issues) != 4 {
		t.Fatalf("issues = %v", issues)
	}
	clean := AuditLaunchArgs([]string{
		"--remote-debugging-port=54321",
		"--user-data-dir=/tmp/p",
		"--no-first-run",
		"--remote-allow-origins=http://127.0.0.1:54321",
	}, false)
	if len(clean) != 0 {
		t.Fatalf("clean issues = %v", clean)
	}
}
