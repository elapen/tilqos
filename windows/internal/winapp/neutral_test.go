package winapp

// Tests for the platform-neutral parts of the Windows app; they run on any OS.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tilqos/internal/core"
	"tilqos/internal/engine"
)

func TestClassifyVK(t *testing.T) {
	cases := map[uint16]engine.KeyClass{
		'A': engine.KeyText, '7': engine.KeyText, 0x20: engine.KeyText, // letters, digits, space
		0xBA: engine.KeyText, 0xC0: engine.KeyText, 0xDE: engine.KeyText, 0xE2: engine.KeyText, // OEM keys
		0x60: engine.KeyText, 0x6E: engine.KeyText, 0x6A: engine.KeyText, // keypad
		vkBack: engine.KeyBackspace, vkTab: engine.KeyTab,
		vkReturn: engine.KeyNavigation, vkEscape: engine.KeyEscape, vkLeft: engine.KeyNavigation,
		vkHome: engine.KeyNavigation, vkNext: engine.KeyNavigation, vkDelete: engine.KeyNavigation,
		vkInsert: engine.KeyNavigation,
		vkShift:  engine.KeyModifier, 0xA2: engine.KeyModifier, vkRMenu: engine.KeyModifier,
		vkLWin: engine.KeyModifier, vkCapital: engine.KeyModifier, vkNumLock: engine.KeyModifier,
		vkScroll: engine.KeyModifier,
		vkF1:     engine.KeyOther, vkF24: engine.KeyOther, vkPause: engine.KeyOther, vkApps: engine.KeyOther,
		vkSnapshot: engine.KeyOther, 0xAD: engine.KeyOther, 0xE5: engine.KeyOther, 0xE7: engine.KeyOther,
		0x19: engine.KeyOther, vkClear: engine.KeyOther,
	}
	for vk, want := range cases {
		if got := ClassifyVK(vk); got != want {
			t.Errorf("ClassifyVK(%#x) = %v, want %v", vk, got, want)
		}
	}
}

func TestPhysicalKeyCodes(t *testing.T) {
	// The Q key: scan 0x10 whatever the layout calls it.
	if code, ok := PhysicalKeyCode('Q', 0x10, false); !ok || code != 0x10 {
		t.Fatalf("Q: %#x %v", code, ok)
	}
	// Keypad 7 with Num Lock on gets its own code; the same scan code with Num Lock off is
	// Home (navigation), and the dedicated Home key is extended.
	if code, ok := PhysicalKeyCode(vkNumpad0+7, 0x47, false); !ok || code != 0x200+vkNumpad0+7 {
		t.Fatalf("numpad 7: %#x", code)
	}
	if code, _ := PhysicalKeyCode(vkHome, 0x47, true); code != 0x147 {
		t.Fatalf("Home: %#x", code)
	}
	if _, ok := PhysicalKeyCode('A', 0, false); ok {
		t.Fatal("a key without a scan code has no physical identity")
	}
	// Round trip through the table builder's inverse.
	for code := uint16(1); code < 0x180; code++ {
		if code&0xFF >= 0x80 || code&0xFF == 0 {
			continue
		}
		scan, ok := ScanForKeyCode(code)
		if !ok {
			t.Fatalf("%#x has no scan", code)
		}
		back, _ := PhysicalKeyCode('X', scan&0xFF, scan&0xE000 != 0)
		if back != code {
			t.Fatalf("%#x → %#x → %#x", code, scan, back)
		}
	}
	for vk := uint16(vkNumpad0); vk <= vkNumpad9; vk++ {
		code, _ := PhysicalKeyCode(vk, 0x4F, false)
		if got, ok := NumpadVK(code); !ok || got != vk {
			t.Fatalf("numpad %#x", vk)
		}
		if !KeypadCodes()[code] {
			t.Fatalf("numpad %#x must count as keypad", vk)
		}
	}
	if code, _ := PhysicalKeyCode(vkDecimal, 0x53, false); code >= KeyCodeCount || !KeypadCodes()[code] {
		t.Fatal("VK_DECIMAL code")
	}
}

func TestLayoutIdentity(t *testing.T) {
	cases := []struct {
		hkl  uintptr
		id   string
		lang core.Lang
		ime  bool
	}{
		{0x04090409, "04090409", core.LangEN, false},
		{0x08090809, "08090809", core.LangEN, false},
		{0x04190419, "04190419", core.LangRU, false},
		{0x043F043F, "043F043F", core.LangKK, false},
		{0x0419043F, "0419043F", core.LangKK, false},   // Kazakh input language, Russian keys
		{0xF0020409, "F0020409", core.LangEN, false},   // a US variant (e.g. Dvorak)
		{0x04110411, "04110411", core.LangNone, true},  // Japanese
		{0xE0010404, "E0010404", core.LangNone, true},  // legacy IME
		{0x04070407, "04070407", core.LangNone, false}, // German: rendered, never proposed
	}
	for _, c := range cases {
		if got := LayoutID(c.hkl); got != c.id {
			t.Errorf("LayoutID(%#x) = %s", c.hkl, got)
		}
		if got := LangForHKL(c.hkl); got != c.lang {
			t.Errorf("LangForHKL(%#x) = %s", c.hkl, got)
		}
		if got := IsIMELayout(c.hkl); got != c.ime {
			t.Errorf("IsIMELayout(%#x) = %v", c.hkl, got)
		}
	}
	// 64-bit HKLs of variants are sign-extended; only the low half names them.
	if LayoutID(uintptr(0xFFFFFFFFF0020409)&^uintptr(0)) != "F0020409" {
		t.Fatal("sign-extended HKL")
	}
}

func TestEmbeddedIconsParse(t *testing.T) {
	for _, name := range []string{"app.ico", "app-off.ico"} {
		data, err := IconAsset(name)
		if err != nil {
			t.Fatal(err)
		}
		images, err := ParseICO(data)
		if err != nil || len(images) < 5 {
			t.Fatalf("%s: %v (%d images)", name, err, len(images))
		}
		for _, size := range []int{16, 20, 24, 32, 48} {
			best, ok := BestIcon(images, size)
			if !ok || best.Width != size {
				t.Errorf("%s: best for %d is %d", name, size, best.Width)
			}
		}
		if best, _ := BestIcon(images, 30); best.Width != 32 {
			t.Errorf("30 px should use 32, got %d", best.Width)
		}
		if best, _ := BestIcon(images, 1000); best.Width != 256 {
			t.Errorf("oversized request should use the largest, got %d", best.Width)
		}
	}
	if _, err := ParseICO([]byte("not an icon")); err == nil {
		t.Fatal("garbage parsed as an icon")
	}
}

func TestLoggerRotates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "log.txt")
	log := NewLogger(path, 200)
	log.SetEnabled(true) // a logger is off until the user turns diagnostics on
	for i := 0; i < 20; i++ {
		log.Printf("line %d with some padding", i)
	}
	log.Close()
	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	old, err := os.ReadFile(path + ".old")
	if err != nil {
		t.Fatal("expected a rotated file")
	}
	if len(current) > 200 || !strings.Contains(string(current), "line 19") || len(old) == 0 {
		t.Fatalf("rotation wrong: %d bytes current", len(current))
	}
	var nilLogger *Logger
	nilLogger.Printf("must not panic")
}
