//go:build windows && (amd64 || arm64)

package winapp

// The focused text field, as far as Windows lets another process see and touch it — the
// counterpart of AXTextField in Injector.swift.
//
// macOS offers one accessibility API for every text field. Windows has none that works in
// pure Go without a large COM surface (UI Automation's text pattern), so this covers what
// plain window messages reach: standard Edit controls (Notepad before Windows 11, dialogs,
// WinForms and Delphi text boxes, most classic applications) and RichEdit (WordPad, the
// Windows 11 Notepad, many business applications). There the replacement is verified before
// anything is deleted, made with EM_REPLACESEL (so the control's own Ctrl+Z still works) and
// read back. Everywhere else — browsers, Electron, Office, terminals — there is no field:
// the buffer is trusted and the edit is typed, as in the Swift app when accessibility is
// unavailable.

import (
	"strings"
	"unicode/utf16"

	"tilqos/internal/core"
	"tilqos/internal/win32"
)

// maxFieldText is the longest text read whole. Past it the round trip costs more than the
// check is worth (the Swift app draws the same line at 20,000 characters), and positions
// stay exact: EM_GETSEL reports them in 16 bits.
const maxFieldText = 20_000

type editKind int

const (
	notEdit editKind = iota
	plainEdit
	richEdit
)

func editKindOf(hwnd uintptr) editKind {
	if strings.EqualFold(win32.RealGetWindowClass(hwnd), "Edit") {
		return plainEdit
	}
	if strings.Contains(strings.ToLower(win32.GetClassName(hwnd)), "richedit") {
		return richEdit
	}
	return notEdit
}

// breakMode is how a control counts a line break in its character positions.
type breakMode int

const (
	// breaksUnknown: not decided yet — the text so far had no line break.
	breaksUnknown breakMode = iota
	// breaksAsReturned: as WM_GETTEXT returns them (Edit, RichEdit 1.0: CR LF is two).
	breaksAsReturned
	// breaksFolded: CR LF counts as one position (RichEdit 2.0 and later).
	breaksFolded
	// breaksUnreadable: neither matches; the text cannot be lined up with positions.
	breaksUnreadable
)

// editField is core.TextFieldAccess over an Edit or RichEdit control. One is made per
// replacement, so the line-break decision is cached for exactly that long.
type editField struct {
	hwnd     uintptr
	readOnly bool
	breaks   breakMode
}

// FocusedField is the window's focused control when it is an Edit or RichEdit control whose
// text we may read: never a password field.
func (a *App) FocusedField(window uintptr) core.TextFieldAccess {
	if window == 0 {
		return nil
	}
	thread, _ := win32.GetWindowThreadProcessId(window)
	if thread == 0 {
		// A window that is gone: GetGUIThreadInfo(0) would describe the foreground thread.
		return nil
	}
	info, ok := win32.GetGUIThreadInfo(thread)
	if !ok || info.HwndFocus == 0 {
		return nil
	}
	focus := info.HwndFocus
	if editKindOf(focus) == notEdit {
		return nil
	}
	style := win32.GetWindowLongPtr(focus, win32.GWL_STYLE)
	if style&win32.ES_PASSWORD != 0 {
		return nil
	}
	return &editField{hwnd: focus, readOnly: style&win32.ES_READONLY != 0}
}

// text reads the control's text in the units its positions count.
//
// Edit controls and RichEdit 1.0 count a line break as the two characters WM_GETTEXT
// returns; RichEdit 2.0 and later count it as one while WM_GETTEXT still returns CR LF. The
// class name does not say which (a Delphi TRichEdit wraps 1.0), so the control is asked for
// its own length in positions and the text that lines up with it is used.
func (f *editField) text() ([]uint16, bool) {
	raw, ok := win32.EditText(f.hwnd, maxFieldText)
	if !ok {
		return nil, false
	}
	if !containsCRLF(raw) {
		return raw, true
	}
	folded := foldCRLF(raw)
	if f.breaks == breaksUnknown {
		f.breaks = breaksUnreadable
		if length, ok := win32.EditPositionLength(f.hwnd); ok {
			switch length {
			case len(raw):
				f.breaks = breaksAsReturned
			case len(folded):
				f.breaks = breaksFolded
			}
		}
	}
	switch f.breaks {
	case breaksAsReturned:
		return raw, true
	case breaksFolded:
		return folded, true
	}
	return nil, false
}

func containsCRLF(units []uint16) bool {
	for i := 0; i+1 < len(units); i++ {
		if units[i] == '\r' && units[i+1] == '\n' {
			return true
		}
	}
	return false
}

func foldCRLF(units []uint16) []uint16 {
	folded := make([]uint16, 0, len(units))
	for i := 0; i < len(units); i++ {
		if units[i] == '\r' && i+1 < len(units) && units[i+1] == '\n' {
			folded = append(folded, '\r')
			i++
			continue
		}
		folded = append(folded, units[i])
	}
	return folded
}

// SelectedRange is a single EM_GETSEL: while our selection is up, every round trip is a
// moment in which a key the user presses could land on it.
func (f *editField) SelectedRange() (core.Range16, bool) {
	start, end, ok := win32.EditSelection(f.hwnd)
	if !ok || end < start {
		return core.Range16{}, false
	}
	return core.Range16{Location: start, Length: end - start}, true
}

func (f *editField) SetSelectedRange(r core.Range16) bool {
	return win32.EditSetSelection(f.hwnd, r.Location, r.Location+r.Length)
}

func (f *editField) StringIn(r core.Range16) (string, bool) {
	units, ok := f.text()
	if !ok || r.Location < 0 || r.Length < 0 || r.Location+r.Length > len(units) {
		return "", false
	}
	return string(utf16.Decode(units[r.Location : r.Location+r.Length])), true
}

func (f *editField) SelectedText() (string, bool) {
	selection, ok := f.SelectedRange()
	if !ok {
		return "", false
	}
	return f.StringIn(selection)
}

func (f *editField) SetSelectedText(text string) bool {
	return win32.EditReplaceSelection(f.hwnd, text)
}

func (f *editField) CanSetSelectedRange() bool { return true }
func (f *editField) CanSetSelectedText() bool  { return !f.readOnly }
