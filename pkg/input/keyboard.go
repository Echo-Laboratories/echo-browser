package input

import (
	"context"
	"math"
	"math/rand/v2"
	"runtime"
	"time"
	"unicode"
	"unicode/utf8"
)

// Typing controls cadence and optional neighboring-key mistakes.
type Typing struct {
	// WPM is words-per-minute (5 chars = 1 word). Zero uses 180.
	WPM float64
	// Mistakes enables ~2% neighboring-key typos that are then corrected.
	Mistakes bool
	// MistakeRate overrides the default 0.02 when Mistakes is set. Clamped to [0, 0.08].
	MistakeRate float64
}

func (t Typing) wpm() float64 {
	if t.WPM <= 0 {
		return 180
	}
	return t.WPM
}

func (t Typing) mistakeRate() float64 {
	if !t.Mistakes {
		return 0
	}
	r := t.MistakeRate
	if r <= 0 {
		r = 0.02
	}
	if r > 0.08 {
		r = 0.08
	}
	return r
}

// Keyboard synthesizes keyDown/char/keyUp via Input.dispatchKeyEvent.
type Keyboard struct {
	call Caller
}

func NewKeyboard(call Caller) *Keyboard {
	return &Keyboard{call: call}
}

// Type types text with a lognormal inter-key delay. ASCII is emitted as real
// keys; other runes use a char event (IME-style), never whole-string insertText.
func (k *Keyboard) Type(ctx context.Context, text string, spec Typing) error {
	rate := spec.mistakeRate()
	for i, r := range text {
		if err := ctx.Err(); err != nil {
			return err
		}
		if rate > 0 && rand.Float64() < rate {
			if n := neighbor(r); n != 0 {
				if err := k.emitRune(ctx, n); err != nil {
					return err
				}
				if err := k.pause(ctx, spec, r, false); err != nil {
					return err
				}
				if err := k.Press(ctx, "Backspace"); err != nil {
					return err
				}
				if err := k.pause(ctx, spec, r, false); err != nil {
					return err
				}
			}
		}
		if err := k.emitRune(ctx, r); err != nil {
			return err
		}
		last := i+utf8.RuneLen(r) >= len(text)
		if !last {
			if err := k.pause(ctx, spec, r, true); err != nil {
				return err
			}
		}
	}
	return nil
}

func (k *Keyboard) emitRune(ctx context.Context, r rune) error {
	for _, e := range eventsForRune(r) {
		if err := dispatch(ctx, k.call, "Input.dispatchKeyEvent", e.params()); err != nil {
			return err
		}
	}
	return nil
}

// Press taps a named key (Enter, Tab, Backspace, …).
func (k *Keyboard) Press(ctx context.Context, name string) error {
	ev, err := eventsForNamed(name)
	if err != nil {
		return err
	}
	for _, e := range ev {
		if err := dispatch(ctx, k.call, "Input.dispatchKeyEvent", e.params()); err != nil {
			return err
		}
	}
	return nil
}

// SelectAll sends Ctrl/Cmd+A using real modifier keys.
func (k *Keyboard) SelectAll(ctx context.Context) error {
	mod := ModCtrl
	if runtime.GOOS == "darwin" {
		mod = ModMeta
	}
	down := modifierDown(mod)
	if err := dispatch(ctx, k.call, "Input.dispatchKeyEvent", down.params()); err != nil {
		return err
	}
	aDown := KeyEvent{
		Type:                  "keyDown",
		Modifiers:             mod,
		Key:                   "a",
		Code:                  "KeyA",
		WindowsVirtualKeyCode: 65,
		NativeVirtualKeyCode:  65,
	}
	if err := dispatch(ctx, k.call, "Input.dispatchKeyEvent", aDown.params()); err != nil {
		return err
	}
	aUp := aDown
	aUp.Type = "keyUp"
	if err := dispatch(ctx, k.call, "Input.dispatchKeyEvent", aUp.params()); err != nil {
		return err
	}
	up := modifierUp(mod)
	return dispatch(ctx, k.call, "Input.dispatchKeyEvent", up.params())
}

func (k *Keyboard) pause(ctx context.Context, spec Typing, r rune, wordAware bool) error {
	mean := 60000.0 / (spec.wpm() * 5) // ms per char
	// Lognormal with sigma 0.35 around `mean`.
	delay := lognormal(mean, 0.35)
	if wordAware {
		if r == ' ' {
			delay += rand.Float64() * 80
		}
		if r == '.' || r == ',' || r == '!' || r == '?' || r == ';' || r == ':' {
			delay += 50 + rand.Float64()*150
		}
	}
	if delay < 15 {
		delay = 15
	}
	if delay > 800 {
		delay = 800
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(time.Duration(delay) * time.Millisecond):
		return nil
	}
}

func lognormal(mean, sigma float64) float64 {
	// Convert desired mean to lognormal mu: mean = exp(mu + sigma^2/2)
	mu := math.Log(mean) - 0.5*sigma*sigma
	// Box-Muller
	u1 := rand.Float64()
	u2 := rand.Float64()
	if u1 < 1e-12 {
		u1 = 1e-12
	}
	z := math.Sqrt(-2*math.Log(u1)) * math.Cos(2*math.Pi*u2)
	return math.Exp(mu + sigma*z)
}

var qwertyRows = []string{
	"1234567890-=",
	"qwertyuiop[]",
	"asdfghjkl;'",
	"zxcvbnm,./",
}

func neighbor(r rune) rune {
	if unicode.IsUpper(r) {
		n := neighbor(unicode.ToLower(r))
		if n == 0 {
			return 0
		}
		return unicode.ToUpper(n)
	}
	for _, row := range qwertyRows {
		i := indexRune(row, r)
		if i < 0 {
			continue
		}
		opts := make([]rune, 0, 4)
		if i > 0 {
			opts = append(opts, []rune(row)[i-1])
		}
		if i+1 < len([]rune(row)) {
			opts = append(opts, []rune(row)[i+1])
		}
		if len(opts) == 0 {
			return 0
		}
		return opts[rand.IntN(len(opts))]
	}
	return 0
}

func indexRune(s string, r rune) int {
	for i, c := range s {
		if c == r {
			return i
		}
	}
	return -1
}

// DelayToWPM converts a per-character millisecond delay to WPM.
func DelayToWPM(delayMs int) float64 {
	if delayMs <= 0 {
		delayMs = 80
	}
	return (60000.0 / float64(delayMs)) / 5.0
}
