package page

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeCaller struct {
	mu       sync.Mutex
	methods  []string
	params   []map[string]any
	handlers map[string][]func(json.RawMessage)
}

func newFake() *fakeCaller {
	return &fakeCaller{handlers: make(map[string][]func(json.RawMessage))}
}

func (f *fakeCaller) Call(ctx context.Context, method string, params map[string]any) (json.RawMessage, error) {
	f.mu.Lock()
	f.methods = append(f.methods, method)
	f.params = append(f.params, params)
	f.mu.Unlock()
	switch method {
	case "Page.enable", "Page.setLifecycleEventsEnabled", "Page.bringToFront":
		return json.RawMessage(`{}`), nil
	case "Page.getFrameTree":
		return json.RawMessage(`{"frameTree":{"frame":{"id":"f1"}}}`), nil
	case "Page.createIsolatedWorld":
		return json.RawMessage(`{"executionContextId":7}`), nil
	case "Page.getLayoutMetrics":
		return json.RawMessage(`{"cssVisualViewport":{"clientWidth":800,"clientHeight":600}}`), nil
	case "Page.navigate":
		go func() {
			time.Sleep(5 * time.Millisecond)
			f.emit("Page.lifecycleEvent", json.RawMessage(`{"name":"load","frameId":"f1","loaderId":"L1"}`))
		}()
		return json.RawMessage(`{"frameId":"f1","loaderId":"L1"}`), nil
	case "Runtime.evaluate":
		if params["contextId"] == nil || params["contextId"] == 0 {
			tPanic("evaluate without contextId")
		}
		expr, _ := params["expression"].(string)
		if strings.Contains(expr, "querySelector") {
			val, _ := json.Marshal(box{Found: true, Visible: true, X: 40, Y: 50, Width: 80, Height: 20})
			return json.RawMessage(`{"result":{"type":"object","value":` + string(val) + `}}`), nil
		}
		if strings.Contains(expr, "document.title") {
			return json.RawMessage(`{"result":{"type":"string","value":"Hello"}}`), nil
		}
		return json.RawMessage(`{"result":{"type":"string","value":""}}`), nil
	case "Input.dispatchMouseEvent", "Input.dispatchKeyEvent":
		return json.RawMessage(`{}`), nil
	default:
		return json.RawMessage(`{}`), nil
	}
}

func (f *fakeCaller) On(method string, fn func(json.RawMessage)) func() {
	f.mu.Lock()
	f.handlers[method] = append(f.handlers[method], fn)
	f.mu.Unlock()
	return func() {}
}

func (f *fakeCaller) emit(method string, params json.RawMessage) {
	f.mu.Lock()
	hs := append([]func(json.RawMessage){}, f.handlers[method]...)
	f.mu.Unlock()
	for _, h := range hs {
		h(params)
	}
}

func (f *fakeCaller) hasMethod(name string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, m := range f.methods {
		if m == name {
			return true
		}
	}
	return false
}

func tPanic(msg string) { panic(msg) }

func TestGotoWaitsForLoad(t *testing.T) {
	f := newFake()
	p := New(f, "echo_test")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := p.Goto(ctx, "https://example.com"); err != nil {
		t.Fatal(err)
	}
	if !f.hasMethod("Page.navigate") {
		t.Fatal("missing navigate")
	}
	if f.hasMethod("Network.enable") {
		t.Fatal("Network.enable must not be sent")
	}
}

func TestEvaluateAlwaysSendsContextID(t *testing.T) {
	f := newFake()
	p := New(f, "echo_test")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	title, err := p.Title(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if title != "Hello" {
		t.Fatalf("title=%q", title)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	found := false
	for i, m := range f.methods {
		if m == "Runtime.evaluate" {
			found = true
			if f.params[i]["contextId"] != 7 {
				t.Fatalf("contextId=%v", f.params[i]["contextId"])
			}
		}
	}
	if !found {
		t.Fatal("no evaluate")
	}
}

func TestClickUsesMouseNotJSClick(t *testing.T) {
	f := newFake()
	p := New(f, "echo_test")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := p.Locator("#btn").Click(ctx); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var pressed bool
	for i, m := range f.methods {
		if m == "Runtime.evaluate" {
			expr, _ := f.params[i]["expression"].(string)
			if strings.Contains(expr, ".click(") {
				t.Fatal("JS click is forbidden")
			}
		}
		if m == "Input.dispatchMouseEvent" && f.params[i]["type"] == "mousePressed" {
			pressed = true
		}
	}
	if !pressed {
		t.Fatal("expected mousePressed")
	}
}
