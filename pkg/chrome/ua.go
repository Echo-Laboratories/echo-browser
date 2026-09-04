package chrome

import (
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

// UABrand is one Client Hints brand entry.
type UABrand struct {
	Brand   string `json:"brand"`
	Version string `json:"version"`
}

// UAHigh is navigator.userAgentData.getHighEntropyValues.
type UAHigh struct {
	Architecture    string    `json:"architecture"`
	Bitness         string    `json:"bitness"`
	Model           string    `json:"model"`
	PlatformVersion string    `json:"platformVersion"`
	Wow64           bool      `json:"wow64"`
	FullVersion     string    `json:"fullVersion"`
	FullVersionList []UABrand `json:"fullVersionList"`
	Brands          []UABrand `json:"brands"`
	Mobile          bool      `json:"mobile"`
	Platform        string    `json:"platform"`
}

// UALive is an isolated-world snapshot of UA + Client Hints.
type UALive struct {
	UserAgent  string    `json:"userAgent"`
	Platform   string    `json:"platform"`
	Language   string    `json:"language"`
	Languages  []string  `json:"languages"`
	Brands     []UABrand `json:"brands"`
	Mobile     bool      `json:"mobile"`
	CHPlatform string    `json:"chPlatform"`
	High       *UAHigh   `json:"high"`
}

// UALiveExpr reads UA and high-entropy Client Hints. Isolated world only.
const UALiveExpr = `(async () => {
  let high = null;
  try {
    if (navigator.userAgentData && navigator.userAgentData.getHighEntropyValues) {
      high = await navigator.userAgentData.getHighEntropyValues([
        "architecture", "bitness", "model", "platformVersion",
        "fullVersionList", "fullVersion", "wow64"
      ]);
    }
  } catch (e) {}
  const brands = [];
  try {
    if (navigator.userAgentData && navigator.userAgentData.brands) {
      for (const b of navigator.userAgentData.brands) {
        brands.push({brand: b.brand, version: b.version});
      }
    }
  } catch (e) {}
  return {
    userAgent: navigator.userAgent,
    platform: navigator.platform,
    language: navigator.language,
    languages: navigator.languages ? [...navigator.languages] : [],
    brands: brands,
    mobile: !!(navigator.userAgentData && navigator.userAgentData.mobile),
    chPlatform: navigator.userAgentData ? navigator.userAgentData.platform : "",
    high: high
  };
})()`

func hasHeadless(ua string, brands []UABrand) bool {
	if strings.Contains(strings.ToLower(ua), "headless") {
		return true
	}
	for _, b := range brands {
		if strings.Contains(strings.ToLower(b.Brand), "headless") {
			return true
		}
	}
	return false
}

func rewriteBrands(brands []UABrand) []any {
	out := make([]any, 0, len(brands))
	for _, b := range brands {
		brand := b.Brand
		if strings.EqualFold(brand, "HeadlessChrome") {
			brand = "Google Chrome"
		}
		out = append(out, map[string]any{"brand": brand, "version": b.Version})
	}
	return out
}

func liveBrands(live UALive) []UABrand {
	if live.High != nil && len(live.High.Brands) > 0 {
		return live.High.Brands
	}
	return live.Brands
}

func liveFullList(live UALive) []UABrand {
	if live.High != nil && len(live.High.FullVersionList) > 0 {
		return live.High.FullVersionList
	}
	return nil
}

func majorOf(fullVersion string) string {
	if i := strings.IndexByte(fullVersion, '.'); i > 0 {
		return fullVersion[:i]
	}
	return fullVersion
}

// greasedBrand is Chromium's version-seeded GREASE brand (not the Puppeteer
// "Not-A.Brand"/"99" triple). See user_agent_utils.cc.
func greasedBrand(major int) (brand, version string) {
	chars := []string{" ", "(", ":", "-", ".", "/", ")", ";", "=", "?", "_"}
	if major < 0 {
		major = 0
	}
	return "Not" + chars[major%10] + "A" + chars[(major/10)%10] + "Brand", "8"
}

var osPlatformVersion = sync.OnceValue(func() string {
	if runtime.GOOS == "darwin" {
		out, err := exec.Command("sw_vers", "-productVersion").Output()
		if err == nil {
			if v := strings.TrimSpace(string(out)); v != "" {
				return v
			}
		}
	}
	if runtime.GOOS == "windows" {
		return "15.0.0"
	}
	return "6.5.0"
})

func navigatorCH() (navPlatform, chPlatform, arch string) {
	navPlatform, chPlatform, arch = "Win32", "Windows", "x86"
	switch runtime.GOOS {
	case "darwin":
		navPlatform, chPlatform = "MacIntel", "macOS"
		arch = "x86"
		if runtime.GOARCH == "arm64" {
			arch = "arm"
		}
	case "linux":
		navPlatform, chPlatform, arch = "Linux x86_64", "Linux", "x86"
		if runtime.GOARCH == "arm64" {
			navPlatform, arch = "Linux aarch64", "arm"
		}
	}
	if runtime.GOOS == "windows" && runtime.GOARCH == "arm64" {
		arch = "arm"
	}
	return
}

func machineMetadata(ua, fullVersion string, live UALive) map[string]any {
	major := majorOf(fullVersion)
	maj, _ := strconv.Atoi(major)
	grease, greaseVer := greasedBrand(maj)
	nav, ch, arch := navigatorCH()
	if live.Platform != "" {
		nav = live.Platform
	}
	if live.CHPlatform != "" {
		ch = live.CHPlatform
	}
	if live.High != nil && live.High.Architecture != "" {
		arch = live.High.Architecture
	}
	bitness := "64"
	platformVersion := osPlatformVersion()
	fullVer := fullVersion
	wow64 := false
	if live.High != nil {
		if live.High.Bitness != "" {
			bitness = live.High.Bitness
		}
		if live.High.PlatformVersion != "" {
			platformVersion = live.High.PlatformVersion
		}
		if live.High.FullVersion != "" {
			fullVer = live.High.FullVersion
		}
		wow64 = live.High.Wow64
		if live.High.Platform != "" {
			ch = live.High.Platform
		}
	}
	brands := []any{
		map[string]any{"brand": grease, "version": greaseVer},
		map[string]any{"brand": "Chromium", "version": major},
		map[string]any{"brand": "Google Chrome", "version": major},
	}
	full := []any{
		map[string]any{"brand": grease, "version": "10.0.0.0"},
		map[string]any{"brand": "Chromium", "version": fullVer},
		map[string]any{"brand": "Google Chrome", "version": fullVer},
	}
	meta := map[string]any{
		"brands":          brands,
		"fullVersionList": full,
		"fullVersion":     fullVer,
		"platform":        ch,
		"platformVersion": platformVersion,
		"architecture":    arch,
		"model":           "",
		"mobile":          false,
		"bitness":         bitness,
		"wow64":           wow64,
	}
	params := map[string]any{
		"userAgent":         ua,
		"platform":          nav,
		"userAgentMetadata": meta,
	}
	if len(live.Languages) > 0 {
		params["acceptLanguage"] = strings.Join(live.Languages, ",")
	}
	return params
}

// UAOverrideFromLive builds Emulation.setUserAgentOverride params.
// skip is true when the live snapshot already has a headed Chrome UA and brands.
func UAOverrideFromLive(ua, fullVersion string, live UALive) (params map[string]any, skip bool) {
	brands := liveBrands(live)
	full := liveFullList(live)
	headless := hasHeadless(live.UserAgent, brands) || hasHeadless("", full)
	if !headless && len(brands) > 0 {
		return nil, true
	}
	if ua == "" {
		ua = strings.ReplaceAll(live.UserAgent, "HeadlessChrome", "Chrome")
	}
	params = machineMetadata(ua, fullVersion, live)
	meta := params["userAgentMetadata"].(map[string]any)
	if rewritten := rewriteBrands(brands); len(rewritten) > 0 {
		meta["brands"] = rewritten
	}
	if rewritten := rewriteBrands(full); len(rewritten) > 0 {
		meta["fullVersionList"] = rewritten
	}
	return params, false
}

// UAOverride is the fallback when a live snapshot is unavailable: UA string only.
func UAOverride(ua, fullVersion string) map[string]any {
	_ = fullVersion
	return map[string]any{"userAgent": ua}
}
