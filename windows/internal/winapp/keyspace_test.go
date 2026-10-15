package winapp

// The Windows key-code space end to end, on any OS: fixture tables shaped like the Windows
// US, Russian and Kazakh layouts (indexed by scan code, keypad by pseudo-code, exactly as
// buildTable indexes the real ones), the hook's key-code mapping, and the real engine with
// the real models.
//
// The fixtures are hand-written, so they only approximate KBDUS/KBDRU/KBDKAZ — on Windows
// the tables come from ToUnicodeEx. What this proves is that the scan-code indexing, the
// keypad handling and the engine agree with each other.

import (
	"strings"
	"testing"
	"unicode"

	"tilqos/internal/config"
	"tilqos/internal/core"
	"tilqos/internal/engine"
	"tilqos/internal/golden"
)

// Scan codes (set 1) of the main block, row by row, and what each layout puts there:
// unshifted then shifted.
var fixtureRows = []struct {
	scans      []uint16
	us, ru, kk []string
}{
	{
		scans: []uint16{0x29, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D},
		us:    []string{"`~", "1!", "2@", "3#", "4$", "5%", "6^", "7&", "8*", "9(", "0)", "-_", "=+"},
		ru:    []string{"ёЁ", "1!", "2\"", "3№", "4;", "5%", "6:", "7?", "8*", "9(", "0)", "-_", "=+"},
		kk:    []string{"()", "\"!", "әӘ", "іІ", "ңҢ", "ғҒ", ",;", ".:", "үҮ", "ұҰ", "қҚ", "өӨ", "һҺ"},
	},
	{
		scans: []uint16{0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1A, 0x1B, 0x2B},
		us:    []string{"qQ", "wW", "eE", "rR", "tT", "yY", "uU", "iI", "oO", "pP", "[{", "]}", "\\|"},
		ru:    []string{"йЙ", "цЦ", "уУ", "кК", "еЕ", "нН", "гГ", "шШ", "щЩ", "зЗ", "хХ", "ъЪ", "\\/"},
	},
	{
		scans: []uint16{0x1E, 0x1F, 0x20, 0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28},
		us:    []string{"aA", "sS", "dD", "fF", "gG", "hH", "jJ", "kK", "lL", ";:", "'\""},
		ru:    []string{"фФ", "ыЫ", "вВ", "аА", "пП", "рР", "оО", "лЛ", "дД", "жЖ", "эЭ"},
	},
	{
		scans: []uint16{0x2C, 0x2D, 0x2E, 0x2F, 0x30, 0x31, 0x32, 0x33, 0x34, 0x35},
		us:    []string{"zZ", "xX", "cC", "vV", "bB", "nN", "mM", ",<", ".>", "/?"},
		ru:    []string{"яЯ", "чЧ", "сС", "мМ", "иИ", "тТ", "ьЬ", "бБ", "юЮ", ".,"},
		kk:    []string{"яЯ", "чЧ", "сС", "мМ", "иИ", "тТ", "ьЬ", "бБ", "юЮ", "№?"},
	},
}

// The Kazakh rows follow Microsoft's published KBDKAZ layout (KLID 0000043F): the number
// row carries ( " ә і ң ғ , . ү ұ қ ө һ and no layer of it has digits, the letters are
// ЙЦУКЕН, and the key right of Ю is № / ?.

func fixtureTable(id string, lang core.Lang, pick func(row int) []string, decimal string) *core.KeyMapTable {
	forward := make([]string, KeyCodeCount*core.VariantCount)
	set := func(code uint16, mods core.ModState, text string) {
		forward[int(code)*core.VariantCount+int(mods)] = text
	}
	for rowIndex, row := range fixtureRows {
		pairs := pick(rowIndex)
		for i, scan := range row.scans {
			runes := []rune(pairs[i])
			plain, shifted := string(runes[0]), string(runes[1])
			set(scan, 0, plain)
			set(scan, core.ModShift, shifted)
			// Caps Lock affects letters only; Shift+Caps gives lower case back.
			if unicode.IsLetter(runes[0]) {
				set(scan, core.ModCaps, shifted)
				set(scan, core.ModShift|core.ModCaps, plain)
			} else {
				set(scan, core.ModCaps, plain)
				set(scan, core.ModShift|core.ModCaps, shifted)
			}
		}
	}
	for _, mods := range []core.ModState{0, core.ModShift, core.ModCaps, core.ModShift | core.ModCaps} {
		set(0x39, mods, " ")
	}
	for digit := uint16(0); digit <= 9; digit++ {
		code, _ := PhysicalKeyCode(vkNumpad0+digit, 0x47, false)
		set(code, 0, string(rune('0'+digit)))
	}
	code, _ := PhysicalKeyCode(vkDecimal, 0x53, false)
	set(code, 0, decimal)
	return core.NewKeyMapTable(id, id, lang, KeyCodeCount, forward, nil, KeypadCodes())
}

type keyspacePlatform struct {
	tables  []*core.KeyMapTable
	current string
	seq     uint64
	// doc is what the application shows: every key as the active layout renders it.
	doc      string
	injected []core.Replacement
}

func (p *keyspacePlatform) Tables() []*core.KeyMapTable { return p.tables }
func (p *keyspacePlatform) Table(id string) *core.KeyMapTable {
	for _, table := range p.tables {
		if table.SourceID == id {
			return table
		}
	}
	return nil
}
func (p *keyspacePlatform) CurrentLayoutID(uintptr) string { return p.current }
func (p *keyspacePlatform) SelectLayout(_ uintptr, id string) bool {
	p.current = id
	return true
}
func (p *keyspacePlatform) AppName(uintptr) string       { return "notepad.exe" }
func (p *keyspacePlatform) IsPasswordField(uintptr) bool { return false }
func (p *keyspacePlatform) CanInject(uintptr) bool       { return true }

// No readable field: the edit is typed, as in browsers and most modern applications.
func (p *keyspacePlatform) FocusedField(uintptr) core.TextFieldAccess { return nil }
func (p *keyspacePlatform) Keystrokes(uint64) core.KeystrokeSender    { return p }

// SendKeys records what the edit replaced, reading the deleted characters off the document.
func (p *keyspacePlatform) SendKeys(deleting int, inserting string) bool {
	characters := core.Graphemes(p.doc)
	if deleting > len(characters) {
		return false
	}
	kept := strings.Join(characters[:len(characters)-deleting], "")
	p.injected = append(p.injected, core.Replacement{
		DeletedText: strings.Join(characters[len(characters)-deleting:], ""), InsertText: inserting,
	})
	p.doc = kept + inserting
	return true
}
func (p *keyspacePlatform) KeySeq() uint64                        { return p.seq }
func (p *keyspacePlatform) ReplayAccept(config.AcceptKey)         {}
func (p *keyspacePlatform) ShowSuggestion(engine.Suggestion)      {}
func (p *keyspacePlatform) HideSuggestion()                       {}
func (p *keyspacePlatform) SetAcceptKey(config.AcceptKey, bool)   {}
func (p *keyspacePlatform) CopySelection() (string, func(), bool) { return "", nil, false }
func (p *keyspacePlatform) Paste(string, func()) bool             { return false }
func (p *keyspacePlatform) ReadClipboard() (string, bool)         { return "", false }
func (p *keyspacePlatform) WriteClipboard(string) bool            { return false }
func (p *keyspacePlatform) Notify(string, string)                 {}
func (p *keyspacePlatform) PlaySound()                            {}
func (p *keyspacePlatform) Beep()                                 {}

type keyspaceHarness struct {
	t        *testing.T
	platform *keyspacePlatform
	engine   *engine.Engine
	byLang   map[string]*core.KeyMapTable
	clock    float64
}

func newKeyspaceHarness(t *testing.T) *keyspaceHarness {
	us := fixtureTable("04090409", core.LangEN, func(row int) []string { return fixtureRows[row].us }, ".")
	ru := fixtureTable("04190419", core.LangRU, func(row int) []string { return fixtureRows[row].ru }, ",")
	kk := fixtureTable("043F043F", core.LangKK, func(row int) []string {
		if fixtureRows[row].kk != nil {
			return fixtureRows[row].kk
		}
		return fixtureRows[row].ru
	}, ",")
	platform := &keyspacePlatform{tables: []*core.KeyMapTable{us, ru, kk}, current: us.SourceID}
	e := engine.New(platform, config.NewMemoryStore(config.Default()), golden.Models(t), nil, nil)
	e.Foreground(1)
	return &keyspaceHarness{t: t, platform: platform, engine: e, byLang: map[string]*core.KeyMapTable{"en": us, "ru": ru, "kk": kk}}
}

// typeOn presses the keys that produce text on layout on, whatever layout is active — the
// way the hook reports them: scan-code key codes plus modifier flags.
func (h *keyspaceHarness) typeOn(text, on string) {
	h.t.Helper()
	strokes, ok := h.byLang[on].Strokes(text)
	if !ok {
		h.t.Fatalf("%s cannot type %q", on, text)
	}
	for _, stroke := range strokes {
		h.clock += 0.1
		h.platform.seq++
		if text, ok := h.platform.Table(h.platform.current).Text(stroke); ok {
			h.platform.doc += text
		}
		h.engine.HandleKey(engine.KeyEvent{
			Class: engine.KeyText, Key: stroke.Key,
			Shift: stroke.Mods&core.ModShift != 0, Caps: stroke.Mods&core.ModCaps != 0,
			LayoutID: h.platform.current, Window: 1, Time: h.clock, Seq: h.platform.seq,
		})
	}
}

func (h *keyspaceHarness) activate(lang string) { h.platform.current = h.byLang[lang].SourceID }

func TestWindowsKeySpaceLatinCyrillic(t *testing.T) {
	h := newKeyspaceHarness(t)
	h.typeOn("привет ", "ru") // on US: "ghbdtn "
	if len(h.platform.injected) != 1 || h.platform.injected[0].InsertText != "привет " ||
		h.platform.injected[0].DeletedText != "ghbdtn " {
		t.Fatalf("injected %+v", h.platform.injected)
	}
	if h.platform.current != "04190419" {
		t.Fatalf("layout not switched: %s", h.platform.current)
	}
}

func TestWindowsKeySpaceKazakhOnRussian(t *testing.T) {
	h := newKeyspaceHarness(t)
	h.activate("ru")
	h.typeOn("сәлеметсіз ", "kk") // Kazakh letters live on the number row: "с2леметс3з "
	if len(h.platform.injected) != 1 {
		t.Fatalf("injected %+v", h.platform.injected)
	}
	// The planner keeps the common prefix `с`: only what differs is replaced.
	r := h.platform.injected[0]
	if r.DeletedText != "2леметс3з " || r.InsertText != "әлеметсіз " || h.platform.doc != "сәлеметсіз " {
		t.Fatalf("got %+v, screen %q", r, h.platform.doc)
	}
}

func TestWindowsKeySpaceDigitsOnKazakh(t *testing.T) {
	h := newKeyspaceHarness(t)
	h.activate("ru")
	h.typeOn("мне ", "ru")
	h.activate("kk")
	h.typeOn("25", "en") // number row on the Kazakh layout: "әғ"
	h.activate("ru")
	h.typeOn(" лет ", "ru")
	found := false
	for _, r := range h.platform.injected {
		if strings.Contains(r.InsertText, "25") && strings.Contains(r.DeletedText, "әғ") {
			found = true
		}
	}
	if !found {
		t.Fatalf("әғ should have become 25: %+v", h.platform.injected)
	}
}

func TestWindowsKeySpaceNumpadDigitsAreDigitsEverywhere(t *testing.T) {
	h := newKeyspaceHarness(t)
	h.activate("kk")
	// The keypad types digits on every layout: resolving "2" back to a key must prefer
	// the number row, and keypad strokes must render as digits under Kazakh too.
	two, _ := h.byLang["en"].Stroke("2")
	if two.Key != 0x03 {
		t.Fatalf("\"2\" must resolve to the number row, got %#x", two.Key)
	}
	for _, digit := range []uint16{2, 5} {
		code, _ := PhysicalKeyCode(vkNumpad0+digit, 0x50, false)
		h.clock += 0.1
		h.platform.seq++
		if text, ok := h.platform.Table(h.platform.current).Text(core.KeyStroke{Key: code}); ok {
			h.platform.doc += text
		}
		h.engine.HandleKey(engine.KeyEvent{Class: engine.KeyText, Key: code, LayoutID: h.platform.current, Window: 1, Time: h.clock, Seq: h.platform.seq})
	}
	h.typeOn(" ", "kk")
	if len(h.platform.injected) != 0 {
		t.Fatalf("keypad digits are correct on any layout: %+v", h.platform.injected)
	}
}

func TestWindowsKeySpaceCapsLock(t *testing.T) {
	h := newKeyspaceHarness(t)
	h.clock = 0
	// "ghbdtn" with Caps Lock on shows "GHBDTN": an acronym, left alone like on macOS.
	for _, c := range "ghbdtn " {
		stroke, _ := h.byLang["en"].Stroke(string(c))
		h.clock += 0.1
		h.platform.seq++
		if c != ' ' {
			stroke.Mods |= core.ModCaps
		}
		if text, ok := h.platform.Table(h.platform.current).Text(stroke); ok {
			h.platform.doc += text
		}
		h.engine.HandleKey(engine.KeyEvent{Class: engine.KeyText, Key: stroke.Key, Caps: c != ' ',
			LayoutID: h.platform.current, Window: 1, Time: h.clock, Seq: h.platform.seq})
	}
	if len(h.platform.injected) != 0 {
		t.Fatalf("all caps is treated as an acronym: %+v", h.platform.injected)
	}
}
