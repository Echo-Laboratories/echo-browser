package echo_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	echo "github.com/Echo-Laboratories/echo-browser"
	"github.com/Echo-Laboratories/echo-browser/pkg/stealth"
)

func TestTrustedClickAndType(t *testing.T) {
	if os.Getenv("ECHO_E2E") == "" {
		t.Skip("set ECHO_E2E=1 to run Chrome e2e tests")
	}

	dir, err := filepath.Abs(filepath.Join("testdata", "detectors"))
	if err != nil {
		t.Fatal(err)
	}
	fs := http.FileServer(http.Dir(dir))
	srv := httptest.NewServer(fs)
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	opts := echo.Options{Ephemeral: true, Headless: os.Getenv("CI") != ""}
	b, err := echo.Launch(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })

	page, err := b.Page(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := page.Goto(ctx, srv.URL+"/trusted-click.html"); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("#target").Click(ctx); err != nil {
		t.Fatal(err)
	}

	raw, err := page.Locator("#log").InnerText(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(raw)
	if !strings.Contains(raw, "isTrusted=true") {
		t.Fatalf("expected trusted click, log=%q", raw)
	}
	if !strings.Contains(raw, "webdriver=false") {
		t.Fatalf("expected navigator.webdriver false, log=%q", raw)
	}

	if err := page.Locator("#name").Fill(ctx, "echo"); err != nil {
		t.Fatal(err)
	}
	got, err := page.Locator("#name").InputValue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got != "echo" {
		t.Fatalf("typed %q", got)
	}
}

func TestHeadlessFingerprint(t *testing.T) {
	if os.Getenv("ECHO_E2E") == "" {
		t.Skip("set ECHO_E2E=1 to run Chrome e2e tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	b, err := echo.Launch(ctx, echo.Options{Ephemeral: true, Headless: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	page, err := b.Page(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var snap map[string]any
	if err := page.Evaluate(ctx, stealth.FingerprintExpr, &snap); err != nil {
		t.Fatal(err)
	}
	t.Logf("%v", snap)
	if snap["webdriver"] != false {
		t.Fatalf("webdriver=%v", snap["webdriver"])
	}
	if snap["headlessUA"] == true {
		t.Fatalf("userAgent still HeadlessChrome: %v", snap["userAgent"])
	}
	ua, _ := snap["userAgent"].(string)
	if strings.Contains(ua, "HeadlessChrome") {
		t.Fatalf("ua=%s", ua)
	}
	if !strings.Contains(ua, ".0.0.0 Safari/537.36") {
		t.Fatalf("ua should be reduced Chrome/N.0.0.0, got %s", ua)
	}
	if strings.Contains(ua, "Opening in existing") {
		t.Fatalf("product-version stdout leaked into UA: %s", ua)
	}
	sw, _ := snap["screenWidth"].(float64)
	sh, _ := snap["screenHeight"].(float64)
	if sw == 800 && sh == 600 {
		t.Fatalf("headless default 800x600 screen still in effect: %v x %v", sw, sh)
	}
}

func TestDetectRebrowser(t *testing.T) {
	if os.Getenv("ECHO_E2E_DETECT") == "" {
		t.Skip("set ECHO_E2E_DETECT=1 to hit live detector pages")
	}
	for _, headless := range []bool{false, true} {
		name := "headed"
		if headless {
			name = "headless"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			b, err := echo.Launch(ctx, echo.Options{Ephemeral: true, Headless: headless})
			if err != nil {
				t.Fatal(err)
			}
			defer b.Close()
			page, err := b.Page(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if err := page.Goto(ctx, "https://bot-detector.rebrowser.net/"); err != nil {
				t.Fatal(err)
			}
			if err := page.Wait(ctx, 4*time.Second); err != nil {
				t.Fatal(err)
			}
			var raw string
			if err := page.Evaluate(ctx, `document.querySelector('#detections-json') ? (document.querySelector('#detections-json').value || document.querySelector('#detections-json').innerText) : ''`, &raw); err != nil {
				t.Fatal(err)
			}
			t.Log(raw)
			if !strings.Contains(raw, "runtimeEnableLeak") {
				t.Fatalf("missing runtimeEnableLeak: %s", raw)
			}
			if strings.Contains(raw, `"No leak detected."`) == false {
				t.Fatal("runtimeEnableLeak did not pass")
			}
			if strings.Contains(raw, "No webdriver presented") == false {
				t.Fatal("navigator.webdriver presented")
			}
		})
	}
}
