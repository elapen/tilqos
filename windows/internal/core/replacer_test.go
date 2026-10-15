package core_test

// TextReplacerTests.swift, ported: the replacement logic against simulated apps, including
// the ones that misbehave. For each one the text must end up right or be left untouched —
// never both old and new.

import (
	"strings"
	"testing"
	"time"
	"unicode"
	"unicode/utf16"

	"tilqos/internal/core"
)

// fakeQuirks are the quirks real apps have.
type fakeQuirks struct {
	// ignoresSelection: accepts a new selection but keeps the old one (and says it
	// succeeded).
	ignoresSelection bool
	// selectionDelay: applies a new selection only after this many reads of it.
	selectionDelay int
	// insertsWithoutDeleting: "replace the selection" inserts at the end of it.
	insertsWithoutDeleting bool
	// ignoresWrites: "replace the selection" does nothing (and says it succeeded).
	ignoresWrites bool
	// writeReportsFailure: "replace the selection" works but reports an error.
	writeReportsFailure bool
	unreadable          bool
	noSelection         bool
	// lagReads: the last keystroke has not reached the field yet for this many reads.
	lagReads    int
	laggingText string
}

// fakeField is a text field with a caret, counted in UTF-16 units like NSString and Edit
// controls.
type fakeField struct {
	text             []uint16
	selection        core.Range16
	quirks           fakeQuirks
	pendingSelection *core.Range16
	pendingReads     int
	lagReadsLeft     int
}

func newFakeField(text string, quirks fakeQuirks) *fakeField {
	f := &fakeField{text: utf16.Encode([]rune(text)), quirks: quirks, lagReadsLeft: quirks.lagReads}
	f.selection = core.Range16{Location: len(f.text)}
	return f
}

func (f *fakeField) String() string { return string(utf16.Decode(f.text)) }

func (f *fakeField) SelectedRange() (core.Range16, bool) {
	if f.quirks.noSelection {
		return core.Range16{}, false
	}
	if f.pendingSelection != nil {
		f.pendingReads--
		if f.pendingReads < 0 {
			f.selection = *f.pendingSelection
			f.pendingSelection = nil
		}
	}
	if f.lagReadsLeft > 0 {
		f.lagReadsLeft--
		if f.lagReadsLeft == 0 {
			// The keystroke arrives: text and caret move together.
			f.text = append(f.text, utf16.Encode([]rune(f.quirks.laggingText))...)
			f.selection = core.Range16{Location: len(f.text)}
		}
	}
	return f.selection, true
}

func (f *fakeField) SetSelectedRange(r core.Range16) bool {
	if f.quirks.ignoresSelection {
		return true
	}
	if f.quirks.selectionDelay > 0 {
		pending := r
		f.pendingSelection = &pending
		f.pendingReads = f.quirks.selectionDelay
		return true
	}
	f.selection = r
	return true
}

func (f *fakeField) StringIn(r core.Range16) (string, bool) {
	if f.quirks.unreadable || r.Location < 0 || r.Location+r.Length > len(f.text) {
		return "", false
	}
	return string(utf16.Decode(f.text[r.Location : r.Location+r.Length])), true
}

func (f *fakeField) SelectedText() (string, bool) {
	if f.quirks.unreadable {
		return "", false
	}
	return f.StringIn(f.selection)
}

func (f *fakeField) replace(r core.Range16, with string) {
	inserted := utf16.Encode([]rune(with))
	out := make([]uint16, 0, len(f.text)-r.Length+len(inserted))
	out = append(out, f.text[:r.Location]...)
	out = append(out, inserted...)
	out = append(out, f.text[r.Location+r.Length:]...)
	f.text = out
}

func (f *fakeField) SetSelectedText(text string) bool {
	if f.quirks.ignoresWrites {
		return true
	}
	length := core.UTF16Len(text)
	if f.quirks.insertsWithoutDeleting {
		at := f.selection.Location + f.selection.Length
		f.replace(core.Range16{Location: at}, text)
		f.selection = core.Range16{Location: at + length}
		return true
	}
	f.replace(f.selection, text)
	f.selection = core.Range16{Location: f.selection.Location + length}
	return !f.quirks.writeReportsFailure
}

func (f *fakeField) CanSetSelectedRange() bool { return true }
func (f *fakeField) CanSetSelectedText() bool  { return true }

// backspace is what a Backspace keystroke does to it: the selection, or the last character.
func (f *fakeField) backspace() {
	if f.selection.Length > 0 {
		f.replace(f.selection, "")
		f.selection.Length = 0
		return
	}
	if f.selection.Location == 0 {
		return
	}
	before := string(utf16.Decode(f.text[:f.selection.Location]))
	characters := core.Graphemes(before)
	last := core.UTF16Len(characters[len(characters)-1])
	start := f.selection.Location - last
	f.replace(core.Range16{Location: start, Length: last}, "")
	f.selection = core.Range16{Location: start}
}

func (f *fakeField) typeText(text string) {
	f.replace(f.selection, text)
	f.selection = core.Range16{Location: f.selection.Location + core.UTF16Len(text)}
}

// fakeKeys are keystrokes that land in the fake field, and a record of them.
type fakeKeys struct {
	field      *fakeField
	backspaces int
	chunks     []string
}

func (k *fakeKeys) SendKeys(deleting int, inserting string) bool {
	k.backspaces += deleting
	for i := 0; i < deleting; i++ {
		if k.field != nil {
			k.field.backspace()
		}
	}
	for _, chunk := range core.Chunks(inserting, core.ChunkLimit) {
		k.chunks = append(k.chunks, chunk)
		if k.field != nil {
			k.field.typeText(chunk)
		}
	}
	return true
}

var replacerFix = core.Replacement{DeletedText: "ghbdtn ", InsertText: "привет "}

func runReplacer(field *fakeField, replacement core.Replacement, preferField, verify bool) (core.ReplaceOutcome, *fakeKeys, *core.TextReplacer) {
	keys := &fakeKeys{field: field}
	replacer := &core.TextReplacer{Keys: keys, Wait: func(time.Duration) {}, PreferAccessibility: preferField, Verify: verify}
	if field != nil {
		replacer.Field = field
	}
	return replacer.Apply(replacement), keys, replacer
}

func applied(strategy core.ReplaceStrategy, inserted string) core.ReplaceOutcome {
	return core.ReplaceOutcome{Result: core.ReplaceApplied, Strategy: strategy, Inserted: inserted}
}

// MARK: - Well-behaved apps

func TestWellBehavedAppIsEditedThroughTheField(t *testing.T) {
	field := newFakeField("я сказал ghbdtn ", fakeQuirks{})
	outcome, keys, _ := runReplacer(field, replacerFix, true, true)
	if outcome != applied(core.StrategyAccessibility, "привет ") || field.String() != "я сказал привет " ||
		field.selection != (core.Range16{Location: len(field.text)}) || keys.backspaces != 0 {
		t.Fatalf("%+v %q %+v %d", outcome, field.String(), field.selection, keys.backspaces)
	}
}

func TestAsynchronousSelectionIsWaitedFor(t *testing.T) {
	field := newFakeField("ghbdtn ", fakeQuirks{selectionDelay: 2})
	outcome, _, _ := runReplacer(field, replacerFix, true, true)
	if outcome.Strategy != core.StrategyAccessibility || field.String() != "привет " {
		t.Fatalf("%+v %q", outcome, field.String())
	}
}

func TestWriteThatReportsFailureButWorksIsAccepted(t *testing.T) {
	field := newFakeField("ghbdtn ", fakeQuirks{writeReportsFailure: true})
	outcome, keys, _ := runReplacer(field, replacerFix, true, true)
	if outcome.Strategy != core.StrategyAccessibility || field.String() != "привет " || keys.backspaces != 0 {
		t.Fatalf("a second, synthetic attempt would have doubled it: %+v %q %d", outcome, field.String(), keys.backspaces)
	}
}

// MARK: - The reported bug: text lands next to the old text

func TestAppIgnoringTheSelectionNeverGetsTextNextToTheOldOne(t *testing.T) {
	field := newFakeField("ok ghbdtn ", fakeQuirks{ignoresSelection: true})
	outcome, keys, replacer := runReplacer(field, replacerFix, true, true)
	if field.String() != "ok привет " || outcome.Strategy != core.StrategySynthetic || keys.backspaces != 7 ||
		!replacer.AccessibilityUnreliable {
		t.Fatalf("%+v %q %d %v", outcome, field.String(), keys.backspaces, replacer.AccessibilityUnreliable)
	}
}

func TestAppInsertingWithoutDeletingIsRepaired(t *testing.T) {
	field := newFakeField("ok ghbdtn ", fakeQuirks{insertsWithoutDeleting: true})
	outcome, _, replacer := runReplacer(field, replacerFix, true, true)
	if field.String() != "ok привет " || outcome.Strategy != core.StrategySynthetic || !replacer.AccessibilityUnreliable {
		t.Fatalf("the stray old text must be gone: %+v %q", outcome, field.String())
	}
}

func TestAppIgnoringTheWriteFallsBackCleanly(t *testing.T) {
	field := newFakeField("ok ghbdtn ", fakeQuirks{ignoresWrites: true})
	outcome, _, replacer := runReplacer(field, replacerFix, true, true)
	if field.String() != "ok привет " || outcome.Strategy != core.StrategySynthetic || !replacer.AccessibilityUnreliable {
		t.Fatalf("%+v %q", outcome, field.String())
	}
}

// MARK: - The screen disagrees with the buffer

func TestMismatchTouchesNothing(t *testing.T) {
	field := newFakeField("ok ghbdtm ", fakeQuirks{})
	outcome, keys, replacer := runReplacer(field, replacerFix, true, true)
	if outcome.Result != core.ReplaceMismatch || field.String() != "ok ghbdtm " || keys.backspaces != 0 ||
		len(keys.chunks) != 0 || replacer.AccessibilityUnreliable {
		t.Fatalf("%+v %q %d %v", outcome, field.String(), keys.backspaces, keys.chunks)
	}
}

func TestMismatchAlsoStopsTheKeystrokeRoute(t *testing.T) {
	field := newFakeField("ok ghbdtm ", fakeQuirks{})
	outcome, keys, _ := runReplacer(field, replacerFix, false, true)
	if outcome.Result != core.ReplaceMismatch || keys.backspaces != 0 {
		t.Fatalf("%+v %d", outcome, keys.backspaces)
	}
}

// The separator that triggered the fix has not reached the app yet.
func TestLaggingKeystrokeIsWaitedFor(t *testing.T) {
	field := newFakeField("ghbdtn", fakeQuirks{lagReads: 2, laggingText: " "})
	outcome, _, _ := runReplacer(field, replacerFix, true, true)
	if outcome.Strategy != core.StrategyAccessibility || field.String() != "привет " {
		t.Fatalf("%+v %q", outcome, field.String())
	}
}

// Apps capitalise the first word of a sentence behind our back.
func TestHostCapitalisationIsCarriedOver(t *testing.T) {
	field := newFakeField("Ghbdtn ", fakeQuirks{})
	outcome, _, _ := runReplacer(field, replacerFix, true, true)
	if outcome != applied(core.StrategyAccessibility, "Привет ") || field.String() != "Привет " {
		t.Fatalf("%+v %q", outcome, field.String())
	}
}

func TestSelectedTextIsNeverDeleted(t *testing.T) {
	field := newFakeField("ok ghbdtn ", fakeQuirks{})
	field.selection = core.Range16{Location: 0, Length: 2}
	outcome, keys, _ := runReplacer(field, replacerFix, true, true)
	if outcome.Result != core.ReplaceFailed || outcome.Reason != "text is selected" || field.String() != "ok ghbdtn " ||
		keys.backspaces != 0 {
		t.Fatalf("%+v %q %d", outcome, field.String(), keys.backspaces)
	}
}

// MARK: - Apps we cannot read

func TestUnreadableFieldUsesKeystrokes(t *testing.T) {
	field := newFakeField("ghbdtn ", fakeQuirks{unreadable: true})
	outcome, keys, _ := runReplacer(field, replacerFix, true, true)
	if outcome.Strategy != core.StrategySynthetic || keys.backspaces != 7 || field.String() != "привет " {
		t.Fatalf("%+v %d %q", outcome, keys.backspaces, field.String())
	}
}

func TestNoFocusedFieldUsesKeystrokes(t *testing.T) {
	outcome, keys, _ := runReplacer(nil, replacerFix, true, true)
	if outcome != applied(core.StrategySynthetic, "привет ") || keys.backspaces != 7 ||
		strings.Join(keys.chunks, "|") != "привет " {
		t.Fatalf("%+v %d %q", outcome, keys.backspaces, keys.chunks)
	}
}

func TestTerminalsSkipTheCheck(t *testing.T) {
	// A terminal's text is its scrollback; it would never match.
	field := newFakeField("$ ls\nsomething else entirely", fakeQuirks{})
	outcome, keys, _ := runReplacer(field, replacerFix, false, false)
	if outcome.Strategy != core.StrategySynthetic || keys.backspaces != 7 {
		t.Fatalf("%+v %d", outcome, keys.backspaces)
	}
}

// MARK: - The user types on (Windows)

func TestTypingOnAbandonsTheEditUntouched(t *testing.T) {
	for _, quirks := range []fakeQuirks{{}, {unreadable: true}} {
		field := newFakeField("ok ghbdtn ", quirks)
		keys := &fakeKeys{field: field}
		replacer := &core.TextReplacer{Field: field, Keys: keys, Wait: func(time.Duration) {},
			PreferAccessibility: true, Verify: true, Current: func() bool { return false }}
		outcome := replacer.Apply(replacerFix)
		if !outcome.Abandoned || outcome.Result != core.ReplaceFailed || field.String() != "ok ghbdtn " ||
			keys.backspaces != 0 || replacer.AccessibilityUnreliable {
			t.Fatalf("quirks %+v: %+v %q %d", quirks, outcome, field.String(), keys.backspaces)
		}
	}
}

// A key that arrives while our selection is up must not replace it: the selection is
// taken down again and the text left as it was.
func TestTypingOnWhileSelectedTakesTheSelectionDown(t *testing.T) {
	field := newFakeField("ok ghbdtn ", fakeQuirks{})
	calls := 0
	replacer := &core.TextReplacer{Field: field, Keys: &fakeKeys{field: field}, Wait: func(time.Duration) {},
		PreferAccessibility: true, Verify: true,
		Current: func() bool { calls++; return calls == 1 }}
	outcome := replacer.Apply(replacerFix)
	if !outcome.Abandoned || field.String() != "ok ghbdtn " || field.selection != (core.Range16{Location: 10}) {
		t.Fatalf("%+v %q %+v", outcome, field.String(), field.selection)
	}
}

// MARK: - Long text

func TestLongInsertIsChunked(t *testing.T) {
	long := core.Replacement{DeletedText: "ghbdtn rfr ltkf e nt,z ", InsertText: "привет как дела у тебя, всё хорошо? "}
	field := newFakeField(long.DeletedText, fakeQuirks{unreadable: true})
	_, keys, _ := runReplacer(field, long, true, true)
	if field.String() != long.InsertText || len(keys.chunks) <= 1 {
		t.Fatalf("%q %q", field.String(), keys.chunks)
	}
	for _, chunk := range keys.chunks {
		if core.UTF16Len(chunk) > core.ChunkLimit {
			t.Fatalf("chunk %q is %d units", chunk, core.UTF16Len(chunk))
		}
	}
}

func TestChunksNeverSplitACharacter(t *testing.T) {
	text := "👨‍👩‍👧‍👦 сәлем 🇰🇿 hello"
	chunks := core.Chunks(text, 8)
	if strings.Join(chunks, "") != text {
		t.Fatalf("chunks %q do not add up", chunks)
	}
	for _, chunk := range chunks {
		if core.CharCount(chunk) > 1 && core.UTF16Len(chunk) > 8 {
			t.Fatalf("chunk %q is %d units", chunk, core.UTF16Len(chunk))
		}
		for _, r := range chunk {
			if r == unicode.ReplacementChar {
				t.Fatalf("split surrogate in %q", chunk)
			}
		}
	}
}
