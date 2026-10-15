//go:build windows && (amd64 || arm64)

package winapp

// Building the key tables from the layouts Windows actually has installed — the counterpart
// of KeyMapTable.build(from:) over UCKeyTranslate. Nothing is hardcoded: every table comes
// from ToUnicodeEx, so any layout the user adds later works too.

import (
	"runtime"
	"sync"
	"unicode/utf16"

	"tilqos/internal/core"
	"tilqos/internal/engine"
	"tilqos/internal/win32"
)

// Layouts owns the installed layouts and their tables. Safe for concurrent use.
type Layouts struct {
	mu      sync.RWMutex
	list    []uintptr
	tables  []*core.KeyMapTable
	byID    map[string]*core.KeyMapTable
	hklByID map[string]uintptr
	log     *Logger
}

func NewLayouts(log *Logger) *Layouts {
	return &Layouts{byID: map[string]*core.KeyMapTable{}, hklByID: map[string]uintptr{}, log: log}
}

// Refresh rebuilds the tables when the set of installed layouts changed. Returns true when
// it did.
func (l *Layouts) Refresh() bool {
	list := win32.GetKeyboardLayoutList()
	l.mu.RLock()
	same := len(list) == len(l.list)
	for i := 0; same && i < len(list); i++ {
		same = list[i] == l.list[i]
	}
	l.mu.RUnlock()
	if same {
		return false
	}

	var tables []*core.KeyMapTable
	byID := map[string]*core.KeyMapTable{}
	hklByID := map[string]uintptr{}
	for _, hkl := range list {
		id := LayoutID(hkl)
		hklByID[id] = hkl
		if IsIMELayout(hkl) {
			continue
		}
		table := buildTable(hkl)
		tables = append(tables, table)
		byID[id] = table
	}

	l.mu.Lock()
	l.list, l.tables, l.byID, l.hklByID = list, tables, byID, hklByID
	l.mu.Unlock()
	if l.log != nil {
		l.log.Printf("layouts: %d installed, %d with tables", len(list), len(tables))
	}
	return true
}

func (l *Layouts) Tables() []*core.KeyMapTable {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.tables
}

func (l *Layouts) Table(id string) *core.KeyMapTable {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.byID[id]
}

// HKL returns the input locale handle behind an ID.
func (l *Layouts) HKL(id string) (uintptr, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	hkl, ok := l.hklByID[id]
	return hkl, ok
}

// FirstFor returns the first installed layout of a language.
func (l *Layouts) FirstFor(lang core.Lang) *core.KeyMapTable {
	for _, table := range l.Tables() {
		if table.Lang == lang {
			return table
		}
	}
	return nil
}

// buildTable reads one layout through ToUnicodeEx, for every physical key and every
// modifier variant (Shift, AltGr, Caps Lock).
func buildTable(hkl uintptr) *core.KeyMapTable {
	// Dead keys: ToUnicodeEx's "no state change" flag keeps a dead key from arming the
	// layout. Windows before 10 1607 ignores the flag, so the arming is also undone by
	// flushDeadKey — on this very OS thread, which the goroutine must therefore not leave.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	forward := make([]string, KeyCodeCount*core.VariantCount)
	dead := make([]bool, len(forward))
	var buf [16]uint16

	for code := uint16(0); code < KeyCodeCount; code++ {
		var vk, scan uint32
		if numpad, ok := NumpadVK(code); ok {
			vk = uint32(numpad)
			scan = win32.MapVirtualKeyEx(vk, win32.MAPVK_VK_TO_VSC, hkl)
		} else if withPrefix, ok := ScanForKeyCode(code); ok {
			if plain := withPrefix & 0xFF; plain == 0 || plain >= 0x80 {
				continue
			}
			vk = win32.MapVirtualKeyEx(withPrefix, win32.MAPVK_VSC_TO_VK_EX, hkl)
			scan = withPrefix & 0xFF
		} else {
			continue
		}
		// Only keys that can type: modifiers, navigation, F-keys and friends never enter
		// the buffer as text.
		if vk == 0 || vk > 0xFF || ClassifyVK(uint16(vk)) != engine.KeyText {
			continue
		}

		for variant := 0; variant < core.VariantCount; variant++ {
			mods := core.ModState(variant)
			var state [256]byte
			if mods&core.ModShift != 0 {
				state[win32.VK_SHIFT] = 0x80
				state[win32.VK_LSHIFT] = 0x80
			}
			if mods&core.ModOption != 0 { // AltGr is Ctrl+Alt to Windows
				state[win32.VK_CONTROL] = 0x80
				state[win32.VK_LCONTROL] = 0x80
				state[win32.VK_MENU] = 0x80
				state[win32.VK_RMENU] = 0x80
			}
			if mods&core.ModCaps != 0 {
				state[win32.VK_CAPITAL] = 0x01
			}
			n := win32.ToUnicodeEx(vk, scan, &state, buf[:], win32.TOUNICODE_NO_STATE_CHANGE, hkl)
			index := int(code)*core.VariantCount + variant
			switch {
			case n > 0:
				text := string(utf16.Decode(buf[:n]))
				// Return, Tab, Escape and Backspace report a character but never take part
				// in a word. Space is kept — it is the word boundary.
				if isControlText(text) {
					continue
				}
				forward[index] = text
			case n < 0:
				// A dead key: the buffer holds its standalone (spacing) character.
				if buf[0] >= 0x20 {
					forward[index] = string(utf16.Decode(buf[:1]))
					dead[index] = true
				}
				flushDeadKey(hkl)
			}
		}
	}

	langID := uint16(hkl)
	name := win32.LocaleDisplayName(langID)
	if name == "" {
		name = LayoutID(hkl)
	}
	return core.NewKeyMapTable(LayoutID(hkl), name, LangForHKL(hkl), KeyCodeCount, forward, dead, KeypadCodes())
}

// flushDeadKey clears a dead key armed by ToUnicodeEx on systems that ignore the
// no-state-change flag, by translating a space after it.
func flushDeadKey(hkl uintptr) {
	var state [256]byte
	var buf [8]uint16
	scan := win32.MapVirtualKeyEx(win32.VK_SPACE, win32.MAPVK_VK_TO_VSC, hkl)
	for i := 0; i < 2; i++ {
		if win32.ToUnicodeEx(win32.VK_SPACE, scan, &state, buf[:], 0, hkl) >= 0 {
			return
		}
	}
}

func isControlText(text string) bool {
	runes := []rune(text)
	return len(runes) == 1 && (runes[0] < 0x20 || runes[0] == 0x7F)
}

// inputThread is the thread whose keyboard layout applies to a foreground window. UWP apps
// are hosted in an ApplicationFrameWindow whose own thread does not read their input; the
// Windows.UI.Core.CoreWindow child does. For classic windows the focused control's thread
// wins, which matters for windows that embed another thread's controls.
func inputThread(foreground uintptr) uint32 {
	thread, _ := win32.GetWindowThreadProcessId(foreground)
	if win32.GetClassName(foreground) == "ApplicationFrameWindow" {
		if coreWindow := win32.FindWindowEx(foreground, 0, "Windows.UI.Core.CoreWindow"); coreWindow != 0 {
			thread, _ = win32.GetWindowThreadProcessId(coreWindow)
		}
		return thread
	}
	if info, ok := win32.GetGUIThreadInfo(thread); ok && info.HwndFocus != 0 {
		if focusThread, _ := win32.GetWindowThreadProcessId(info.HwndFocus); focusThread != 0 {
			thread = focusThread
		}
	}
	return thread
}

// currentLayout is the input locale of the thread that receives keys in a window.
func currentLayout(foreground uintptr) uintptr {
	if foreground == 0 {
		foreground = win32.GetForegroundWindow()
	}
	return win32.GetKeyboardLayout(inputThread(foreground))
}
