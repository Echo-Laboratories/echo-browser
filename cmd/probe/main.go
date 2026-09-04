// Probe public bot-detector pages with real Chrome (headed or headless).
//
//	go run ./cmd/probe
//	go run ./cmd/probe -headless
//	go run ./cmd/probe -hidden
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	echo "github.com/Echo-Laboratories/echo-browser"
	"github.com/Echo-Laboratories/echo-browser/pkg/chrome"
	"github.com/Echo-Laboratories/echo-browser/pkg/stealth"
)

type site struct {
	name   string
	url    string
	wait   time.Duration
	scrape string
}

func main() {
	headless := flag.Bool("headless", false, "use Chrome --headless=new")
	hidden := flag.Bool("hidden", false, "headed but off-screen")
	proxy := flag.String("proxy", "", "Chrome --proxy-server URL (user:pass rewritten locally)")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	bin, err := chrome.Find()
	if err != nil {
		fatal(err)
	}
	ver, err := chrome.ProductVersion(bin)
	if err != nil {
		fatal(err)
	}
	mode := "headed"
	if *headless {
		mode = "headless"
	} else if *hidden {
		mode = "hidden"
	}
	fmt.Printf("=== echo probe (%s) chrome %s (%s) ===\n", mode, ver, bin)
	if strings.Contains(strings.ToLower(bin), "chromium") || strings.Contains(strings.ToLower(bin), "headless-shell") {
		fatal(fmt.Errorf("refusing non-Google Chrome binary %s", bin))
	}

	b, err := echo.Launch(ctx, echo.Options{
		Headless:  *headless,
		Hidden:    *hidden,
		Ephemeral: true,
		Proxy:     *proxy,
	})
	if err != nil {
		fatal(err)
	}
	defer b.Close()

	page, err := b.Page(ctx)
	if err != nil {
		fatal(err)
	}

	var snap map[string]any
	if err := page.Evaluate(ctx, stealth.FingerprintExpr, &snap); err != nil {
		fatal(fmt.Errorf("fingerprint: %w", err))
	}
	printJSON("about:blank fingerprint", snap)
	if err := checkFingerprint(snap, *headless); err != nil {
		fatal(err)
	}

	sites := []site{
		{
			name:   "rebrowser",
			url:    "https://bot-detector.rebrowser.net/",
			wait:   4 * time.Second,
			scrape: `document.querySelector('#detections-json') ? document.querySelector('#detections-json').value || document.querySelector('#detections-json').innerText : document.body.innerText.slice(0, 4000)`,
		},
		{
			name: "sannysoft",
			url:  "https://bot.sannysoft.com/",
			wait: 3 * time.Second,
			scrape: `JSON.stringify([...document.querySelectorAll('table tr')].slice(0, 40).map(r => ({
  name: r.cells[0] ? r.cells[0].innerText.trim() : '',
  result: r.cells[1] ? r.cells[1].innerText.trim() : '',
  cls: r.cells[1] ? r.cells[1].className : ''
})))`,
		},
		{
			name:   "vastel-headless",
			url:    "https://arh.antoinevastel.com/bots/areyouheadless",
			wait:   3 * time.Second,
			scrape: `(document.querySelector('#res') && document.querySelector('#res').innerText) || document.body.innerText.slice(0, 2000)`,
		},
		{
			name: "intoli",
			url:  "https://intoli.com/blog/not-possible-to-block-chrome-headless/chrome-headless-test.html",
			wait: 3 * time.Second,
			scrape: `JSON.stringify([...document.querySelectorAll('table tr')].map(r => ({
  name: r.cells[0] ? r.cells[0].innerText.trim() : '',
  result: r.cells[1] ? r.cells[1].innerText.trim() : ''
})))`,
		},
		{
			name:   "brotector",
			url:    "https://kaliiiiiiiiii.github.io/brotector/",
			wait:   5 * time.Second,
			scrape: `document.body.innerText.slice(0, 4000)`,
		},
	}

	var failed []string
	for _, s := range sites {
		fmt.Printf("\n--- %s (%s) ---\n", s.name, s.url)
		if err := page.Goto(ctx, s.url); err != nil {
			fmt.Printf("goto error: %v\n", err)
			failed = append(failed, s.name+": goto")
			continue
		}
		if err := page.Wait(ctx, s.wait); err != nil {
			fmt.Printf("wait error: %v\n", err)
			failed = append(failed, s.name+": wait")
			continue
		}
		var out string
		if err := page.Evaluate(ctx, s.scrape, &out); err != nil {
			fmt.Printf("scrape error: %v\n", err)
			failed = append(failed, s.name+": scrape")
			continue
		}
		fmt.Println(pretty(out))
		if err := checkDetector(s.name, out); err != nil {
			fmt.Printf("FAIL: %v\n", err)
			failed = append(failed, err.Error())
		}
	}
	if len(failed) > 0 {
		fatal(fmt.Errorf("detector leaks: %s", strings.Join(failed, "; ")))
	}
	fmt.Println("\n=== probe passed ===")
}

func checkFingerprint(snap map[string]any, headless bool) error {
	if snap["webdriver"] != false {
		return fmt.Errorf("fingerprint: webdriver=%v", snap["webdriver"])
	}
	if snap["headlessUA"] == true {
		return fmt.Errorf("fingerprint: HeadlessChrome in UA: %v", snap["userAgent"])
	}
	if snap["headlessBrand"] == true {
		return fmt.Errorf("fingerprint: HeadlessChrome in UA-CH brands")
	}
	if snap["webdriverOwn"] != nil {
		return fmt.Errorf("fingerprint: navigator.webdriver own property %v", snap["webdriverOwn"])
	}
	if snap["cdc"] != "undefined" || snap["playwrightBinding"] != "undefined" || snap["puppeteerEval"] != "undefined" {
		return fmt.Errorf("fingerprint: automation binding")
	}
	ua, _ := snap["userAgent"].(string)
	if strings.Contains(ua, "HeadlessChrome") {
		return fmt.Errorf("fingerprint: ua %s", ua)
	}
	if !headless {
		ow, _ := snap["outerWidth"].(float64)
		oh, _ := snap["outerHeight"].(float64)
		if ow == 0 && oh == 0 {
			return fmt.Errorf("fingerprint: headed outer window is 0x0")
		}
	}
	return nil
}

func checkDetector(name, out string) error {
	if strings.Contains(out, "HeadlessChrome") {
		return fmt.Errorf("%s: HeadlessChrome visible", name)
	}
	if strings.Contains(out, "__playwright") || strings.Contains(out, "__puppeteer") || strings.Contains(out, "$cdc_") {
		return fmt.Errorf("%s: automation binding visible", name)
	}
	switch name {
	case "rebrowser":
		if !strings.Contains(out, "runtimeEnableLeak") {
			return fmt.Errorf("rebrowser: missing runtimeEnableLeak")
		}
		if !strings.Contains(out, `"No leak detected."`) {
			return fmt.Errorf("rebrowser: runtime.enable leak")
		}
		if !strings.Contains(out, "No webdriver presented") {
			return fmt.Errorf("rebrowser: navigator.webdriver presented")
		}
	case "sannysoft":
		if strings.Contains(strings.ToLower(out), `"name": "webdriver"`) && strings.Contains(out, `"failed"`) {
			return fmt.Errorf("sannysoft: webdriver failed")
		}
	case "intoli":
		low := strings.ToLower(out)
		if strings.Contains(low, "webdriver") && strings.Contains(low, `"result": "true"`) {
			return fmt.Errorf("intoli: webdriver true")
		}
	case "vastel-headless":
		if strings.Contains(strings.ToLower(out), "you are chrome headless") {
			return fmt.Errorf("vastel: flagged as headless")
		}
	}
	return nil
}

func pretty(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "(empty)"
	}
	var v any
	if json.Unmarshal([]byte(s), &v) == nil {
		b, err := json.MarshalIndent(v, "", "  ")
		if err == nil {
			return string(b)
		}
	}
	return s
}

func printJSON(title string, v any) {
	fmt.Printf("\n--- %s ---\n", title)
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Println(v)
		return
	}
	fmt.Println(string(b))
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
