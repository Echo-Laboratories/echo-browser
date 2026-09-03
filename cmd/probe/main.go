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
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	mode := "headed"
	if *headless {
		mode = "headless"
	} else if *hidden {
		mode = "hidden"
	}
	fmt.Printf("=== echo probe (%s) ===\n", mode)

	b, err := echo.Launch(ctx, echo.Options{
		Headless:  *headless,
		Hidden:    *hidden,
		Ephemeral: true,
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

	for _, s := range sites {
		fmt.Printf("\n--- %s (%s) ---\n", s.name, s.url)
		if err := page.Goto(ctx, s.url); err != nil {
			fmt.Printf("goto error: %v\n", err)
			continue
		}
		if err := page.Wait(ctx, s.wait); err != nil {
			fmt.Printf("wait error: %v\n", err)
			continue
		}
		var out string
		if err := page.Evaluate(ctx, s.scrape, &out); err != nil {
			fmt.Printf("scrape error: %v\n", err)
			continue
		}
		fmt.Println(pretty(out))
	}
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
