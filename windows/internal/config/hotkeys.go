package config

// Hotkeys (the Windows counterpart of Core/Hotkeys.swift).
//
// Why not the macOS ⌥⇧ chords: Alt+Shift is Windows' default input-language switch, so
// Alt+Shift+letter both fires our action and flips the layout (the same complaint VS Code
// users have about Alt+Shift+F), and Ctrl+Alt+letter collides with AltGr characters on many
// European layouts. The defaults instead follow Punto Switcher, which Windows users of this
// kind of tool already know:
//
//	Pause            fix the last word — or, right after an automatic fix, undo it
//	Shift+Pause      fix the selected text (via the clipboard, which is restored)
//	Alt+Pause        undo the last fix
//	Alt+ScrollLock   transliterate the selection (Punto's translit key)
//
// Pause/Break and Scroll Lock do nothing in almost every modern application, so taking
// them over costs nothing. Laptops without the keys reach them with Fn (Fn+B / Fn+P / Fn+K
// on most models), and every binding can be changed in config.json.

import (
	"fmt"
	"strconv"
	"strings"

	"tilqos/internal/core"
	"tilqos/internal/i18n"
)

// Action is something a hotkey or a menu item can trigger. The names match the Swift
// HotkeyAction raw values.
type Action string

const (
	ActionFixLastWord      Action = "fixLastWord"
	ActionFixSelection     Action = "fixSelection"
	ActionUndoLastFix      Action = "undoLastFix"
	ActionForceEnglish     Action = "forceEnglish"
	ActionForceRussian     Action = "forceRussian"
	ActionForceKazakh      Action = "forceKazakh"
	ActionConvertClipboard Action = "convertClipboard"
	ActionTransliterate    Action = "transliterate"
	ActionToggleEnabled    Action = "toggleEnabled"
)

// AllActions in menu order.
var AllActions = []Action{
	ActionFixLastWord, ActionFixSelection, ActionUndoLastFix, ActionForceEnglish,
	ActionForceRussian, ActionForceKazakh, ActionConvertClipboard, ActionTransliterate,
	ActionToggleEnabled,
}

func (a Action) DisplayName() string {
	switch a {
	case ActionFixLastWord:
		return i18n.T(i18n.ActFixLastWord)
	case ActionFixSelection:
		return i18n.T(i18n.ActFixSelection)
	case ActionUndoLastFix:
		return i18n.T(i18n.ActUndoLastFix)
	case ActionForceEnglish:
		return i18n.F(i18n.ActSwitchTo, core.LangEN.DisplayName())
	case ActionForceRussian:
		return i18n.F(i18n.ActSwitchTo, core.LangRU.DisplayName())
	case ActionForceKazakh:
		return i18n.F(i18n.ActSwitchTo, core.LangKK.DisplayName())
	case ActionConvertClipboard:
		return i18n.T(i18n.ActConvertClipboard)
	case ActionTransliterate:
		return i18n.T(i18n.ActTransliterate)
	case ActionToggleEnabled:
		return i18n.T(i18n.ActToggleEnabled)
	}
	return string(a)
}

// DefaultHotkeys is the default binding per action; "" means unbound (menu only).
func DefaultHotkeys() map[string]string {
	return map[string]string{
		string(ActionFixLastWord):      "Pause",
		string(ActionFixSelection):     "Shift+Pause",
		string(ActionUndoLastFix):      "Alt+Pause",
		string(ActionTransliterate):    "Alt+ScrollLock",
		string(ActionForceEnglish):     "",
		string(ActionForceRussian):     "",
		string(ActionForceKazakh):      "",
		string(ActionConvertClipboard): "",
		string(ActionToggleEnabled):    "",
	}
}

// Mods are the modifiers of a hotkey.
type Mods uint8

const (
	ModShift Mods = 1 << iota
	ModCtrl
	ModAlt
	ModWin
)

// Hotkey is a Windows virtual-key code plus modifiers.
type Hotkey struct {
	VK   uint16
	Mods Mods
}

// Windows virtual-key codes used by the spec names. Kept here, not in the Windows-only
// package, so parsing is testable anywhere.
const (
	VKBack       = 0x08
	VKTab        = 0x09
	VKReturn     = 0x0D
	VKPause      = 0x13
	VKCapital    = 0x14
	VKEscape     = 0x1B
	VKSpace      = 0x20
	VKPrior      = 0x21
	VKNext       = 0x22
	VKEnd        = 0x23
	VKHome       = 0x24
	VKLeft       = 0x25
	VKUp         = 0x26
	VKRight      = 0x27
	VKDown       = 0x28
	VKSnapshot   = 0x2C
	VKInsert     = 0x2D
	VKDelete     = 0x2E
	VKApps       = 0x5D
	VKNumpad0    = 0x60
	VKF1         = 0x70
	VKF24        = 0x87
	VKNumLock    = 0x90
	VKScrollLock = 0x91
)

var namedKeys = map[string]uint16{
	"pause": VKPause, "break": VKPause, "scrolllock": VKScrollLock, "scroll": VKScrollLock,
	"insert": VKInsert, "ins": VKInsert, "delete": VKDelete, "del": VKDelete,
	"home": VKHome, "end": VKEnd, "pageup": VKPrior, "pgup": VKPrior, "pagedown": VKNext,
	"pgdn": VKNext, "left": VKLeft, "up": VKUp, "right": VKRight, "down": VKDown,
	"space": VKSpace, "tab": VKTab, "enter": VKReturn, "return": VKReturn, "esc": VKEscape,
	"escape": VKEscape, "backspace": VKBack, "printscreen": VKSnapshot, "prtsc": VKSnapshot,
	"apps": VKApps, "menu": VKApps, "numlock": VKNumLock,
	"`": 0xC0, "grave": 0xC0, "-": 0xBD, "minus": 0xBD, "=": 0xBB, "equals": 0xBB,
	"[": 0xDB, "]": 0xDD, ";": 0xBA, "'": 0xDE, ",": 0xBC, "comma": 0xBC, ".": 0xBE,
	"period": 0xBE, "/": 0xBF, "slash": 0xBF, "\\": 0xDC, "backslash": 0xDC,
}

var canonicalNames = map[uint16]string{
	VKPause: "Pause", VKScrollLock: "ScrollLock", VKInsert: "Insert", VKDelete: "Delete",
	VKHome: "Home", VKEnd: "End", VKPrior: "PageUp", VKNext: "PageDown", VKLeft: "Left",
	VKUp: "Up", VKRight: "Right", VKDown: "Down", VKSpace: "Space", VKTab: "Tab",
	VKReturn: "Enter", VKEscape: "Esc", VKBack: "Backspace", VKSnapshot: "PrintScreen",
	VKApps: "Apps", VKNumLock: "NumLock", 0xC0: "`", 0xBD: "-", 0xBB: "=", 0xDB: "[",
	0xDD: "]", 0xBA: ";", 0xDE: "'", 0xBC: ",", 0xBE: ".", 0xBF: "/", 0xDC: "\\",
}

// ParseHotkey parses specs like "Pause", "Shift+Pause", "Ctrl+Alt+Z", "Win+F9".
func ParseHotkey(spec string) (Hotkey, error) {
	parts := strings.Split(strings.TrimSpace(spec), "+")
	// "Ctrl++" would split into empty parts; a plus key is spelled "=" with Shift anyway.
	var hotkey Hotkey
	keySeen := false
	for index, raw := range parts {
		part := strings.ToLower(strings.TrimSpace(raw))
		if part == "" {
			return Hotkey{}, fmt.Errorf("empty part in %q", spec)
		}
		last := index == len(parts)-1
		switch part {
		case "ctrl", "control":
			hotkey.Mods |= ModCtrl
			continue
		case "alt":
			hotkey.Mods |= ModAlt
			continue
		case "shift":
			hotkey.Mods |= ModShift
			continue
		case "win", "windows", "meta":
			hotkey.Mods |= ModWin
			continue
		}
		if !last {
			return Hotkey{}, fmt.Errorf("%q: the key must come last", spec)
		}
		vk, ok := keyCode(part)
		if !ok {
			return Hotkey{}, fmt.Errorf("%q: unknown key %q", spec, raw)
		}
		hotkey.VK = vk
		keySeen = true
	}
	if !keySeen {
		return Hotkey{}, fmt.Errorf("%q has no key", spec)
	}
	return hotkey, nil
}

func keyCode(name string) (uint16, bool) {
	if vk, ok := namedKeys[name]; ok {
		return vk, true
	}
	if len(name) == 1 {
		c := name[0]
		switch {
		case c >= 'a' && c <= 'z':
			return uint16(c - 'a' + 'A'), true
		case c >= '0' && c <= '9':
			return uint16(c), true
		}
	}
	if strings.HasPrefix(name, "f") {
		if n, err := strconv.Atoi(name[1:]); err == nil && n >= 1 && n <= 24 {
			return uint16(VKF1 + n - 1), true
		}
	}
	if strings.HasPrefix(name, "numpad") {
		if n, err := strconv.Atoi(name[6:]); err == nil && n >= 0 && n <= 9 {
			return uint16(VKNumpad0 + n), true
		}
	}
	return 0, false
}

// String formats a hotkey the way ParseHotkey reads it.
func (h Hotkey) String() string {
	var parts []string
	if h.Mods&ModCtrl != 0 {
		parts = append(parts, "Ctrl")
	}
	if h.Mods&ModAlt != 0 {
		parts = append(parts, "Alt")
	}
	if h.Mods&ModShift != 0 {
		parts = append(parts, "Shift")
	}
	if h.Mods&ModWin != 0 {
		parts = append(parts, "Win")
	}
	return strings.Join(append(parts, KeyName(h.VK)), "+")
}

// KeyName is the display name of a virtual key.
func KeyName(vk uint16) string {
	switch {
	case vk >= 'A' && vk <= 'Z', vk >= '0' && vk <= '9':
		return string(rune(vk))
	case vk >= VKF1 && vk <= VKF24:
		return "F" + strconv.Itoa(int(vk-VKF1+1))
	case vk >= VKNumpad0 && vk <= VKNumpad0+9:
		return "Numpad" + strconv.Itoa(int(vk-VKNumpad0))
	}
	if name, ok := canonicalNames[vk]; ok {
		return name
	}
	return fmt.Sprintf("VK%02X", vk)
}

// Valid: a key that types text needs a modifier, or it would fire while typing (the Swift
// rule). Keys that never type — Pause, Scroll Lock, F-keys — may stand alone.
func (h Hotkey) Valid() bool {
	if h.VK == 0 {
		return false
	}
	if h.Mods != 0 {
		return true
	}
	return h.VK == VKPause || h.VK == VKScrollLock || (h.VK >= VKF1 && h.VK <= VKF24)
}

// Matches reports an exact match: the same key with exactly the same modifiers.
func (h Hotkey) Matches(vk uint16, mods Mods) bool { return h.VK == vk && h.Mods == mods }

// Binding is a parsed hotkey and the action it triggers.
type Binding struct {
	Hotkey Hotkey
	Action Action
}

// Bindings parses a hotkey map, dropping empty and invalid entries, in a stable order.
func Bindings(hotkeys map[string]string) []Binding {
	var out []Binding
	for _, action := range AllActions {
		spec := hotkeys[string(action)]
		if spec == "" {
			continue
		}
		hotkey, err := ParseHotkey(spec)
		if err != nil || !hotkey.Valid() {
			continue
		}
		out = append(out, Binding{Hotkey: hotkey, Action: action})
	}
	return out
}
