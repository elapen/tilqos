//go:build windows && (amd64 || arm64)

package winapp

// The keyboard and mouse hooks: the counterpart of EventTap.swift.
//
// Windows calls a low-level hook synchronously on the path of every key press in the
// system, on the thread that installed it, and silently removes a hook that takes longer
// than LowLevelHooksTimeout. So the hooks live on a dedicated OS thread that does nothing
// but pump its message queue, and a callback only:
//
//   - drops our own injected events (dwExtraInfo carries injectMarker),
//   - decides synchronously whether to swallow the key — a hotkey, or the accept key while
//     a suggestion is on screen (both read from atomics the engine and settings publish),
//   - samples the state that has to be taken *now* — modifiers, the foreground window's
//     layout — and hands a KeyEvent to the engine goroutine without blocking.
//
// All real work (tables, detection, injection) happens on the engine goroutine.

import (
	"runtime"
	"sync/atomic"
	"syscall"
	"time"

	"tilqos/internal/config"
	"tilqos/internal/engine"
	"tilqos/internal/win32"
)

// injectMarker is written into dwExtraInfo of everything we inject ("TILQOS" + version,
// the same value the macOS app stamps into its events), so the hook can tell our own
// input from the user's.
const injectMarker uintptr = 0x5449_4C51_4F53_0001

// Events the hook thread and the UI send to the engine goroutine.
type (
	mouseEvent      struct{}
	focusEvent      struct{}
	foregroundEvent struct{ window uintptr }
	acceptEvent     struct{ seq uint64 }
	hotkeyEvent     struct {
		action config.Action
		seq    uint64
	}
)

type acceptKey struct{ vk uint16 }

// Hooks owns the hook thread.
type Hooks struct {
	events chan<- any
	log    *Logger
	start  time.Time
	// isPassword says whether a password field has the focus in a window. Asked on the hook
	// thread, before a key is described to the engine, so that the keys typed into a password
	// field are never even handed over.
	isPassword func(window uintptr) bool

	keySeq   atomic.Uint64
	accept   atomic.Pointer[acceptKey]
	bindings atomic.Pointer[[]config.Binding]
	dropped  atomic.Uint64
	running  atomic.Bool

	threadID         uint32
	lastKeyboardTick atomic.Uint32

	// Touched only on the hook thread.
	keyboardHook  uintptr
	mouseHook     uintptr
	eventHooks    []uintptr
	swallowedUp   map[uint16]bool
	lastFocus     [3]uintptr
	lastReinstall uint32
}

// The hook procedures are C function pointers; they reach the instance through here. Set
// before the hooks are installed and never changed while they are.
var activeHooks *Hooks

// Callbacks are created once: syscall.NewCallback has a hard, process-wide limit.
var (
	keyboardCallback = syscall.NewCallback(keyboardProc)
	mouseCallback    = syscall.NewCallback(mouseProc)
	winEventCallback = syscall.NewCallback(winEventProc)
)

// StartHooks starts the hook thread and waits until the hooks are installed.
func StartHooks(events chan<- any, log *Logger, isPassword func(window uintptr) bool) (*Hooks, error) {
	h := &Hooks{events: events, log: log, start: time.Now(), isPassword: isPassword, swallowedUp: map[uint16]bool{}}
	empty := []config.Binding{}
	h.bindings.Store(&empty)
	activeHooks = h
	ready := make(chan error, 1)
	go h.run(ready)
	return h, <-ready
}

func (h *Hooks) run(ready chan<- error) {
	// This goroutine owns the thread for good: the hooks are bound to it.
	runtime.LockOSThread()
	h.threadID = win32.GetCurrentThreadId()
	// Pumping promptly under load is what keeps Windows from dropping the hook.
	win32.SetCurrentThreadPriority(win32.THREAD_PRIORITY_TIME_CRITICAL)

	if err := h.install(); err != nil {
		ready <- err
		return
	}
	h.running.Store(true)
	ready <- nil

	// A thread timer (no window): WM_TIMER lands in this queue every 5 s for the watchdog.
	win32.SetTimer(0, 1, 5000)
	var msg win32.MSG
	for {
		result := win32.GetMessage(&msg, 0)
		if result == 0 || result == -1 {
			break
		}
		if msg.Message == win32.WM_TIMER && msg.Hwnd == 0 {
			h.checkHealth()
			continue
		}
		win32.DispatchMessage(&msg)
	}
	h.uninstall()
	h.running.Store(false)
}

func (h *Hooks) install() error {
	module := win32.GetModuleHandle()
	keyboard, err := win32.SetWindowsHookEx(win32.WH_KEYBOARD_LL, keyboardCallback, module, 0)
	if err != nil {
		return err
	}
	mouse, err := win32.SetWindowsHookEx(win32.WH_MOUSE_LL, mouseCallback, module, 0)
	if err != nil {
		win32.UnhookWindowsHookEx(keyboard)
		return err
	}
	h.keyboardHook, h.mouseHook = keyboard, mouse
	h.lastKeyboardTick.Store(win32.GetTickCount())
	// Out-of-context WinEvents arrive through this thread's message queue too.
	for _, event := range []uint32{win32.EVENT_SYSTEM_FOREGROUND, win32.EVENT_OBJECT_FOCUS} {
		if hook := win32.SetWinEventHook(event, event, winEventCallback,
			win32.WINEVENT_OUTOFCONTEXT|win32.WINEVENT_SKIPOWNPROCESS); hook != 0 {
			h.eventHooks = append(h.eventHooks, hook)
		}
	}
	h.log.Printf("hooks installed (keyboard, mouse, %d WinEvent)", len(h.eventHooks))
	return nil
}

func (h *Hooks) uninstall() {
	if h.keyboardHook != 0 {
		win32.UnhookWindowsHookEx(h.keyboardHook)
		h.keyboardHook = 0
	}
	if h.mouseHook != 0 {
		win32.UnhookWindowsHookEx(h.mouseHook)
		h.mouseHook = 0
	}
	for _, hook := range h.eventHooks {
		win32.UnhookWinEvent(hook)
	}
	h.eventHooks = nil
}

// checkHealth reinstalls the low-level hooks if Windows seems to have dropped them.
//
// Windows removes a low-level hook without telling anyone once it times out (a long GC
// pause, a suspended process). There is no way to ask whether a hook is still installed, so
// the watchdog compares the system's last-input time with the last time our keyboard hook
// ran. Mouse-only activity can trip it too; a spurious reinstall costs microseconds and is
// rate-limited to once a minute.
func (h *Hooks) checkHealth() {
	now := win32.GetTickCount()
	lastInput := win32.GetLastInputTick()
	sinceHook := int32(lastInput - h.lastKeyboardTick.Load())
	if sinceHook > 30_000 && int32(now-h.lastReinstall) > 60_000 {
		h.lastReinstall = now
		h.uninstall()
		if err := h.install(); err != nil {
			h.log.Printf("watchdog: reinstalling hooks failed: %v", err)
			h.running.Store(false)
			return
		}
		h.running.Store(true)
		h.log.Printf("watchdog: hooks reinstalled")
	}
}

// Stop removes the hooks and ends the thread.
func (h *Hooks) Stop() {
	if h.threadID != 0 {
		win32.PostThreadMessage(h.threadID, win32.WM_QUIT, 0, 0)
	}
}

// Running reports whether the hooks are installed.
func (h *Hooks) Running() bool { return h != nil && h.running.Load() }

// KeySeq is the sequence number of the newest user key press.
func (h *Hooks) KeySeq() uint64 { return h.keySeq.Load() }

// TakeDropped returns and clears the number of events lost to a full queue.
func (h *Hooks) TakeDropped() uint64 { return h.dropped.Swap(0) }

// SetAccept arms (or disarms) swallowing of the suggestion's accept key.
func (h *Hooks) SetAccept(vk uint16, active bool) {
	if active {
		h.accept.Store(&acceptKey{vk: vk})
	} else {
		h.accept.Store(nil)
	}
}

// SetBindings publishes the hotkeys.
func (h *Hooks) SetBindings(bindings []config.Binding) {
	copied := append([]config.Binding{}, bindings...)
	h.bindings.Store(&copied)
}

func (h *Hooks) post(event any) {
	select {
	case h.events <- event:
	default:
		// Never block the hook. The engine resets its buffer when it sees drops.
		h.dropped.Add(1)
	}
}

// MARK: - Callbacks

func keyboardProc(nCode, wParam, lParam uintptr) uintptr {
	if int32(nCode) == win32.HC_ACTION {
		if h := activeHooks; h != nil && h.onKey(uint32(wParam), win32.PtrAt[win32.KBDLLHOOKSTRUCT](lParam)) {
			return 1 // swallowed
		}
	}
	return win32.CallNextHookEx(nCode, wParam, lParam)
}

type modifiers struct {
	shift, ctrl, alt, win, caps bool
}

func readModifiers() modifiers {
	// The asynchronous state reflects every key processed before this one, which is exactly
	// the set of modifiers held while this key went down.
	return modifiers{
		shift: win32.KeyDown(win32.VK_SHIFT),
		ctrl:  win32.KeyDown(win32.VK_CONTROL),
		alt:   win32.KeyDown(win32.VK_MENU),
		win:   win32.KeyDown(win32.VK_LWIN) || win32.KeyDown(win32.VK_RWIN),
		caps:  win32.KeyToggled(win32.VK_CAPITAL),
	}
}

func (m modifiers) hotkeyMods() config.Mods {
	var mods config.Mods
	if m.shift {
		mods |= config.ModShift
	}
	if m.ctrl {
		mods |= config.ModCtrl
	}
	if m.alt {
		mods |= config.ModAlt
	}
	if m.win {
		mods |= config.ModWin
	}
	return mods
}

// onKey handles one keyboard event on the hook thread; true swallows it.
func (h *Hooks) onKey(message uint32, kb *win32.KBDLLHOOKSTRUCT) bool {
	h.lastKeyboardTick.Store(kb.Time)
	if kb.DwExtraInfo == injectMarker {
		return false // our own output
	}
	vk := uint16(kb.VkCode)
	down := message == win32.WM_KEYDOWN || message == win32.WM_SYSKEYDOWN
	if !down {
		// The release of a key whose press we swallowed goes too, so no application sees a
		// lone key-up.
		if h.swallowedUp[vk] {
			delete(h.swallowedUp, vk)
			return true
		}
		return false
	}

	if kb.Flags&win32.LLKHF_INJECTED != 0 {
		// Another program is typing (an on-screen keyboard, a password manager's auto-type,
		// a remote-control tool). Never read it — it may be a password — but the screen just
		// changed in a way we cannot follow, so the buffer has to go.
		if !IsModifierVK(vk) {
			h.post(engine.KeyEvent{Class: engine.KeyOther, Foreign: true, Window: win32.GetForegroundWindow()})
		}
		return false
	}

	// Auto-repeat of a key whose press we swallowed (a held Pause, a held Tab after accepting
	// a suggestion): keep swallowing, act only once.
	if h.swallowedUp[vk] {
		return true
	}

	mods := readModifiers()

	// Hotkeys first: they carry Alt, which would otherwise read as a shortcut.
	hotkeyMods := mods.hotkeyMods()
	for _, binding := range *h.bindings.Load() {
		if binding.Hotkey.Matches(vk, hotkeyMods) {
			seq := h.keySeq.Add(1)
			h.swallowedUp[vk] = true
			if mods.alt || mods.win {
				// With the key swallowed, the application would see Alt (or Win) pressed and
				// released alone and open its menu bar (or the Start menu). An unassigned key
				// in between prevents that — AutoHotkey's "menu mask key".
				sendMaskKey()
			}
			h.post(hotkeyEvent{action: binding.Action, seq: seq})
			return true
		}
	}

	// The accept key, only while a suggestion is on screen. Taken once, then disarmed. Tab
	// may come with Shift (as on macOS); Insert only alone — Shift+Insert pastes in exactly
	// the terminals where Insert is the accept key.
	armed := h.accept.Load()
	if armed != nil && vk == armed.vk && !mods.ctrl && !mods.alt && !mods.win &&
		!(vk == win32.VK_INSERT && mods.shift) {
		if h.accept.CompareAndSwap(armed, nil) {
			seq := h.keySeq.Add(1)
			h.swallowedUp[vk] = true
			h.post(acceptEvent{seq: seq})
			return true
		}
	}

	if IsModifierVK(vk) {
		return false
	}
	foreground := win32.GetForegroundWindow()
	if _, pid := win32.GetWindowThreadProcessId(foreground); pid == ourPID {
		// Our own menu or dialog: not text in anyone's document, and the application
		// behind it keeps its caret.
		return false
	}
	seq := h.keySeq.Add(1)
	if h.isPassword != nil && h.isPassword(foreground) {
		// A password field has the focus: what is typed into it is not read. The engine hears
		// only that a key was pressed, no key code and no layout, and drops its buffer.
		h.post(engine.KeyEvent{
			Class: engine.KeyOther, Window: foreground, Time: time.Since(h.start).Seconds(), Seq: seq,
		})
		return false
	}
	hkl := currentLayout(foreground)
	class := ClassifyVK(vk)
	code, ok := PhysicalKeyCode(vk, kb.ScanCode, kb.Flags&win32.LLKHF_EXTENDED != 0)
	if !ok && class == engine.KeyText {
		// A typing key we cannot place physically: its text is unknowable.
		class = engine.KeyOther
	}
	h.post(engine.KeyEvent{
		Class:    class,
		Key:      code,
		Shift:    mods.shift,
		Caps:     mods.caps,
		Ctrl:     mods.ctrl,
		Alt:      mods.alt,
		Win:      mods.win,
		LayoutID: LayoutID(hkl),
		Window:   foreground,
		Time:     time.Since(h.start).Seconds(),
		Seq:      seq,
	})
	return false
}

func mouseProc(nCode, wParam, lParam uintptr) uintptr {
	if int32(nCode) == win32.HC_ACTION {
		switch uint32(wParam) {
		case win32.WM_LBUTTONDOWN, win32.WM_RBUTTONDOWN, win32.WM_MBUTTONDOWN, win32.WM_XBUTTONDOWN:
			// The caret moved somewhere we cannot see — unless the click went to the
			// taskbar or to our own tray menu, which leave the application's caret alone
			// (and resetting there would make the menu's "fix"/"undo" useless).
			if h := activeHooks; h != nil {
				info := win32.PtrAt[win32.MSLLHOOKSTRUCT](lParam)
				if !isShellOrOwnWindow(win32.WindowFromPoint(info.Pt)) {
					h.post(mouseEvent{})
				}
			}
		}
	}
	return win32.CallNextHookEx(nCode, wParam, lParam)
}

// ourPID identifies our own windows.
var ourPID = uint32(syscall.Getpid())

// isShellOrOwnWindow: the taskbar, the notification area, Start — or one of our windows.
func isShellOrOwnWindow(hwnd uintptr) bool {
	if hwnd == 0 {
		return false
	}
	root := win32.GetAncestor(hwnd, win32.GA_ROOT)
	if root == 0 {
		root = hwnd
	}
	if _, pid := win32.GetWindowThreadProcessId(root); pid == ourPID {
		return true
	}
	return shellClasses[win32.GetClassName(root)]
}

// winEventProc receives EVENT_SYSTEM_FOREGROUND and EVENT_OBJECT_FOCUS. Arguments narrower
// than a register (DWORD, LONG) arrive with undefined upper bits, hence the truncations.
func winEventProc(hook, event, hwnd, idObject, idChild, thread, eventTime uintptr) uintptr {
	h := activeHooks
	if h == nil {
		return 0
	}
	switch uint32(event) {
	case win32.EVENT_SYSTEM_FOREGROUND:
		h.post(foregroundEvent{window: hwnd})
	case win32.EVENT_OBJECT_FOCUS:
		key := [3]uintptr{hwnd, uintptr(int32(idObject)), uintptr(int32(idChild))}
		if key != h.lastFocus {
			h.lastFocus = key
			h.post(focusEvent{})
		}
	}
	return 0
}
