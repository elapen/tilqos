//go:build windows && (amd64 || arm64)

package winapp

// The UI thread: the main goroutine, locked to the process's main OS thread. It owns the
// hidden main window (tray icon owner, clipboard owner), the suggestion popup and the tray
// menu, and runs the message loop. Other goroutines reach it through Post/Call, never by
// touching its windows directly.

import (
	"errors"
	"sync"
	"syscall"
	"time"

	"tilqos/internal/win32"
)

const (
	wmTray = win32.WM_APP + 1 // tray icon notifications
	wmRun  = win32.WM_APP + 2 // run queued closures
)

// Class names live for the whole process: RegisterClassEx keeps pointing at them.
var (
	mainClassName  = win32.UTF16Ptr("TilqosMain")
	popupClassName = win32.UTF16Ptr("TilqosSuggestion")
	windowTitle    = win32.UTF16Ptr("Tilqos")
)

// One window procedure for every window we own (callbacks are a limited resource).
var windowProcCallback = syscall.NewCallback(windowProc)

// theUI lets the window procedure find the UI; set before the first window is created.
var theUI *UI

// UI is the UI thread's state.
type UI struct {
	app            *App
	instance       uintptr
	hwnd           uintptr
	threadID       uint32
	taskbarCreated uint32
	popup          *Popup
	tray           *Tray

	mu    sync.Mutex
	queue []func()
}

func newUI(app *App) (*UI, error) {
	ui := &UI{app: app, instance: win32.GetModuleHandle(), threadID: win32.GetCurrentThreadId()}
	theUI = ui

	cursor := win32.LoadArrowCursor()
	for _, class := range []*win32.WNDCLASSEXW{
		{LpfnWndProc: windowProcCallback, HInstance: ui.instance, HCursor: cursor, LpszClassName: mainClassName},
		{LpfnWndProc: windowProcCallback, HInstance: ui.instance, HCursor: cursor, LpszClassName: popupClassName},
	} {
		class.CbSize = uint32(sizeofWNDCLASSEXW)
		if _, err := win32.RegisterClassEx(class); err != nil {
			return nil, err
		}
	}

	hwnd, err := win32.CreateWindowEx(win32.WS_EX_TOOLWINDOW, mainClassName, windowTitle,
		win32.WS_OVERLAPPED, 0, 0, 0, 0, 0, 0, ui.instance)
	if err != nil {
		return nil, err
	}
	ui.hwnd = hwnd
	// Lock, disconnect and sleep arrive as messages to this window; see handle.
	win32.WTSRegisterSessionNotification(hwnd, win32.NOTIFY_FOR_THIS_SESSION)
	// Explorer broadcasts this after it restarts; the tray icon has to be added again.
	ui.taskbarCreated = win32.RegisterWindowMessage("TaskbarCreated")

	if ui.popup, err = newPopup(ui); err != nil {
		return nil, err
	}
	ui.tray = newTray(ui)
	return ui, nil
}

func windowProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	if ui := theUI; ui != nil {
		if result, handled := ui.handle(hwnd, uint32(msg), wParam, lParam); handled {
			return result
		}
	}
	return win32.DefWindowProc(hwnd, uint32(msg), wParam, lParam)
}

func (ui *UI) handle(hwnd uintptr, msg uint32, wParam, lParam uintptr) (uintptr, bool) {
	// Windows being created have no handle recorded yet: default processing.
	if ui.popup != nil && hwnd == ui.popup.hwnd && hwnd != 0 {
		return ui.popup.handle(msg, wParam, lParam)
	}
	if hwnd != ui.hwnd || hwnd == 0 {
		return 0, false
	}
	switch msg {
	case wmRun:
		ui.drain()
		return 0, true
	case wmTray:
		ui.tray.onNotify(uint32(lParam))
		return 0, true
	case win32.WM_CLOSE:
		// Nobody closes the hidden window but us.
		return 0, true
	case win32.WM_WTSSESSION_CHANGE:
		switch wParam {
		case win32.WTS_SESSION_LOCK, win32.WTS_CONSOLE_DISCONNECT, win32.WTS_REMOTE_DISCONNECT:
			ui.app.wipeSessionMemory("lock")
		}
		return 0, true
	case win32.WM_POWERBROADCAST:
		if wParam == win32.PBT_APMSUSPEND {
			ui.app.wipeSessionMemory("sleep")
		}
		// The system's default answer stands for every power message.
		return 0, false
	case win32.WM_ENDSESSION:
		// Logoff or shutdown: the process may be ended as soon as this returns, before any
		// deferred cleanup runs, so what was learned is written now.
		if wParam != 0 && ui.app.learned != nil {
			if err := ui.app.learned.Flush(); err != nil {
				ui.app.log.Printf("saving learned words at logoff failed: %v", err)
			}
		}
		return 0, true
	}
	if msg == ui.taskbarCreated && msg != 0 {
		ui.tray.add()
		return 0, true
	}
	return 0, false
}

// Post runs fn on the UI thread, asynchronously.
func (ui *UI) Post(fn func()) {
	ui.mu.Lock()
	ui.queue = append(ui.queue, fn)
	ui.mu.Unlock()
	win32.PostMessage(ui.hwnd, wmRun, 0, 0)
}

var errUITimeout = errors.New("the UI thread did not respond")

// Call runs fn on the UI thread and waits for it (with a safety timeout). The UI thread
// never waits for other goroutines, so this cannot deadlock.
func (ui *UI) Call(fn func()) error {
	if win32.GetCurrentThreadId() == ui.threadID {
		fn()
		return nil
	}
	done := make(chan struct{})
	ui.Post(func() {
		fn()
		close(done)
	})
	select {
	case <-done:
		return nil
	case <-time.After(5 * time.Second):
		return errUITimeout
	}
}

func (ui *UI) drain() {
	ui.mu.Lock()
	queue := ui.queue
	ui.queue = nil
	ui.mu.Unlock()
	for _, fn := range queue {
		fn()
	}
}

// Loop pumps messages until Quit.
func (ui *UI) Loop() {
	var msg win32.MSG
	for win32.GetMessage(&msg, 0) > 0 {
		win32.TranslateMessage(&msg)
		win32.DispatchMessage(&msg)
	}
}

// Quit ends the message loop.
func (ui *UI) Quit() { win32.PostQuitMessage(0) }

// Close removes the tray icon and destroys the windows.
func (ui *UI) Close() {
	win32.WTSUnRegisterSessionNotification(ui.hwnd)
	ui.tray.remove()
	ui.popup.destroy()
	win32.DestroyWindow(ui.hwnd)
}

// messageBox shows a modal message owned by the hidden window.
func messageBox(text string, flags uint32) {
	win32.MessageBox(0, text, "Tilqos", flags|win32.MB_SETFOREGROUND)
}

// confirm asks a yes/no question; No is the default button.
func confirm(text string) bool {
	return win32.MessageBox(0, text, "Tilqos",
		win32.MB_YESNO|win32.MB_ICONQUESTION|win32.MB_DEFBUTTON2|win32.MB_SETFOREGROUND) == win32.IDYES
}
