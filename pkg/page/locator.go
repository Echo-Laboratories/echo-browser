package page

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/Echo-Laboratories/echo-browser/pkg/input"
)

const defaultWait = 30 * time.Second

// Locator is a CSS selector bound to a page. Actions wait until the node exists.
type Locator struct {
	page     *Page
	selector string
}

type box struct {
	Found   bool    `json:"found"`
	Visible bool    `json:"visible"`
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	Width   float64 `json:"width"`
	Height  float64 `json:"height"`
}

const boxBody = `
const el = document.querySelector(sel);
if (!el) return {found: false, visible: false, x:0, y:0, width:0, height:0};
const r = el.getBoundingClientRect();
const style = window.getComputedStyle(el);
const visible = r.width > 0 && r.height > 0 && style.visibility !== 'hidden' && style.display !== 'none' && parseFloat(style.opacity || '1') > 0;
return {found: true, visible, x: r.x, y: r.y, width: r.width, height: r.height};
`

func (l *Locator) queryBox(ctx context.Context) (box, error) {
	var b box
	err := l.page.evaluate(ctx, boundExpr(l.selector, boxBody), &b)
	if err == errNotFound {
		return box{}, nil
	}
	return b, err
}

func (l *Locator) waitVisible(ctx context.Context) (box, error) {
	ctx, cancel := withDefaultTimeout(ctx, defaultWait)
	defer cancel()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		b, err := l.queryBox(ctx)
		if err != nil && err != errNotFound {
			if ctx.Err() != nil {
				return box{}, fmt.Errorf("locator %s: %w", l.selector, ctx.Err())
			}
			// Isolated world may not be ready during navigation; retry.
		}
		if err == nil && b.Found && b.Visible {
			return b, nil
		}
		select {
		case <-ctx.Done():
			return box{}, fmt.Errorf("locator %s: waiting for visible: %w", l.selector, ctx.Err())
		case <-tick.C:
		}
	}
}

func (l *Locator) scrollIntoView(ctx context.Context, b box) (box, error) {
	vp, err := l.page.viewportSize(ctx)
	if err != nil || vp.Height == 0 {
		return b, nil
	}
	inView := func(bb box) bool {
		cy := bb.Y + bb.Height/2
		cx := bb.X + bb.Width/2
		return cy >= 0 && cy <= vp.Height && cx >= 0 && cx <= vp.Width
	}
	if inView(b) {
		return b, nil
	}
	if err := l.page.BringToFront(ctx); err != nil {
		return b, err
	}
	_ = l.page.mouse.Move(ctx, vp.Width/2, vp.Height/2)
	for i := 0; i < 16; i++ {
		cy := b.Y + b.Height/2
		delta := cy - vp.Height/2
		step := 140.0
		if delta < 0 {
			step = -140
		}
		if math.Abs(delta) < 140 {
			step = delta
		}
		if err := l.page.mouse.Wheel(ctx, 0, step); err != nil {
			return b, err
		}
		select {
		case <-ctx.Done():
			return b, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
		nb, err := l.queryBox(ctx)
		if err != nil {
			return b, err
		}
		b = nb
		if inView(b) {
			return b, nil
		}
	}
	return b, nil
}

func (l *Locator) clickAt(ctx context.Context, n int) error {
	b, err := l.waitVisible(ctx)
	if err != nil {
		return err
	}
	b, err = l.scrollIntoView(ctx, b)
	if err != nil {
		return err
	}
	if err := l.page.BringToFront(ctx); err != nil {
		return err
	}
	x, y := input.TargetPoint(b.X, b.Y, b.Width, b.Height)
	if n == 2 {
		return l.page.mouse.DblClick(ctx, x, y)
	}
	return l.page.mouse.Click(ctx, x, y)
}

// Click waits, scrolls into view, then performs a trusted mouse click.
func (l *Locator) Click(ctx context.Context) error {
	return l.clickAt(ctx, 1)
}

// DblClick is a trusted double-click.
func (l *Locator) DblClick(ctx context.Context) error {
	return l.clickAt(ctx, 2)
}

// Hover moves the pointer onto the element without pressing.
func (l *Locator) Hover(ctx context.Context) error {
	b, err := l.waitVisible(ctx)
	if err != nil {
		return err
	}
	b, err = l.scrollIntoView(ctx, b)
	if err != nil {
		return err
	}
	if err := l.page.BringToFront(ctx); err != nil {
		return err
	}
	x, y := input.TargetPoint(b.X, b.Y, b.Width, b.Height)
	return l.page.mouse.Move(ctx, x, y)
}

// Fill clicks the field, selects all, and types value.
func (l *Locator) Fill(ctx context.Context, value string) error {
	if err := l.Click(ctx); err != nil {
		return err
	}
	if err := l.page.keyboard.SelectAll(ctx); err != nil {
		return err
	}
	if err := l.page.keyboard.Press(ctx, "Backspace"); err != nil {
		return err
	}
	return l.page.keyboard.Type(ctx, value, input.Typing{WPM: 220})
}

// Type types into the element after clicking it.
func (l *Locator) Type(ctx context.Context, text string, spec input.Typing) error {
	if err := l.Click(ctx); err != nil {
		return err
	}
	return l.page.keyboard.Type(ctx, text, spec)
}

// TypeSequentially is Greenlight-compatible (delayMs between keys).
func (l *Locator) TypeSequentially(ctx context.Context, text string, delayMs int) error {
	return l.Type(ctx, text, input.Typing{WPM: input.DelayToWPM(delayMs)})
}

// TypeWithMistakes types with a low neighboring-key error rate.
func (l *Locator) TypeWithMistakes(ctx context.Context, text string, delayMs int) error {
	return l.Type(ctx, text, input.Typing{WPM: input.DelayToWPM(delayMs), Mistakes: true})
}

// Press clicks the element then taps a named key (Enter, Tab, …).
func (l *Locator) Press(ctx context.Context, key string) error {
	if err := l.Click(ctx); err != nil {
		return err
	}
	return l.page.keyboard.Press(ctx, key)
}

// InputValue returns the current value of an input or textarea.
func (l *Locator) InputValue(ctx context.Context) (string, error) {
	if _, err := l.waitVisible(ctx); err != nil {
		return "", err
	}
	var v string
	body := `
const el = document.querySelector(sel);
if (!el) return null;
return el.value;
`
	if err := l.page.evaluate(ctx, boundExpr(l.selector, body), &v); err != nil {
		return "", err
	}
	return v, nil
}

// InnerText waits for the element and returns its innerText.
func (l *Locator) InnerText(ctx context.Context) (string, error) {
	if _, err := l.waitVisible(ctx); err != nil {
		return "", err
	}
	var text string
	body := `
const el = document.querySelector(sel);
if (!el) return null;
return el.innerText;
`
	if err := l.page.evaluate(ctx, boundExpr(l.selector, body), &text); err != nil {
		return "", err
	}
	return text, nil
}

// GetAttribute returns an attribute, or "" if missing.
func (l *Locator) GetAttribute(ctx context.Context, name string) (string, error) {
	if _, err := l.waitVisible(ctx); err != nil {
		return "", err
	}
	rawName, _ := json.Marshal(name)
	body := fmt.Sprintf(`
const el = document.querySelector(sel);
if (!el) return null;
const v = el.getAttribute(%s);
return v === null ? "" : v;
`, rawName)
	var v string
	if err := l.page.evaluate(ctx, boundExpr(l.selector, body), &v); err != nil {
		return "", err
	}
	return v, nil
}

// Count returns how many nodes match (no wait).
func (l *Locator) Count(ctx context.Context) (int, error) {
	var n int
	body := `return document.querySelectorAll(sel).length;`
	if err := l.page.evaluate(ctx, boundExpr(l.selector, body), &n); err != nil {
		return 0, err
	}
	return n, nil
}

// IsVisible is true when the first match exists and has a non-empty box.
func (l *Locator) IsVisible(ctx context.Context) (bool, error) {
	b, err := l.queryBox(ctx)
	if err != nil {
		return false, err
	}
	return b.Found && b.Visible, nil
}

// Check clicks a checkbox if it is not already checked.
func (l *Locator) Check(ctx context.Context) error {
	checked, err := l.isChecked(ctx)
	if err != nil {
		return err
	}
	if checked {
		return nil
	}
	return l.Click(ctx)
}

func (l *Locator) isChecked(ctx context.Context) (bool, error) {
	if _, err := l.waitVisible(ctx); err != nil {
		return false, err
	}
	var v bool
	body := `
const el = document.querySelector(sel);
if (!el) return false;
return !!el.checked;
`
	if err := l.page.evaluate(ctx, boundExpr(l.selector, body), &v); err != nil {
		return false, err
	}
	return v, nil
}

// Select chooses an option on a native <select> by visible text or value, via typing.
func (l *Locator) Select(ctx context.Context, value string) error {
	if err := l.Click(ctx); err != nil {
		return err
	}
	return l.page.keyboard.Type(ctx, value, input.Typing{WPM: 240})
}
