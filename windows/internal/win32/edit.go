//go:build windows && (amd64 || arm64)

package win32

// Reading and editing standard Edit and RichEdit controls in other processes. Every message
// used here is below WM_USER, so the system marshals its buffers across processes; the
// RichEdit-only messages (EM_EXGETSEL, EM_GETTEXTEX, …) are above it and are not usable
// from outside the process.

import (
	"syscall"
	"unsafe"
)

const (
	WM_GETTEXT       = 0x000D
	WM_GETTEXTLENGTH = 0x000E
	EM_GETSEL        = 0x00B0
	EM_SETSEL        = 0x00B1
	EM_GETLINECOUNT  = 0x00BA
	EM_LINEINDEX     = 0x00BB
	EM_LINELENGTH    = 0x00C1
	EM_REPLACESEL    = 0x00C2
	ES_READONLY      = 0x0800
	SMTO_ABORTIFHUNG = 0x0002
	SMTO_ERRORONEXIT = 0x0020
)

var (
	procSendMessageTimeoutW = user32.NewProc("SendMessageTimeoutW")
	procRealGetWindowClassW = user32.NewProc("RealGetWindowClassW")
)

// SendMessageTimeout sends a message and waits at most timeoutMs for the answer, giving up
// at once on a hung window. ok is false on timeout or failure.
func SendMessageTimeout(hwnd uintptr, msg uint32, wParam, lParam uintptr, timeoutMs uint32) (uintptr, bool) {
	var result uintptr
	r, _, _ := procSendMessageTimeoutW.Call(hwnd, uintptr(msg), wParam, lParam,
		SMTO_ABORTIFHUNG|SMTO_ERRORONEXIT, uintptr(timeoutMs), uintptr(unsafe.Pointer(&result)))
	return result, r != 0
}

// RealGetWindowClass is the class a window's behaviour comes from: "Edit" for a superclassed
// edit control (a WinForms TextBox, a Delphi TEdit) whose own class name differs.
func RealGetWindowClass(hwnd uintptr) string {
	var buf [256]uint16
	n, _, _ := procRealGetWindowClassW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 || n > uintptr(len(buf)) {
		return ""
	}
	return syscall.UTF16ToString(buf[:n])
}

// editTimeout bounds every call into another process: a hung application must never hang
// us (the engine waits meanwhile, and with it every keystroke behind it).
const editTimeout = 100

// EditTextLength is WM_GETTEXTLENGTH.
func EditTextLength(hwnd uintptr) (int, bool) {
	n, ok := SendMessageTimeout(hwnd, WM_GETTEXTLENGTH, 0, 0, editTimeout)
	return int(n), ok
}

// EditText reads the whole text (WM_GETTEXT) as UTF-16 units, provided it is at most max
// units long.
//
// The buffer has room for twice the reported length: whether a rich edit's
// WM_GETTEXTLENGTH counts a paragraph break as one unit or as its CR LF, the text fits. A
// text that fills the buffer anyway grew between the two messages and is refused rather
// than returned cut short.
func EditText(hwnd uintptr, max int) ([]uint16, bool) {
	length, ok := EditTextLength(hwnd)
	if !ok || length < 0 || length > max {
		return nil, false
	}
	buf := make([]uint16, 2*length+2)
	n, ok := SendMessageTimeout(hwnd, WM_GETTEXT, uintptr(len(buf)), uintptr(unsafe.Pointer(&buf[0])), editTimeout)
	if !ok || int(n) >= len(buf)-1 {
		return nil, false
	}
	return buf[:n], true
}

// EditSelection is EM_GETSEL through its return value (start in the low word, end in the
// high word), which is exact below 65536; above it the control answers -1, refused here.
func EditSelection(hwnd uintptr) (start, end int, ok bool) {
	r, ok := SendMessageTimeout(hwnd, EM_GETSEL, 0, 0, editTimeout)
	if !ok || uint32(r) == 0xFFFFFFFF {
		return 0, 0, false
	}
	return int(r & 0xFFFF), int(r >> 16 & 0xFFFF), true
}

// EditPositionLength is the length of the text in the control's own character positions:
// the index of the last line plus its length. Edit controls and RichEdit 1.0 count a line
// break as two positions (CR LF), RichEdit 2.0 and later as one (CR); comparing this with
// the text WM_GETTEXT returns tells which.
func EditPositionLength(hwnd uintptr) (int, bool) {
	lines, ok := SendMessageTimeout(hwnd, EM_GETLINECOUNT, 0, 0, editTimeout)
	if !ok || lines == 0 {
		return 0, false
	}
	start, ok := SendMessageTimeout(hwnd, EM_LINEINDEX, lines-1, 0, editTimeout)
	if !ok || int32(start) < 0 {
		return 0, false
	}
	length, ok := SendMessageTimeout(hwnd, EM_LINELENGTH, start, 0, editTimeout)
	if !ok || int32(length) < 0 {
		return 0, false
	}
	return int(int32(start)) + int(int32(length)), true
}

// EditSetSelection is EM_SETSEL.
func EditSetSelection(hwnd uintptr, start, end int) bool {
	_, ok := SendMessageTimeout(hwnd, EM_SETSEL, uintptr(start), uintptr(end), editTimeout)
	return ok
}

// EditReplaceSelection is EM_REPLACESEL with undo allowed, so the control's own Ctrl+Z
// takes the correction back.
func EditReplaceSelection(hwnd uintptr, text string) bool {
	units, err := syscall.UTF16FromString(text)
	if err != nil {
		return false
	}
	_, ok := SendMessageTimeout(hwnd, EM_REPLACESEL, 1, uintptr(unsafe.Pointer(&units[0])), editTimeout)
	return ok
}
