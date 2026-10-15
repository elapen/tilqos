package core

// Port of Input/KeyStroke.swift and the platform-neutral part of Input/KeyMapTable.swift.
//
// The Swift app keys its tables by macOS virtual key code; the Windows app keys them by
// physical key (scan code, see internal/winapp/keys.go). Nothing in this package cares:
// a table is just "(key code, modifier state) → text" for one layout, with the key-code
// range and the keypad keys supplied by whoever builds it.

// ModState is the modifier combination that changes which character a key produces.
//
// Command/Control (and Windows' Ctrl/Win) are deliberately absent: they produce no
// character, and a keystroke carrying either is a shortcut, not text. ModOption is macOS
// Option and Windows AltGr — the third level of a layout.
type ModState uint8

const (
	ModShift  ModState = 1 << 0
	ModOption ModState = 1 << 1
	ModCaps   ModState = 1 << 2
	// VariantCount is all 8 combinations; it sizes the key tables.
	VariantCount = 8
)

// ModsByCost is the order used when building the reverse (character → keystroke) map.
// Fewer modifiers wins, so a character reachable both plainly and via Option/AltGr
// resolves to the plain keystroke.
var ModsByCost = []ModState{
	0, ModShift, ModCaps, ModShift | ModCaps,
	ModOption, ModOption | ModShift, ModOption | ModCaps, ModOption | ModShift | ModCaps,
}

// TypingVariants are the modifier combinations ordinary typing uses. Option/AltGr is
// excluded on purpose: layouts diverge wildly there, and those characters never appear in
// a mistyped word.
var TypingVariants = []ModState{0, ModShift, ModCaps, ModShift | ModCaps}

// KeyStroke is one physical key press, stored as the key plus the modifiers that affect
// its character — never as the character itself.
//
// This is the central data decision of the app. Kazakh letters sit on the number row, so a
// character-to-character table cannot round-trip Kazakh text; keeping the keystroke lets
// any layout re-render the same input losslessly.
type KeyStroke struct {
	Key  uint16
	Mods ModState
}

func (s KeyStroke) tableIndex() int { return int(s.Key)*VariantCount + int(s.Mods) }

// RecordedStroke is a keystroke as recorded: what was pressed, when (seconds, any
// monotonic origin), and which layout was active at that moment.
type RecordedStroke struct {
	Stroke   KeyStroke
	Time     float64
	LayoutID string
}

// KeyMapTable is the character each key produces under one keyboard layout, both
// directions.
type KeyMapTable struct {
	SourceID string
	Name     string
	Lang     Lang

	keyCount int
	// forward[stroke.tableIndex()] — the text a keystroke produces, "" for none.
	forward []string
	// dead marks forward entries that are dead keys (Windows); nil when there are none.
	dead []bool
	// wordStrokes says whether each forward entry is part of a word. Precomputed: the
	// question is asked for every keystroke, on the hook's time budget, and for every stroke
	// of every evaluation.
	wordStrokes []bool
	// reverse is the cheapest keystroke that produces a given Character.
	reverse map[string]KeyStroke
}

// NewKeyMapTable builds a table from its forward entries.
//
// forward has keyCount*VariantCount entries, indexed key*VariantCount+mods; "" means the
// keystroke produces nothing. keypad lists the numeric keypad keys, which produce the same
// digits on every layout and so must never win when resolving a character back to a key.
// dead may be nil.
func NewKeyMapTable(sourceID, name string, lang Lang, keyCount int, forward []string, dead []bool, keypad map[uint16]bool) *KeyMapTable {
	if len(forward) != keyCount*VariantCount {
		panic("forward table must be keyCount * VariantCount entries")
	}
	if dead != nil && len(dead) != len(forward) {
		panic("dead table must match the forward table")
	}
	table := &KeyMapTable{
		SourceID: sourceID, Name: name, Lang: lang,
		keyCount: keyCount, forward: forward, dead: dead,
		wordStrokes: make([]bool, len(forward)),
		reverse:     make(map[string]KeyStroke, 128),
	}
	for index, text := range forward {
		table.wordStrokes[index] = text != "" && AllChars(text, IsWordCharacter)
	}
	// Main block first, keypad only as a last resort; within each, the cheapest modifier
	// combination wins.
	for _, keypadPass := range []bool{false, true} {
		for _, mods := range ModsByCost {
			for key := 0; key < keyCount; key++ {
				code := uint16(key)
				if keypad[code] != keypadPass {
					continue
				}
				stroke := KeyStroke{Key: code, Mods: mods}
				text := forward[stroke.tableIndex()]
				if text == "" || CharCount(text) != 1 {
					continue
				}
				if dead != nil && dead[stroke.tableIndex()] {
					continue
				}
				key := Canon(text)
				if _, taken := table.reverse[key]; !taken {
					table.reverse[key] = stroke
				}
			}
		}
	}
	return table
}

// KeyCount is the size of the key-code space.
func (t *KeyMapTable) KeyCount() int { return t.keyCount }

// Text is the text a single keystroke produces under this layout.
func (t *KeyMapTable) Text(stroke KeyStroke) (string, bool) {
	index := stroke.tableIndex()
	if index >= len(t.forward) {
		return "", false
	}
	text := t.forward[index]
	return text, text != ""
}

// ProducesWordCharacter reports whether the keystroke produces part of a word — a letter, a
// digit or an apostrophe.
func (t *KeyMapTable) ProducesWordCharacter(stroke KeyStroke) bool {
	index := stroke.tableIndex()
	return index < len(t.wordStrokes) && t.wordStrokes[index]
}

// IsDeadKey reports whether the keystroke is a dead key on this layout: it produces no
// character by itself but changes the next one.
func (t *KeyMapTable) IsDeadKey(stroke KeyStroke) bool {
	index := stroke.tableIndex()
	return t.dead != nil && index < len(t.dead) && t.dead[index]
}

// ProducesText reports whether the key produces text in any typing variant.
func (t *KeyMapTable) ProducesText(key uint16) bool {
	for _, mods := range ModsByCost {
		if _, ok := t.Text(KeyStroke{Key: key, Mods: mods}); ok {
			return true
		}
	}
	return false
}

// Render re-renders a keystroke sequence under this layout. Keys that produce nothing
// here are dropped.
func (t *KeyMapTable) Render(strokes []KeyStroke) string {
	out := make([]byte, 0, len(strokes)*2)
	for _, stroke := range strokes {
		if text, ok := t.Text(stroke); ok {
			out = append(out, text...)
		}
	}
	return string(out)
}

// Stroke is the keystroke a user would press on this layout to get one Character.
func (t *KeyMapTable) Stroke(character string) (KeyStroke, bool) {
	stroke, ok := t.reverse[Canon(character)]
	return stroke, ok
}

// Strokes recovers the keystroke sequence behind already-typed text; false if any
// character is unreachable on this layout.
func (t *KeyMapTable) Strokes(text string) ([]KeyStroke, bool) {
	var out []KeyStroke
	it := graphemeIter{s: text}
	for c := it.next(); c != ""; c = it.next() {
		stroke, ok := t.Stroke(c)
		if !ok {
			return nil, false
		}
		out = append(out, stroke)
	}
	return out, true
}

// ReverseLen is the number of characters the reverse map resolves (for tests).
func (t *KeyMapTable) ReverseLen() int { return len(t.reverse) }

// DifferingKeyCodes lists key codes whose character differs between two layouts, for the
// given variants.
func (t *KeyMapTable) DifferingKeyCodes(other *KeyMapTable, variants []ModState) []uint16 {
	var result []uint16
	for key := 0; key < t.keyCount && key < other.keyCount; key++ {
		for _, mods := range variants {
			stroke := KeyStroke{Key: uint16(key), Mods: mods}
			a, _ := t.Text(stroke)
			b, _ := other.Text(stroke)
			if a != b {
				result = append(result, uint16(key))
				break
			}
		}
	}
	return result
}
