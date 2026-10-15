package winapp

// Key classification and the physical key-code space of the Windows tables.
//
// Platform-neutral on purpose (pure arithmetic on documented virtual-key and scan-code
// values), so it is unit-tested on any OS.
//
// Why scan codes and not virtual keys: a virtual key is assigned by the *active layout*
// (on German QWERTZ the key right of T is VK_Z, on US QWERTY it is VK_Y), while the whole
// app rests on re-reading the same *physical* keys under another layout. The scan code
// names the physical key, so the tables are indexed by it — the Windows counterpart of the
// macOS virtual key code, which is also positional.

import "tilqos/internal/engine"

// Key codes of the Windows tables:
//
//	0x000–0x0FF  scan code (set 1, make code)
//	0x100–0x1FF  scan code with the E0 prefix (LLKHF_EXTENDED)
//	0x200–0x27F  numeric keypad with Num Lock on: 0x200 + VK (VK_NUMPAD0…9, VK_DECIMAL)
//
// The keypad needs its own codes because Num Lock changes what the same scan code does:
// with Num Lock on, scan 0x47 types "7"; with it off, it is Home.
const (
	extendedKeyBase = 0x100
	numpadKeyBase   = 0x200
	// KeyCodeCount is the size of the key-code space.
	KeyCodeCount = 0x280
)

// Windows virtual keys used below.
const (
	vkBack     = 0x08
	vkTab      = 0x09
	vkClear    = 0x0C
	vkReturn   = 0x0D
	vkShift    = 0x10
	vkControl  = 0x11
	vkMenu     = 0x12
	vkPause    = 0x13
	vkCapital  = 0x14
	vkEscape   = 0x1B
	vkPrior    = 0x21
	vkNext     = 0x22
	vkEnd      = 0x23
	vkHome     = 0x24
	vkLeft     = 0x25
	vkUp       = 0x26
	vkRight    = 0x27
	vkDown     = 0x28
	vkSnapshot = 0x2C
	vkInsert   = 0x2D
	vkDelete   = 0x2E
	vkLWin     = 0x5B
	vkRWin     = 0x5C
	vkApps     = 0x5D
	vkNumpad0  = 0x60
	vkNumpad9  = 0x69
	vkDecimal  = 0x6E
	vkF1       = 0x70
	vkF24      = 0x87
	vkNumLock  = 0x90
	vkScroll   = 0x91
	vkLShift   = 0xA0
	vkRMenu    = 0xA5
)

// IsModifierVK: keys that only change other keys. Pressing one alone changes nothing on
// screen (the layout-switch chord Alt+Shift included).
func IsModifierVK(vk uint16) bool {
	switch {
	case vk == vkShift, vk == vkControl, vk == vkMenu, vk == vkCapital, vk == vkNumLock,
		vk == vkScroll, vk == vkLWin, vk == vkRWin:
		return true
	case vk >= vkLShift && vk <= vkRMenu:
		return true
	}
	return false
}

// ClassifyVK maps a virtual key to what it means for the keystroke buffer.
func ClassifyVK(vk uint16) engine.KeyClass {
	switch {
	case IsModifierVK(vk):
		return engine.KeyModifier
	case vk == vkBack:
		return engine.KeyBackspace
	case vk == vkTab:
		return engine.KeyTab
	case vk == vkEscape:
		return engine.KeyEscape
	case vk == vkReturn, vk >= vkPrior && vk <= vkDown, vk == vkInsert, vk == vkDelete:
		return engine.KeyNavigation
	case vk < 0x20, // control keys, IME keys (kana, junja, final, kanji, convert, …)
		vk == vkSnapshot, vk == vkPause, vk == vkApps, vk == 0x5F, // VK_SLEEP
		vk >= vkF1 && vk <= vkF24,
		vk >= 0xA6 && vk <= 0xB7, // browser, volume, media, launch keys
		vk == 0xE5,               // VK_PROCESSKEY: an IME is composing
		vk == 0xE7,               // VK_PACKET: Unicode injected by someone
		vk >= 0xF6:               // VK_ATTN … VK_OEM_CLEAR
		return engine.KeyOther
	}
	return engine.KeyText
}

// PhysicalKeyCode turns what a low-level hook reports into the table key code. ok is false
// when the key has no usable identity (no scan code).
func PhysicalKeyCode(vk uint16, scan uint32, extended bool) (uint16, bool) {
	if !extended && ((vk >= vkNumpad0 && vk <= vkNumpad9) || vk == vkDecimal) {
		return numpadKeyBase + vk, true
	}
	scan &= 0xFF
	if scan == 0 || scan >= 0x80 {
		return 0, false
	}
	if extended {
		return extendedKeyBase + uint16(scan), true
	}
	return uint16(scan), true
}

// ScanForKeyCode is the inverse for table building: the scan code (with the E0 prefix as
// 0xE000) to hand to MapVirtualKeyEx, or ok=false for keypad pseudo-codes.
func ScanForKeyCode(code uint16) (scan uint32, ok bool) {
	switch {
	case code >= numpadKeyBase:
		return 0, false
	case code >= extendedKeyBase:
		return 0xE000 | uint32(code-extendedKeyBase), true
	}
	return uint32(code), true
}

// NumpadVK is the virtual key of a keypad pseudo-code.
func NumpadVK(code uint16) (uint16, bool) {
	if code < numpadKeyBase {
		return 0, false
	}
	vk := code - numpadKeyBase
	if (vk >= vkNumpad0 && vk <= vkNumpad9) || vk == vkDecimal {
		return vk, true
	}
	return 0, false
}

// KeypadCodes lists the key codes that are keypad keys: they type the same digits on every
// layout, so they must never win when a character is resolved back to a keystroke.
func KeypadCodes() map[uint16]bool {
	codes := map[uint16]bool{}
	for vk := uint16(vkNumpad0); vk <= vkNumpad9; vk++ {
		codes[numpadKeyBase+vk] = true
	}
	codes[numpadKeyBase+vkDecimal] = true
	// Keypad scan codes that do not depend on Num Lock: * - + and /, Enter.
	for _, scan := range []uint16{0x37, 0x4A, 0x4E} {
		codes[scan] = true
	}
	codes[extendedKeyBase+0x35] = true // keypad /
	codes[extendedKeyBase+0x1C] = true // keypad Enter
	// And the keypad digits' own scan codes (Num Lock off), for completeness.
	for scan := uint16(0x47); scan <= 0x53; scan++ {
		codes[scan] = true
	}
	return codes
}
