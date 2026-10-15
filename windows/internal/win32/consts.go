//go:build windows && (amd64 || arm64)

package win32

// Hooks and events.
const (
	WH_KEYBOARD_LL = 13
	WH_MOUSE_LL    = 14
	HC_ACTION      = 0

	LLKHF_EXTENDED = 0x01
	LLKHF_INJECTED = 0x10
	LLKHF_UP       = 0x80

	EVENT_SYSTEM_FOREGROUND = 0x0003
	EVENT_OBJECT_FOCUS      = 0x8005
	WINEVENT_OUTOFCONTEXT   = 0x0000
	WINEVENT_SKIPOWNPROCESS = 0x0002
	OBJID_CLIENT            = -4
	OBJID_CARET             = -8
)

// Window messages.
const (
	WM_NULL                   = 0x0000
	WM_CREATE                 = 0x0001
	WM_DESTROY                = 0x0002
	WM_CLOSE                  = 0x0010
	WM_ENDSESSION             = 0x0016
	WM_QUIT                   = 0x0012
	WM_PAINT                  = 0x000F
	WM_ERASEBKGND             = 0x0014
	WM_MOUSEACTIVATE          = 0x0021
	WM_INPUTLANGCHANGEREQUEST = 0x0050
	WM_NCHITTEST              = 0x0084
	WM_KEYDOWN                = 0x0100
	WM_KEYUP                  = 0x0101
	WM_SYSKEYDOWN             = 0x0104
	WM_SYSKEYUP               = 0x0105
	WM_COMMAND                = 0x0111
	WM_TIMER                  = 0x0113
	WM_LBUTTONDOWN            = 0x0201
	WM_LBUTTONUP              = 0x0202
	WM_RBUTTONDOWN            = 0x0204
	WM_RBUTTONUP              = 0x0205
	WM_MBUTTONDOWN            = 0x0207
	WM_XBUTTONDOWN            = 0x020B
	WM_CONTEXTMENU            = 0x007B
	WM_DPICHANGED             = 0x02E0
	WM_APP                    = 0x8000

	// Sleep and lock: the moments at which what was typed or corrected is wiped from memory.
	WM_POWERBROADCAST    = 0x0218
	PBT_APMSUSPEND       = 0x0004
	WM_WTSSESSION_CHANGE = 0x02B1
	// wParam of WM_WTSSESSION_CHANGE.
	WTS_CONSOLE_DISCONNECT = 0x2
	WTS_REMOTE_DISCONNECT  = 0x4
	WTS_SESSION_LOCK       = 0x7
	// Flag of WTSRegisterSessionNotification: only this session.
	NOTIFY_FOR_THIS_SESSION = 0

	HTTRANSPARENT = -1
	MA_NOACTIVATE = 3
)

// Window styles.
const (
	WS_POPUP                 = 0x80000000
	WS_OVERLAPPED            = 0x00000000
	WS_EX_TOPMOST            = 0x00000008
	WS_EX_TRANSPARENT        = 0x00000020
	WS_EX_TOOLWINDOW         = 0x00000080
	WS_EX_LAYERED            = 0x00080000
	WS_EX_NOACTIVATE         = 0x08000000
	CS_DROPSHADOW            = 0x00020000
	GWL_STYLE                = -16
	ES_PASSWORD              = 0x0020
	SW_HIDE                  = 0
	SW_SHOWNORMAL            = 1
	SW_SHOWNOACTIVATE        = 4
	SWP_NOACTIVATE           = 0x0010
	SWP_SHOWWINDOW           = 0x0040
	SWP_NOOWNERZORDER        = 0x0200
	LWA_ALPHA                = 0x00000002
	HWND_TOPMOST             = ^uintptr(0) // (HWND)-1
	GA_ROOT                  = 2
	IDC_ARROW                = 32512
	MONITOR_DEFAULTTONEAREST = 0x00000002
	MDT_EFFECTIVE_DPI        = 0
	SM_CXSMICON              = 49
	SM_CYSMICON              = 50
)

// Menus.
const (
	MF_STRING       = 0x00000000
	MF_GRAYED       = 0x00000001
	MF_DISABLED     = 0x00000002
	MF_CHECKED      = 0x00000008
	MF_POPUP        = 0x00000010
	MF_SEPARATOR    = 0x00000800
	MFT_RADIOCHECK  = 0x00000200
	TPM_RIGHTBUTTON = 0x0002
	TPM_NONOTIFY    = 0x0080
	TPM_RETURNCMD   = 0x0100
	TPM_BOTTOMALIGN = 0x0020
)

// Tray icon.
const (
	NIM_ADD      = 0x00000000
	NIM_MODIFY   = 0x00000001
	NIM_DELETE   = 0x00000002
	NIF_MESSAGE  = 0x00000001
	NIF_ICON     = 0x00000002
	NIF_TIP      = 0x00000004
	NIF_INFO     = 0x00000010
	NIIF_INFO    = 0x00000001
	NIIF_WARNING = 0x00000002
	NIIF_NOSOUND = 0x00000010
)

// Keyboard input.
const (
	INPUT_KEYBOARD        = 1
	KEYEVENTF_EXTENDEDKEY = 0x0001
	KEYEVENTF_KEYUP       = 0x0002
	KEYEVENTF_UNICODE     = 0x0004
	KEYEVENTF_SCANCODE    = 0x0008

	MAPVK_VK_TO_VSC    = 0
	MAPVK_VSC_TO_VK_EX = 3
	MAPVK_VK_TO_VSC_EX = 4

	// TOUNICODE_NO_STATE_CHANGE is ToUnicodeEx's "keyboard state is not changed" flag
	// (bit 2, Windows 10 1607+): translating a dead key does not arm it.
	TOUNICODE_NO_STATE_CHANGE = 0x4
)

// Virtual keys.
const (
	VK_BACK     = 0x08
	VK_TAB      = 0x09
	VK_CLEAR    = 0x0C
	VK_RETURN   = 0x0D
	VK_SHIFT    = 0x10
	VK_CONTROL  = 0x11
	VK_MENU     = 0x12
	VK_PAUSE    = 0x13
	VK_CAPITAL  = 0x14
	VK_ESCAPE   = 0x1B
	VK_SPACE    = 0x20
	VK_PRIOR    = 0x21
	VK_NEXT     = 0x22
	VK_END      = 0x23
	VK_HOME     = 0x24
	VK_LEFT     = 0x25
	VK_UP       = 0x26
	VK_RIGHT    = 0x27
	VK_DOWN     = 0x28
	VK_SNAPSHOT = 0x2C
	VK_INSERT   = 0x2D
	VK_DELETE   = 0x2E
	VK_LWIN     = 0x5B
	VK_RWIN     = 0x5C
	VK_APPS     = 0x5D
	VK_NUMPAD0  = 0x60
	VK_NUMPAD9  = 0x69
	VK_DECIMAL  = 0x6E
	VK_NUMLOCK  = 0x90
	VK_SCROLL   = 0x91
	VK_LSHIFT   = 0xA0
	VK_RSHIFT   = 0xA1
	VK_LCONTROL = 0xA2
	VK_RCONTROL = 0xA3
	VK_LMENU    = 0xA4
	VK_RMENU    = 0xA5
	// VK_MASK is unassigned (0xE8); injecting it between Alt down and Alt up stops the
	// release from opening the menu bar (AutoHotkey's "menu mask key").
	VK_MASK = 0xE8
)

// Clipboard.
const (
	CF_TEXT            = 1
	CF_BITMAP          = 2
	CF_METAFILEPICT    = 3
	CF_OEMTEXT         = 7
	CF_PALETTE         = 9
	CF_UNICODETEXT     = 13
	CF_ENHMETAFILE     = 14
	CF_OWNERDISPLAY    = 0x0080
	CF_DSPBITMAP       = 0x0082
	CF_DSPMETAFILEPICT = 0x0083
	CF_DSPENHMETAFILE  = 0x008E
	GMEM_MOVEABLE      = 0x0002
)

// Processes, tokens, registry, errors.
const (
	PROCESS_QUERY_LIMITED_INFORMATION = 0x1000
	TOKEN_QUERY                       = 0x0008
	TokenElevation                    = 20
	ERROR_ALREADY_EXISTS              = 183
	ERROR_ACCESS_DENIED               = 5
	HKEY_CURRENT_USER                 = 0x80000001
	KEY_QUERY_VALUE                   = 0x0001
	KEY_SET_VALUE                     = 0x0002
	REG_SZ                            = 1
	REG_DWORD                         = 4
	THREAD_PRIORITY_TIME_CRITICAL     = 15
	LOAD_LIBRARY_SEARCH_SYSTEM32      = 0x00000800
	LOCALE_SLOCALIZEDDISPLAYNAME      = 0x00000002
	MB_OK                             = 0x00000000
	MB_ICONINFORMATION                = 0x00000040
	MB_ICONWARNING                    = 0x00000030
	MB_ICONERROR                      = 0x00000010
	MB_SETFOREGROUND                  = 0x00010000
	MB_YESNO                          = 0x00000004
	MB_ICONQUESTION                   = 0x00000020
	MB_DEFBUTTON2                     = 0x00000100
	IDYES                             = 6
)

// GDI.
const (
	TRANSPARENT         = 1
	FW_NORMAL           = 400
	FW_SEMIBOLD         = 600
	FW_BOLD             = 700
	DEFAULT_CHARSET     = 1
	OUT_DEFAULT_PRECIS  = 0
	CLIP_DEFAULT_PRECIS = 0
	CLEARTYPE_QUALITY   = 5
	DEFAULT_PITCH       = 0
	DT_SINGLELINE       = 0x0020
	DT_VCENTER          = 0x0004
	DT_CENTER           = 0x0001
	DT_LEFT             = 0x0000
	DT_NOPREFIX         = 0x0800
	DT_END_ELLIPSIS     = 0x8000
	PS_SOLID            = 0
	NULL_BRUSH          = 5
)

// DPI.
const (
	// DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 is (DPI_AWARENESS_CONTEXT)-4.
	DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 = ^uintptr(3)
	PROCESS_PER_MONITOR_DPI_AWARE              = 2
)

// COM.
const (
	COINIT_MULTITHREADED     = 0x0
	COINIT_APARTMENTTHREADED = 0x2
	VT_I4                    = 3
	CHILDID_SELF             = 0
)

// IID_IAccessible is {618736E0-3C3D-11CF-810C-00AA00389B71}.
var IID_IAccessible = GUID{0x618736E0, 0x3C3D, 0x11CF, [8]byte{0x81, 0x0C, 0x00, 0xAA, 0x00, 0x38, 0x9B, 0x71}}
