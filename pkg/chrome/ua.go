package chrome

import (
	"runtime"
	"strings"
)

// UAOverride is Emulation.setUserAgentOverride params that match headed Chrome:
// reduced UA string plus UA-CH brands (so --user-agent does not empty userAgentData).
func UAOverride(ua, fullVersion string) map[string]any {
	major := fullVersion
	if i := strings.IndexByte(fullVersion, '.'); i > 0 {
		major = fullVersion[:i]
	}
	navPlatform, chPlatform, platformVersion, arch := "Win32", "Windows", "10.0.0", "x86"
	switch runtime.GOOS {
	case "darwin":
		navPlatform, chPlatform, platformVersion, arch = "MacIntel", "macOS", "15.0.0", "arm"
	case "linux":
		navPlatform, chPlatform, platformVersion, arch = "Linux x86_64", "Linux", "6.5.0", "x86"
	}
	brands := []map[string]any{
		{"brand": "Chromium", "version": major},
		{"brand": "Google Chrome", "version": major},
		{"brand": "Not-A.Brand", "version": "99"},
	}
	full := []map[string]any{
		{"brand": "Chromium", "version": fullVersion},
		{"brand": "Google Chrome", "version": fullVersion},
		{"brand": "Not-A.Brand", "version": "10.0.0.0"},
	}
	return map[string]any{
		"userAgent":      ua,
		"acceptLanguage": "en-US,en",
		"platform":       navPlatform,
		"userAgentMetadata": map[string]any{
			"brands":          brands,
			"fullVersionList": full,
			"fullVersion":     fullVersion,
			"platform":        chPlatform,
			"platformVersion": platformVersion,
			"architecture":    arch,
			"model":           "",
			"mobile":          false,
			"bitness":         "64",
			"wow64":           false,
		},
	}
}
