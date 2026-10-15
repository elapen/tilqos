package engine

// The privacy promises, end to end through the engine against the fake platform of
// engine_test.go: the idle timer (P1), the two-minute undo list and its wipes (P2), clipboard
// limits and refusals (P3), the log (P4), password fields (P7). Time is driven by hand.

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"tilqos/internal/config"
	"tilqos/internal/core"
	"tilqos/internal/golden"
	"tilqos/internal/translit"
)

// fakeSched is time that only moves when a test says so.
type fakeSched struct {
	now     float64
	timers  []*fakeSchedTimer
	counter int
}

type fakeSchedTimer struct {
	s   *fakeSched
	id  int
	due float64
	f   func()
}

func (t *fakeSchedTimer) Stop() bool {
	for i, other := range t.s.timers {
		if other.id == t.id {
			t.s.timers = append(t.s.timers[:i], t.s.timers[i+1:]...)
			return true
		}
	}
	return false
}

func (s *fakeSched) Now() float64 { return s.now }
func (s *fakeSched) AfterFunc(d time.Duration, f func()) core.Timer {
	s.counter++
	timer := &fakeSchedTimer{s: s, id: s.counter, due: s.now + d.Seconds(), f: f}
	s.timers = append(s.timers, timer)
	return timer
}
func (s *fakeSched) Time() time.Time {
	return time.Unix(2_000_000, 0).Add(time.Duration(s.now * float64(time.Second)))
}
func (s *fakeSched) Advance(seconds float64) {
	target := s.now + seconds
	for {
		var next *fakeSchedTimer
		for _, timer := range s.timers {
			if timer.due <= target && (next == nil || timer.due < next.due) {
				next = timer
			}
		}
		if next == nil {
			break
		}
		next.Stop()
		s.now = max(s.now, next.due)
		next.f()
	}
	s.now = target
}

// privPlatform is the fake platform with the rules a real clipboard applies: flagged content
// is refused (nothing is read), and every read is counted.
type privPlatform struct {
	*fakePlatform
	flagged   bool
	reads     int
	copyCalls int
}

func (p *privPlatform) ReadClipboard() (string, bool) {
	if p.flagged {
		return "", false
	}
	p.reads++
	return p.fakePlatform.ReadClipboard()
}

func (p *privPlatform) CopySelection() (string, func(), bool) {
	if p.flagged {
		return "", nil, false
	}
	p.copyCalls++
	return p.fakePlatform.CopySelection()
}

type captureLog struct{ lines []string }

func (c *captureLog) Printf(format string, args ...any) {
	c.lines = append(c.lines, fmt.Sprintf(format, args...))
}
func (c *captureLog) text() string { return strings.Join(c.lines, "\n") }

type privHarness struct {
	*harness
	plat  *privPlatform
	sched *fakeSched
	log   *captureLog
}

func newPrivHarness(t *testing.T, cfg config.Config) *privHarness {
	t.Helper()
	fake, tables := newFake(t)
	plat := &privPlatform{fakePlatform: fake}
	store := config.NewMemoryStore(cfg)
	logger := &captureLog{}
	sched := &fakeSched{now: 50}
	engine := New(plat, store, golden.Models(t), nil, logger)
	engine.SetSleep(func(time.Duration) {})
	engine.SetClock(sched.Time)
	engine.SetScheduler(sched, nil)
	h := &harness{t: t, fake: fake, tables: tables, store: store, engine: engine, window: 1}
	fake.current[1] = tables["abc"].SourceID
	fake.current[2] = tables["abc"].SourceID
	engine.Foreground(1)
	return &privHarness{harness: h, plat: plat, sched: sched, log: logger}
}

// MARK: - P1

func TestIdleTimerEmptiesTheEngineBufferWithoutAnotherKey(t *testing.T) {
	h := newPrivHarness(t, config.Default())
	h.typeText("ghb", "abc")
	if h.engine.BufferLen() != 3 {
		t.Fatal("setup")
	}
	h.sched.Advance(4.9)
	if h.engine.BufferLen() != 3 {
		t.Fatal("still inside the five seconds")
	}
	h.sched.Advance(0.2)
	if h.engine.BufferLen() != 0 || h.engine.ScreenText() != "" {
		t.Fatal("five seconds without typing: the buffer is gone, without another key")
	}
}

func TestIdleTimerForgetsTheTextReadFromTheField(t *testing.T) {
	h := newPrivHarness(t, config.Default())
	h.fake.readable = true
	h.fake.doc.set("мы вчера долго говорили, ")
	h.typeText("где ", "ru") // `ult `: judged with the Russian text in front of it
	if h.engine.contextText == "" {
		t.Fatal("setup: the engine read the context from the field")
	}
	h.sched.Advance(5.2)
	if h.engine.contextText != "" || h.engine.contextRead {
		t.Fatal("the three words read from the field must not outlive the buffer")
	}
}

func TestEngineClampsTheConfiguredIdleResetToFiveSeconds(t *testing.T) {
	cfg := config.Default()
	cfg.BufferIdleReset = 600 // a hand-edited file; Normalize pulls it back
	h := newPrivHarness(t, cfg)
	h.typeText("gh", "abc")
	h.sched.Advance(5.2)
	if h.engine.BufferLen() != 0 {
		t.Fatal("the promise is five seconds, whatever the file says")
	}
}

func TestPasswordFieldKeysAreNeverBuffered(t *testing.T) {
	h := newPrivHarness(t, config.Default())
	h.typeText("gh", "abc")
	h.fake.password = true
	h.typeText("hunter2", "abc")
	if h.engine.BufferLen() != 0 {
		t.Fatalf("buffer holds %d strokes", h.engine.BufferLen())
	}
}

// MARK: - P2

func TestUndoIsWipedByTheTimerAtTwoMinutes(t *testing.T) {
	h := newPrivHarness(t, config.Default())
	h.typeText("привет ", "ru") // auto-fixed
	if !h.engine.Status().CanUndo {
		t.Fatal("setup: a fix to undo")
	}
	h.sched.Advance(119)
	if !h.engine.Status().CanUndo {
		t.Fatal("one second before the end it can still be undone")
	}
	h.sched.Advance(2)
	// No call to UndoCount or Perform in between: only the timer can have changed the status.
	if h.engine.Status().CanUndo {
		t.Fatal("the timer must have wiped the fix and told the menu")
	}
	if len(h.sched.timers) != 0 {
		t.Fatalf("%d timers left running", len(h.sched.timers))
	}
}

func TestUndoAfterExpiryDoesNothingAndDoesNotPanic(t *testing.T) {
	h := newPrivHarness(t, config.Default())
	h.typeText("привет ", "ru")
	sends := len(h.fake.sends)
	h.sched.Advance(121)
	h.engine.UndoLastFix(0)
	h.engine.Perform(config.ActionUndoLastFix, 0)
	h.engine.Perform(config.ActionFixLastWord, 0) // the Pause key: nothing to take back, nothing buffered
	if len(h.fake.sends) != sends || len(h.fake.notified) != 0 || h.engine.UndoCount() != 0 {
		t.Fatalf("something happened: %+v %v", h.fake.sends, h.fake.notified)
	}
}

func TestEachFixExpiresOnItsOwnClock(t *testing.T) {
	h := newPrivHarness(t, config.Default())
	h.typeText("привет ", "ru")
	h.sched.Advance(90)
	h.fake.current[1] = h.tables["abc"].SourceID
	h.typeText("мир ", "ru") // `vbh `: typed on ABC, fixed when the word ends
	if h.engine.UndoCount() != 2 {
		t.Fatalf("setup: %d fixes", h.engine.UndoCount())
	}
	h.sched.Advance(31)
	if h.engine.UndoCount() != 1 {
		t.Fatalf("the first fix is 121 s old and gone, the second remains: %d", h.engine.UndoCount())
	}
	h.sched.Advance(90)
	if h.engine.UndoCount() != 0 {
		t.Fatal("both gone")
	}
}

func TestWipeForgetsBufferOfferAndUndo(t *testing.T) {
	h := newPrivHarness(t, config.Default())
	h.typeText("привет ", "ru")
	h.typeText("gh", "abc")
	if h.engine.UndoCount() != 1 || h.engine.BufferLen() == 0 {
		t.Fatal("setup")
	}
	h.engine.Wipe("lock")
	if h.engine.BufferLen() != 0 || h.engine.UndoCount() != 0 || h.engine.Status().CanUndo {
		t.Fatal("lock or sleep must leave nothing behind")
	}
	if len(h.sched.timers) != 0 {
		t.Fatal("and no timer")
	}

	s := newPrivHarness(t, suggestMode())
	s.typeText("привет ", "ru")
	if s.engine.PendingOutcome() == nil {
		t.Fatal("setup: an offer")
	}
	hidden := s.fake.hidden
	s.engine.Wipe("sleep")
	if s.engine.PendingOutcome() != nil || s.fake.hidden != hidden+1 || s.fake.armed {
		t.Fatal("the offer on screen goes too, and the accept key is released")
	}
}

func TestAnotherWindowInFrontWipesTheUndoList(t *testing.T) {
	h := newPrivHarness(t, config.Default())
	h.typeText("привет ", "ru")
	h.engine.Foreground(1) // the same window again (the late foreground event): nothing changes
	if h.engine.UndoCount() != 1 {
		t.Fatal("the same window must keep its undo")
	}
	h.engine.Foreground(3)
	if h.engine.UndoCount() != 0 {
		t.Fatal("another window in front: the corrections made in the old one are wiped")
	}
}

// MARK: - P3

func TestClipboardTextOverTheLimitIsIgnoredAndNotKept(t *testing.T) {
	h := newPrivHarness(t, config.Default())
	long := strings.Repeat("q", core.MaxClipboardText+1)
	h.fake.clipboard, h.fake.hasClipboard = long, true
	h.engine.TransliterateClipboard(nil)
	h.engine.ConvertClipboard()
	if h.fake.clipboard != long {
		t.Fatal("left exactly as it was")
	}
	if h.engine.UndoCount() != 0 {
		t.Fatal("and no copy of it kept for undo")
	}
	if h.fake.beeps == 0 {
		t.Fatal("the user hears that nothing was done")
	}

	// Exactly at the limit it is converted.
	atLimit := strings.Repeat("qalaisyz ", 1111) + "q" // 10,000 characters
	if core.ExceedsClipboardLimit(atLimit) {
		t.Fatal("setup")
	}
	h.fake.clipboard = atLimit
	toCyr := translit.ToCyrillic
	h.engine.TransliterateClipboard(&toCyr)
	if h.fake.clipboard == atLimit || h.engine.UndoCount() != 1 {
		t.Fatal("10,000 characters are fine")
	}
}

func TestConvertClipboardHonoursTheLimitToo(t *testing.T) {
	h := newPrivHarness(t, config.Default())
	small := strings.Repeat("ghbdtn ", 1428) // 9,996 characters: converted
	h.fake.clipboard, h.fake.hasClipboard = small, true
	h.engine.ConvertClipboard()
	if h.fake.clipboard == small {
		t.Fatal("positive control: text under the limit is converted")
	}
	big := strings.Repeat("ghbdtn ", 1430) // 10,010 characters: ignored
	h.fake.clipboard = big
	h.engine.ConvertClipboard()
	if h.fake.clipboard != big {
		t.Fatal("text over the limit must be left alone")
	}
}

func TestSelectionOverTheLimitIsIgnoredAndTheClipboardRestored(t *testing.T) {
	h := newPrivHarness(t, config.Default())
	h.fake.selection = strings.Repeat("g", core.MaxClipboardText+1)
	h.engine.Perform(config.ActionFixSelection, 0)
	toCyr := translit.ToCyrillic
	h.engine.Transliterate(&toCyr, 0)
	if len(h.fake.pasted) != 0 {
		t.Fatalf("pasted %d characters", len(h.fake.pasted[0]))
	}
	if h.fake.restoreCalled != 2 {
		t.Fatalf("the clipboard must be put back each time, restored %d", h.fake.restoreCalled)
	}
	if h.engine.UndoCount() != 0 {
		t.Fatal("nothing kept")
	}
}

func TestFlaggedClipboardIsNeitherReadNorConvertedNorKept(t *testing.T) {
	h := newPrivHarness(t, config.Default())
	h.fake.clipboard, h.fake.hasClipboard = "qalaisyz", true
	h.fake.selection = "ghbdtn"
	h.plat.flagged = true
	toCyr := translit.ToCyrillic
	h.engine.ConvertClipboard()
	h.engine.TransliterateClipboard(&toCyr)
	h.engine.Perform(config.ActionFixSelection, 0)
	h.engine.Transliterate(&toCyr, 0)

	if h.fake.clipboard != "qalaisyz" || len(h.fake.pasted) != 0 {
		t.Fatalf("converted: clipboard %q, pasted %q", h.fake.clipboard, h.fake.pasted)
	}
	if h.plat.reads != 0 || h.plat.copyCalls != 0 {
		t.Fatalf("read %d times, copied %d times", h.plat.reads, h.plat.copyCalls)
	}
	if h.engine.UndoCount() != 0 {
		t.Fatal("a copy of it was kept for undo")
	}
}

// MARK: - P7

func TestCommandsDoNothingWhileAPasswordFieldIsInFront(t *testing.T) {
	h := newPrivHarness(t, config.Default())
	h.typeText("привет ", "ru") // an undoable fix, made earlier in a normal field
	h.fake.clipboard, h.fake.hasClipboard = "ytn", true
	h.fake.selection = "ghbdtn"
	h.fake.password = true
	pastedBefore, sendsBefore := len(h.fake.pasted), len(h.fake.sends)
	toCyr := translit.ToCyrillic

	h.engine.Perform(config.ActionConvertClipboard, 0)
	h.engine.Perform(config.ActionFixSelection, 0)
	h.engine.Perform(config.ActionFixLastWord, 0)
	h.engine.Perform(config.ActionUndoLastFix, 0)
	h.engine.Transliterate(&toCyr, 0)
	h.engine.TransliterateClipboard(&toCyr)

	if h.plat.reads != 0 || h.plat.copyCalls != 0 {
		t.Fatalf("the clipboard was read %d times and the selection copied %d times", h.plat.reads, h.plat.copyCalls)
	}
	if len(h.fake.pasted) != pastedBefore || len(h.fake.sends) != sendsBefore || h.fake.clipboard != "ytn" {
		t.Fatal("something was changed")
	}
}

func TestNothingIsLoggedWhileAPasswordFieldIsInFront(t *testing.T) {
	cfg := config.Default()
	cfg.Diagnostics = true
	h := newPrivHarness(t, cfg)
	h.fake.password = true
	before := len(h.log.lines)
	h.typeText("hunter2 ", "abc")
	h.engine.HandleKey(KeyEvent{Class: KeyBackspace, Window: 1, Time: h.clock})
	h.engine.Perform(config.ActionConvertClipboard, 0)
	if got := h.log.lines[before:]; len(got) != 0 {
		t.Fatalf("logged while a password field was in front: %v", got)
	}
}

// MARK: - P4

func TestTheLogNeverContainsTypedCopiedOrNamedThings(t *testing.T) {
	cfg := config.Default()
	cfg.Diagnostics = true
	h := newPrivHarness(t, cfg)
	h.fake.readable = true

	// An automatic fix, taken back at once (which teaches an exception).
	h.typeText("привет ", "ru")
	h.engine.Perform(config.ActionFixLastWord, h.fake.keySeq)
	// A word fixed by hand, a word left alone, a mismatch between buffer and screen.
	h.typeText("rfr", "abc")
	h.engine.Perform(config.ActionFixLastWord, h.fake.keySeq)
	h.typeText(" qalaisyz ", "abc")
	h.fake.doc.set("something else entirely ")
	h.typeText("ghbdtn ", "abc")
	// An offer, accepted.
	h.store.Update(func(c *config.Config) { c.DefaultFixMode = config.ModeSuggest })
	h.engine.RefreshConfig()
	h.fake.current[1] = h.tables["abc"].SourceID
	h.typeText("как ", "ru")
	if h.engine.PendingOutcome() != nil {
		h.fake.keySeq++
		h.engine.HandleAccept(h.fake.keySeq)
	}
	// Selection and clipboard routes, transliteration of typed text, a command with nothing
	// to do, a password field, a window switch, the master switch.
	h.fake.selection = "Ghbdtn rfr ltkf?"
	h.engine.Perform(config.ActionFixSelection, 0)
	h.fake.selection = ""
	h.fake.clipboard, h.fake.hasClipboard = "secret-clipboard-text qalaisyz", true
	h.engine.Perform(config.ActionTransliterate, 0)
	h.engine.Perform(config.ActionUndoLastFix, 0)
	h.typeText("qalaisyz aga", "abc")
	h.engine.Perform(config.ActionTransliterate, h.fake.keySeq)
	h.engine.Perform(config.ActionConvertClipboard, 0)
	h.fake.password = true
	h.typeText("hunter2", "abc")
	h.fake.password = false
	h.engine.Foreground(2)
	h.engine.Foreground(3)
	h.store.Update(func(c *config.Config) { c.Enabled = false })
	h.engine.RefreshConfig()
	h.store.Update(func(c *config.Config) { c.Enabled = true })
	h.engine.RefreshConfig()

	log := h.log.text()
	if len(h.log.lines) < 10 {
		t.Fatalf("the scenario logged only %d lines; it is not exercising the log:\n%s", len(h.log.lines), log)
	}
	for _, line := range []string{"fixed ", "detect:", "learned", "buffer cleared"} {
		if !strings.Contains(log, line) {
			t.Errorf("the scenario never logged %q", line)
		}
	}
	lowered := strings.ToLower(log)
	forbidden := []string{
		"ghbdtn", "привет", "rfr", "как", "qalaisyz", "қалайсыз", "ltkf", "hunter2",
		"secret-clipboard-text", "something else entirely",
		"notepad", "cmd.exe", "winword", ".exe",
	}
	for _, word := range forbidden {
		if strings.Contains(lowered, word) {
			t.Errorf("the log contains %q:\n%s", word, log)
		}
	}
	// Not a single Cyrillic or Kazakh letter in any line either.
	for _, r := range log {
		if r >= 0x0400 && r <= 0x04FF {
			t.Fatalf("a Cyrillic letter in the log:\n%s", log)
		}
	}
}
