//go:build windows && (amd64 || arm64)

package winapp

// The notification-area icon: the counterpart of the macOS status item.

import (
	"unicode/utf16"
	"unsafe"

	"tilqos/internal/win32"
)

// Tray is used on the UI thread only.
type Tray struct {
	ui      *UI
	iconOn  uintptr
	iconOff uintptr
	enabled bool
	tip     string
	added   bool
}

func newTray(ui *UI) *Tray {
	size := win32.GetSystemMetrics(win32.SM_CXSMICON)
	if size <= 0 {
		size = 16
	}
	t := &Tray{ui: ui, enabled: true, tip: "Tilqos"}
	t.iconOn = loadIcon("app.ico", size)
	t.iconOff = loadIcon("app-off.ico", size)
	t.add()
	return t
}

// loadIcon builds an HICON from the embedded .ico, at the size closest to the one asked for.
func loadIcon(name string, size int32) uintptr {
	data, err := IconAsset(name)
	if err != nil {
		return 0
	}
	images, err := ParseICO(data)
	if err != nil {
		return 0
	}
	image, ok := BestIcon(images, int(size))
	if !ok {
		return 0
	}
	return win32.CreateIconFromResourceEx(image.Data, size, size)
}

func copyUTF16(dst []uint16, text string) {
	units := utf16.Encode([]rune(text))
	if len(units) > len(dst)-1 {
		units = units[:len(dst)-1]
	}
	n := copy(dst, units)
	dst[n] = 0
}

func (t *Tray) data() win32.NOTIFYICONDATAW {
	var d win32.NOTIFYICONDATAW
	d.CbSize = uint32(unsafe.Sizeof(d))
	d.HWnd = t.ui.hwnd
	d.UID = 1
	d.UFlags = win32.NIF_MESSAGE | win32.NIF_ICON | win32.NIF_TIP
	d.UCallbackMessage = wmTray
	d.HIcon = t.iconOn
	if !t.enabled {
		d.HIcon = t.iconOff
	}
	copyUTF16(d.SzTip[:], t.tip)
	return d
}

func (t *Tray) add() {
	d := t.data()
	t.added = win32.ShellNotifyIcon(win32.NIM_ADD, &d)
	if !t.added {
		// Already there (after a TaskbarCreated race): update instead.
		t.added = win32.ShellNotifyIcon(win32.NIM_MODIFY, &d)
	}
}

// Update refreshes the icon and tooltip.
func (t *Tray) Update(enabled bool, tip string) {
	t.enabled, t.tip = enabled, tip
	d := t.data()
	if !win32.ShellNotifyIcon(win32.NIM_MODIFY, &d) {
		t.add()
	}
}

// Balloon shows a notification next to the icon (a toast on Windows 10 and 11).
func (t *Tray) Balloon(title, text string, warning bool) {
	d := t.data()
	d.UFlags |= win32.NIF_INFO
	copyUTF16(d.SzInfoTitle[:], title)
	copyUTF16(d.SzInfo[:], text)
	d.DwInfoFlags = win32.NIIF_INFO | win32.NIIF_NOSOUND
	if warning {
		d.DwInfoFlags = win32.NIIF_WARNING | win32.NIIF_NOSOUND
	}
	win32.ShellNotifyIcon(win32.NIM_MODIFY, &d)
}

func (t *Tray) remove() {
	if t.added {
		d := t.data()
		win32.ShellNotifyIcon(win32.NIM_DELETE, &d)
		t.added = false
	}
}

func (t *Tray) onNotify(event uint32) {
	switch event {
	case win32.WM_LBUTTONUP, win32.WM_RBUTTONUP, win32.WM_CONTEXTMENU:
		t.ui.app.showMenu()
	}
}
