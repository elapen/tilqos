//go:build windows && (amd64 || arm64)

package winapp

// The suggestion hint: the counterpart of SuggestionPanel.swift.
//
// It must never take focus — a window that activates would steal the caret from the field
// the user is typing into — so it is a WS_EX_NOACTIVATE tool window, shown without
// activation, click-through (WS_EX_TRANSPARENT + HTTRANSPARENT), topmost, and layered for
// a touch of translucency. The accept key is taken by the keyboard hook, not by the popup.

import (
	"syscall"
	"unsafe"

	"tilqos/internal/core"
	"tilqos/internal/engine"
	"tilqos/internal/win32"
)

var sizeofWNDCLASSEXW = unsafe.Sizeof(win32.WNDCLASSEXW{})

const popupTimerID = 1

// Popup is the suggestion window. Used on the UI thread only.
type Popup struct {
	ui      *UI
	hwnd    uintptr
	visible bool
	id      uint64

	before, after, badge string
	dpi                  uint32
	dark                 bool
	width, height        int32
}

func newPopup(ui *UI) (*Popup, error) {
	p := &Popup{ui: ui, dpi: 96}
	hwnd, err := win32.CreateWindowEx(
		win32.WS_EX_TOOLWINDOW|win32.WS_EX_TOPMOST|win32.WS_EX_NOACTIVATE|win32.WS_EX_LAYERED|win32.WS_EX_TRANSPARENT,
		popupClassName, nil, win32.WS_POPUP, 0, 0, 10, 10, 0, 0, ui.instance)
	if err != nil {
		return nil, err
	}
	p.hwnd = hwnd
	win32.SetLayeredWindowAttributes(hwnd, 248)
	return p, nil
}

func (p *Popup) destroy() {
	if p.hwnd != 0 {
		win32.DestroyWindow(p.hwnd)
		p.hwnd = 0
	}
}

// trimmed keeps the end of the text, which is what sits at the caret (like the macOS panel).
func trimmed(text string, limit int) string {
	collapsed := []rune{}
	for _, r := range text {
		if r == '\n' || r == '\r' || r == '\t' {
			r = ' '
		}
		collapsed = append(collapsed, r)
	}
	characters := core.Graphemes(string(collapsed))
	if len(characters) <= limit {
		return string(collapsed)
	}
	out := "…"
	for _, c := range characters[len(characters)-limit:] {
		out += c
	}
	return out
}

// Brand accent, matching the app icon (the macOS panel's #514FE6).
var accentColor = win32.RGB(0x51, 0x4F, 0xE6)

type palette struct {
	background, border, secondary, primary uint32
}

func (p *Popup) palette() palette {
	if p.dark {
		return palette{win32.RGB(0x2C, 0x2C, 0x2E), win32.RGB(0x48, 0x48, 0x4A), win32.RGB(0x98, 0x98, 0x9D), win32.RGB(0xFF, 0xFF, 0xFF)}
	}
	return palette{win32.RGB(0xFB, 0xFB, 0xFD), win32.RGB(0xD1, 0xD1, 0xD6), win32.RGB(0x86, 0x86, 0x8B), win32.RGB(0x1C, 0x1C, 0x1E)}
}

// scale converts 96-DPI pixels to the monitor's.
func (p *Popup) scale(v int32) int32 { return int32((int64(v)*int64(p.dpi) + 48) / 96) }

type popupFonts struct{ secondary, primary, badge uintptr }

func (p *Popup) createFonts() popupFonts {
	// Negative heights are character heights: 9 pt, 10 pt and 8 pt at 72 points per inch.
	height := func(points int32) int32 { return -int32((int64(points)*int64(p.dpi) + 36) / 72) }
	return popupFonts{
		secondary: win32.CreateFont(height(9), win32.FW_NORMAL, "Segoe UI"),
		primary:   win32.CreateFont(height(10), win32.FW_SEMIBOLD, "Segoe UI"),
		badge:     win32.CreateFont(height(8), win32.FW_BOLD, "Segoe UI"),
	}
}

func (f popupFonts) delete() {
	win32.DeleteObject(f.secondary)
	win32.DeleteObject(f.primary)
	win32.DeleteObject(f.badge)
}

type popupLayout struct {
	wBefore, wArrow, wAfter, wBadge, hBadge int32
}

func (p *Popup) measure(hdc uintptr, fonts popupFonts) popupLayout {
	var l popupLayout
	old := win32.SelectObject(hdc, fonts.secondary)
	if p.before != "" {
		l.wBefore = win32.TextExtent(hdc, p.before).CX
		l.wArrow = win32.TextExtent(hdc, "→").CX
	}
	win32.SelectObject(hdc, fonts.primary)
	l.wAfter = win32.TextExtent(hdc, p.after).CX
	win32.SelectObject(hdc, fonts.badge)
	badge := win32.TextExtent(hdc, p.badge)
	l.wBadge = badge.CX + 2*p.scale(6)
	l.hBadge = badge.CY + 2*p.scale(2)
	win32.SelectObject(hdc, old)
	return l
}

// Show presents a suggestion below the caret (or the mouse pointer when the caret is
// unknown), on the right monitor, at the right DPI.
func (p *Popup) Show(s engine.Suggestion, caret win32.RECT, hasCaret bool) {
	anchor := win32.POINT{X: caret.Left, Y: caret.Bottom}
	if !hasCaret {
		cursor, _ := win32.GetCursorPos()
		anchor = win32.POINT{X: cursor.X + 12, Y: cursor.Y + 20}
		caret = win32.RECT{Left: anchor.X, Top: cursor.Y, Right: anchor.X, Bottom: anchor.Y}
	}
	monitor := win32.MonitorFromPoint(anchor)
	p.dpi = win32.GetDpiForMonitor(monitor)
	p.dark = appsUseDarkTheme()
	p.before = trimmed(s.Before, 42)
	p.after = trimmed(s.After, 42)
	p.badge = s.AcceptLabel
	p.id = s.ID

	// Measure with the fonts the paint will use.
	screen := win32.GetDC(0)
	memory := win32.CreateCompatibleDC(screen)
	fonts := p.createFonts()
	l := p.measure(memory, fonts)
	fonts.delete()
	win32.DeleteDC(memory)
	win32.ReleaseDC(0, screen)

	gap := p.scale(7)
	width := p.scale(12) + l.wAfter + gap + l.wBadge + p.scale(8)
	if p.before != "" {
		width += l.wBefore + gap + l.wArrow + gap
	}
	height := max(p.scale(30), l.hBadge+2*p.scale(7))
	p.width, p.height = width, height

	// Just below the caret, nudged back on screen, flipped above when there is no room.
	x, y := anchor.X, caret.Bottom+p.scale(6)
	if info, ok := win32.GetMonitorInfo(monitor); ok {
		work := info.RcWork
		if y+height > work.Bottom-4 {
			y = caret.Top - p.scale(6) - height
		}
		x = min(max(x, work.Left+4), work.Right-width-4)
		y = min(max(y, work.Top+4), work.Bottom-height-4)
	}

	radius := p.scale(16)
	win32.SetWindowRgn(p.hwnd, win32.CreateRoundRectRgn(0, 0, width+1, height+1, radius, radius), false)
	win32.SetWindowPos(p.hwnd, win32.HWND_TOPMOST, x, y, width, height,
		win32.SWP_NOACTIVATE|win32.SWP_SHOWWINDOW|win32.SWP_NOOWNERZORDER)
	win32.InvalidateRect(p.hwnd, true)
	p.visible = true

	win32.KillTimer(p.hwnd, popupTimerID)
	if s.Timeout > 0 {
		win32.SetTimer(p.hwnd, popupTimerID, uint32(s.Timeout.Milliseconds()))
	}
}

// Hide takes the popup down.
func (p *Popup) Hide() {
	win32.KillTimer(p.hwnd, popupTimerID)
	if p.visible {
		win32.ShowWindow(p.hwnd, win32.SW_HIDE)
		p.visible = false
	}
	// A hidden popup must not keep the words it showed in memory until the next suggestion.
	p.before, p.after, p.badge = "", "", ""
}

func (p *Popup) handle(msg uint32, wParam, lParam uintptr) (uintptr, bool) {
	switch msg {
	case win32.WM_PAINT:
		p.paint()
		return 0, true
	case win32.WM_ERASEBKGND:
		return 1, true // everything is painted in WM_PAINT
	case win32.WM_NCHITTEST:
		transparent := int32(win32.HTTRANSPARENT)
		return uintptr(transparent), true // clicks go to the window underneath
	case win32.WM_MOUSEACTIVATE:
		return win32.MA_NOACTIVATE, true
	case win32.WM_TIMER:
		if wParam == popupTimerID {
			id := p.id
			p.Hide()
			p.ui.app.send(timeoutEvent{id: id})
			return 0, true
		}
	}
	return 0, false
}

func (p *Popup) paint() {
	var ps win32.PAINTSTRUCT
	hdc := win32.BeginPaint(p.hwnd, &ps)
	if hdc == 0 {
		return
	}
	defer win32.EndPaint(p.hwnd, &ps)

	colors := p.palette()
	full := win32.RECT{Right: p.width, Bottom: p.height}
	background := win32.CreateSolidBrush(colors.background)
	win32.FillRect(hdc, &full, background)
	win32.DeleteObject(background)

	// A hairline border along the rounded edge.
	radius := p.scale(16)
	pen := win32.CreatePen(1, colors.border)
	oldPen := win32.SelectObject(hdc, pen)
	oldBrush := win32.SelectObject(hdc, win32.GetStockObject(win32.NULL_BRUSH))
	win32.RoundRect(hdc, 0, 0, p.width, p.height, radius, radius)
	win32.SelectObject(hdc, oldBrush)
	win32.SelectObject(hdc, oldPen)
	win32.DeleteObject(pen)

	fonts := p.createFonts()
	defer fonts.delete()
	oldFont := win32.SelectObject(hdc, fonts.secondary)
	defer win32.SelectObject(hdc, oldFont)
	l := p.measure(hdc, fonts)
	win32.SetBkMode(hdc, win32.TRANSPARENT)
	const line = win32.DT_SINGLELINE | win32.DT_VCENTER | win32.DT_NOPREFIX
	gap := p.scale(7)
	x := p.scale(12)

	// Showing what it was next to what it would become lets the user judge the suggestion
	// without having to remember what they typed.
	if p.before != "" {
		win32.SelectObject(hdc, fonts.secondary)
		win32.SetTextColor(hdc, colors.secondary)
		r := win32.RECT{Left: x, Right: x + l.wBefore, Bottom: p.height}
		win32.DrawText(hdc, p.before, &r, line)
		x += l.wBefore + gap
		r = win32.RECT{Left: x, Right: x + l.wArrow, Bottom: p.height}
		win32.DrawText(hdc, "→", &r, line)
		x += l.wArrow + gap
	}
	win32.SelectObject(hdc, fonts.primary)
	win32.SetTextColor(hdc, colors.primary)
	r := win32.RECT{Left: x, Right: x + l.wAfter, Bottom: p.height}
	win32.DrawText(hdc, p.after, &r, line)
	x += l.wAfter + gap

	// The accept key as a filled badge.
	top := (p.height - l.hBadge) / 2
	accent := win32.CreateSolidBrush(accentColor)
	accentPen := win32.CreatePen(1, accentColor)
	oldBrush = win32.SelectObject(hdc, accent)
	oldPen = win32.SelectObject(hdc, accentPen)
	badgeRadius := p.scale(10)
	win32.RoundRect(hdc, x, top, x+l.wBadge, top+l.hBadge, badgeRadius, badgeRadius)
	win32.SelectObject(hdc, oldBrush)
	win32.SelectObject(hdc, oldPen)
	win32.DeleteObject(accent)
	win32.DeleteObject(accentPen)
	win32.SelectObject(hdc, fonts.badge)
	win32.SetTextColor(hdc, win32.RGB(0xFF, 0xFF, 0xFF))
	r = win32.RECT{Left: x, Top: top, Right: x + l.wBadge, Bottom: top + l.hBadge}
	win32.DrawText(hdc, p.badge, &r, line|win32.DT_CENTER)
}

// appsUseDarkTheme reads the "Choose your app mode" setting.
func appsUseDarkTheme() bool {
	var key syscall.Handle
	path := win32.UTF16Ptr(`Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`)
	if syscall.RegOpenKeyEx(syscall.HKEY_CURRENT_USER, path, 0, syscall.KEY_QUERY_VALUE, &key) != nil {
		return false
	}
	defer syscall.RegCloseKey(key)
	var value, kind uint32
	size := uint32(4)
	if syscall.RegQueryValueEx(key, win32.UTF16Ptr("AppsUseLightTheme"), nil, &kind, (*byte)(unsafe.Pointer(&value)), &size) != nil {
		return false
	}
	return kind == win32.REG_DWORD && value == 0
}
