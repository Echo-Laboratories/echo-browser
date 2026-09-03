package input

import (
	"context"
	"math"
	"math/rand/v2"
	"time"
)

// Mouse synthesizes trusted pointer events via Input.dispatchMouseEvent.
type Mouse struct {
	call Caller
	X    float64
	Y    float64
}

func NewMouse(call Caller, x, y float64) *Mouse {
	return &Mouse{call: call, X: x, Y: y}
}

type point struct{ x, y float64 }

// Move animates the pointer from the last position to (x, y) along a jittered
// cubic Bezier. Duration follows a Fitts's-law estimate.
func (m *Mouse) Move(ctx context.Context, x, y float64) error {
	path := bezierPath(m.X, m.Y, x, y)
	if len(path) == 0 {
		path = []point{{x, y}}
	}
	for _, p := range path {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := m.dispatch(ctx, "mouseMoved", p.x, p.y, "none", 0, 0); err != nil {
			return err
		}
		m.X, m.Y = p.x, p.y
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(16 * time.Millisecond):
		}
	}
	m.X, m.Y = x, y
	return nil
}

// Click moves to (x, y), hovers briefly, then left-presses and releases.
func (m *Mouse) Click(ctx context.Context, x, y float64) error {
	return m.clickN(ctx, x, y, 1)
}

// DblClick performs a two-count left click at (x, y).
func (m *Mouse) DblClick(ctx context.Context, x, y float64) error {
	return m.clickN(ctx, x, y, 2)
}

func (m *Mouse) clickN(ctx context.Context, x, y float64, n int) error {
	if err := m.Move(ctx, x, y); err != nil {
		return err
	}
	hover := 40 + time.Duration(rand.IntN(80))*time.Millisecond
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(hover):
	}
	for i := 1; i <= n; i++ {
		if err := m.dispatch(ctx, "mousePressed", x, y, "left", 1, i); err != nil {
			return err
		}
		hold := 40 + time.Duration(rand.IntN(80))*time.Millisecond
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(hold):
		}
		if err := m.dispatch(ctx, "mouseReleased", x, y, "left", 0, i); err != nil {
			return err
		}
		if i < n {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(40 * time.Millisecond):
			}
		}
	}
	return nil
}

// Wheel sends a mouseWheel event at the current pointer with CSS-pixel deltas.
func (m *Mouse) Wheel(ctx context.Context, deltaX, deltaY float64) error {
	return dispatch(ctx, m.call, "Input.dispatchMouseEvent", map[string]any{
		"type":        "mouseWheel",
		"x":           m.X,
		"y":           m.Y,
		"deltaX":      deltaX,
		"deltaY":      deltaY,
		"pointerType": "mouse",
	})
}

func (m *Mouse) dispatch(ctx context.Context, typ string, x, y float64, button string, buttons, clickCount int) error {
	params := map[string]any{
		"type":        typ,
		"x":           x,
		"y":           y,
		"button":      button,
		"buttons":     buttons,
		"clickCount":  clickCount,
		"pointerType": "mouse",
	}
	return dispatch(ctx, m.call, "Input.dispatchMouseEvent", params)
}

func bezierPath(x0, y0, x1, y1 float64) []point {
	dx := x1 - x0
	dy := y1 - y0
	dist := math.Hypot(dx, dy)
	if dist < 1 {
		return []point{{x1, y1}}
	}
	// Fitts's law: T = a + b * log2(D/W + 1). Width assumed ~12px.
	const a, b, width = 90.0, 170.0, 12.0
	ms := a + b*math.Log2(dist/width+1)
	if ms < 80 {
		ms = 80
	}
	if ms > 1200 {
		ms = 1200
	}
	steps := int(ms / 16)
	if steps < 8 {
		steps = 8
	}
	if steps > 80 {
		steps = 80
	}
	// Perpendicular offset for control points, 10–30% of distance.
	px, py := -dy/dist, dx/dist
	off := dist * (0.10 + rand.Float64()*0.20)
	if rand.IntN(2) == 0 {
		off = -off
	}
	cx1 := x0 + dx*0.3 + px*off
	cy1 := y0 + dy*0.3 + py*off
	cx2 := x0 + dx*0.7 + px*off*0.5
	cy2 := y0 + dy*0.7 + py*off*0.5
	out := make([]point, 0, steps)
	for i := 1; i <= steps; i++ {
		t := float64(i) / float64(steps)
		x := cubic(x0, cx1, cx2, x1, t)
		y := cubic(y0, cy1, cy2, y1, t)
		x += (rand.Float64() - 0.5)
		y += (rand.Float64() - 0.5)
		out = append(out, point{x, y})
	}
	out[len(out)-1] = point{x1, y1}
	return out
}

func cubic(p0, p1, p2, p3, t float64) float64 {
	u := 1 - t
	return u*u*u*p0 + 3*u*u*t*p1 + 3*u*t*t*p2 + t*t*t*p3
}

// TargetPoint picks a click point inside a box, slightly off-center.
func TargetPoint(x, y, w, h float64) (float64, float64) {
	if w < 2 {
		w = 2
	}
	if h < 2 {
		h = 2
	}
	tx := x + w*(0.35+rand.Float64()*0.30)
	ty := y + h*(0.35+rand.Float64()*0.30)
	return tx, ty
}
