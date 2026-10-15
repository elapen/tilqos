//go:build windows && (amd64 || arm64)

// Package win32 holds the raw Win32 declarations Tilqos uses: DLL procedures,
// constants and structures, with nothing but the standard library (syscall + unsafe).
//
// Only 64-bit Windows (amd64, arm64) is supported, and every structure that crosses the
// API boundary carries compile-time size and offset assertions below. A wrong layout does
// not compile, on either architecture, instead of corrupting memory at run time.
package win32

import "unsafe"

// POINT is a Win32 POINT.
type POINT struct {
	X, Y int32
}

// RECT is a Win32 RECT.
type RECT struct {
	Left, Top, Right, Bottom int32
}

// SIZE is a Win32 SIZE.
type SIZE struct {
	CX, CY int32
}

// MSG is a Win32 MSG.
type MSG struct {
	Hwnd     uintptr
	Message  uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	Pt       POINT
	LPrivate uint32
}

// KBDLLHOOKSTRUCT is what a WH_KEYBOARD_LL hook receives.
type KBDLLHOOKSTRUCT struct {
	VkCode      uint32
	ScanCode    uint32
	Flags       uint32
	Time        uint32
	DwExtraInfo uintptr
}

// MSLLHOOKSTRUCT is what a WH_MOUSE_LL hook receives.
type MSLLHOOKSTRUCT struct {
	Pt          POINT
	MouseData   uint32
	Flags       uint32
	Time        uint32
	DwExtraInfo uintptr
}

// KEYBDINPUT is the keyboard member of INPUT's union.
type KEYBDINPUT struct {
	WVk         uint16
	WScan       uint16
	DwFlags     uint32
	Time        uint32
	DwExtraInfo uintptr
}

// INPUT is a Win32 INPUT with the keyboard member of the union. The union is as large as
// its largest member, MOUSEINPUT (32 bytes on 64-bit), hence the trailing padding.
type INPUT struct {
	Type uint32
	Ki   KEYBDINPUT
	_    [8]byte
}

// GUITHREADINFO is a Win32 GUITHREADINFO.
type GUITHREADINFO struct {
	CbSize        uint32
	Flags         uint32
	HwndActive    uintptr
	HwndFocus     uintptr
	HwndCapture   uintptr
	HwndMenuOwner uintptr
	HwndMoveSize  uintptr
	HwndCaret     uintptr
	RcCaret       RECT
}

// WNDCLASSEXW is a Win32 WNDCLASSEXW.
type WNDCLASSEXW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

// GUID is a Win32 GUID.
type GUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

// NOTIFYICONDATAW is the full (Vista+) NOTIFYICONDATAW.
type NOTIFYICONDATAW struct {
	CbSize           uint32
	HWnd             uintptr
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            uintptr
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32 // union with uTimeout
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         GUID
	HBalloonIcon     uintptr
}

// MONITORINFO is a Win32 MONITORINFO.
type MONITORINFO struct {
	CbSize    uint32
	RcMonitor RECT
	RcWork    RECT
	DwFlags   uint32
}

// LASTINPUTINFO is a Win32 LASTINPUTINFO.
type LASTINPUTINFO struct {
	CbSize uint32
	DwTime uint32
}

// PAINTSTRUCT is a Win32 PAINTSTRUCT.
type PAINTSTRUCT struct {
	Hdc         uintptr
	FErase      int32
	RcPaint     RECT
	FRestore    int32
	FIncUpdate  int32
	RgbReserved [32]byte
}

// TOKEN_ELEVATION is a Win32 TOKEN_ELEVATION.
type TOKEN_ELEVATION struct {
	TokenIsElevated uint32
}

// VARIANT is an OLE VARIANT (only the VT_I4 member is used).
type VARIANT struct {
	Vt         uint16
	WReserved1 uint16
	WReserved2 uint16
	WReserved3 uint16
	Val        int64
	_          uintptr // the BRECORD member makes the union two pointers wide
}

// MARK: - Layout assertions
//
// `var _ [0]struct{} = [N - want]struct{}{}` compiles only when N == want: a larger N
// yields an array that is not [0]struct{}, a smaller one a negative constant, which
// overflows uintptr. Offsets are asserted wherever padding is involved.

var (
	_ [0]struct{} = [unsafe.Sizeof(POINT{}) - 8]struct{}{}
	_ [0]struct{} = [unsafe.Sizeof(RECT{}) - 16]struct{}{}
	_ [0]struct{} = [unsafe.Sizeof(SIZE{}) - 8]struct{}{}

	_ [0]struct{} = [unsafe.Sizeof(MSG{}) - 48]struct{}{}
	_ [0]struct{} = [unsafe.Offsetof(MSG{}.Message) - 8]struct{}{}
	_ [0]struct{} = [unsafe.Offsetof(MSG{}.WParam) - 16]struct{}{}
	_ [0]struct{} = [unsafe.Offsetof(MSG{}.LParam) - 24]struct{}{}
	_ [0]struct{} = [unsafe.Offsetof(MSG{}.Time) - 32]struct{}{}
	_ [0]struct{} = [unsafe.Offsetof(MSG{}.Pt) - 36]struct{}{}
	_ [0]struct{} = [unsafe.Offsetof(MSG{}.LPrivate) - 44]struct{}{}

	_ [0]struct{} = [unsafe.Sizeof(KBDLLHOOKSTRUCT{}) - 24]struct{}{}
	_ [0]struct{} = [unsafe.Offsetof(KBDLLHOOKSTRUCT{}.DwExtraInfo) - 16]struct{}{}

	_ [0]struct{} = [unsafe.Sizeof(MSLLHOOKSTRUCT{}) - 32]struct{}{}
	_ [0]struct{} = [unsafe.Offsetof(MSLLHOOKSTRUCT{}.MouseData) - 8]struct{}{}
	_ [0]struct{} = [unsafe.Offsetof(MSLLHOOKSTRUCT{}.DwExtraInfo) - 24]struct{}{}

	_ [0]struct{} = [unsafe.Sizeof(KEYBDINPUT{}) - 24]struct{}{}
	_ [0]struct{} = [unsafe.Offsetof(KEYBDINPUT{}.DwFlags) - 4]struct{}{}
	_ [0]struct{} = [unsafe.Offsetof(KEYBDINPUT{}.DwExtraInfo) - 16]struct{}{}
	_ [0]struct{} = [unsafe.Sizeof(INPUT{}) - 40]struct{}{}
	_ [0]struct{} = [unsafe.Offsetof(INPUT{}.Ki) - 8]struct{}{}

	_ [0]struct{} = [unsafe.Sizeof(GUITHREADINFO{}) - 72]struct{}{}
	_ [0]struct{} = [unsafe.Offsetof(GUITHREADINFO{}.HwndActive) - 8]struct{}{}
	_ [0]struct{} = [unsafe.Offsetof(GUITHREADINFO{}.HwndCaret) - 48]struct{}{}
	_ [0]struct{} = [unsafe.Offsetof(GUITHREADINFO{}.RcCaret) - 56]struct{}{}

	_ [0]struct{} = [unsafe.Sizeof(WNDCLASSEXW{}) - 80]struct{}{}
	_ [0]struct{} = [unsafe.Offsetof(WNDCLASSEXW{}.LpfnWndProc) - 8]struct{}{}
	_ [0]struct{} = [unsafe.Offsetof(WNDCLASSEXW{}.CbClsExtra) - 16]struct{}{}
	_ [0]struct{} = [unsafe.Offsetof(WNDCLASSEXW{}.HInstance) - 24]struct{}{}
	_ [0]struct{} = [unsafe.Offsetof(WNDCLASSEXW{}.LpszClassName) - 64]struct{}{}
	_ [0]struct{} = [unsafe.Offsetof(WNDCLASSEXW{}.HIconSm) - 72]struct{}{}

	_ [0]struct{} = [unsafe.Sizeof(GUID{}) - 16]struct{}{}

	_ [0]struct{} = [unsafe.Sizeof(NOTIFYICONDATAW{}) - 976]struct{}{}
	_ [0]struct{} = [unsafe.Offsetof(NOTIFYICONDATAW{}.HWnd) - 8]struct{}{}
	_ [0]struct{} = [unsafe.Offsetof(NOTIFYICONDATAW{}.UID) - 16]struct{}{}
	_ [0]struct{} = [unsafe.Offsetof(NOTIFYICONDATAW{}.UCallbackMessage) - 24]struct{}{}
	_ [0]struct{} = [unsafe.Offsetof(NOTIFYICONDATAW{}.HIcon) - 32]struct{}{}
	_ [0]struct{} = [unsafe.Offsetof(NOTIFYICONDATAW{}.SzTip) - 40]struct{}{}
	_ [0]struct{} = [unsafe.Offsetof(NOTIFYICONDATAW{}.DwState) - 296]struct{}{}
	_ [0]struct{} = [unsafe.Offsetof(NOTIFYICONDATAW{}.SzInfo) - 304]struct{}{}
	_ [0]struct{} = [unsafe.Offsetof(NOTIFYICONDATAW{}.UVersion) - 816]struct{}{}
	_ [0]struct{} = [unsafe.Offsetof(NOTIFYICONDATAW{}.SzInfoTitle) - 820]struct{}{}
	_ [0]struct{} = [unsafe.Offsetof(NOTIFYICONDATAW{}.DwInfoFlags) - 948]struct{}{}
	_ [0]struct{} = [unsafe.Offsetof(NOTIFYICONDATAW{}.GuidItem) - 952]struct{}{}
	_ [0]struct{} = [unsafe.Offsetof(NOTIFYICONDATAW{}.HBalloonIcon) - 968]struct{}{}

	_ [0]struct{} = [unsafe.Sizeof(MONITORINFO{}) - 40]struct{}{}
	_ [0]struct{} = [unsafe.Offsetof(MONITORINFO{}.RcWork) - 20]struct{}{}

	_ [0]struct{} = [unsafe.Sizeof(LASTINPUTINFO{}) - 8]struct{}{}

	_ [0]struct{} = [unsafe.Sizeof(PAINTSTRUCT{}) - 72]struct{}{}
	_ [0]struct{} = [unsafe.Offsetof(PAINTSTRUCT{}.RcPaint) - 12]struct{}{}
	_ [0]struct{} = [unsafe.Offsetof(PAINTSTRUCT{}.RgbReserved) - 36]struct{}{}

	_ [0]struct{} = [unsafe.Sizeof(TOKEN_ELEVATION{}) - 4]struct{}{}

	_ [0]struct{} = [unsafe.Sizeof(VARIANT{}) - 24]struct{}{}
	_ [0]struct{} = [unsafe.Offsetof(VARIANT{}.Val) - 8]struct{}{}
)
