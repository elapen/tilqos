//go:build windows && (amd64 || arm64)

package winapp

// Clipboard access. Always on the UI thread: the clipboard is opened against our hidden
// window, and that window's thread must keep pumping messages while other applications
// work with the clipboard (they may send it WM_DESTROYCLIPBOARD and friends).

import (
	"strings"
	"time"
	"unicode/utf16"
	"unsafe"

	"tilqos/internal/win32"
)

func openClipboard(owner uintptr) bool {
	// Another application may hold it for a moment.
	for attempt := 0; attempt < 20; attempt++ {
		if win32.OpenClipboard(owner) {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func globalBytes(handle uintptr) []byte {
	size := win32.GlobalSize(handle)
	if size <= 0 {
		return nil
	}
	pointer := win32.GlobalLock(handle)
	if pointer == 0 {
		return nil
	}
	defer win32.GlobalUnlock(handle)
	data := make([]byte, size)
	copy(data, unsafe.Slice(win32.PtrAt[byte](pointer), size))
	return data
}

func newGlobal(data []byte) uintptr {
	size := len(data)
	if size == 0 {
		size = 1
	}
	handle := win32.GlobalAlloc(size)
	if handle == 0 {
		return 0
	}
	pointer := win32.GlobalLock(handle)
	if pointer == 0 {
		win32.GlobalFree(handle)
		return 0
	}
	copy(unsafe.Slice(win32.PtrAt[byte](pointer), size), data)
	win32.GlobalUnlock(handle)
	return handle
}

// winClipboard is the open clipboard as the rules see it. The clipboard must be open (and
// stay open) while it is used.
type winClipboard struct{}

func (winClipboard) formats() []clipFormatInfo {
	var out []clipFormatInfo
	for format := win32.EnumClipboardFormats(0); format != 0; format = win32.EnumClipboardFormats(format) {
		info := clipFormatInfo{id: format}
		if format >= 0xC000 {
			info.name = win32.GetClipboardFormatName(format)
		}
		out = append(out, info)
	}
	return out
}

func (winClipboard) data(id uint32) ([]byte, bool) {
	handle := win32.GetClipboardData(id)
	if handle == 0 {
		return nil, false
	}
	data := globalBytes(handle)
	return data, data != nil
}

// saveClipboard copies every restorable format. ok is false — and nothing was copied — when
// the clipboard is flagged as sensitive (a password manager's copy: it is neither read nor
// overwritten), too big to put back, or could not be opened.
func saveClipboard(owner uintptr) (saved []clipFormat, ok bool) {
	if !openClipboard(owner) {
		return nil, false
	}
	defer win32.CloseClipboard()
	return saveGuarded(winClipboard{}, clipboardSaveLimit)
}

// restoreClipboard puts saved formats back (an empty save empties the clipboard).
func restoreClipboard(owner uintptr, saved []clipFormat) {
	if !openClipboard(owner) {
		return
	}
	defer win32.CloseClipboard()
	win32.EmptyClipboard()
	for _, f := range saved {
		if handle := newGlobal(f.data); handle != 0 && !win32.SetClipboardData(f.format, handle) {
			win32.GlobalFree(handle)
		}
	}
}

// readClipboardText returns CF_UNICODETEXT if the rules allow it: not when the content is
// flagged as sensitive (its text is then never read), not when it is longer than
// core.MaxClipboardText characters. ok is false in those cases and when there is no text.
func readClipboardText(owner uintptr) (string, bool) {
	if !openClipboard(owner) {
		return "", false
	}
	defer win32.CloseClipboard()
	text, reading := readGuardedText(winClipboard{})
	return text, reading == clipText
}

// writeClipboardText replaces the clipboard with text. temporary marks it as not for
// clipboard history (used for the paste of a conversion).
func writeClipboardText(owner uintptr, text string, temporary bool) bool {
	if !openClipboard(owner) {
		return false
	}
	defer win32.CloseClipboard()
	win32.EmptyClipboard()
	// Windows expects CRLF line breaks in CF_UNICODETEXT.
	normalized := strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\n", "\r\n")
	units := append(utf16.Encode([]rune(normalized)), 0)
	data := make([]byte, 2*len(units))
	for i, unit := range units {
		data[2*i], data[2*i+1] = byte(unit), byte(unit>>8)
	}
	handle := newGlobal(data)
	if handle == 0 || !win32.SetClipboardData(win32.CF_UNICODETEXT, handle) {
		if handle != 0 {
			win32.GlobalFree(handle)
		}
		return false
	}
	if temporary {
		for _, name := range historyExclusions {
			if format := win32.RegisterClipboardFormat(name); format != 0 {
				if flag := newGlobal([]byte{0, 0, 0, 0}); flag != 0 && !win32.SetClipboardData(format, flag) {
					win32.GlobalFree(flag)
				}
			}
		}
	}
	return true
}
