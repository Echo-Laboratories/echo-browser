package page

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/Echo-Laboratories/echo-browser/pkg/input"
)

// Caller is a stealth-gated CDP session for one page target.
type Caller interface {
	Call(ctx context.Context, method string, params map[string]any) (json.RawMessage, error)
	On(method string, fn func(params json.RawMessage)) func()
}

// Page is a tab. Reads go through an isolated world; writes go through Input.
type Page struct {
	call      Caller
	worldName string
	mouse     *input.Mouse
	keyboard  *input.Keyboard

	mu        sync.Mutex
	enabled   bool
	frameID   string
	contextID int
	chromeUA  string
	chromeVer string
}

// New wraps a page-target CDP session.
func New(call Caller, worldName string) *Page {
	adapt := callerAdapt{call}
	p := &Page{
		call:      call,
		worldName: worldName,
		mouse:     input.NewMouse(adapt, 12, 18),
		keyboard:  input.NewKeyboard(adapt),
	}
	return p
}

type callerAdapt struct{ c Caller }

func (a callerAdapt) Call(ctx context.Context, method string, params map[string]any) (json.RawMessage, error) {
	return a.c.Call(ctx, method, params)
}

// SetIdentity applies a headed-style UA + Client Hints on this page (headless).
func (p *Page) SetIdentity(ua, fullVersion string) {
	p.mu.Lock()
	p.chromeUA = ua
	p.chromeVer = fullVersion
	p.mu.Unlock()
}

// Mouse returns the pointer device (last position is preserved).
func (p *Page) Mouse() *input.Mouse { return p.mouse }

// Keyboard returns the key device.
func (p *Page) Keyboard() *input.Keyboard { return p.keyboard }

// Goto navigates and waits for the load lifecycle event.
func (p *Page) Goto(ctx context.Context, url string) error {
	ctx, cancel := withDefaultTimeout(ctx, 60*time.Second)
	defer cancel()
	if err := p.enable(ctx); err != nil {
		return err
	}

	type lc struct {
		Name     string `json:"name"`
		FrameID  string `json:"frameId"`
		LoaderID string `json:"loaderId"`
	}
	events := make(chan lc, 16)
	unsub := p.call.On("Page.lifecycleEvent", func(params json.RawMessage) {
		var e lc
		if json.Unmarshal(params, &e) == nil {
			select {
			case events <- e:
			default:
			}
		}
	})
	defer unsub()

	raw, err := p.call.Call(ctx, "Page.navigate", map[string]any{"url": url})
	if err != nil {
		return fmt.Errorf("page: navigate: %w", err)
	}
	var nav struct {
		FrameID   string `json:"frameId"`
		LoaderID  string `json:"loaderId"`
		ErrorText string `json:"errorText"`
	}
	if err := json.Unmarshal(raw, &nav); err != nil {
		return fmt.Errorf("page: navigate decode: %w", err)
	}
	if nav.ErrorText != "" {
		return fmt.Errorf("page: navigate: %s", nav.ErrorText)
	}
	p.mu.Lock()
	p.frameID = nav.FrameID
	p.contextID = 0
	p.mu.Unlock()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("page: goto %s: %w", url, ctx.Err())
		case e := <-events:
			if nav.LoaderID != "" && e.LoaderID != nav.LoaderID {
				continue
			}
			if e.Name == "load" {
				return nil
			}
		}
	}
}

// Evaluate runs JS in the isolated world (never the page world) and unmarshals
// the JSON value into dest. Dest may be nil.
func (p *Page) Evaluate(ctx context.Context, expression string, dest any) error {
	return p.evaluate(ctx, expression, dest)
}

// Title is document.title from the isolated world.
func (p *Page) Title(ctx context.Context) (string, error) {
	var title string
	if err := p.evaluate(ctx, "document.title", &title); err != nil {
		return "", err
	}
	return title, nil
}

// URL is location.href from the isolated world.
func (p *Page) URL(ctx context.Context) (string, error) {
	var u string
	if err := p.evaluate(ctx, "location.href", &u); err != nil {
		return "", err
	}
	return u, nil
}

// Wait sleeps. Prefer locator waits; this exists for Greenlight's YellowLight.
func (p *Page) Wait(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// YellowLight pauses for milliseconds (Greenlight alias).
func (p *Page) YellowLight(ms int) {
	_ = p.Wait(context.Background(), time.Duration(ms)*time.Millisecond)
}

// Locator finds elements by CSS selector (queried in the isolated world).
func (p *Page) Locator(selector string) *Locator {
	return &Locator{page: p, selector: selector}
}

// BringToFront focuses the tab before sending input.
func (p *Page) BringToFront(ctx context.Context) error {
	_, err := p.call.Call(ctx, "Page.bringToFront", nil)
	return err
}

type viewport struct {
	Width  float64
	Height float64
}

func (p *Page) viewportSize(ctx context.Context) (viewport, error) {
	raw, err := p.call.Call(ctx, "Page.getLayoutMetrics", nil)
	if err != nil {
		return viewport{}, err
	}
	var res struct {
		CSSVisualViewport struct {
			ClientWidth  float64 `json:"clientWidth"`
			ClientHeight float64 `json:"clientHeight"`
		} `json:"cssVisualViewport"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return viewport{}, err
	}
	return viewport{Width: res.CSSVisualViewport.ClientWidth, Height: res.CSSVisualViewport.ClientHeight}, nil
}

func withDefaultTimeout(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, d)
}
