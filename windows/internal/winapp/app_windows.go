//go:build windows && (amd64 || arm64)

// Package winapp is the Windows application: hooks, injection, clipboard, tray, popup and
// the glue that feeds the platform-neutral engine (the counterpart of AppController.swift,
// EventTap.swift, Injector.swift and the UI files of the macOS app).
package winapp

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"tilqos/internal/config"
	"tilqos/internal/core"
	"tilqos/internal/engine"
	"tilqos/internal/i18n"
	"tilqos/internal/models"
	"tilqos/internal/win32"
)

// Version is set at build time (-ldflags "-X tilqos/internal/winapp.Version=...").
var Version = "1.0.0"

type (
	timeoutEvent       struct{ id uint64 }
	configChangedEvent struct{}
)

// App ties everything together. It implements engine.Platform; those methods run on the
// engine goroutine unless noted.
type App struct {
	dir     string
	log     *Logger
	store   *config.Store
	models  *core.Models
	layouts *Layouts
	ui      *UI
	hooks   *Hooks
	engine  *engine.Engine
	learned *config.LearnerStore
	caret   *caretLocator
	events  chan any

	// lastUserWindow is the last foreground window that belongs to an application rather
	// than to the taskbar or the Start menu — what "the current app" means in the menu.
	lastUserWindow atomic.Uintptr

	namesMu sync.Mutex
	names   map[uint32]cachedName

	elevationMu sync.Mutex
	elevation   map[uint32]cachedElevation
	selfAdmin   bool

	lastLayoutRefresh time.Time
}

type cachedName struct {
	name string
	at   time.Time
}

type cachedElevation struct {
	elevated bool
	at       time.Time
}

// Run is the whole program. It returns the process exit code.
func Run(args []string) int {
	// The main goroutine becomes the UI thread for good.
	runtime.LockOSThread()
	win32.SecureDLLSearch()
	// Until the settings are read, the Windows display language decides.
	i18n.Set(i18n.FromLangID(systemUILangID()))

	mutex, err := win32.CreateMutex(`Local\Tilqos.SingleInstance`)
	if errno, ok := err.(syscall.Errno); ok && errno == win32.ERROR_ALREADY_EXISTS {
		messageBox(i18n.T(i18n.MsgAlreadyRunning), win32.MB_ICONINFORMATION)
		return 0
	}
	if mutex != 0 {
		defer syscall.CloseHandle(syscall.Handle(mutex))
	}

	dpiMode := win32.EnablePerMonitorDPI()
	dir := dataDir()
	// Earlier builds kept their files in the roaming profile; move them (and delete the log
	// that was there) before anything is read.
	migrateErr := config.MigrateData(config.RoamingDir(os.UserConfigDir), dir)

	store, configErr := config.NewStore(filepath.Join(dir, "config.json"))
	if os.Getenv("TILQOS_DEBUG") == "1" {
		// For this run only: the choice is not written to config.json.
		store.ForceDiagnostics(true)
	}
	applyUILanguage(store.Get())
	// The log is off unless diagnostics are: no file is created, nothing is written.
	log := NewLogger(filepath.Join(dir, "tilqos.log"), logMaxBytes)
	log.SetEnabled(store.Get().Diagnostics)
	defer log.Close()
	log.Printf("start: version=%s arch=%s dpi=%s", Version, runtime.GOARCH, dpiMode)
	if migrateErr != nil {
		log.Printf("moving files from the roaming profile: %v", migrateErr)
	}
	if configErr != nil {
		log.Printf("settings: %v", configErr)
	}

	loaded, err := models.Load()
	if err != nil {
		log.Printf("models: %v", err)
		messageBox(i18n.F(i18n.MsgModelsMissing, err.Error()), win32.MB_ICONERROR)
		return 1
	}

	a := &App{
		dir:       dir,
		log:       log,
		store:     store,
		models:    loaded,
		layouts:   NewLayouts(log),
		caret:     newCaretLocator(),
		events:    make(chan any, 4096),
		names:     map[uint32]cachedName{},
		elevation: map[uint32]cachedElevation{},
		selfAdmin: processElevated(syscall.Handle(currentProcess())),
	}
	a.layouts.Refresh()
	a.lastLayoutRefresh = time.Now()

	if a.ui, err = newUI(a); err != nil {
		log.Printf("ui: %v", err)
		messageBox(i18n.F(i18n.MsgWindowFailed, err.Error()), win32.MB_ICONERROR)
		return 1
	}
	defer a.ui.Close()

	// What the app learns from the user: salted hashes only, next to config.json.
	learned, learnErr := config.OpenLearner(config.LearnerPath(store.Path()))
	if learnErr != nil {
		log.Printf("learning: %v", learnErr)
	}
	if upgrade, ok := learned.Learner().Upgraded(); ok && upgrade.Before > 0 {
		log.Printf("learning: kept %d of %d entries under the new rules", upgrade.Kept, upgrade.Before)
	}
	a.learned = learned
	defer func() {
		if err := learned.Close(); err != nil {
			log.Printf("saving learned words failed: %v", err)
		}
	}()

	a.engine = engine.New(a, store, loaded, learned.Learner(), log)
	// The timers behind the privacy promises (idle reset of the buffer, two-minute undo). They
	// fire on goroutines of their own and hand the work to the engine's goroutine.
	a.engine.SetScheduler(core.RealScheduler{}, func(f func()) {
		a.send(func(*engine.Engine) { f() })
	})
	foreground := win32.GetForegroundWindow()
	a.lastUserWindow.Store(foreground)
	a.engine.Foreground(foreground)

	a.hooks, err = StartHooks(a.events, log, a.IsPasswordField)
	if err != nil {
		log.Printf("hooks: %v", err)
		a.ui.tray.Balloon(i18n.T(i18n.BalloonHookTitle), i18n.F(i18n.BalloonHookText, err.Error()), true)
	}
	a.hooks.SetBindings(config.Bindings(store.Get().Hotkeys))

	store.Subscribe(func(c config.Config) {
		applyUILanguage(c) // before the tray text below is built
		log.SetEnabled(c.Diagnostics)
		a.hooks.SetBindings(config.Bindings(c.Hotkeys))
		a.send(configChangedEvent{})
		a.ui.Post(func() { a.refreshTray(c) })
	})
	a.refreshTray(store.Get())

	go a.engineLoop()

	if configErr != nil {
		a.ui.tray.Balloon(i18n.T(i18n.BalloonSettingsReset), configErr.Error(), true)
	} else if !store.Get().WelcomeShown {
		keys := store.Get().Hotkeys
		a.ui.tray.Balloon(i18n.T(i18n.BalloonWelcomeTitle),
			i18n.F(i18n.BalloonWelcomeText, keys[string(config.ActionFixLastWord)],
				keys[string(config.ActionFixSelection)], keys[string(config.ActionUndoLastFix)]), false)
		a.update(func(c *config.Config) { c.WelcomeShown = true })
	}

	a.ui.Loop()

	log.Printf("exit")
	if a.hooks != nil {
		a.hooks.Stop()
	}
	return 0
}

// logMaxBytes is the size at which the log is rotated to tilqos.log.old, which is replaced at
// the next rotation: the two files together stay under twice this.
const logMaxBytes = 512 << 10

// dataDir is %LOCALAPPDATA%\Tilqos: settings, learned words and the log all stay on this
// machine, out of the roaming profile.
func dataDir() string {
	return config.DataDir(os.Getenv, os.UserCacheDir, os.TempDir)
}

// wipeSessionMemory has the engine forget what was typed and corrected: the keystroke buffer,
// the text read from the field, the offer on screen and the undo list. Called on screen lock,
// sign-out/disconnect and sleep, from the UI thread; the engine does it on its own goroutine.
func (a *App) wipeSessionMemory(reason string) {
	a.send(func(e *engine.Engine) { e.Wipe(reason) })
}

func (a *App) refreshTray(c config.Config) {
	tip := i18n.T(i18n.TrayEnabled)
	if !c.Enabled {
		tip = i18n.T(i18n.TrayDisabled)
	}
	a.ui.tray.Update(c.Enabled, tip)
}

// send queues work for the engine goroutine without ever blocking the caller (the UI
// thread must not wait on the engine, which may be waiting on the UI).
func (a *App) send(event any) {
	select {
	case a.events <- event:
	default:
		go func() { a.events <- event }()
	}
}

// engineLoop is the engine goroutine: every engine call happens here, in event order.
func (a *App) engineLoop() {
	for event := range a.events {
		if dropped := a.hooks.TakeDropped(); dropped > 0 {
			a.log.Printf("event queue overflow: %d dropped", dropped)
			a.engine.Reset("overflow")
		}
		switch event := event.(type) {
		case engine.KeyEvent:
			a.engine.HandleKey(event)
		case mouseEvent:
			a.engine.HandleMouse()
		case focusEvent:
			a.engine.HandleFocusChange()
		case foregroundEvent:
			a.onForeground(event.window)
		case acceptEvent:
			a.engine.HandleAccept(event.seq)
		case hotkeyEvent:
			a.engine.Perform(event.action, event.seq)
		case timeoutEvent:
			a.engine.HandleSuggestionTimeout(event.id)
		case configChangedEvent:
			a.engine.RefreshConfig()
		case func(*engine.Engine):
			event(a.engine)
		}
	}
}

// shellClasses are windows that take the foreground but are not "an application the user
// types into": the taskbar, the notification area, Start and search.
var shellClasses = map[string]bool{
	"Shell_TrayWnd": true, "Shell_SecondaryTrayWnd": true, "NotifyIconOverflowWindow": true,
	"TopLevelWindowForOverflowXamlIsland": true, "XamlExplorerHostIslandWindow": true,
	"Windows.UI.Core.CoreWindow": true,
}

func (a *App) onForeground(window uintptr) {
	// Layouts can only have changed while the user was elsewhere (in Settings).
	if a.layouts.Refresh() {
		a.lastLayoutRefresh = time.Now()
		a.engine.Reset("layoutsChanged")
	}
	if window != 0 && shellClasses[win32.GetClassName(window)] {
		// The taskbar, the tray, Start: the application's caret has not moved, so neither
		// the buffer nor a pending undo is invalidated. Leaving for a real application is.
		return
	}
	if window != 0 {
		a.lastUserWindow.Store(window)
	}
	a.engine.Foreground(window)
}

// MARK: - engine.Platform

func (a *App) Tables() []*core.KeyMapTable { return a.layouts.Tables() }

func (a *App) Table(id string) *core.KeyMapTable {
	table := a.layouts.Table(id)
	// A layout added since the last refresh: rebuild, at most every two seconds.
	if table == nil && id != "" && time.Since(a.lastLayoutRefresh) > 2*time.Second {
		a.lastLayoutRefresh = time.Now()
		if a.layouts.Refresh() {
			table = a.layouts.Table(id)
		}
	}
	return table
}

func (a *App) CurrentLayoutID(window uintptr) string { return LayoutID(currentLayout(window)) }

// SelectLayout asks the window to switch its input language. The request is asynchronous
// (the application's thread handles WM_INPUTLANGCHANGEREQUEST), so it is confirmed briefly.
func (a *App) SelectLayout(window uintptr, id string) bool {
	hkl, ok := a.layouts.HKL(id)
	if !ok {
		return false
	}
	if window == 0 {
		window = win32.GetForegroundWindow()
	}
	win32.PostMessage(window, win32.WM_INPUTLANGCHANGEREQUEST, 0, hkl)
	// Some applications only react when the focused control gets the request.
	thread, _ := win32.GetWindowThreadProcessId(window)
	if thread != 0 {
		if info, ok := win32.GetGUIThreadInfo(thread); ok && info.HwndFocus != 0 && info.HwndFocus != window {
			win32.PostMessage(info.HwndFocus, win32.WM_INPUTLANGCHANGEREQUEST, 0, hkl)
		}
	}
	for attempt := 0; attempt < 20; attempt++ {
		if uint32(currentLayout(window)) == uint32(hkl) {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	a.log.Printf("layout switch to %s not confirmed by the application (%s)", id, engine.LogPrivate)
	return false
}

// AppName is the lowercased executable name behind a window (also used by the UI thread).
func (a *App) AppName(window uintptr) string {
	if window == 0 {
		return ""
	}
	_, pid := win32.GetWindowThreadProcessId(window)
	// A UWP app's frame belongs to ApplicationFrameHost; the app is its CoreWindow child.
	if win32.GetClassName(window) == "ApplicationFrameWindow" {
		if coreWindow := win32.FindWindowEx(window, 0, "Windows.UI.Core.CoreWindow"); coreWindow != 0 {
			_, pid = win32.GetWindowThreadProcessId(coreWindow)
		}
	}
	if pid == 0 {
		return ""
	}
	a.namesMu.Lock()
	cached, ok := a.names[pid]
	a.namesMu.Unlock()
	if ok && time.Since(cached.at) < time.Minute {
		return cached.name
	}
	name := ""
	if process, err := win32.OpenProcess(win32.PROCESS_QUERY_LIMITED_INFORMATION, pid); err == nil {
		name = strings.ToLower(filepath.Base(win32.ProcessImagePath(process)))
		syscall.CloseHandle(process)
	}
	a.namesMu.Lock()
	if len(a.names) > 256 {
		a.names = map[uint32]cachedName{}
	}
	a.names[pid] = cachedName{name: name, at: time.Now()}
	a.namesMu.Unlock()
	return name
}

// IsPasswordField: a standard edit control (or rich edit) with ES_PASSWORD has focus.
// Browsers and custom-drawn fields do not expose this; see README.
func (a *App) IsPasswordField(window uintptr) bool {
	thread, _ := win32.GetWindowThreadProcessId(window)
	if thread == 0 {
		// A window that is gone: GetGUIThreadInfo(0) would describe the foreground thread.
		return false
	}
	info, ok := win32.GetGUIThreadInfo(thread)
	if !ok || info.HwndFocus == 0 {
		return false
	}
	if !strings.Contains(strings.ToLower(win32.GetClassName(info.HwndFocus)), "edit") {
		return false
	}
	return win32.GetWindowLongPtr(info.HwndFocus, win32.GWL_STYLE)&win32.ES_PASSWORD != 0
}

// CanInject is false for windows of elevated processes when we are not elevated: UIPI
// drops our SendInput without any error, and the hook does not even see their keys.
func (a *App) CanInject(window uintptr) bool {
	if a.selfAdmin {
		return true
	}
	_, pid := win32.GetWindowThreadProcessId(window)
	if pid == 0 {
		return false
	}
	a.elevationMu.Lock()
	cached, ok := a.elevation[pid]
	a.elevationMu.Unlock()
	if ok && time.Since(cached.at) < 10*time.Second {
		return !cached.elevated
	}
	elevated := false
	process, err := win32.OpenProcess(win32.PROCESS_QUERY_LIMITED_INFORMATION, pid)
	if err != nil {
		// Refused outright: treat it as out of reach.
		elevated = err == syscall.Errno(win32.ERROR_ACCESS_DENIED)
	} else {
		elevated = processElevated(process)
		syscall.CloseHandle(process)
	}
	a.elevationMu.Lock()
	a.elevation[pid] = cachedElevation{elevated: elevated, at: time.Now()}
	a.elevationMu.Unlock()
	return !elevated
}

func currentProcess() uintptr {
	process, _ := syscall.GetCurrentProcess()
	return uintptr(process)
}

// processElevated reads TokenElevation. A token we may not even query belongs to a
// process above our integrity level, which counts as elevated.
func processElevated(process syscall.Handle) bool {
	var token syscall.Token
	if err := syscall.OpenProcessToken(process, win32.TOKEN_QUERY, &token); err != nil {
		return err == syscall.Errno(win32.ERROR_ACCESS_DENIED)
	}
	defer token.Close()
	var elevation win32.TOKEN_ELEVATION
	var returned uint32
	if err := syscall.GetTokenInformation(token, win32.TokenElevation, (*byte)(unsafe.Pointer(&elevation)),
		uint32(unsafe.Sizeof(elevation)), &returned); err != nil {
		return false
	}
	return elevation.TokenIsElevated != 0
}

func (a *App) KeySeq() uint64 { return a.hooks.KeySeq() }

func acceptVK(key config.AcceptKey) uint16 {
	if key == config.AcceptInsert {
		return win32.VK_INSERT
	}
	return win32.VK_TAB
}

func (a *App) ReplayAccept(key config.AcceptKey) {
	if err := tapKeys(acceptVK(key)); err != nil {
		a.log.Printf("replaying the accept key failed: %v", err)
	}
}

func (a *App) ShowSuggestion(s engine.Suggestion) {
	caret, ok := a.caret.Locate(s.Window)
	a.ui.Post(func() { a.ui.popup.Show(s, caret, ok) })
}

func (a *App) HideSuggestion() { a.ui.Post(func() { a.ui.popup.Hide() }) }

func (a *App) SetAcceptKey(key config.AcceptKey, active bool) {
	a.hooks.SetAccept(acceptVK(key), active)
}

// CopySelection copies the selection with Ctrl+C and reads it back, keeping the previous
// clipboard contents to restore afterwards.
func (a *App) CopySelection() (string, func(), bool) {
	// The hotkey that got us here is usually still held (Shift of Shift+Pause); Ctrl+C with
	// Shift is a different shortcut.
	if !waitForModifierRelease(true, time.Second) {
		return "", nil, false
	}
	var saved []clipFormat
	var saveOK bool
	if a.ui.Call(func() { saved, saveOK = saveClipboard(a.ui.hwnd) }) != nil || !saveOK {
		// Flagged as sensitive (a password manager's copy), or too big to put back: Ctrl+C
		// would overwrite it, so nothing is sent and nothing is read.
		return "", nil, false
	}
	restore := func() { a.ui.Call(func() { restoreClipboard(a.ui.hwnd, saved) }) }

	sequence := win32.GetClipboardSequenceNumber()
	if err := tapKeys(win32.VK_CONTROL, 'C'); err != nil {
		return "", restore, false
	}
	// Wait for the application to put the selection on the clipboard. Nothing selected
	// usually means nothing is copied at all.
	deadline := time.Now().Add(500 * time.Millisecond)
	for win32.GetClipboardSequenceNumber() == sequence {
		if time.Now().After(deadline) {
			return "", restore, false
		}
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(20 * time.Millisecond) // some applications set several formats in a row
	var text string
	var ok bool
	if a.ui.Call(func() { text, ok = readClipboardText(a.ui.hwnd) }) != nil || !ok {
		return "", restore, false
	}
	return text, restore, text != ""
}

// Paste replaces the selection with text via the clipboard, then restores the clipboard.
// There is no signal for "the application has read the clipboard", so the restore waits a
// moment: long enough for any application to paste, short enough to go unnoticed.
func (a *App) Paste(text string, restore func()) bool {
	var written bool
	if a.ui.Call(func() { written = writeClipboardText(a.ui.hwnd, text, true) }) != nil || !written {
		restore()
		return false
	}
	if !waitForModifierRelease(true, time.Second) {
		restore()
		return false
	}
	if err := tapKeys(win32.VK_CONTROL, 'V'); err != nil {
		restore()
		return false
	}
	time.Sleep(300 * time.Millisecond)
	restore()
	return true
}

func (a *App) ReadClipboard() (string, bool) {
	var text string
	var ok bool
	if a.ui.Call(func() { text, ok = readClipboardText(a.ui.hwnd) }) != nil {
		return "", false
	}
	return text, ok
}

func (a *App) WriteClipboard(text string) bool {
	var ok bool
	if a.ui.Call(func() { ok = writeClipboardText(a.ui.hwnd, text, false) }) != nil {
		return false
	}
	return ok
}

func (a *App) Notify(title, message string) {
	a.ui.Post(func() { a.ui.tray.Balloon(title, message, false) })
}

func (a *App) PlaySound() { win32.MessageBeep(win32.MB_ICONINFORMATION) }

// Beep is the system's default sound, as NSSound.beep() is on macOS: a command found
// nothing to act on.
func (a *App) Beep() { win32.MessageBeep(win32.MB_OK) }

// Compile-time check that App satisfies the engine's platform contract.
var _ engine.Platform = (*App)(nil)

// String helps log lines.
func (a *App) String() string { return fmt.Sprintf("Tilqos %s", Version) }
