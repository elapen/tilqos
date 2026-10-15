//go:build windows && (amd64 || arm64)

package winapp

// Typing on the user's behalf: the counterpart of SyntheticKeys in Injector.swift.
//
// core.TextReplacer decides how an edit is made. Where the focused control is a standard
// Edit or RichEdit control (field_windows.go) it is verified first and usually made through
// the control itself; everywhere else it is typed: Backspace presses, then the replacement
// as KEYEVENTF_UNICODE key events (VK_PACKET → WM_CHAR, independent of the active layout),
// all in ONE SendInput call so the user's own keystrokes cannot interleave, and all marked
// with injectMarker so our hook ignores them.
//
// UIPI: a process cannot inject into windows of a higher integrity level (elevated apps)
// unless it is elevated itself; SendInput then fails silently. Such windows are detected
// beforehand and skipped (see CanInject).

import (
	"errors"
	"time"
	"unicode/utf16"

	"tilqos/internal/core"
	"tilqos/internal/win32"
)

var (
	errModifiersHeld   = errors.New("modifier keys are still held")
	errTypingContinued = errors.New("typing continued before the edit could be made")
	errInjectBlocked   = errors.New("SendInput did not accept all events")
)

func keyInput(vk uint16, flags uint32) win32.INPUT {
	var scan uint16
	if vk != 0 {
		scan = uint16(win32.MapVirtualKeyEx(uint32(vk), win32.MAPVK_VK_TO_VSC, 0))
	}
	switch vk {
	case win32.VK_RCONTROL, win32.VK_RMENU, win32.VK_LWIN, win32.VK_RWIN, win32.VK_INSERT,
		win32.VK_DELETE, win32.VK_HOME, win32.VK_END, win32.VK_PRIOR, win32.VK_NEXT,
		win32.VK_LEFT, win32.VK_RIGHT, win32.VK_UP, win32.VK_DOWN:
		flags |= win32.KEYEVENTF_EXTENDEDKEY
	}
	return win32.INPUT{
		Type: win32.INPUT_KEYBOARD,
		Ki:   win32.KEYBDINPUT{WVk: vk, WScan: scan, DwFlags: flags, DwExtraInfo: injectMarker},
	}
}

func unicodeInput(unit uint16, flags uint32) win32.INPUT {
	return win32.INPUT{
		Type: win32.INPUT_KEYBOARD,
		Ki:   win32.KEYBDINPUT{WScan: unit, DwFlags: win32.KEYEVENTF_UNICODE | flags, DwExtraInfo: injectMarker},
	}
}

func send(inputs []win32.INPUT) error {
	n, err := win32.SendInput(inputs)
	if int(n) != len(inputs) {
		if err != nil {
			return err
		}
		return errInjectBlocked
	}
	return nil
}

// tapKeys presses and releases keys in order (a chord: all down, then all up in reverse).
func tapKeys(vks ...uint16) error {
	inputs := make([]win32.INPUT, 0, 2*len(vks))
	for _, vk := range vks {
		inputs = append(inputs, keyInput(vk, 0))
	}
	for i := len(vks) - 1; i >= 0; i-- {
		inputs = append(inputs, keyInput(vks[i], win32.KEYEVENTF_KEYUP))
	}
	return send(inputs)
}

// sendMaskKey taps an unassigned key so a released Alt or Win does not open the menu bar
// or the Start menu after a swallowed hotkey. Called from the hook callback: two events.
func sendMaskKey() {
	send([]win32.INPUT{keyInput(win32.VK_MASK, 0), keyInput(win32.VK_MASK, win32.KEYEVENTF_KEYUP)})
}

func modifiersHeld(includeShift bool) bool {
	if win32.KeyDown(win32.VK_CONTROL) || win32.KeyDown(win32.VK_MENU) ||
		win32.KeyDown(win32.VK_LWIN) || win32.KeyDown(win32.VK_RWIN) {
		return true
	}
	return includeShift && win32.KeyDown(win32.VK_SHIFT)
}

// waitForModifierRelease waits until Ctrl, Alt and Win (and Shift if asked) are up.
//
// A synthetic Backspace pressed while the user still holds Ctrl is Ctrl+Backspace — delete
// a word — and with Alt it is Undo in many edit controls; a Ctrl+C while Shift is held is a
// different shortcut. Forcing the keys up instead would break the user's next chord, so the
// edit is abandoned if they are not released in time.
func waitForModifierRelease(includeShift bool, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for modifiersHeld(includeShift) {
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(5 * time.Millisecond)
	}
	return true
}

var errNothingToDelete = errors.New("refusing to insert text without deleting any")

// replacementInputs builds the key events of one edit: Backspace deleting times, then the
// text as KEYEVENTF_UNICODE events.
//
// The text is laid out chunk by chunk (core.Chunks: at most 20 UTF-16 units, never splitting
// a surrogate pair or a character), as the Swift injector sends it. On macOS each chunk must
// be a separate event, because one event carries at most 20 units; on Windows every event
// carries exactly one unit, so there is nothing to truncate and the chunks go out in the
// same single SendInput call as the deletions — splitting the call would let the user's own
// keystrokes land in the middle of the correction.
//
// A pure insertion is refused: every correction replaces text that was typed, and inserting
// without deleting is exactly the failure (text next to the old text) this must never cause.
func replacementInputs(deleting int, inserting string) ([]win32.INPUT, error) {
	if deleting <= 0 {
		return nil, errNothingToDelete
	}
	inputs := make([]win32.INPUT, 0, 2*deleting+2*core.UTF16Len(inserting))
	for i := 0; i < deleting; i++ {
		inputs = append(inputs, keyInput(win32.VK_BACK, 0), keyInput(win32.VK_BACK, win32.KEYEVENTF_KEYUP))
	}
	for _, chunk := range core.Chunks(inserting, core.ChunkLimit) {
		// Surrogate pairs go as two units; TranslateMessage delivers two WM_CHARs, which
		// applications combine.
		for _, unit := range utf16.Encode([]rune(chunk)) {
			inputs = append(inputs, unicodeInput(unit, 0), unicodeInput(unit, win32.KEYEVENTF_KEYUP))
		}
	}
	return inputs, nil
}

// keystrokes is core.KeystrokeSender over SendInput. SendInput's contract is that a batch is
// inserted serially, not interspersed with other input, and that input blocked by UIPI
// inserts nothing — so the insertion only ever travels together with, and after, the
// deletions it replaces.
type keystrokes struct {
	a *App
	// expectSeq is the newest key press the edit was planned against (0: whatever is newest
	// when the edit starts).
	expectSeq uint64
}

func (a *App) Keystrokes(expectSeq uint64) core.KeystrokeSender {
	return keystrokes{a: a, expectSeq: expectSeq}
}

// SendKeys waits for the user to let go of Ctrl, Alt and Win (a synthetic Backspace under a
// held Ctrl deletes a word) and refuses if they typed anything meanwhile: the text before
// the caret would no longer be what the edit was planned against.
func (k keystrokes) SendKeys(deleting int, inserting string) bool {
	expect := k.expectSeq
	if expect == 0 {
		expect = k.a.hooks.KeySeq()
	}
	if !waitForModifierRelease(false, 700*time.Millisecond) {
		k.a.log.Printf("edit not sent: %v", errModifiersHeld)
		return false
	}
	if k.a.hooks.KeySeq() != expect {
		k.a.log.Printf("edit not sent: %v", errTypingContinued)
		return false
	}
	inputs, err := replacementInputs(deleting, inserting)
	if err == nil {
		err = send(inputs)
	}
	if err != nil {
		k.a.log.Printf("edit not sent: %v", err)
		return false
	}
	return true
}
