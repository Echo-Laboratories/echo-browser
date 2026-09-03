package input

import (
	"context"
	"encoding/json"
	"math"
	"testing"
	"time"
)

type recCaller struct {
	methods []string
	params  []map[string]any
}

func (r *recCaller) Call(ctx context.Context, method string, params map[string]any) (json.RawMessage, error) {
	r.methods = append(r.methods, method)
	r.params = append(r.params, params)
	return json.RawMessage(`{}`), nil
}

func TestBezierPathReachesTarget(t *testing.T) {
	path := bezierPath(0, 0, 400, 300)
	if len(path) < 8 {
		t.Fatalf("path too short: %d", len(path))
	}
	last := path[len(path)-1]
	if math.Abs(last.x-400) > 0.01 || math.Abs(last.y-300) > 0.01 {
		t.Fatalf("end = %+v", last)
	}
}

func TestBezierPathLongerForDistance(t *testing.T) {
	short := bezierPath(0, 0, 10, 10)
	long := bezierPath(0, 0, 800, 600)
	if len(long) <= len(short) {
		t.Fatalf("short=%d long=%d", len(short), len(long))
	}
}

func TestClickEmitsMovePressRelease(t *testing.T) {
	rec := &recCaller{}
	m := NewMouse(rec, 0, 0)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := m.Click(ctx, 100, 80); err != nil {
		t.Fatal(err)
	}
	var moved, pressed, released int
	for _, p := range rec.params {
		switch p["type"] {
		case "mouseMoved":
			moved++
		case "mousePressed":
			pressed++
			if p["button"] != "left" || p["clickCount"] != 1 {
				t.Fatalf("press %+v", p)
			}
		case "mouseReleased":
			released++
		}
	}
	if moved < 2 {
		t.Fatalf("expected a path of mouseMoved, got %d", moved)
	}
	if pressed != 1 || released != 1 {
		t.Fatalf("pressed=%d released=%d", pressed, released)
	}
}

func TestTargetPointInsideBox(t *testing.T) {
	for i := 0; i < 50; i++ {
		x, y := TargetPoint(10, 20, 100, 40)
		if x < 10 || x > 110 || y < 20 || y > 60 {
			t.Fatalf("out of box: %f %f", x, y)
		}
	}
}
