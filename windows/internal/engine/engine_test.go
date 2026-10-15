package engine

// Pipeline behaviour, end to end, against a fake platform that uses the real macOS tables
// and models from the golden data and keeps a simulated document: what ends up on screen,
// when the layout switches, how the buffer follows the screen, how suggestions, undo and
// learning behave, and what happens when the screen and the buffer disagree.

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"tilqos/internal/config"
	"tilqos/internal/core"
	"tilqos/internal/golden"
	"tilqos/internal/translit"
)

// fakeDoc is the text of the focused control: what the application shows. It is also the
// control's TextFieldAccess when the fake says the field is readable (an Edit control).
type fakeDoc struct {
	text      []uint16
	selection core.Range16
	// capitalize makes the application capitalise the first letter of the document when a
	// space follows it, as word processors do.
	capitalize bool
	// autocorrect replaces the whole document when a space is typed, behind our back.
	autocorrect map[string]string
	writes      int
	// onRead runs at the start of the next selection read, to simulate a key the user
	// presses while a replacement is being checked.
	onRead func()
}

func (d *fakeDoc) String() string { return string(utf16.Decode(d.text)) }

func (d *fakeDoc) set(text string) {
	d.text = utf16.Encode([]rune(text))
	d.selection = core.Range16{Location: len(d.text)}
}

func (d *fakeDoc) replace(r core.Range16, with string) {
	inserted := utf16.Encode([]rune(with))
	out := append([]uint16{}, d.text[:r.Location]...)
	out = append(out, inserted...)
	out = append(out, d.text[r.Location+r.Length:]...)
	d.text = out
	d.selection = core.Range16{Location: r.Location + len(inserted)}
}

func (d *fakeDoc) typeText(text string) {
	d.replace(d.selection, text)
	if text == " " {
		if replacement, ok := d.autocorrect[d.String()]; ok {
			d.set(replacement)
		}
		if d.capitalize {
			if runes := []rune(d.String()); len(runes) > 0 {
				d.set(strings.ToUpper(string(runes[:1])) + string(runes[1:]))
			}
		}
	}
}

func (d *fakeDoc) backspace() {
	if d.selection.Length > 0 {
		d.replace(d.selection, "")
		return
	}
	if d.selection.Location == 0 {
		return
	}
	characters := core.Graphemes(string(utf16.Decode(d.text[:d.selection.Location])))
	last := core.UTF16Len(characters[len(characters)-1])
	d.replace(core.Range16{Location: d.selection.Location - last, Length: last}, "")
}

func (d *fakeDoc) SelectedRange() (core.Range16, bool) {
	if hook := d.onRead; hook != nil {
		d.onRead = nil
		hook()
	}
	return d.selection, true
}
func (d *fakeDoc) SetSelectedRange(r core.Range16) bool { d.selection = r; return true }
func (d *fakeDoc) StringIn(r core.Range16) (string, bool) {
	if r.Location < 0 || r.Location+r.Length > len(d.text) {
		return "", false
	}
	return string(utf16.Decode(d.text[r.Location : r.Location+r.Length])), true
}
func (d *fakeDoc) SelectedText() (string, bool) { return d.StringIn(d.selection) }
func (d *fakeDoc) SetSelectedText(text string) bool {
	d.writes++
	d.replace(d.selection, text)
	return true
}
func (d *fakeDoc) CanSetSelectedRange() bool { return true }
func (d *fakeDoc) CanSetSelectedText() bool  { return true }

type send struct {
	deleting  int
	inserting string
}

type fakePlatform struct {
	tables  []*core.KeyMapTable
	byID    map[string]*core.KeyMapTable
	current map[uintptr]string
	apps    map[uintptr]string

	doc *fakeDoc
	// readable: the focused control is an Edit control whose text can be read.
	readable bool

	password  bool
	blocked   bool
	keySeq    uint64
	sends     []send
	selected  []string
	shown     []Suggestion
	hidden    int
	acceptKey config.AcceptKey
	armed     bool
	replayed  []config.AcceptKey
	notified  []string

	clipboard     string
	hasClipboard  bool
	selection     string
	pasted        []string
	restoreCalled int
	beeps         int
}

func newFake(t *testing.T) (*fakePlatform, map[string]*core.KeyMapTable) {
	tables, _, _ := golden.Tables(t)
	f := &fakePlatform{
		byID:    map[string]*core.KeyMapTable{},
		current: map[uintptr]string{},
		apps:    map[uintptr]string{1: "notepad.exe", 2: "cmd.exe", 3: "winword.exe"},
		doc:     &fakeDoc{},
	}
	for _, short := range []string{"abc", "ru", "kk"} {
		f.tables = append(f.tables, tables[short])
		f.byID[tables[short].SourceID] = tables[short]
	}
	return f, tables
}

func (f *fakePlatform) Tables() []*core.KeyMapTable           { return f.tables }
func (f *fakePlatform) Table(id string) *core.KeyMapTable     { return f.byID[id] }
func (f *fakePlatform) CurrentLayoutID(window uintptr) string { return f.current[window] }
func (f *fakePlatform) SelectLayout(window uintptr, id string) bool {
	f.selected = append(f.selected, id)
	f.current[window] = id
	return true
}
func (f *fakePlatform) AppName(window uintptr) string       { return f.apps[window] }
func (f *fakePlatform) IsPasswordField(window uintptr) bool { return f.password }
func (f *fakePlatform) CanInject(window uintptr) bool       { return !f.blocked }
func (f *fakePlatform) FocusedField(window uintptr) core.TextFieldAccess {
	if !f.readable {
		return nil
	}
	return f.doc
}

// fakeKeys types into the document, refusing like the real sender when a key press arrived
// after the one the edit was planned against.
type fakeKeys struct {
	f         *fakePlatform
	expectSeq uint64
}

func (k fakeKeys) SendKeys(deleting int, inserting string) bool {
	if k.expectSeq != 0 && k.f.keySeq != k.expectSeq {
		return false
	}
	if deleting == 0 {
		return false // never insert without deleting
	}
	k.f.sends = append(k.f.sends, send{deleting, inserting})
	for i := 0; i < deleting; i++ {
		k.f.doc.backspace()
	}
	k.f.doc.replace(k.f.doc.selection, inserting)
	return true
}

func (f *fakePlatform) Keystrokes(expectSeq uint64) core.KeystrokeSender {
	return fakeKeys{f: f, expectSeq: expectSeq}
}
func (f *fakePlatform) KeySeq() uint64                    { return f.keySeq }
func (f *fakePlatform) ReplayAccept(key config.AcceptKey) { f.replayed = append(f.replayed, key) }
func (f *fakePlatform) ShowSuggestion(s Suggestion)       { f.shown = append(f.shown, s) }
func (f *fakePlatform) HideSuggestion()                   { f.hidden++ }
func (f *fakePlatform) SetAcceptKey(key config.AcceptKey, active bool) {
	f.acceptKey, f.armed = key, active
}
func (f *fakePlatform) CopySelection() (string, func(), bool) {
	restore := func() { f.restoreCalled++ }
	return f.selection, restore, f.selection != ""
}
func (f *fakePlatform) Paste(text string, restore func()) bool {
	f.pasted = append(f.pasted, text)
	restore()
	return true
}
func (f *fakePlatform) ReadClipboard() (string, bool) { return f.clipboard, f.hasClipboard }
func (f *fakePlatform) WriteClipboard(text string) bool {
	f.clipboard, f.hasClipboard = text, true
	return true
}
func (f *fakePlatform) Notify(title, message string) { f.notified = append(f.notified, title) }
func (f *fakePlatform) PlaySound()                   {}
func (f *fakePlatform) Beep()                        { f.beeps++ }

type harness struct {
	t      *testing.T
	fake   *fakePlatform
	tables map[string]*core.KeyMapTable
	store  *config.Store
	engine *Engine
	clock  float64
	window uintptr
}

func newHarness(t *testing.T) *harness {
	return newHarnessWith(t, config.Default())
}

func newHarnessWith(t *testing.T, cfg config.Config) *harness {
	fake, tables := newFake(t)
	store := config.NewMemoryStore(cfg)
	engine := New(fake, store, golden.Models(t), nil, nil)
	engine.SetSleep(func(time.Duration) {})
	h := &harness{t: t, fake: fake, tables: tables, store: store, engine: engine, window: 1}
	fake.current[1] = tables["abc"].SourceID
	fake.current[2] = tables["abc"].SourceID
	engine.Foreground(1)
	return h
}

// typeText presses the keys that produce text on layout on, with the window's current
// layout active — i.e. what a user types, whatever layout is selected. The application
// shows each key before the engine hears of it, as with the real hook.
func (h *harness) typeText(text, on string) {
	h.t.Helper()
	strokes, ok := h.tables[on].Strokes(text)
	if !ok {
		h.t.Fatalf("%s cannot type %q", on, text)
	}
	for _, stroke := range strokes {
		h.press(stroke)
	}
}

func (h *harness) press(stroke core.KeyStroke) {
	h.clock += 0.1
	h.fake.keySeq++
	if produced, ok := h.fake.Table(h.fake.current[h.window]).Text(stroke); ok {
		h.fake.doc.typeText(produced)
	}
	h.engine.HandleKey(KeyEvent{
		Class:    KeyText,
		Key:      stroke.Key,
		Shift:    stroke.Mods&core.ModShift != 0,
		Caps:     stroke.Mods&core.ModCaps != 0,
		Ctrl:     stroke.Mods&core.ModOption != 0,
		Alt:      stroke.Mods&core.ModOption != 0,
		LayoutID: h.fake.current[h.window],
		Window:   h.window,
		Time:     h.clock,
		Seq:      h.fake.keySeq,
	})
}

func (h *harness) key(class KeyClass) {
	h.clock += 0.1
	h.fake.keySeq++
	if class == KeyBackspace {
		h.fake.doc.backspace()
	}
	h.engine.HandleKey(KeyEvent{Class: class, LayoutID: h.fake.current[h.window], Window: h.window, Time: h.clock, Seq: h.fake.keySeq})
}

func (h *harness) screen() string { return h.fake.doc.String() }

func lastSend(t *testing.T, f *fakePlatform) send {
	t.Helper()
	if len(f.sends) == 0 {
		t.Fatal("nothing was typed")
	}
	return f.sends[len(f.sends)-1]
}

func suggestMode() config.Config {
	cfg := config.Default()
	cfg.DefaultFixMode = config.ModeSuggest
	return cfg
}

func TestAutoFixReplacesSwitchesAndRewritesTheBuffer(t *testing.T) {
	h := newHarness(t)
	h.typeText("привет ", "ru") // on ABC: "ghbdtn "
	if s := lastSend(t, h.fake); s.deleting != 7 || s.inserting != "привет " {
		t.Fatalf("sent %+v", s)
	}
	if h.screen() != "привет " {
		t.Fatalf("screen %q", h.screen())
	}
	if got := h.fake.current[1]; got != h.tables["ru"].SourceID {
		t.Fatalf("layout not switched to Russian: %s", got)
	}
	if got := h.engine.ScreenText(); got != "привет " {
		t.Fatalf("buffer should describe the corrected screen, got %q", got)
	}
	if h.engine.UndoCount() != 1 {
		t.Fatal("fix not recorded for undo")
	}
	// The next word, typed on the now-Russian layout, is judged against corrected text.
	h.typeText("мир ", "ru")
	if len(h.fake.sends) != 1 || h.screen() != "привет мир " {
		t.Fatalf("correct text must not be touched: %+v %q", h.fake.sends, h.screen())
	}
}

func TestReadableFieldIsVerifiedAndEditedDirectly(t *testing.T) {
	h := newHarness(t)
	h.fake.readable = true
	h.fake.doc.set("мы говорили ")
	h.typeText("привет ", "ru")
	if h.screen() != "мы говорили привет " || len(h.fake.sends) != 0 || h.fake.doc.writes != 1 {
		t.Fatalf("screen %q sends %v writes %d", h.screen(), h.fake.sends, h.fake.doc.writes)
	}
}

func TestMismatchTouchesNothingAndDropsTheBuffer(t *testing.T) {
	h := newHarness(t)
	h.fake.readable = true
	// The application autocorrects the word behind our back as the space arrives.
	h.fake.doc.autocorrect = map[string]string{"ghbdtn ": "ghbdtm "}
	h.typeText("привет ", "ru")
	if h.screen() != "ghbdtm " || len(h.fake.sends) != 0 || h.fake.doc.writes != 0 {
		t.Fatalf("a mismatch must leave the screen alone: %q %v", h.screen(), h.fake.sends)
	}
	if h.engine.BufferLen() != 0 || h.engine.UndoCount() != 0 {
		t.Fatal("the buffer no longer describes the screen and must go")
	}
}

// A key the user presses while the replacement is being checked is queued behind our window
// messages; it would land between them. The edit is abandoned untouched, and the buffer,
// which still follows the screen, is kept for that key.
func TestFieldEditIsAbandonedWhenTypingContinues(t *testing.T) {
	h := newHarness(t)
	h.fake.readable = true
	strokes, _ := h.tables["abc"].Strokes("k")
	h.fake.doc.onRead = func() {
		// The first read is the context in front of the buffer; the key comes during the
		// replacement's own check.
		h.fake.doc.onRead = func() { h.fake.keySeq++ }
	}
	h.typeText("привет ", "ru")
	if h.screen() != "ghbdtn " || len(h.fake.sends) != 0 || h.fake.doc.writes != 0 {
		t.Fatalf("nothing may be touched: %q %v writes=%d", h.screen(), h.fake.sends, h.fake.doc.writes)
	}
	if h.engine.BufferLen() != 7 {
		t.Fatalf("the buffer still follows the screen and must be kept, has %d", h.engine.BufferLen())
	}
	// The queued key arrives: the application shows it, then the engine hears of it.
	h.fake.doc.typeText("k")
	h.clock += 0.1
	h.engine.HandleKey(KeyEvent{Class: KeyText, Key: strokes[0].Key, LayoutID: h.fake.current[1], Window: 1, Time: h.clock, Seq: h.fake.keySeq})
	if h.engine.ScreenText() != h.screen() {
		t.Fatalf("buffer %q, screen %q", h.engine.ScreenText(), h.screen())
	}
}

func TestPureInsertionIsNeverMade(t *testing.T) {
	h := newHarness(t)
	h.fake.readable = true
	h.fake.doc.set("abc ")
	outcome := h.engine.replace(core.Replacement{InsertText: "X"}, 0)
	if outcome.Result != core.ReplaceFailed || h.screen() != "abc " || h.fake.doc.writes != 0 {
		t.Fatalf("%+v %q", outcome, h.screen())
	}
}

// After an idle pause the buffer starts over, and the words in front of it are read again:
// they now include what was typed before the pause.
func TestContextIsReadAgainAfterAnIdlePause(t *testing.T) {
	h := newHarness(t)
	h.fake.readable = true
	h.fake.current[1] = h.tables["ru"].SourceID
	h.typeText("мы вчера долго говорили, ", "ru")
	h.clock += 10 // longer than the idle reset
	h.fake.current[1] = h.tables["abc"].SourceID
	h.typeText("где ", "ru") // `ult`: an English word too, offered only with Russian context
	if len(h.fake.shown) != 1 || h.fake.shown[0].After != "где" {
		t.Fatalf("expected an offer from the Russian text in front of the new buffer: %+v", h.fake.shown)
	}
}

func TestAppCapitalisationIsCarriedOver(t *testing.T) {
	h := newHarness(t)
	h.fake.readable = true
	h.fake.doc.capitalize = true
	h.typeText("привет ", "ru")
	if h.screen() != "Привет " {
		t.Fatalf("screen %q", h.screen())
	}
}

func TestPauseRightAfterAFixUndoesItAndLearns(t *testing.T) {
	h := newHarness(t)
	h.typeText("привет ", "ru")
	h.engine.Perform(config.ActionFixLastWord, h.fake.keySeq)
	if s := lastSend(t, h.fake); s.deleting != 7 || s.inserting != "ghbdtn " || h.screen() != "ghbdtn " {
		t.Fatalf("undo sent %+v, screen %q", s, h.screen())
	}
	if got := h.fake.current[1]; got != h.tables["abc"].SourceID {
		t.Fatalf("layout not restored: %s", got)
	}
	if !core.NewStringSet(h.store.Get().WordExceptions...).Contains("ghbdtn") {
		t.Fatalf("a prompt undo must teach an exception: %v", h.store.Get().WordExceptions)
	}
	// Typing it again is now left alone.
	h.typeText("привет ", "ru")
	if len(h.fake.sends) != 2 {
		t.Fatal("the learned exception must be honoured")
	}
}

func TestUndoIsRefusedOnceTheTextHasChanged(t *testing.T) {
	h := newHarness(t)
	h.typeText("привет ", "ru")
	// The user switches back to ABC by hand and keeps typing the next Russian word.
	h.fake.current[1] = h.tables["abc"].SourceID
	h.typeText("rfr", "abc")
	h.engine.UndoLastFix(0)
	if len(h.fake.sends) != 1 || len(h.fake.notified) != 1 {
		t.Fatalf("blind undo after further typing must be refused: %+v %v", h.fake.sends, h.fake.notified)
	}
	// Pause now means "fix the last word" instead: the word in progress, "rfr" → "как".
	h.engine.Perform(config.ActionFixLastWord, h.fake.keySeq)
	if s := lastSend(t, h.fake); s.deleting != 3 || s.inserting != "как" || h.screen() != "привет как" {
		t.Fatalf("manual fix sent %+v, screen %q", s, h.screen())
	}
	// Asked for explicitly: a lesson.
	if decision, _ := h.engine.Learner().Decision("rfr"); decision.Kind != core.DecisionConvert {
		t.Fatalf("a manual fix is remembered: %+v", decision)
	}
}

func TestFixIsSkippedWhenTypingRacedAhead(t *testing.T) {
	h := newHarness(t)
	h.typeText("ghbdtn", "abc")
	// The space arrives, but by the time the engine sees it the hook has already seen
	// another key press.
	strokes, _ := h.tables["abc"].Strokes(" ")
	h.clock += 0.1
	h.fake.keySeq += 2
	h.engine.HandleKey(KeyEvent{Class: KeyText, Key: strokes[0].Key, LayoutID: h.fake.current[1], Window: 1, Time: h.clock, Seq: h.fake.keySeq - 1})
	if len(h.fake.sends) != 0 {
		t.Fatal("typing after the user typed on would delete the wrong characters")
	}
	if h.engine.BufferLen() != 7 {
		t.Fatal("nothing was touched, so the buffer still follows the screen")
	}
}

func TestSuggestionIsAcceptedWithTheAcceptKey(t *testing.T) {
	h := newHarnessWith(t, suggestMode())
	h.typeText("привет ", "ru")
	if len(h.fake.shown) != 1 || !h.fake.armed || h.fake.acceptKey != config.AcceptTab {
		t.Fatalf("expected a suggestion armed with Tab: shown=%d armed=%v key=%s", len(h.fake.shown), h.fake.armed, h.fake.acceptKey)
	}
	s := h.fake.shown[0]
	if s.Before != "ghbdtn" || s.After != "привет" || s.AcceptLabel != "Tab" || s.Timeout != 4*time.Second {
		t.Fatalf("suggestion %+v", s)
	}
	h.fake.keySeq++
	h.engine.HandleAccept(h.fake.keySeq)
	if h.screen() != "привет " || h.fake.armed {
		t.Fatalf("accept left %q, armed=%v", h.screen(), h.fake.armed)
	}
	// Accepted: next time, without asking.
	if decision, _ := h.engine.Learner().Decision("ghbdtn"); decision.Kind != core.DecisionConvert || decision.Lang != core.LangRU {
		t.Fatalf("decision %+v", decision)
	}
}

// Typing on past an offer, moving on with Enter or the arrows, a Tab that did not accept it,
// Ctrl+Esc, letting it time out: none of that is a "no". People type on because they did not
// see the offer at least as often as because they meant the word, and counting it taught the
// app to stop fixing ordinary words for good. The offer goes, and comes back next time.
func TestPassingAnOfferByTeachesNothing(t *testing.T) {
	h := newHarnessWith(t, suggestMode())
	passes := map[string]func(){
		"typed on": func() { h.typeText("x", "abc") },
		"enter":    func() { h.key(KeyNavigation) },
		"tab":      func() { h.key(KeyTab) },
		"ctrl+esc": func() {
			h.fake.keySeq++
			h.engine.HandleKey(KeyEvent{Class: KeyEscape, Ctrl: true, LayoutID: h.fake.current[1], Window: 1, Seq: h.fake.keySeq})
		},
		"timeout": func() { h.engine.HandleSuggestionTimeout(h.fake.shown[len(h.fake.shown)-1].ID) },
	}
	for name, pass := range passes {
		for round := 0; round < 2; round++ {
			h.engine.HandleMouse() // a fresh start somewhere else
			h.fake.current[1] = h.tables["abc"].SourceID
			h.typeText("привет ", "ru")
			if h.engine.PendingOutcome() == nil {
				t.Fatalf("%s, round %d: expected an offer", name, round)
			}
			pass()
			if h.engine.PendingOutcome() != nil || h.fake.armed {
				t.Fatalf("%s: the offer must go", name)
			}
		}
	}
	if total := h.engine.Learner().Counts().Total(); total != 0 {
		t.Fatalf("passing offers by taught %d entries", total)
	}
}

// Escape is the one deliberate "no": twice for the same word, and it is not offered again.
func TestEscapeTwiceStopsAnOffer(t *testing.T) {
	h := newHarnessWith(t, suggestMode())
	for round := 0; round < 2; round++ {
		h.fake.current[1] = h.tables["abc"].SourceID
		h.typeText("привет ", "ru")
		if h.engine.PendingOutcome() == nil {
			t.Fatalf("round %d: expected an offer", round)
		}
		h.key(KeyEscape)
		if h.engine.PendingOutcome() != nil || h.fake.armed || h.engine.BufferLen() != 0 {
			t.Fatal("Escape takes the offer down and ends the line")
		}
	}
	if decision, _ := h.engine.Learner().Decision("ghbdtn"); decision.Kind != core.DecisionKeep {
		t.Fatalf("refused twice means keep: %+v", decision)
	}
	shown := len(h.fake.shown)
	h.typeText("привет ", "ru")
	if len(h.fake.shown) != shown {
		t.Fatal("a word the user refused is not offered again")
	}
}

func TestBackspaceDismissesTheOfferWithoutCountingIt(t *testing.T) {
	h := newHarnessWith(t, suggestMode())
	h.typeText("привет ", "ru")
	if h.engine.PendingOutcome() == nil {
		t.Fatal("expected an offer")
	}
	h.key(KeyBackspace)
	if h.engine.PendingOutcome() != nil || h.fake.armed {
		t.Fatal("Backspace changes the text under the offer; it must go")
	}
	if _, ok := h.engine.Learner().Decision("ghbdtn"); ok || h.engine.Learner().Counts().Total() != 0 {
		t.Fatal("editing the word is not ignoring the offer")
	}
	// The hook swallowed an accept key after all (a race): it must be given back.
	h.engine.HandleAccept(h.fake.keySeq)
	if len(h.fake.replayed) != 1 || len(h.fake.sends) != 0 {
		t.Fatal("a late accept key must be replayed, not applied")
	}
}

func TestSuggestionTimeout(t *testing.T) {
	h := newHarnessWith(t, suggestMode())
	h.typeText("привет ", "ru")
	if len(h.fake.shown) != 1 {
		t.Fatal("expected an offer")
	}
	id := h.fake.shown[0].ID
	h.engine.HandleSuggestionTimeout(id + 1) // a stale timer
	if h.engine.PendingOutcome() == nil {
		t.Fatal("a stale timeout must not dismiss the current suggestion")
	}
	h.engine.HandleSuggestionTimeout(id)
	if h.engine.PendingOutcome() != nil || h.fake.armed {
		t.Fatal("timeout must dismiss")
	}
	if total := h.engine.Learner().Counts().Total(); total != 0 {
		t.Fatalf("an offer that timed out may simply not have been seen, yet %d entries were learned", total)
	}
}

func TestTerminalsOnlySuggestAndUseInsert(t *testing.T) {
	h := newHarness(t)
	h.window = 2 // cmd.exe
	h.typeText("привет ", "ru")
	if len(h.fake.sends) != 0 || len(h.fake.shown) != 1 {
		t.Fatal("terminals are suggest-only by default")
	}
	if h.fake.acceptKey != config.AcceptInsert || h.fake.shown[0].AcceptLabel != "Ins" {
		t.Fatalf("Tab is load-bearing in terminals; accept key %s", h.fake.acceptKey)
	}
	h.fake.keySeq++
	h.engine.HandleAccept(h.fake.keySeq)
	if s := lastSend(t, h.fake); s.inserting != "привет " {
		t.Fatalf("accepted %+v", s)
	}
}

func TestAppRuleOffAndPasswordFieldsDoNothing(t *testing.T) {
	h := newHarness(t)
	h.engine.SetAppMode("notepad.exe", config.ModeOff, true)
	h.typeText("привет ", "ru")
	if len(h.fake.sends) != 0 || h.engine.BufferLen() != 0 {
		t.Fatal("an app switched off must not be watched")
	}
	h.engine.SetAppMode("notepad.exe", "", false)
	h.fake.password = true
	h.typeText("привет ", "ru")
	if len(h.fake.sends) != 0 || h.engine.BufferLen() != 0 {
		t.Fatal("password fields must not be buffered")
	}
}

func TestResetsFollowTheCaret(t *testing.T) {
	cases := map[string]func(h *harness){
		"mouse":      func(h *harness) { h.engine.HandleMouse() },
		"navigation": func(h *harness) { h.key(KeyNavigation) },
		"escape":     func(h *harness) { h.key(KeyEscape) },
		"tab":        func(h *harness) { h.key(KeyTab) },
		"other":      func(h *harness) { h.key(KeyOther) },
		"focus":      func(h *harness) { h.engine.HandleFocusChange() },
		"foreground": func(h *harness) { h.engine.Foreground(3) },
		"foreign": func(h *harness) {
			h.engine.HandleKey(KeyEvent{Class: KeyText, Foreign: true, Window: h.window})
		},
		"shortcut": func(h *harness) {
			strokes, _ := h.tables["abc"].Strokes("c")
			h.engine.HandleKey(KeyEvent{Class: KeyText, Key: strokes[0].Key, Ctrl: true, LayoutID: h.fake.current[1], Window: 1})
		},
	}
	for name, action := range cases {
		h := newHarness(t)
		h.typeText("ghbdtn", "abc")
		action(h)
		if h.engine.BufferLen() != 0 {
			t.Errorf("%s: buffer not reset", name)
		}
	}
}

func TestBackspaceAndModifiers(t *testing.T) {
	h := newHarness(t)
	h.typeText("ghbdtnx", "abc")
	h.key(KeyBackspace)
	h.key(KeyModifier)
	if h.engine.ScreenText() != "ghbdtn" {
		t.Fatalf("buffer %q", h.engine.ScreenText())
	}
	h.typeText(" ", "abc")
	if h.screen() != "привет " {
		t.Fatalf("screen %q", h.screen())
	}
}

func TestErasingAFixAndRetypingItIsARejection(t *testing.T) {
	h := newHarness(t)
	h.typeText("привет ", "ru")
	for i := 0; i < 7; i++ {
		h.key(KeyBackspace)
	}
	h.fake.current[1] = h.tables["abc"].SourceID
	h.typeText("привет ", "ru")
	if h.screen() != "ghbdtn " {
		t.Fatalf("the same fix must not be made twice: %q", h.screen())
	}
}

func TestCtrlAltIsAltGrOnlyWhenTheLayoutHasText(t *testing.T) {
	h := newHarness(t)
	// ABC's Option layer has "å" on the A key: recorded as a stroke.
	strokes, _ := h.tables["abc"].Strokes("a")
	key := strokes[0].Key
	h.engine.HandleKey(KeyEvent{Class: KeyText, Key: key, Ctrl: true, Alt: true, LayoutID: h.fake.current[1], Window: 1, Time: 1})
	if h.engine.BufferLen() != 1 || h.engine.typing.Buffer.Recorded()[0].Stroke.Mods&core.ModOption == 0 {
		t.Fatal("AltGr text must be recorded with the third-level modifier")
	}
	// Option+E is a dead key with no character of its own on ABC: a shortcut, not text.
	strokes, _ = h.tables["abc"].Strokes("e")
	h.engine.HandleKey(KeyEvent{Class: KeyText, Key: strokes[0].Key, Ctrl: true, Alt: true, LayoutID: h.fake.current[1], Window: 1, Time: 2})
	if h.engine.BufferLen() != 0 {
		t.Fatal("Ctrl+Alt on a key without a third level is a shortcut")
	}
}

func TestSelectionAndClipboardConversions(t *testing.T) {
	h := newHarness(t)
	// Punctuation is converted key for key too; on the macOS Russian layout "?" stays on
	// the same key.
	h.fake.selection = "Ghbdtn rfr ltkf?"
	h.engine.Perform(config.ActionFixSelection, 0)
	if len(h.fake.pasted) != 1 || h.fake.pasted[0] != "Привет как дела?" {
		t.Fatalf("pasted %q", h.fake.pasted)
	}
	if h.fake.current[1] != h.tables["ru"].SourceID || h.fake.restoreCalled != 1 {
		t.Fatal("selection conversion must switch layout and restore the clipboard")
	}

	h.fake.selection = ""
	h.fake.clipboard, h.fake.hasClipboard = "ytn", true
	h.fake.current[1] = h.tables["abc"].SourceID
	h.engine.Perform(config.ActionConvertClipboard, 0)
	if h.fake.clipboard != "нет" {
		t.Fatalf("clipboard %q", h.fake.clipboard)
	}
}

func TestForegroundEventForTheCurrentWindowKeepsTheBuffer(t *testing.T) {
	h := newHarness(t)
	h.typeText("ghb", "abc")
	// The posted foreground event arrives after the first keys already carried the window.
	h.engine.Foreground(1)
	h.typeText("dtn ", "abc")
	if s := lastSend(t, h.fake); s.deleting != 7 {
		t.Fatalf("the start of the word was lost: %+v", s)
	}
}

func TestTerminalsAreNeverSentCtrlC(t *testing.T) {
	h := newHarness(t)
	h.engine.Foreground(2) // cmd.exe
	h.window = 2
	h.fake.selection = "qalaisyz"
	h.fake.clipboard, h.fake.hasClipboard = "qalai", true
	h.engine.Perform(config.ActionTransliterate, 0)
	if len(h.fake.pasted) != 0 || h.fake.clipboard != "қалай" {
		t.Fatalf("a terminal must get the clipboard fallback, not Ctrl+C: pasted %q, clipboard %q", h.fake.pasted, h.fake.clipboard)
	}
	h.engine.Perform(config.ActionFixSelection, 0)
	if len(h.fake.pasted) != 0 {
		t.Fatal("fixing a selection in a terminal must do nothing")
	}
}

func TestEditorLineCopyIsNotASelection(t *testing.T) {
	h := newHarness(t)
	h.fake.apps[4] = "code.exe"
	h.fake.current[4] = h.tables["abc"].SourceID
	h.engine.Foreground(4)
	h.window = 4
	h.fake.selection = "ghbdtn\n" // VS Code copies the current line when nothing is selected
	h.engine.Perform(config.ActionFixSelection, 0)
	if len(h.fake.pasted) != 0 || h.fake.restoreCalled != 1 {
		t.Fatalf("a copied line must not be pasted back: %q", h.fake.pasted)
	}
}

func TestTransliterationOfSelectionAndClipboard(t *testing.T) {
	h := newHarness(t)
	h.fake.selection = "qalaisyz"
	h.engine.Perform(config.ActionTransliterate, 0)
	if len(h.fake.pasted) != 1 || h.fake.pasted[0] != "қалайсыз" {
		t.Fatalf("pasted %q", h.fake.pasted)
	}
	if table := h.fake.Table(h.fake.current[1]); table == nil || table.Lang.Script() != core.ScriptCyrillic {
		t.Fatal("after writing Cyrillic the layout must follow")
	}

	// Nothing selected: the clipboard is converted, and undo puts it back.
	h.fake.selection = ""
	h.fake.clipboard, h.fake.hasClipboard = "сәлеметсіз бе", true
	toLatin := translit.ToLatin
	h.engine.Transliterate(&toLatin, 0)
	if h.fake.clipboard != "salemetsiz be" {
		t.Fatalf("clipboard %q", h.fake.clipboard)
	}
	h.engine.UndoLastFix(0)
	if h.fake.clipboard != "сәлеметсіз бе" {
		t.Fatalf("undo must restore the clipboard, got %q", h.fake.clipboard)
	}
}

// Nothing selected and a line just typed: that line, where it stands, through the same
// verified replacement as a fix, with an undo that puts it back. The clipboard is not touched.
func TestTransliterationOfTheTextJustTyped(t *testing.T) {
	h := newHarness(t)
	h.typeText("qalaisyz aga", "abc")
	if h.screen() != "qalaisyz aga" {
		t.Fatalf("typing it changed it already: %q", h.screen())
	}
	h.fake.keySeq++ // the hotkey
	h.engine.Perform(config.ActionTransliterate, h.fake.keySeq)
	if h.screen() != "қалайсыз аға" {
		t.Fatalf("screen %q", h.screen())
	}
	if s := lastSend(t, h.fake); s.deleting != 12 || s.inserting != "қалайсыз аға" {
		t.Fatalf("sent %+v", s)
	}
	if len(h.fake.pasted) != 0 || h.fake.restoreCalled != 0 || h.fake.hasClipboard || h.fake.beeps != 0 {
		t.Fatal("neither the selection nor the clipboard is involved")
	}
	if table := h.fake.Table(h.fake.current[1]); table == nil || table.Lang.Script() != core.ScriptCyrillic {
		t.Fatal("after writing Cyrillic the layout must follow")
	}
	if h.engine.BufferLen() != 0 || h.engine.UndoCount() != 1 {
		t.Fatalf("buffer %d, undo %d", h.engine.BufferLen(), h.engine.UndoCount())
	}
	h.engine.UndoLastFix(0)
	if h.screen() != "qalaisyz aga" {
		t.Fatalf("undo left %q", h.screen())
	}
}

// Only the part that changes is retyped, and the direction comes from what was typed, never
// from the rest of the line.
func TestTransliterationOfTypedTextRetypesOnlyWhatChanges(t *testing.T) {
	h := newHarness(t)
	h.fake.doc.set("Алматы, ")
	h.typeText("2024 jyly", "abc")
	h.engine.Perform(config.ActionTransliterate, 0)
	if h.screen() != "Алматы, 2024 жылы" {
		t.Fatalf("screen %q", h.screen())
	}
	if s := lastSend(t, h.fake); s.deleting != 4 || s.inserting != "жылы" {
		t.Fatalf("sent %+v", s)
	}
}

// Where the field can be read, the words on screen are the ones converted: the application
// may have changed what was typed (here `qalaisyz` became `Qalaisyn`), and the letters on
// screen are the ones the user wants in the other alphabet (`Қалайсың`, the informal "how
// are you"). Made through the field itself; the line before is not touched.
func TestTransliterationOfTypedTextReadsTheScreen(t *testing.T) {
	h := newHarnessWith(t, suggestMode())
	h.fake.readable = true
	h.fake.doc.set("мәтін:\r\n")
	h.fake.doc.autocorrect = map[string]string{"мәтін:\r\nqalaisyz ": "мәтін:\r\nQalaisyn "}
	h.typeText("qalaisyz aga", "abc")
	if h.screen() != "мәтін:\r\nQalaisyn aga" || h.engine.ScreenText() != "qalaisyz aga" {
		t.Fatalf("screen %q, buffer %q", h.screen(), h.engine.ScreenText())
	}
	h.fake.keySeq++
	h.engine.Perform(config.ActionTransliterate, h.fake.keySeq)
	if h.screen() != "мәтін:\r\nҚалайсың аға" || len(h.fake.sends) != 0 || h.fake.doc.writes != 1 {
		t.Fatalf("screen %q sends %v writes %d", h.screen(), h.fake.sends, h.fake.doc.writes)
	}
}

// A selection on screen wins over the text just typed: it is what the user is looking at.
func TestTransliterationPrefersAVisibleSelection(t *testing.T) {
	h := newHarness(t)
	h.fake.readable = true
	h.typeText("qalai", "abc")
	// The application selects text without a keystroke (autocompletion, say).
	h.fake.doc.selection = core.Range16{Location: 0, Length: 5}
	h.fake.selection = "qalai"
	h.fake.keySeq++
	h.engine.Perform(config.ActionTransliterate, h.fake.keySeq)
	if len(h.fake.pasted) != 1 || h.fake.pasted[0] != "қалай" || len(h.fake.sends) != 0 || h.fake.doc.writes != 0 {
		t.Fatalf("pasted %q sends %v writes %d", h.fake.pasted, h.fake.sends, h.fake.doc.writes)
	}
}

// A key pressed after the hotkey but before it was handled: the typed text is left alone,
// and nothing else is tried in its place.
func TestTransliterationOfTypedTextStepsAsideWhenTypingContinues(t *testing.T) {
	h := newHarness(t)
	h.typeText("qalaisyz", "abc")
	h.fake.keySeq += 2 // the hotkey, then a key still on its way to the engine
	h.engine.Perform(config.ActionTransliterate, h.fake.keySeq-1)
	if len(h.fake.sends) != 0 || len(h.fake.pasted) != 0 || h.fake.restoreCalled != 0 || h.fake.beeps != 0 {
		t.Fatalf("sends %v pasted %q beeps %d", h.fake.sends, h.fake.pasted, h.fake.beeps)
	}
	if h.engine.BufferLen() != 8 {
		t.Fatal("nothing was touched, so the buffer still follows the screen")
	}
}

// Nothing to act on is said with a beep rather than silence.
func TestNothingToDoBeeps(t *testing.T) {
	h := newHarness(t)
	h.engine.Perform(config.ActionTransliterate, 0) // nothing typed, selected or copied
	if h.fake.beeps != 1 || h.engine.UndoCount() != 0 {
		t.Fatalf("empty clipboard: beeps %d", h.fake.beeps)
	}
	h.fake.selection = "2024"
	restored := h.fake.restoreCalled
	h.engine.Perform(config.ActionTransliterate, 0)
	if h.fake.beeps != 2 || len(h.fake.pasted) != 0 || h.fake.restoreCalled != restored+1 {
		t.Fatalf("a selection with nothing to convert: beeps %d pasted %q", h.fake.beeps, h.fake.pasted)
	}
	h.engine.Perform(config.ActionFixSelection, 0)
	if h.fake.beeps != 3 || len(h.fake.pasted) != 0 || h.fake.restoreCalled != restored+2 {
		t.Fatalf("a selection that needs no fix: beeps %d", h.fake.beeps)
	}
	h.fake.selection = ""
	h.engine.Perform(config.ActionFixSelection, 0)
	if h.fake.beeps != 4 {
		t.Fatalf("no selection to fix: beeps %d", h.fake.beeps)
	}
}

func TestCyrillicMemoryPicksKazakhForRussianLookingText(t *testing.T) {
	h := newHarness(t)
	// This app is written in Kazakh.
	h.fake.current[1] = h.tables["kk"].SourceID
	h.typeText("сәлем ", "kk")
	if got := h.store.Get().RememberedCyrillic("notepad.exe"); got != h.tables["kk"].SourceID {
		t.Fatalf("remembered %q", got)
	}
	// Now a Russian-looking word typed on ABC by mistake: the letters cannot tell Russian
	// from Kazakh, so the app's remembered Kazakh layout wins.
	h.fake.current[1] = h.tables["abc"].SourceID
	h.typeText("привет ", "ru")
	if got := h.fake.current[1]; got != h.tables["kk"].SourceID {
		t.Fatalf("expected the remembered Kazakh layout, got %s", got)
	}
}

func TestCyrillicPreferenceOverridesTheMemory(t *testing.T) {
	cfg := config.Default()
	cfg.CyrillicPreference = config.CyrillicPreferRussian
	h := newHarnessWith(t, cfg)
	h.fake.current[1] = h.tables["kk"].SourceID
	h.typeText("сәлем ", "kk")
	h.fake.current[1] = h.tables["abc"].SourceID
	h.typeText("привет ", "ru")
	if got := h.fake.current[1]; got != h.tables["ru"].SourceID {
		t.Fatalf("expected the preferred Russian layout, got %s", got)
	}
}

func TestDisabledEngineWatchesNothingButHotkeysStillToggle(t *testing.T) {
	h := newHarness(t)
	h.engine.Perform(config.ActionToggleEnabled, 0)
	h.typeText("привет ", "ru")
	if len(h.fake.sends) != 0 || h.engine.BufferLen() != 0 {
		t.Fatal("disabled means disabled")
	}
	h.engine.Perform(config.ActionToggleEnabled, 0)
	h.fake.doc.set("")
	h.fake.current[1] = h.tables["abc"].SourceID
	h.typeText("привет ", "ru")
	if len(h.fake.sends) != 1 {
		t.Fatal("toggling back on must work")
	}
}

func TestBlockedWindowIsNotTypedInto(t *testing.T) {
	h := newHarness(t)
	h.fake.blocked = true
	h.typeText("привет ", "ru")
	if len(h.fake.sends) != 0 {
		t.Fatal("elevated windows cannot receive input; nothing may be sent")
	}
}

func TestPresetsChangeWhatIsAppliedAndWhatIsOffered(t *testing.T) {
	cautious := config.Default()
	cautious.SetSensitivity(config.SensitivityCautious)
	h := newHarnessWith(t, cautious)
	// `rfr` → `как`: a frequent three-letter word is a confident (0.9) fix, below the
	// cautious automatic threshold (0.93): offered, not imposed.
	h.typeText("как ", "ru")
	if len(h.fake.sends) != 0 || len(h.fake.shown) != 1 {
		t.Fatalf("cautious: sends %v, offers %d", h.fake.sends, len(h.fake.shown))
	}
	balanced := newHarness(t)
	balanced.typeText("как ", "ru")
	if balanced.screen() != "как " {
		t.Fatalf("balanced: %q", balanced.screen())
	}
}

func TestStatusSnapshot(t *testing.T) {
	h := newHarness(t)
	h.typeText("привет ", "ru")
	status := h.engine.Status()
	if status.App != "notepad.exe" || status.Mode != config.ModeAuto || !status.CanUndo || status.Language != core.LangRU {
		t.Fatalf("status %+v", status)
	}
	h.typeText("x", "abc")
	if h.engine.Status().CanUndo {
		t.Fatal("undo is not offered once the text changed")
	}
}

func ExampleAcceptLabel() {
	fmt.Println(AcceptLabel(config.AcceptTab), AcceptLabel(config.AcceptInsert))
	// Output: Tab Ins
}
