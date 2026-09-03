package input

import (
	"context"
	"testing"
	"time"
)

func TestEventsForLetter(t *testing.T) {
	ev := eventsForRune('a')
	if len(ev) != 3 {
		t.Fatalf("len=%d", len(ev))
	}
	if ev[0].Type != "keyDown" || ev[1].Type != "char" || ev[2].Type != "keyUp" {
		t.Fatalf("types %+v", ev)
	}
	if ev[1].Text != "a" || ev[0].Code != "KeyA" || ev[0].WindowsVirtualKeyCode != 65 {
		t.Fatalf("%+v", ev)
	}
	if ev[0].Modifiers != 0 {
		t.Fatalf("modifiers %d", ev[0].Modifiers)
	}
}

func TestEventsForShiftLetter(t *testing.T) {
	ev := eventsForRune('A')
	if ev[0].Modifiers != ModShift {
		t.Fatalf("modifiers %d", ev[0].Modifiers)
	}
	if ev[1].Text != "A" {
		t.Fatalf("text %s", ev[1].Text)
	}
}

func TestBackspaceNoChar(t *testing.T) {
	ev, err := eventsForNamed("Backspace")
	if err != nil {
		t.Fatal(err)
	}
	if len(ev) != 2 {
		t.Fatalf("len=%d", len(ev))
	}
	if ev[0].WindowsVirtualKeyCode != 8 || ev[0].Type != "keyDown" {
		t.Fatalf("%+v", ev[0])
	}
}

func TestTypeEmitsPerCharacter(t *testing.T) {
	rec := &recCaller{}
	k := NewKeyboard(rec)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := k.Type(ctx, "ab", Typing{WPM: 600}); err != nil {
		t.Fatal(err)
	}
	var downs int
	for _, p := range rec.params {
		if p["type"] == "keyDown" {
			downs++
		}
		if rec.methods[0] != "Input.dispatchKeyEvent" {
			t.Fatalf("method %s", rec.methods[0])
		}
	}
	if downs != 2 {
		t.Fatalf("keyDown count %d params %d", downs, len(rec.params))
	}
}

func TestSelectAllUsesModifier(t *testing.T) {
	rec := &recCaller{}
	k := NewKeyboard(rec)
	if err := k.SelectAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(rec.params) < 4 {
		t.Fatalf("events %d", len(rec.params))
	}
	foundA := false
	for _, p := range rec.params {
		if p["key"] == "a" && p["modifiers"] != 0 {
			foundA = true
		}
	}
	if !foundA {
		t.Fatalf("no modified a: %+v", rec.params)
	}
}

func TestDelayToWPM(t *testing.T) {
	if DelayToWPM(67) < 100 {
		t.Fatalf("got %f", DelayToWPM(67))
	}
}
