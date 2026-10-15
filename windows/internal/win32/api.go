//go:build windows && (amd64 || arm64)

package win32

import (
	"syscall"
	"unsafe"
)

// DLLs. SetDefaultDllDirectories(LOAD_LIBRARY_SEARCH_SYSTEM32) is called at start-up
// (see SecureDLLSearch) before any of the non-KnownDLLs (shcore, oleacc) load, so none of
// them can be planted next to the executable.
var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	advapi32 = syscall.NewLazyDLL("advapi32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	shcore   = syscall.NewLazyDLL("shcore.dll")
	oleacc   = syscall.NewLazyDLL("oleacc.dll")
	ole32    = syscall.NewLazyDLL("ole32.dll")
	wtsapi32 = syscall.NewLazyDLL("wtsapi32.dll")
)

var (
	procSetWindowsHookExW             = user32.NewProc("SetWindowsHookExW")
	procUnhookWindowsHookEx           = user32.NewProc("UnhookWindowsHookEx")
	procCallNextHookEx                = user32.NewProc("CallNextHookEx")
	procGetMessageW                   = user32.NewProc("GetMessageW")
	procTranslateMessage              = user32.NewProc("TranslateMessage")
	procDispatchMessageW              = user32.NewProc("DispatchMessageW")
	procPostThreadMessageW            = user32.NewProc("PostThreadMessageW")
	procPostMessageW                  = user32.NewProc("PostMessageW")
	procSendMessageW                  = user32.NewProc("SendMessageW")
	procPostQuitMessage               = user32.NewProc("PostQuitMessage")
	procSetWinEventHook               = user32.NewProc("SetWinEventHook")
	procUnhookWinEvent                = user32.NewProc("UnhookWinEvent")
	procGetForegroundWindow           = user32.NewProc("GetForegroundWindow")
	procGetWindowThreadProcessId      = user32.NewProc("GetWindowThreadProcessId")
	procGetGUIThreadInfo              = user32.NewProc("GetGUIThreadInfo")
	procGetKeyboardLayout             = user32.NewProc("GetKeyboardLayout")
	procGetKeyboardLayoutList         = user32.NewProc("GetKeyboardLayoutList")
	procToUnicodeEx                   = user32.NewProc("ToUnicodeEx")
	procMapVirtualKeyExW              = user32.NewProc("MapVirtualKeyExW")
	procGetAsyncKeyState              = user32.NewProc("GetAsyncKeyState")
	procGetKeyState                   = user32.NewProc("GetKeyState")
	procSendInput                     = user32.NewProc("SendInput")
	procRegisterClassExW              = user32.NewProc("RegisterClassExW")
	procCreateWindowExW               = user32.NewProc("CreateWindowExW")
	procDefWindowProcW                = user32.NewProc("DefWindowProcW")
	procDestroyWindow                 = user32.NewProc("DestroyWindow")
	procShowWindow                    = user32.NewProc("ShowWindow")
	procSetWindowPos                  = user32.NewProc("SetWindowPos")
	procInvalidateRect                = user32.NewProc("InvalidateRect")
	procBeginPaint                    = user32.NewProc("BeginPaint")
	procEndPaint                      = user32.NewProc("EndPaint")
	procGetDC                         = user32.NewProc("GetDC")
	procReleaseDC                     = user32.NewProc("ReleaseDC")
	procSetLayeredWindowAttributes    = user32.NewProc("SetLayeredWindowAttributes")
	procSetWindowRgn                  = user32.NewProc("SetWindowRgn")
	procSetTimer                      = user32.NewProc("SetTimer")
	procKillTimer                     = user32.NewProc("KillTimer")
	procLoadCursorW                   = user32.NewProc("LoadCursorW")
	procCreateIconFromResourceEx      = user32.NewProc("CreateIconFromResourceEx")
	procDestroyIcon                   = user32.NewProc("DestroyIcon")
	procCreatePopupMenu               = user32.NewProc("CreatePopupMenu")
	procAppendMenuW                   = user32.NewProc("AppendMenuW")
	procTrackPopupMenu                = user32.NewProc("TrackPopupMenu")
	procDestroyMenu                   = user32.NewProc("DestroyMenu")
	procSetForegroundWindow           = user32.NewProc("SetForegroundWindow")
	procRegisterWindowMessageW        = user32.NewProc("RegisterWindowMessageW")
	procGetCursorPos                  = user32.NewProc("GetCursorPos")
	procClientToScreen                = user32.NewProc("ClientToScreen")
	procMonitorFromPoint              = user32.NewProc("MonitorFromPoint")
	procWindowFromPoint               = user32.NewProc("WindowFromPoint")
	procGetMonitorInfoW               = user32.NewProc("GetMonitorInfoW")
	procGetClassNameW                 = user32.NewProc("GetClassNameW")
	procGetWindowLongPtrW             = user32.NewProc("GetWindowLongPtrW")
	procFindWindowExW                 = user32.NewProc("FindWindowExW")
	procMessageBoxW                   = user32.NewProc("MessageBoxW")
	procOpenClipboard                 = user32.NewProc("OpenClipboard")
	procCloseClipboard                = user32.NewProc("CloseClipboard")
	procEmptyClipboard                = user32.NewProc("EmptyClipboard")
	procGetClipboardData              = user32.NewProc("GetClipboardData")
	procSetClipboardData              = user32.NewProc("SetClipboardData")
	procEnumClipboardFormats          = user32.NewProc("EnumClipboardFormats")
	procGetClipboardSequenceNumber    = user32.NewProc("GetClipboardSequenceNumber")
	procGetClipboardFormatNameW       = user32.NewProc("GetClipboardFormatNameW")
	procRegisterClipboardFormatW      = user32.NewProc("RegisterClipboardFormatW")
	procGetLastInputInfo              = user32.NewProc("GetLastInputInfo")
	procSetProcessDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")
	procSetProcessDPIAware            = user32.NewProc("SetProcessDPIAware")
	procGetDpiForSystem               = user32.NewProc("GetDpiForSystem")
	procMessageBeep                   = user32.NewProc("MessageBeep")
	procGetSystemMetrics              = user32.NewProc("GetSystemMetrics")
	procDrawTextW                     = user32.NewProc("DrawTextW")
	procFillRect                      = user32.NewProc("FillRect")
	procGetAncestor                   = user32.NewProc("GetAncestor")
	procIsWindow                      = user32.NewProc("IsWindow")

	procCreateFontW           = gdi32.NewProc("CreateFontW")
	procSelectObject          = gdi32.NewProc("SelectObject")
	procDeleteObject          = gdi32.NewProc("DeleteObject")
	procSetTextColor          = gdi32.NewProc("SetTextColor")
	procSetBkMode             = gdi32.NewProc("SetBkMode")
	procCreateSolidBrush      = gdi32.NewProc("CreateSolidBrush")
	procCreatePen             = gdi32.NewProc("CreatePen")
	procRoundRect             = gdi32.NewProc("RoundRect")
	procCreateRoundRectRgn    = gdi32.NewProc("CreateRoundRectRgn")
	procGetTextExtentPoint32W = gdi32.NewProc("GetTextExtentPoint32W")
	procCreateCompatibleDC    = gdi32.NewProc("CreateCompatibleDC")
	procDeleteDC              = gdi32.NewProc("DeleteDC")
	procGetStockObject        = gdi32.NewProc("GetStockObject")

	procGetModuleHandleW           = kernel32.NewProc("GetModuleHandleW")
	procCreateMutexW               = kernel32.NewProc("CreateMutexW")
	procOpenProcess                = kernel32.NewProc("OpenProcess")
	procQueryFullProcessImageNameW = kernel32.NewProc("QueryFullProcessImageNameW")
	procGetCurrentThreadId         = kernel32.NewProc("GetCurrentThreadId")
	procGlobalAlloc                = kernel32.NewProc("GlobalAlloc")
	procGlobalLock                 = kernel32.NewProc("GlobalLock")
	procGlobalUnlock               = kernel32.NewProc("GlobalUnlock")
	procGlobalSize                 = kernel32.NewProc("GlobalSize")
	procGlobalFree                 = kernel32.NewProc("GlobalFree")
	procSetThreadPriority          = kernel32.NewProc("SetThreadPriority")
	procGetCurrentThread           = kernel32.NewProc("GetCurrentThread")
	procGetLocaleInfoW             = kernel32.NewProc("GetLocaleInfoW")
	procGetTickCount               = kernel32.NewProc("GetTickCount")
	procSetDefaultDllDirectories   = kernel32.NewProc("SetDefaultDllDirectories")

	procRegSetValueExW  = advapi32.NewProc("RegSetValueExW")
	procRegDeleteValueW = advapi32.NewProc("RegDeleteValueW")

	procShellNotifyIconW = shell32.NewProc("Shell_NotifyIconW")
	procShellExecuteW    = shell32.NewProc("ShellExecuteW")

	procGetDpiForMonitor       = shcore.NewProc("GetDpiForMonitor")
	procSetProcessDpiAwareness = shcore.NewProc("SetProcessDpiAwareness")

	procWTSRegisterSessionNotification   = wtsapi32.NewProc("WTSRegisterSessionNotification")
	procWTSUnRegisterSessionNotification = wtsapi32.NewProc("WTSUnRegisterSessionNotification")

	procAccessibleObjectFromWindow = oleacc.NewProc("AccessibleObjectFromWindow")
	procCoInitializeEx             = ole32.NewProc("CoInitializeEx")
	procCoUninitialize             = ole32.NewProc("CoUninitialize")
)

// PtrAt reinterprets an address handed to us by Windows (a hook's lParam, a locked global
// memory block) as a *T. Going through the address of the variable, rather than
// converting the uintptr directly, keeps `go vet`'s unsafeptr check meaningful for the
// rest of the code; the memory is not Go-managed, so the collector is not involved.
func PtrAt[T any](address uintptr) *T {
	return *(**T)(unsafe.Pointer(&address))
}

// UTF16Ptr converts a Go string for an API call. Strings with NUL are truncated at it.
func UTF16Ptr(s string) *uint16 {
	for i := 0; i < len(s); i++ {
		if s[i] == 0 {
			s = s[:i]
			break
		}
	}
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

func boolArg(b bool) uintptr {
	if b {
		return 1
	}
	return 0
}

// MARK: - DLL safety

// SecureDLLSearch restricts DLL loading to System32 (Windows 8+, and Windows 7 with
// KB2533623). Returns false when the call is unavailable.
func SecureDLLSearch() bool {
	if procSetDefaultDllDirectories.Find() != nil {
		return false
	}
	r, _, _ := procSetDefaultDllDirectories.Call(LOAD_LIBRARY_SEARCH_SYSTEM32)
	return r != 0
}

// MARK: - Hooks and messages

func SetWindowsHookEx(idHook int32, fn, hmod uintptr, threadID uint32) (uintptr, error) {
	r, _, err := procSetWindowsHookExW.Call(uintptr(idHook), fn, hmod, uintptr(threadID))
	if r == 0 {
		return 0, err
	}
	return r, nil
}

func UnhookWindowsHookEx(hook uintptr) bool {
	r, _, _ := procUnhookWindowsHookEx.Call(hook)
	return r != 0
}

func CallNextHookEx(nCode, wParam, lParam uintptr) uintptr {
	r, _, _ := procCallNextHookEx.Call(0, nCode, wParam, lParam)
	return r
}

// GetMessage returns 1 for a message, 0 for WM_QUIT and -1 on error.
func GetMessage(msg *MSG, hwnd uintptr) int32 {
	r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(msg)), hwnd, 0, 0)
	return int32(r)
}

func TranslateMessage(msg *MSG) {
	procTranslateMessage.Call(uintptr(unsafe.Pointer(msg)))
}

func DispatchMessage(msg *MSG) {
	procDispatchMessageW.Call(uintptr(unsafe.Pointer(msg)))
}

func PostThreadMessage(threadID uint32, msg uint32, wParam, lParam uintptr) bool {
	r, _, _ := procPostThreadMessageW.Call(uintptr(threadID), uintptr(msg), wParam, lParam)
	return r != 0
}

func PostMessage(hwnd uintptr, msg uint32, wParam, lParam uintptr) bool {
	r, _, _ := procPostMessageW.Call(hwnd, uintptr(msg), wParam, lParam)
	return r != 0
}

func SendMessage(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	r, _, _ := procSendMessageW.Call(hwnd, uintptr(msg), wParam, lParam)
	return r
}

func PostQuitMessage(code int32) {
	procPostQuitMessage.Call(uintptr(code))
}

func SetWinEventHook(eventMin, eventMax uint32, fn uintptr, flags uint32) uintptr {
	r, _, _ := procSetWinEventHook.Call(uintptr(eventMin), uintptr(eventMax), 0, fn, 0, 0, uintptr(flags))
	return r
}

func UnhookWinEvent(hook uintptr) bool {
	r, _, _ := procUnhookWinEvent.Call(hook)
	return r != 0
}

func GetCurrentThreadId() uint32 {
	r, _, _ := procGetCurrentThreadId.Call()
	return uint32(r)
}

func SetCurrentThreadPriority(priority int32) bool {
	thread, _, _ := procGetCurrentThread.Call()
	r, _, _ := procSetThreadPriority.Call(thread, uintptr(priority))
	return r != 0
}

func GetLastInputTick() uint32 {
	info := LASTINPUTINFO{CbSize: uint32(unsafe.Sizeof(LASTINPUTINFO{}))}
	procGetLastInputInfo.Call(uintptr(unsafe.Pointer(&info)))
	return info.DwTime
}

func GetTickCount() uint32 {
	r, _, _ := procGetTickCount.Call()
	return uint32(r)
}

// MARK: - Windows and threads

func GetForegroundWindow() uintptr {
	r, _, _ := procGetForegroundWindow.Call()
	return r
}

// GetWindowThreadProcessId returns the thread and process that own a window.
func GetWindowThreadProcessId(hwnd uintptr) (thread, process uint32) {
	var pid uint32
	r, _, _ := procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	return uint32(r), pid
}

func GetGUIThreadInfo(threadID uint32) (GUITHREADINFO, bool) {
	info := GUITHREADINFO{CbSize: uint32(unsafe.Sizeof(GUITHREADINFO{}))}
	r, _, _ := procGetGUIThreadInfo.Call(uintptr(threadID), uintptr(unsafe.Pointer(&info)))
	return info, r != 0
}

func GetClassName(hwnd uintptr) string {
	var buf [256]uint16
	n, _, _ := procGetClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return syscall.UTF16ToString(buf[:n])
}

func GetWindowLongPtr(hwnd uintptr, index int32) uintptr {
	r, _, _ := procGetWindowLongPtrW.Call(hwnd, uintptr(index))
	return r
}

func FindWindowEx(parent, after uintptr, class string) uintptr {
	r, _, _ := procFindWindowExW.Call(parent, after, uintptr(unsafe.Pointer(UTF16Ptr(class))), 0)
	return r
}

func GetAncestor(hwnd uintptr, flags uint32) uintptr {
	r, _, _ := procGetAncestor.Call(hwnd, uintptr(flags))
	return r
}

func IsWindow(hwnd uintptr) bool {
	r, _, _ := procIsWindow.Call(hwnd)
	return r != 0
}

func GetModuleHandle() uintptr {
	r, _, _ := procGetModuleHandleW.Call(0)
	return r
}

// MARK: - Keyboard

func GetKeyboardLayout(threadID uint32) uintptr {
	r, _, _ := procGetKeyboardLayout.Call(uintptr(threadID))
	return r
}

// GetKeyboardLayoutList returns the installed input locales, in the user's order.
func GetKeyboardLayoutList() []uintptr {
	n, _, _ := procGetKeyboardLayoutList.Call(0, 0)
	if n == 0 {
		return nil
	}
	list := make([]uintptr, n)
	got, _, _ := procGetKeyboardLayoutList.Call(n, uintptr(unsafe.Pointer(&list[0])))
	return list[:got]
}

// ToUnicodeEx translates a key under a given keyboard state and layout.
func ToUnicodeEx(vk, scan uint32, state *[256]byte, buf []uint16, flags uint32, hkl uintptr) int32 {
	r, _, _ := procToUnicodeEx.Call(uintptr(vk), uintptr(scan), uintptr(unsafe.Pointer(state)),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), uintptr(flags), hkl)
	return int32(r)
}

func MapVirtualKeyEx(code, mapType uint32, hkl uintptr) uint32 {
	r, _, _ := procMapVirtualKeyExW.Call(uintptr(code), uintptr(mapType), hkl)
	return uint32(r)
}

// KeyDown reports whether a key is physically held (GetAsyncKeyState high bit).
func KeyDown(vk int32) bool {
	r, _, _ := procGetAsyncKeyState.Call(uintptr(vk))
	return int16(r) < 0
}

// KeyToggled reports a toggle key's state (GetKeyState low bit).
func KeyToggled(vk int32) bool {
	r, _, _ := procGetKeyState.Call(uintptr(vk))
	return r&1 != 0
}

// SendInput injects events and returns how many were accepted.
func SendInput(inputs []INPUT) (uint32, error) {
	if len(inputs) == 0 {
		return 0, nil
	}
	r, _, err := procSendInput.Call(uintptr(len(inputs)), uintptr(unsafe.Pointer(&inputs[0])), unsafe.Sizeof(INPUT{}))
	if uint32(r) != uint32(len(inputs)) {
		return uint32(r), err
	}
	return uint32(r), nil
}

// MARK: - Window classes and windows

func RegisterClassEx(class *WNDCLASSEXW) (uint16, error) {
	r, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(class)))
	if r == 0 {
		return 0, err
	}
	return uint16(r), nil
}

func CreateWindowEx(exStyle uint32, class, title *uint16, style uint32, x, y, w, h int32, parent, menu, instance uintptr) (uintptr, error) {
	r, _, err := procCreateWindowExW.Call(uintptr(exStyle), uintptr(unsafe.Pointer(class)),
		uintptr(unsafe.Pointer(title)), uintptr(style), uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		parent, menu, instance, 0)
	if r == 0 {
		return 0, err
	}
	return r, nil
}

func DefWindowProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return r
}

func DestroyWindow(hwnd uintptr) bool {
	r, _, _ := procDestroyWindow.Call(hwnd)
	return r != 0
}

func ShowWindow(hwnd uintptr, cmd int32) bool {
	r, _, _ := procShowWindow.Call(hwnd, uintptr(cmd))
	return r != 0
}

func SetWindowPos(hwnd, after uintptr, x, y, w, h int32, flags uint32) bool {
	r, _, _ := procSetWindowPos.Call(hwnd, after, uintptr(x), uintptr(y), uintptr(w), uintptr(h), uintptr(flags))
	return r != 0
}

func InvalidateRect(hwnd uintptr, erase bool) {
	procInvalidateRect.Call(hwnd, 0, boolArg(erase))
}

func BeginPaint(hwnd uintptr, ps *PAINTSTRUCT) uintptr {
	r, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(ps)))
	return r
}

func EndPaint(hwnd uintptr, ps *PAINTSTRUCT) {
	procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(ps)))
}

func GetDC(hwnd uintptr) uintptr {
	r, _, _ := procGetDC.Call(hwnd)
	return r
}

func ReleaseDC(hwnd, hdc uintptr) {
	procReleaseDC.Call(hwnd, hdc)
}

func SetLayeredWindowAttributes(hwnd uintptr, alpha byte) bool {
	r, _, _ := procSetLayeredWindowAttributes.Call(hwnd, 0, uintptr(alpha), LWA_ALPHA)
	return r != 0
}

// SetWindowRgn hands the region to the system, which owns it afterwards.
func SetWindowRgn(hwnd, region uintptr, redraw bool) {
	procSetWindowRgn.Call(hwnd, region, boolArg(redraw))
}

func SetTimer(hwnd, id uintptr, milliseconds uint32) uintptr {
	r, _, _ := procSetTimer.Call(hwnd, id, uintptr(milliseconds), 0)
	return r
}

func KillTimer(hwnd, id uintptr) {
	procKillTimer.Call(hwnd, id)
}

func LoadArrowCursor() uintptr {
	r, _, _ := procLoadCursorW.Call(0, IDC_ARROW)
	return r
}

// CreateIconFromResourceEx builds an icon from one ICO image (BMP or PNG data).
func CreateIconFromResourceEx(data []byte, cx, cy int32) uintptr {
	if len(data) == 0 {
		return 0
	}
	r, _, _ := procCreateIconFromResourceEx.Call(uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)),
		1, 0x00030000, uintptr(cx), uintptr(cy), 0)
	return r
}

func DestroyIcon(icon uintptr) {
	procDestroyIcon.Call(icon)
}

// WTSRegisterSessionNotification asks for WM_WTSSESSION_CHANGE messages (lock, unlock,
// disconnect) for this session on hwnd.
func WTSRegisterSessionNotification(hwnd uintptr, flags uint32) bool {
	r, _, _ := procWTSRegisterSessionNotification.Call(hwnd, uintptr(flags))
	return r != 0
}

// WTSUnRegisterSessionNotification ends what WTSRegisterSessionNotification started.
func WTSUnRegisterSessionNotification(hwnd uintptr) bool {
	r, _, _ := procWTSUnRegisterSessionNotification.Call(hwnd)
	return r != 0
}

func RegisterWindowMessage(name string) uint32 {
	r, _, _ := procRegisterWindowMessageW.Call(uintptr(unsafe.Pointer(UTF16Ptr(name))))
	return uint32(r)
}

func MessageBox(hwnd uintptr, text, caption string, flags uint32) int32 {
	r, _, _ := procMessageBoxW.Call(hwnd, uintptr(unsafe.Pointer(UTF16Ptr(text))),
		uintptr(unsafe.Pointer(UTF16Ptr(caption))), uintptr(flags))
	return int32(r)
}

func MessageBeep(kind uint32) {
	procMessageBeep.Call(uintptr(kind))
}

func GetSystemMetrics(index int32) int32 {
	r, _, _ := procGetSystemMetrics.Call(uintptr(index))
	return int32(r)
}

// MARK: - Menus

func CreatePopupMenu() uintptr {
	r, _, _ := procCreatePopupMenu.Call()
	return r
}

func AppendMenu(menu uintptr, flags uint32, id uintptr, text string) {
	var p *uint16
	if flags&MF_SEPARATOR == 0 {
		p = UTF16Ptr(text)
	}
	procAppendMenuW.Call(menu, uintptr(flags), id, uintptr(unsafe.Pointer(p)))
}

// TrackPopupMenu shows a menu and, with TPM_RETURNCMD, returns the chosen command (0 for none).
func TrackPopupMenu(menu uintptr, flags uint32, x, y int32, hwnd uintptr) uint32 {
	r, _, _ := procTrackPopupMenu.Call(menu, uintptr(flags), uintptr(x), uintptr(y), 0, hwnd, 0)
	return uint32(r)
}

func DestroyMenu(menu uintptr) {
	procDestroyMenu.Call(menu)
}

func SetForegroundWindow(hwnd uintptr) bool {
	r, _, _ := procSetForegroundWindow.Call(hwnd)
	return r != 0
}

// MARK: - Geometry, monitors, DPI

func GetCursorPos() (POINT, bool) {
	var pt POINT
	r, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	return pt, r != 0
}

func ClientToScreen(hwnd uintptr, pt *POINT) bool {
	r, _, _ := procClientToScreen.Call(hwnd, uintptr(unsafe.Pointer(pt)))
	return r != 0
}

// MonitorFromPoint takes the POINT by value: an 8-byte struct travels in one register,
// X in the low half, on both x64 and ARM64.
func MonitorFromPoint(pt POINT) uintptr {
	packed := uintptr(uint32(pt.X)) | uintptr(uint32(pt.Y))<<32
	r, _, _ := procMonitorFromPoint.Call(packed, MONITOR_DEFAULTTONEAREST)
	return r
}

// WindowFromPoint takes the POINT by value, packed like MonitorFromPoint's.
func WindowFromPoint(pt POINT) uintptr {
	packed := uintptr(uint32(pt.X)) | uintptr(uint32(pt.Y))<<32
	r, _, _ := procWindowFromPoint.Call(packed)
	return r
}

func GetMonitorInfo(monitor uintptr) (MONITORINFO, bool) {
	info := MONITORINFO{CbSize: uint32(unsafe.Sizeof(MONITORINFO{}))}
	r, _, _ := procGetMonitorInfoW.Call(monitor, uintptr(unsafe.Pointer(&info)))
	return info, r != 0
}

// GetDpiForMonitor returns the effective DPI of a monitor (96 when unknown).
func GetDpiForMonitor(monitor uintptr) uint32 {
	if procGetDpiForMonitor.Find() == nil {
		var x, y uint32
		r, _, _ := procGetDpiForMonitor.Call(monitor, MDT_EFFECTIVE_DPI, uintptr(unsafe.Pointer(&x)), uintptr(unsafe.Pointer(&y)))
		if r == 0 && x != 0 {
			return x
		}
	}
	if procGetDpiForSystem.Find() == nil {
		if r, _, _ := procGetDpiForSystem.Call(); r != 0 {
			return uint32(r)
		}
	}
	return 96
}

// EnablePerMonitorDPI makes the process per-monitor DPI aware (v2 where available).
func EnablePerMonitorDPI() string {
	if procSetProcessDpiAwarenessContext.Find() == nil {
		if r, _, _ := procSetProcessDpiAwarenessContext.Call(DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2); r != 0 {
			return "per-monitor v2"
		}
	}
	if procSetProcessDpiAwareness.Find() == nil {
		if r, _, _ := procSetProcessDpiAwareness.Call(PROCESS_PER_MONITOR_DPI_AWARE); r == 0 {
			return "per-monitor"
		}
	}
	if procSetProcessDPIAware.Find() == nil {
		if r, _, _ := procSetProcessDPIAware.Call(); r != 0 {
			return "system"
		}
	}
	return "unaware"
}

// MARK: - GDI

func CreateFont(height int32, weight int32, face string) uintptr {
	r, _, _ := procCreateFontW.Call(uintptr(height), 0, 0, 0, uintptr(weight), 0, 0, 0,
		DEFAULT_CHARSET, OUT_DEFAULT_PRECIS, CLIP_DEFAULT_PRECIS, CLEARTYPE_QUALITY, DEFAULT_PITCH,
		uintptr(unsafe.Pointer(UTF16Ptr(face))))
	return r
}

func SelectObject(hdc, object uintptr) uintptr {
	r, _, _ := procSelectObject.Call(hdc, object)
	return r
}

func DeleteObject(object uintptr) {
	procDeleteObject.Call(object)
}

func SetTextColor(hdc uintptr, color uint32) {
	procSetTextColor.Call(hdc, uintptr(color))
}

func SetBkMode(hdc uintptr, mode int32) {
	procSetBkMode.Call(hdc, uintptr(mode))
}

func CreateSolidBrush(color uint32) uintptr {
	r, _, _ := procCreateSolidBrush.Call(uintptr(color))
	return r
}

func CreatePen(width int32, color uint32) uintptr {
	r, _, _ := procCreatePen.Call(PS_SOLID, uintptr(width), uintptr(color))
	return r
}

func RoundRect(hdc uintptr, left, top, right, bottom, w, h int32) {
	procRoundRect.Call(hdc, uintptr(left), uintptr(top), uintptr(right), uintptr(bottom), uintptr(w), uintptr(h))
}

func CreateRoundRectRgn(left, top, right, bottom, w, h int32) uintptr {
	r, _, _ := procCreateRoundRectRgn.Call(uintptr(left), uintptr(top), uintptr(right), uintptr(bottom), uintptr(w), uintptr(h))
	return r
}

// TextExtent measures a string in the DC's current font.
func TextExtent(hdc uintptr, text string) SIZE {
	var size SIZE
	units, _ := syscall.UTF16FromString(text)
	if len(units) <= 1 {
		return size
	}
	procGetTextExtentPoint32W.Call(hdc, uintptr(unsafe.Pointer(&units[0])), uintptr(len(units)-1), uintptr(unsafe.Pointer(&size)))
	return size
}

func DrawText(hdc uintptr, text string, rect *RECT, format uint32) {
	units, _ := syscall.UTF16FromString(text)
	if len(units) <= 1 {
		return
	}
	procDrawTextW.Call(hdc, uintptr(unsafe.Pointer(&units[0])), uintptr(len(units)-1), uintptr(unsafe.Pointer(rect)), uintptr(format))
}

func FillRect(hdc uintptr, rect *RECT, brush uintptr) {
	procFillRect.Call(hdc, uintptr(unsafe.Pointer(rect)), brush)
}

func CreateCompatibleDC(hdc uintptr) uintptr {
	r, _, _ := procCreateCompatibleDC.Call(hdc)
	return r
}

func DeleteDC(hdc uintptr) {
	procDeleteDC.Call(hdc)
}

func GetStockObject(index int32) uintptr {
	r, _, _ := procGetStockObject.Call(uintptr(index))
	return r
}

// RGB builds a COLORREF.
func RGB(r, g, b byte) uint32 { return uint32(r) | uint32(g)<<8 | uint32(b)<<16 }

// MARK: - Processes

func CreateMutex(name string) (uintptr, error) {
	r, _, err := procCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(UTF16Ptr(name))))
	if r == 0 {
		return 0, err
	}
	if errno, ok := err.(syscall.Errno); ok && errno == ERROR_ALREADY_EXISTS {
		return r, errno
	}
	return r, nil
}

func OpenProcess(access uint32, pid uint32) (syscall.Handle, error) {
	r, _, err := procOpenProcess.Call(uintptr(access), 0, uintptr(pid))
	if r == 0 {
		return 0, err
	}
	return syscall.Handle(r), nil
}

// ProcessImagePath is the full path of a process's executable.
func ProcessImagePath(process syscall.Handle) string {
	var buf [1024]uint16
	size := uint32(len(buf))
	r, _, _ := procQueryFullProcessImageNameW.Call(uintptr(process), 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if r == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:size])
}

// LocaleDisplayName is the localized name of a language ID ("Русский (Россия)").
func LocaleDisplayName(langID uint16) string {
	var buf [128]uint16
	n, _, _ := procGetLocaleInfoW.Call(uintptr(langID), LOCALE_SLOCALIZEDDISPLAYNAME, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:n])
}

// MARK: - Global memory and the clipboard

func GlobalAlloc(size int) uintptr {
	r, _, _ := procGlobalAlloc.Call(GMEM_MOVEABLE, uintptr(size))
	return r
}

func GlobalLock(h uintptr) uintptr {
	r, _, _ := procGlobalLock.Call(h)
	return r
}

func GlobalUnlock(h uintptr) {
	procGlobalUnlock.Call(h)
}

func GlobalSize(h uintptr) int {
	r, _, _ := procGlobalSize.Call(h)
	return int(r)
}

func GlobalFree(h uintptr) {
	procGlobalFree.Call(h)
}

func OpenClipboard(owner uintptr) bool {
	r, _, _ := procOpenClipboard.Call(owner)
	return r != 0
}

func CloseClipboard() {
	procCloseClipboard.Call()
}

func EmptyClipboard() bool {
	r, _, _ := procEmptyClipboard.Call()
	return r != 0
}

func GetClipboardData(format uint32) uintptr {
	r, _, _ := procGetClipboardData.Call(uintptr(format))
	return r
}

func SetClipboardData(format uint32, h uintptr) bool {
	r, _, _ := procSetClipboardData.Call(uintptr(format), h)
	return r != 0
}

func EnumClipboardFormats(format uint32) uint32 {
	r, _, _ := procEnumClipboardFormats.Call(uintptr(format))
	return uint32(r)
}

func GetClipboardSequenceNumber() uint32 {
	r, _, _ := procGetClipboardSequenceNumber.Call()
	return uint32(r)
}

func GetClipboardFormatName(format uint32) string {
	var buf [256]uint16
	n, _, _ := procGetClipboardFormatNameW.Call(uintptr(format), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return syscall.UTF16ToString(buf[:n])
}

func RegisterClipboardFormat(name string) uint32 {
	r, _, _ := procRegisterClipboardFormatW.Call(uintptr(unsafe.Pointer(UTF16Ptr(name))))
	return uint32(r)
}

// MARK: - Registry (what syscall lacks)

func RegSetString(key syscall.Handle, name, value string) error {
	data, err := syscall.UTF16FromString(value)
	if err != nil {
		return err
	}
	r, _, _ := procRegSetValueExW.Call(uintptr(key), uintptr(unsafe.Pointer(UTF16Ptr(name))), 0, REG_SZ,
		uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)*2))
	if r != 0 {
		return syscall.Errno(r)
	}
	return nil
}

func RegDeleteValue(key syscall.Handle, name string) error {
	r, _, _ := procRegDeleteValueW.Call(uintptr(key), uintptr(unsafe.Pointer(UTF16Ptr(name))))
	if r != 0 {
		return syscall.Errno(r)
	}
	return nil
}

// MARK: - Shell

func ShellNotifyIcon(message uint32, data *NOTIFYICONDATAW) bool {
	r, _, _ := procShellNotifyIconW.Call(uintptr(message), uintptr(unsafe.Pointer(data)))
	return r != 0
}

// ShellOpen opens a file or folder with its default handler.
func ShellOpen(path string) {
	procShellExecuteW.Call(0, uintptr(unsafe.Pointer(UTF16Ptr("open"))), uintptr(unsafe.Pointer(UTF16Ptr(path))), 0, 0, SW_SHOWNORMAL)
}

// MARK: - COM / accessibility

func CoInitializeApartment() bool {
	if procCoInitializeEx.Find() != nil {
		return false
	}
	r, _, _ := procCoInitializeEx.Call(0, COINIT_APARTMENTTHREADED)
	return int32(r) >= 0
}

// CoInitializeMultithreaded joins the multithreaded apartment: calls on proxies need no
// message pump on this thread.
func CoInitializeMultithreaded() bool {
	if procCoInitializeEx.Find() != nil {
		return false
	}
	r, _, _ := procCoInitializeEx.Call(0, COINIT_MULTITHREADED)
	return int32(r) >= 0
}

func CoUninitialize() {
	procCoUninitialize.Call()
}

// CaretFromAccessibility asks a window's accessibility object for its caret rectangle
// (MSAA OBJID_CARET → IAccessible::accLocation), in screen coordinates. Works for apps
// that draw their own caret but implement MSAA (Chromium, Firefox, many others). Must run
// on a thread that called CoInitializeApartment.
func CaretFromAccessibility(hwnd uintptr) (RECT, bool) {
	if procAccessibleObjectFromWindow.Find() != nil {
		return RECT{}, false
	}
	var object uintptr
	caret := int32(OBJID_CARET)
	hr, _, _ := procAccessibleObjectFromWindow.Call(hwnd, uintptr(uint32(caret)),
		uintptr(unsafe.Pointer(&IID_IAccessible)), uintptr(unsafe.Pointer(&object)))
	if int32(hr) < 0 || object == 0 {
		return RECT{}, false
	}
	// The object is an IAccessible*: its first word points at the vtable.
	vtable := *PtrAt[uintptr](object)
	method := func(index int) uintptr {
		return *PtrAt[uintptr](vtable + uintptr(index)*unsafe.Sizeof(uintptr(0)))
	}
	defer syscall.SyscallN(method(2), object) // IUnknown::Release

	var left, top, width, height int32
	child := VARIANT{Vt: VT_I4, Val: CHILDID_SELF}
	// IAccessible::accLocation is vtable slot 22. The VARIANT is 24 bytes, so both the x64
	// and the ARM64 conventions pass it by reference to a caller-owned copy.
	hr, _, _ = syscall.SyscallN(method(22), object,
		uintptr(unsafe.Pointer(&left)), uintptr(unsafe.Pointer(&top)),
		uintptr(unsafe.Pointer(&width)), uintptr(unsafe.Pointer(&height)),
		uintptr(unsafe.Pointer(&child)))
	if int32(hr) != 0 || (width == 0 && height == 0) {
		return RECT{}, false
	}
	return RECT{Left: left, Top: top, Right: left + width, Bottom: top + height}, true
}
