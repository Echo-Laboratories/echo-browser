package chrome

import (
	"strings"
	"testing"
)

func TestUAOverrideFallbackIsUserAgentOnly(t *testing.T) {
	p := UAOverride("Mozilla/5.0 Chrome/152.0.0.0", "152.0.7977.65")
	if p["userAgent"] != "Mozilla/5.0 Chrome/152.0.0.0" {
		t.Fatalf("%v", p)
	}
	if _, ok := p["userAgentMetadata"]; ok {
		t.Fatalf("fallback must not invent Client Hints: %v", p)
	}
	if _, ok := p["acceptLanguage"]; ok {
		t.Fatalf("must not hardcode locale: %v", p)
	}
}

func TestUAOverrideFromLiveSkipsWhenClean(t *testing.T) {
	live := UALive{
		UserAgent: "Mozilla/5.0 Chrome/152.0.0.0 Safari/537.36",
		Brands: []UABrand{
			{Brand: "Chromium", Version: "152"},
			{Brand: "Google Chrome", Version: "152"},
			{Brand: "Not:A-Brand", Version: "24"},
		},
	}
	_, skip := UAOverrideFromLive("Mozilla/5.0 Chrome/152.0.0.0 Safari/537.36", "152.0.7977.65", live)
	if !skip {
		t.Fatal("expected skip when live UA is already headed Chrome")
	}
}

func TestUAOverrideFromLiveRewritesHeadlessBrand(t *testing.T) {
	live := UALive{
		UserAgent: "Mozilla/5.0 HeadlessChrome/152.0.0.0 Safari/537.36",
		Platform:  "MacIntel",
		Languages: []string{"en-GB", "en"},
		Brands: []UABrand{
			{Brand: "Chromium", Version: "152"},
			{Brand: "HeadlessChrome", Version: "152"},
			{Brand: "Not:A-Brand", Version: "24"},
		},
		CHPlatform: "macOS",
		High: &UAHigh{
			Architecture:    "arm",
			Bitness:         "64",
			PlatformVersion: "15.3.0",
			Platform:        "macOS",
			FullVersion:     "152.0.7977.65",
			FullVersionList: []UABrand{
				{Brand: "Chromium", Version: "152.0.7977.65"},
				{Brand: "HeadlessChrome", Version: "152.0.7977.65"},
				{Brand: "Not:A-Brand", Version: "10.0.0.0"},
			},
		},
	}
	p, skip := UAOverrideFromLive("Mozilla/5.0 Chrome/152.0.0.0 Safari/537.36", "152.0.7977.65", live)
	if skip {
		t.Fatal("must override HeadlessChrome brands")
	}
	if p["acceptLanguage"] != "en-GB,en" {
		t.Fatalf("locale %v", p["acceptLanguage"])
	}
	if p["platform"] != "MacIntel" {
		t.Fatalf("platform %v", p["platform"])
	}
	meta := p["userAgentMetadata"].(map[string]any)
	if meta["architecture"] != "arm" || meta["platformVersion"] != "15.3.0" {
		t.Fatalf("meta %v", meta)
	}
	brands := meta["brands"].([]any)
	joined := ""
	for _, b := range brands {
		m := b.(map[string]any)
		joined += m["brand"].(string) + " "
		if m["brand"] == "HeadlessChrome" {
			t.Fatal("HeadlessChrome brand survived")
		}
	}
	if !strings.Contains(joined, "Google Chrome") {
		t.Fatalf("brands %s", joined)
	}
	if !strings.Contains(joined, "Not:A-Brand") {
		t.Fatalf("GREASE dropped: %s", joined)
	}
	raw := fmtMap(p)
	if strings.Contains(raw, "Not-A.Brand") && strings.Contains(raw, `"99"`) {
		t.Fatalf("puppeteer GREASE: %s", raw)
	}
}

func TestUAOverrideFromLiveEmptySnapshotFillsMachineCH(t *testing.T) {
	p, skip := UAOverrideFromLive("Mozilla/5.0 Chrome/152.0.0.0", "152.0.7977.65", UALive{
		UserAgent: "Mozilla/5.0 Chrome/152.0.0.0 Safari/537.36",
		Languages: []string{"en-US", "en"},
	})
	if skip {
		t.Fatal("empty brands must be filled so http pages have Client Hints")
	}
	meta := p["userAgentMetadata"].(map[string]any)
	brands := meta["brands"].([]any)
	joined := ""
	for _, b := range brands {
		m := b.(map[string]any)
		joined += m["brand"].(string) + "/" + m["version"].(string) + " "
		if m["brand"] == "Not-A.Brand" && m["version"] == "99" {
			t.Fatal("puppeteer GREASE")
		}
	}
	if !strings.Contains(joined, "Google Chrome/152") || !strings.Contains(joined, "Chromium/152") {
		t.Fatalf("brands %s", joined)
	}
	if strings.Contains(joined, "Not-A.Brand/99") {
		t.Fatalf("puppeteer GREASE: %s", joined)
	}
}

func TestGreasedBrandTracksMajor(t *testing.T) {
	b152, _ := greasedBrand(152)
	b153, _ := greasedBrand(153)
	if b152 == b153 {
		t.Fatalf("GREASE should change with major: %s", b152)
	}
	if b152 == "Not-A.Brand" {
		t.Fatal("152 GREASE should not be the puppeteer constant")
	}
}

func fmtMap(m map[string]any) string {
	return strings.Join(func() []string {
		var s []string
		for k, v := range m {
			s = append(s, k+":"+stringify(v))
		}
		return s
	}(), " ")
}

func stringify(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case map[string]any:
		return fmtMap(t)
	default:
		return ""
	}
}
