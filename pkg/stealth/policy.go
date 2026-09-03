package stealth

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Action is the verdict for a CDP method.
type Action int

const (
	Allow Action = iota
	Deny
	RequireIsolatedContext
)

// Policy is the CDP allow/deny list. Default is strict: no Runtime.enable,
// no page-world evaluate, no Network/Fetch/DOM/Emulation/Overlay.
type Policy struct {
	// Permissive disables the allowlist (still records calls). For debugging only.
	Permissive bool
}

// ErrDenied is returned when a method is blocked.
type ErrDenied struct {
	Method string
	Reason string
}

func (e *ErrDenied) Error() string {
	return fmt.Sprintf("stealth: blocked %s: %s", e.Method, e.Reason)
}

var defaultActions = map[string]Action{
	"Runtime.enable":                        Deny,
	"Runtime.addBinding":                    Deny,
	"Page.addScriptToEvaluateOnNewDocument": Deny,
	"Page.addBinding":                       Deny,
	"Network.enable":                        Deny,
	"Network.setRequestInterception":        Deny,
	"Fetch.enable":                          Deny,
	"DOM.enable":                            Deny,
	"DOM.getDocument":                       Deny,
	"Overlay.enable":                        Deny,
	"Emulation.setDeviceMetricsOverride":    Deny,
	"Emulation.setUserAgentOverride":        Allow, // headless: strip HeadlessChrome, keep UA-CH brands
	"Emulation.setTimezoneOverride":         Deny,
	"Emulation.setLocaleOverride":           Deny,
	"Emulation.setVisibleSize":              Deny,
	"Input.insertText":                      Deny, // use dispatchKeyEvent instead
	"Runtime.evaluate":                      RequireIsolatedContext,
	"Runtime.callFunctionOn":                RequireIsolatedContext,
}

var defaultAllowPrefixes = []string{
	"Target.",
	"Browser.",
	"Page.",
	"Input.",
}

var defaultDenyPrefixes = []string{
	"Network.",
	"Fetch.",
	"Overlay.",
	"Emulation.",
	"HeapProfiler.",
	"Debugger.",
	"Profiler.",
}

// Allow returns an error if the CDP method must not be sent.
func (p *Policy) Allow(method string, params any) error {
	if p != nil && p.Permissive {
		return nil
	}
	if method == "" {
		return &ErrDenied{Method: method, Reason: "empty method"}
	}
	if action, ok := defaultActions[method]; ok {
		switch action {
		case Allow:
			return nil
		case Deny:
			return &ErrDenied{Method: method, Reason: "method is on the deny list"}
		case RequireIsolatedContext:
			if !hasIsolatedContext(params) {
				return &ErrDenied{Method: method, Reason: "page-world evaluate is forbidden; contextId required"}
			}
			return nil
		}
	}
	for _, pre := range defaultDenyPrefixes {
		if strings.HasPrefix(method, pre) {
			if _, ok := defaultActions[method]; !ok {
				return &ErrDenied{Method: method, Reason: "domain is off by default"}
			}
		}
	}
	if allowedPrefix(method) {
		return nil
	}
	// Unknown domains are denied so a new CDP call cannot silently leak.
	return &ErrDenied{Method: method, Reason: "method is not on the allow list"}
}

func allowedPrefix(method string) bool {
	for _, pre := range defaultAllowPrefixes {
		if strings.HasPrefix(method, pre) {
			return true
		}
	}
	return false
}

func hasIsolatedContext(params any) bool {
	if params == nil {
		return false
	}
	raw, err := json.Marshal(params)
	if err != nil {
		return false
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return false
	}
	v, ok := m["contextId"]
	if !ok || v == nil {
		return false
	}
	switch n := v.(type) {
	case float64:
		return n != 0
	case int:
		return n != 0
	case int64:
		return n != 0
	case json.Number:
		i, err := n.Int64()
		return err == nil && i != 0
	default:
		return false
	}
}

// ForbiddenLaunchFlags are Chrome switches that must not appear in a stealth launch.
var ForbiddenLaunchFlags = []string{
	"--enable-automation",
	"--disable-blink-features=AutomationControlled",
	"--disable-web-security",
	"--headless",
	"--headless=new",
	"--headless=old",
	"--remote-debugging-port=9222",
	"--remote-allow-origins=*",
}

// AuditLaunchArgs returns issues found in a Chrome argv (excluding the binary).
func AuditLaunchArgs(args []string, headlessOK bool) []string {
	var issues []string
	for _, a := range args {
		switch {
		case a == "--enable-automation":
			issues = append(issues, a)
		case a == "--disable-blink-features=AutomationControlled":
			issues = append(issues, a)
		case a == "--disable-web-security":
			issues = append(issues, a)
		case strings.HasPrefix(a, "--headless") && !headlessOK:
			issues = append(issues, a)
		case a == "--remote-debugging-port=9222":
			issues = append(issues, a)
		case a == "--remote-allow-origins=*":
			issues = append(issues, a)
		}
	}
	return issues
}
