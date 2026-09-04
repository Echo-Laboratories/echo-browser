package echo_test

import (
	"context"
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	echo "github.com/Echo-Laboratories/echo-browser"
	"github.com/Echo-Laboratories/echo-browser/pkg/chrome"
	"github.com/Echo-Laboratories/echo-browser/pkg/stealth"
)

func logChrome(t *testing.T) {
	t.Helper()
	bin, err := chrome.Find()
	if err != nil {
		t.Fatal(err)
	}
	ver, err := chrome.ProductVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("chrome %s (%s)", ver, bin)
	low := strings.ToLower(bin)
	if strings.Contains(low, "chromium") || strings.Contains(low, "headless-shell") {
		t.Fatalf("refusing non-Google Chrome binary %s", bin)
	}
}

func TestTrustedClickAndType(t *testing.T) {
	if os.Getenv("ECHO_E2E") == "" {
		t.Skip("set ECHO_E2E=1 to run Chrome e2e tests")
	}
	logChrome(t)

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
	logChrome(t)
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
	if snap["headlessBrand"] == true {
		t.Fatalf("userAgentData still has HeadlessChrome: %v", snap["uaData"])
	}
	// about:blank / data: URLs often have no Client Hints. Detectors run on http(s).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, `<!doctype html><title>fp</title><p>ok</p>`)
	}))
	t.Cleanup(srv.Close)
	if err := page.Goto(ctx, srv.URL); err != nil {
		t.Fatal(err)
	}
	var httpSnap map[string]any
	if err := page.Evaluate(ctx, stealth.FingerprintExpr, &httpSnap); err != nil {
		t.Fatal(err)
	}
	t.Logf("http %v", httpSnap["uaData"])
	if httpSnap["headlessUA"] == true || httpSnap["headlessBrand"] == true {
		t.Fatalf("HeadlessChrome on http page: %v", httpSnap)
	}
	uaData, _ := httpSnap["uaData"].(map[string]any)
	if uaData == nil {
		t.Fatalf("userAgentData missing on http page: %v", httpSnap)
	}
	brands, _ := uaData["brands"].([]any)
	if len(brands) == 0 {
		t.Fatalf("empty UA-CH brands on http page: %v", uaData)
	}
	if snap["webdriverOwn"] != nil {
		t.Fatalf("navigator.webdriver own property is a stealth-script tell: %v", snap["webdriverOwn"])
	}
	if snap["webdriverProto"] == nil {
		t.Fatal("expected Navigator.prototype.webdriver descriptor")
	}
	if snap["cdc"] != "undefined" || snap["playwrightBinding"] != "undefined" || snap["puppeteerEval"] != "undefined" {
		t.Fatalf("automation binding: cdc=%v pw=%v puppeteer=%v", snap["cdc"], snap["playwrightBinding"], snap["puppeteerEval"])
	}
}

func TestProxyHTTP(t *testing.T) {
	if os.Getenv("ECHO_E2E") == "" {
		t.Skip("set ECHO_E2E=1 to run Chrome e2e tests")
	}
	logChrome(t)
	runProxyE2E(t, false)
}

func TestProxyAuthHTTP(t *testing.T) {
	if os.Getenv("ECHO_E2E") == "" {
		t.Skip("set ECHO_E2E=1 to run Chrome e2e tests")
	}
	logChrome(t)
	runProxyE2E(t, true)
}

func runProxyE2E(t *testing.T, auth bool) {
	t.Helper()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, `<!doctype html><html><body><h1 id="ok">proxied</h1></body></html>`)
	}))
	t.Cleanup(origin.Close)

	var mu sync.Mutex
	var sawProxy atomic.Bool
	var sawAuth atomic.Bool
	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("user:pass"))

	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth {
			if r.Header.Get("Proxy-Authorization") != wantAuth {
				w.Header().Set("Proxy-Authenticate", `Basic realm="p"`)
				w.WriteHeader(http.StatusProxyAuthRequired)
				return
			}
			sawAuth.Store(true)
		}
		sawProxy.Store(true)
		if r.Method == http.MethodConnect {
			hj, ok := w.(http.Hijacker)
			if !ok {
				http.Error(w, "hijack", 500)
				return
			}
			client, _, err := hj.Hijack()
			if err != nil {
				return
			}
			defer client.Close()
			dest, err := net.Dial("tcp", r.Host)
			if err != nil {
				return
			}
			defer dest.Close()
			_, _ = io.WriteString(client, "HTTP/1.1 200 Connection Established\r\n\r\n")
			go func() { _, _ = io.Copy(dest, client) }()
			_, _ = io.Copy(client, dest)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		r.RequestURI = ""
		resp, err := http.DefaultTransport.RoundTrip(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		for k, vs := range resp.Header {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	}))
	t.Cleanup(proxy.Close)

	proxyURL := proxy.URL
	if auth {
		u := strings.TrimPrefix(proxy.URL, "http://")
		proxyURL = "http://user:pass@" + u
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	b, err := echo.Launch(ctx, echo.Options{
		Ephemeral: true,
		Headless:  true,
		Proxy:     proxyURL,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })

	page, err := b.Page(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := page.Goto(ctx, origin.URL+"/"); err != nil {
		t.Fatal(err)
	}
	text, err := page.Locator("#ok").InnerText(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if text != "proxied" {
		t.Fatalf("body %q", text)
	}
	if !sawProxy.Load() {
		t.Fatal("proxy did not see the request; Chrome probably bypassed loopback")
	}
	if auth && !sawAuth.Load() {
		t.Fatal("upstream did not receive Proxy-Authorization")
	}
}

func TestDetectRebrowser(t *testing.T) {
	if os.Getenv("ECHO_E2E_DETECT") == "" {
		t.Skip("set ECHO_E2E_DETECT=1 to hit live detector pages")
	}
	logChrome(t)
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
