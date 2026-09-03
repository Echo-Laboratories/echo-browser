package input

import (
	"fmt"
	"strings"
	"unicode"
)

// Modifiers match CDP Input.dispatchKeyEvent modifier bits.
const (
	ModAlt   = 1
	ModCtrl  = 2
	ModMeta  = 4
	ModShift = 8
)

// KeyEvent is one Input.dispatchKeyEvent payload.
type KeyEvent struct {
	Type                  string
	Modifiers             int
	Key                   string
	Code                  string
	WindowsVirtualKeyCode int
	NativeVirtualKeyCode  int
	Text                  string
	UnmodifiedText        string
}

func (e KeyEvent) params() map[string]any {
	p := map[string]any{
		"type":                  e.Type,
		"modifiers":             e.Modifiers,
		"key":                   e.Key,
		"windowsVirtualKeyCode": e.WindowsVirtualKeyCode,
		"nativeVirtualKeyCode":  e.NativeVirtualKeyCode,
	}
	if e.Code != "" {
		p["code"] = e.Code
	}
	if e.Text != "" {
		p["text"] = e.Text
	}
	if e.UnmodifiedText != "" {
		p["unmodifiedText"] = e.UnmodifiedText
	}
	return p
}

type keyDef struct {
	key      string
	code     string
	vk       int
	shiftKey string
}

var usKeys map[rune]keyDef
var namedKeys map[string]keyDef

func init() {
	usKeys = make(map[rune]keyDef)
	add := func(r rune, d keyDef) { usKeys[r] = d }

	for i := 0; i < 26; i++ {
		lower := rune('a' + i)
		upper := rune('A' + i)
		code := "Key" + string(upper)
		vk := 65 + i
		add(lower, keyDef{key: string(lower), code: code, vk: vk, shiftKey: string(upper)})
		add(upper, keyDef{key: string(upper), code: code, vk: vk, shiftKey: string(upper)})
	}
	digitShift := []rune{')', '!', '@', '#', '$', '%', '^', '&', '*', '('}
	for i := 0; i < 10; i++ {
		r := rune('0' + i)
		code := fmt.Sprintf("Digit%d", i)
		vk := 48 + i
		add(r, keyDef{key: string(r), code: code, vk: vk, shiftKey: string(digitShift[i])})
		add(digitShift[i], keyDef{key: string(digitShift[i]), code: code, vk: vk, shiftKey: string(digitShift[i])})
	}
	punct := []struct {
		r, shift rune
		code     string
		vk       int
	}{
		{'-', '_', "Minus", 189},
		{'=', '+', "Equal", 187},
		{'[', '{', "BracketLeft", 219},
		{']', '}', "BracketRight", 221},
		{'\\', '|', "Backslash", 220},
		{';', ':', "Semicolon", 186},
		{'\'', '"', "Quote", 222},
		{',', '<', "Comma", 188},
		{'.', '>', "Period", 190},
		{'/', '?', "Slash", 191},
		{'`', '~', "Backquote", 192},
	}
	for _, p := range punct {
		add(p.r, keyDef{key: string(p.r), code: p.code, vk: p.vk, shiftKey: string(p.shift)})
		add(p.shift, keyDef{key: string(p.shift), code: p.code, vk: p.vk, shiftKey: string(p.shift)})
	}
	add(' ', keyDef{key: " ", code: "Space", vk: 32, shiftKey: " "})
	add('\n', keyDef{key: "Enter", code: "Enter", vk: 13, shiftKey: "Enter"})
	add('\t', keyDef{key: "Tab", code: "Tab", vk: 9, shiftKey: "Tab"})

	namedKeys = map[string]keyDef{
		"Enter":      {key: "Enter", code: "Enter", vk: 13},
		"Return":     {key: "Enter", code: "Enter", vk: 13},
		"Tab":        {key: "Tab", code: "Tab", vk: 9},
		"Escape":     {key: "Escape", code: "Escape", vk: 27},
		"Esc":        {key: "Escape", code: "Escape", vk: 27},
		"Backspace":  {key: "Backspace", code: "Backspace", vk: 8},
		"Delete":     {key: "Delete", code: "Delete", vk: 46},
		"ArrowUp":    {key: "ArrowUp", code: "ArrowUp", vk: 38},
		"ArrowDown":  {key: "ArrowDown", code: "ArrowDown", vk: 40},
		"ArrowLeft":  {key: "ArrowLeft", code: "ArrowLeft", vk: 37},
		"ArrowRight": {key: "ArrowRight", code: "ArrowRight", vk: 39},
		"Home":       {key: "Home", code: "Home", vk: 36},
		"End":        {key: "End", code: "End", vk: 35},
		"PageUp":     {key: "PageUp", code: "PageUp", vk: 33},
		"PageDown":   {key: "PageDown", code: "PageDown", vk: 34},
		"Space":      {key: " ", code: "Space", vk: 32},
		"Shift":      {key: "Shift", code: "ShiftLeft", vk: 16},
		"Control":    {key: "Control", code: "ControlLeft", vk: 17},
		"Ctrl":       {key: "Control", code: "ControlLeft", vk: 17},
		"Alt":        {key: "Alt", code: "AltLeft", vk: 18},
		"Meta":       {key: "Meta", code: "MetaLeft", vk: 91},
		"Command":    {key: "Meta", code: "MetaLeft", vk: 91},
	}
}

func runeNeedsShift(r rune) bool {
	if unicode.IsUpper(r) {
		return true
	}
	shifted := `~!@#$%^&*()_+{}|:"<>?`
	return strings.ContainsRune(shifted, r)
}

func defForRune(r rune) (keyDef, bool) {
	d, ok := usKeys[r]
	return d, ok
}

func defForName(name string) (keyDef, bool) {
	d, ok := namedKeys[name]
	if ok {
		return d, true
	}
	if len(name) == 1 {
		return defForRune([]rune(name)[0])
	}
	return keyDef{}, false
}

func eventsForRune(r rune) []KeyEvent {
	d, ok := defForRune(r)
	if !ok {
		// Non-layout characters: a single char event. Callers may use this
		// only when a real key cannot be synthesized.
		s := string(r)
		return []KeyEvent{{
			Type:           "char",
			Key:            s,
			Text:           s,
			UnmodifiedText: s,
		}}
	}
	mods := 0
	key := d.key
	if runeNeedsShift(r) {
		mods = ModShift
		key = d.shiftKey
		if key == "" {
			key = string(r)
		}
	} else {
		key = d.key
		if unicode.IsLetter(r) {
			key = string(r)
		}
	}
	down := KeyEvent{
		Type:                  "keyDown",
		Modifiers:             mods,
		Key:                   key,
		Code:                  d.code,
		WindowsVirtualKeyCode: d.vk,
		NativeVirtualKeyCode:  d.vk,
	}
	ch := KeyEvent{
		Type:                  "char",
		Modifiers:             mods,
		Key:                   key,
		Code:                  d.code,
		WindowsVirtualKeyCode: d.vk,
		NativeVirtualKeyCode:  d.vk,
		Text:                  string(r),
		UnmodifiedText:        string(r),
	}
	up := KeyEvent{
		Type:                  "keyUp",
		Modifiers:             mods,
		Key:                   key,
		Code:                  d.code,
		WindowsVirtualKeyCode: d.vk,
		NativeVirtualKeyCode:  d.vk,
	}
	// Control keys do not emit char.
	if r == '\n' || r == '\t' {
		down.Key = d.key
		up.Key = d.key
		return []KeyEvent{down, up}
	}
	return []KeyEvent{down, ch, up}
}

func eventsForNamed(name string) ([]KeyEvent, error) {
	d, ok := defForName(name)
	if !ok {
		return nil, fmt.Errorf("input: unknown key %q", name)
	}
	down := KeyEvent{
		Type:                  "keyDown",
		Key:                   d.key,
		Code:                  d.code,
		WindowsVirtualKeyCode: d.vk,
		NativeVirtualKeyCode:  d.vk,
	}
	up := down
	up.Type = "keyUp"
	if d.key == " " {
		ch := down
		ch.Type = "char"
		ch.Text = " "
		ch.UnmodifiedText = " "
		return []KeyEvent{down, ch, up}, nil
	}
	return []KeyEvent{down, up}, nil
}

func modifierDown(mod int) KeyEvent {
	switch mod {
	case ModCtrl:
		return KeyEvent{Type: "keyDown", Key: "Control", Code: "ControlLeft", WindowsVirtualKeyCode: 17, NativeVirtualKeyCode: 17, Modifiers: ModCtrl}
	case ModMeta:
		return KeyEvent{Type: "keyDown", Key: "Meta", Code: "MetaLeft", WindowsVirtualKeyCode: 91, NativeVirtualKeyCode: 91, Modifiers: ModMeta}
	case ModShift:
		return KeyEvent{Type: "keyDown", Key: "Shift", Code: "ShiftLeft", WindowsVirtualKeyCode: 16, NativeVirtualKeyCode: 16, Modifiers: ModShift}
	case ModAlt:
		return KeyEvent{Type: "keyDown", Key: "Alt", Code: "AltLeft", WindowsVirtualKeyCode: 18, NativeVirtualKeyCode: 18, Modifiers: ModAlt}
	}
	return KeyEvent{}
}

func modifierUp(mod int) KeyEvent {
	e := modifierDown(mod)
	e.Type = "keyUp"
	return e
}
