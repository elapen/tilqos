package core_test

// The privacy promises that live in the core package, with time driven by hand (nothing here
// sleeps): the idle timer behind the keystroke buffer (P1), the two-minute undo list (P2), the
// clipboard rules (P3) and the learned-vocabulary wipe (P5). The Swift counterparts are
// Tests/TilqosKitTests/Privacy*Tests.swift.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"tilqos/internal/core"
	"tilqos/internal/golden"
)

// fakeScheduler is time that only moves when a test says so.
type fakeScheduler struct {
	now     float64
	entries []*fakeTimer
	nextID  int
}

type fakeTimer struct {
	sched *fakeScheduler
	id    int
	due   float64
	f     func()
}

func (t *fakeTimer) Stop() bool {
	for i, entry := range t.sched.entries {
		if entry.id == t.id {
			t.sched.entries = append(t.sched.entries[:i], t.sched.entries[i+1:]...)
			return true
		}
	}
	return false
}

func (s *fakeScheduler) Now() float64 { return s.now }

func (s *fakeScheduler) AfterFunc(d time.Duration, f func()) core.Timer {
	s.nextID++
	timer := &fakeTimer{sched: s, id: s.nextID, due: s.now + d.Seconds(), f: f}
	s.entries = append(s.entries, timer)
	return timer
}

// Advance moves time forward, running every timer that falls due on the way, in order.
func (s *fakeScheduler) Advance(seconds float64) {
	target := s.now + seconds
	for {
		var next *fakeTimer
		for _, entry := range s.entries {
			if entry.due <= target && (next == nil || entry.due < next.due) {
				next = entry
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

func (s *fakeScheduler) Pending() int { return len(s.entries) }

func (s *fakeScheduler) Time() time.Time {
	return time.Unix(1_000_000, 0).Add(time.Duration(s.now * float64(time.Second)))
}

func newIdleEngine(t *testing.T, idle float64) (*core.TypingEngine, *fakeScheduler, *core.KeyMapTable) {
	t.Helper()
	tables, _, _ := golden.Tables(t)
	sched := &fakeScheduler{now: 100}
	engine := core.NewTypingEngine(nil, core.NewStrokeBuffer(0, idle))
	engine.Scheduler = sched
	return engine, sched, tables["abc"]
}

func press(engine *core.TypingEngine, sched *fakeScheduler, layout *core.KeyMapTable, text string) {
	strokes, _ := layout.Strokes(text)
	for _, stroke := range strokes {
		engine.Record(stroke, layout, sched.now, []*core.KeyMapTable{layout})
	}
}

// MARK: - P1: the idle timer

func TestIdleTimerEmptiesTheBufferWithoutAnotherKey(t *testing.T) {
	engine, sched, abc := newIdleEngine(t, 5)
	notified := 0
	engine.OnIdleReset = func() { notified++ }
	press(engine, sched, abc, "ghbd")
	if engine.Buffer.Count() != 4 {
		t.Fatal("setup")
	}
	sched.Advance(4.9)
	if engine.Buffer.Count() != 4 || notified != 0 {
		t.Fatal("still inside the five seconds")
	}
	sched.Advance(0.2)
	if !engine.Buffer.IsEmpty() {
		t.Fatal("five seconds without typing: the buffer must be gone, without another key")
	}
	if got := engine.ScreenText(func(string) *core.KeyMapTable { return abc }); got != "" {
		t.Fatalf("nothing of the word may be left to read back, got %q", got)
	}
	if notified != 1 || sched.Pending() != 0 {
		t.Fatalf("notified %d, timers left %d", notified, sched.Pending())
	}
}

func TestIdleTimerMeasuresFromTheLastKeyNotTheFirst(t *testing.T) {
	engine, sched, abc := newIdleEngine(t, 5)
	press(engine, sched, abc, "a")
	sched.Advance(3)
	press(engine, sched, abc, "b")
	sched.Advance(4.9) // 4.9 s after the last key
	if engine.Buffer.Count() != 2 {
		t.Fatal("the timer must wait for the last key")
	}
	sched.Advance(0.2)
	if !engine.Buffer.IsEmpty() {
		t.Fatal("five seconds after the last key")
	}
}

func TestOneTimerCoversAWholeBurstOfTyping(t *testing.T) {
	engine, sched, abc := newIdleEngine(t, 5)
	for i := 0; i < 50; i++ {
		press(engine, sched, abc, "a")
		sched.Advance(0.1)
	}
	if sched.Pending() != 1 {
		t.Fatalf("the keystroke path must schedule nothing per key, found %d timers", sched.Pending())
	}
}

func TestBackspaceCountsAsTyping(t *testing.T) {
	engine, sched, abc := newIdleEngine(t, 5)
	press(engine, sched, abc, "abc")
	sched.Advance(4)
	engine.Backspace()
	sched.Advance(4)
	if engine.Buffer.Count() != 2 {
		t.Fatal("the Backspace restarted the five seconds")
	}
	sched.Advance(1.1)
	if !engine.Buffer.IsEmpty() {
		t.Fatal("and five seconds after it the buffer is gone")
	}
}

func TestResetCancelsTheTimerAndALateCallbackIsIgnored(t *testing.T) {
	engine, sched, abc := newIdleEngine(t, 5)
	// A dispatcher that holds the work back, as the Windows app's event channel does.
	var queue []func()
	engine.Dispatch = func(f func()) { queue = append(queue, f) }
	notified := 0
	engine.OnIdleReset = func() { notified++ }

	press(engine, sched, abc, "ab")
	sched.Advance(5.1) // the timer fired; its work waits in the queue
	if len(queue) != 1 {
		t.Fatalf("queue %d", len(queue))
	}
	if engine.Buffer.Count() != 2 {
		t.Fatal("the timer's goroutine must not touch the buffer: the work belongs to the engine's goroutine")
	}
	// Before the queue is served the user types again: that key is processed first.
	press(engine, sched, abc, "c")
	for _, f := range queue {
		f()
	}
	// The key came in after the timer fired: there was no idleness to report (the buffer
	// itself started over, the old text being more than five seconds old), and the timer
	// goes on counting from "c".
	if notified != 0 || engine.Buffer.Count() != 1 || sched.Pending() != 1 {
		t.Fatalf("notified %d, count %d, timers %d", notified, engine.Buffer.Count(), sched.Pending())
	}
	queue = nil
	sched.Advance(5.1)
	for _, f := range queue {
		f()
	}
	if !engine.Buffer.IsEmpty() || notified != 1 {
		t.Fatalf("five seconds after the last key: buffer %d, notified %d", engine.Buffer.Count(), notified)
	}

	// A reset cancels whatever is on its way.
	engine2, sched2, abc2 := newIdleEngine(t, 5)
	var queue2 []func()
	engine2.Dispatch = func(f func()) { queue2 = append(queue2, f) }
	n := 0
	engine2.OnIdleReset = func() { n++ }
	press(engine2, sched2, abc2, "ab")
	sched2.Advance(5.1)
	engine2.Reset() // before the queued work is served
	for _, f := range queue2 {
		f()
	}
	if n != 0 {
		t.Fatal("a callback outdated by a reset must do nothing")
	}
	if sched2.Pending() != 0 {
		t.Fatal("no timer may survive a reset")
	}
}

func TestACallbackFromBeforeAResetCannotResetTheNewText(t *testing.T) {
	engine, sched, abc := newIdleEngine(t, 5)
	var queue []func()
	engine.Dispatch = func(f func()) { queue = append(queue, f) }
	notified := 0
	engine.OnIdleReset = func() { notified++ }

	press(engine, sched, abc, "ab")
	sched.Advance(5.1) // fired; the work waits in the queue
	engine.Reset()
	press(engine, sched, abc, "c") // new text, new timer
	for _, f := range queue {
		f() // the old callback arrives late
	}
	if notified != 0 || engine.Buffer.Count() != 1 {
		t.Fatalf("the late callback reset the new text: notified %d, count %d", notified, engine.Buffer.Count())
	}
}

func TestIdleResetCannotBeStretchedBeyondFiveSeconds(t *testing.T) {
	if got := core.NewStrokeBuffer(0, 600).IdleReset; got != 5 {
		t.Fatalf("600 -> %v", got)
	}
	if got := core.NewStrokeBuffer(0, 0).IdleReset; got != 5 {
		t.Fatalf("unset -> %v", got)
	}
	if got := core.NewStrokeBuffer(0, 0.2).IdleReset; got != 1 {
		t.Fatalf("0.2 -> %v", got)
	}
	buffer := core.NewStrokeBuffer(0, 3)
	buffer.SetIdleReset(9999)
	if buffer.IdleReset != 5 {
		t.Fatalf("SetIdleReset(9999) -> %v", buffer.IdleReset)
	}
	engine, sched, abc := newIdleEngine(t, 600)
	press(engine, sched, abc, "a")
	sched.Advance(5.1)
	if !engine.Buffer.IsEmpty() {
		t.Fatal("a stretched setting must not stretch the promise")
	}
}

// The real clock and time.AfterFunc, end to end: the timer fires on a goroutine of its own and the
// work reaches the "engine" goroutine (this one) through the dispatcher. Run with -race.
func TestRealSchedulerResetsTheBufferOnItsOwn(t *testing.T) {
	tables, _, _ := golden.Tables(t)
	abc := tables["abc"]
	buffer := core.NewStrokeBuffer(0, 5)
	buffer.IdleReset = 0.05 // below the clamp on purpose: a test must not wait five seconds
	engine := core.NewTypingEngine(nil, buffer)
	engine.Scheduler = core.RealScheduler{}
	work := make(chan func(), 4)
	engine.Dispatch = func(f func()) { work <- f }
	notified := make(chan struct{}, 1)
	engine.OnIdleReset = func() { notified <- struct{}{} }

	strokes, _ := abc.Strokes("ghb")
	start := core.RealScheduler{}.Now()
	for _, stroke := range strokes {
		engine.Record(stroke, abc, start, []*core.KeyMapTable{abc})
	}
	select {
	case f := <-work:
		f() // on this goroutine, as the Windows app does on the engine's
	case <-time.After(3 * time.Second):
		t.Fatal("the real timer never fired")
	}
	select {
	case <-notified:
	default:
		t.Fatal("the idle reset did not run")
	}
	if !engine.Buffer.IsEmpty() {
		t.Fatal("buffer not empty")
	}
}

func TestBufferStaysBounded(t *testing.T) {
	engine, sched, abc := newIdleEngine(t, 5)
	press(engine, sched, abc, strings.Repeat("d", 300))
	if engine.Buffer.Count() != 256 {
		t.Fatalf("count %d", engine.Buffer.Count())
	}
}

// MARK: - P2: the undo list

func fix(original string, at time.Time) core.AppliedFix {
	return core.AppliedFix{
		Kind: core.FixLayout, Time: at, OriginalWord: original, CorrectedWord: "x",
		Replacement: core.Replacement{DeletedText: original, InsertText: "x"}, TypedWords: []string{original},
	}
}

func newUndo(sched *fakeScheduler) *core.UndoStack {
	stack := core.NewUndoStack(0)
	stack.Now = sched.Time
	return stack
}

func TestUndoLifetimeIsTwoMinutes(t *testing.T) {
	if core.DefaultUndoLifetime != 2*time.Minute || core.NewUndoStack(0).Lifetime != 2*time.Minute {
		t.Fatal("two minutes")
	}
}

func TestUndoEntryExpiresAtTwoMinutes(t *testing.T) {
	sched := &fakeScheduler{}
	stack := newUndo(sched)
	stack.Push(fix("ghbdtn", sched.Time()))
	sched.Advance(119)
	if stack.Count() != 1 {
		t.Fatal("one second before the end it can still be undone")
	}
	sched.Advance(1) // exactly two minutes: gone
	if stack.Count() != 0 {
		t.Fatal("two minutes is a ceiling")
	}
	if _, ok := stack.Last(); ok {
		t.Fatal("Last must not return an expired fix")
	}
	if _, ok := stack.PopLast(); ok {
		t.Fatal("PopLast must not return an expired fix")
	}
}

func TestUndoEntriesExpireIndependentlyAndNextExpiryAimsAtTheOldest(t *testing.T) {
	sched := &fakeScheduler{}
	stack := newUndo(sched)
	stack.Push(fix("first", sched.Time()))
	sched.Advance(90)
	stack.Push(fix("second", sched.Time()))
	when, ok := stack.NextExpiry()
	if !ok || !when.Equal(sched.Time().Add(30*time.Second)) {
		t.Fatalf("next expiry %v %v", when, ok)
	}
	sched.Advance(31)
	if stack.Count() != 1 {
		t.Fatal("first is gone, second remains")
	}
	if last, _ := stack.Last(); last.OriginalWord != "second" {
		t.Fatal("wrong survivor")
	}
	sched.Advance(90)
	if _, ok := stack.NextExpiry(); ok || stack.Count() != 0 {
		t.Fatal("all gone")
	}
}

func TestDropExpiredRemovesEverythingDue(t *testing.T) {
	sched := &fakeScheduler{}
	stack := newUndo(sched)
	for _, word := range []string{"aaaa", "bbbb", "cccc"} {
		stack.Push(fix(word, sched.Time()))
	}
	sched.Advance(121) // no accessor has been called since the pushes
	if n := stack.DropExpired(); n != 3 {
		t.Fatalf("dropped %d", n)
	}
	if stack.Count() != 0 {
		t.Fatal("not empty")
	}
}

func TestUndoClearWipesEverything(t *testing.T) {
	sched := &fakeScheduler{}
	stack := newUndo(sched)
	stack.Push(fix("a", sched.Time()))
	stack.Push(fix("b", sched.Time()))
	stack.Clear()
	if !stack.IsEmpty() {
		t.Fatal("not empty")
	}
	if _, ok := stack.PopLast(); ok {
		t.Fatal("popped from an empty stack")
	}
}

func TestUndoKeepsOnlyTheMostRecentFixes(t *testing.T) {
	sched := &fakeScheduler{}
	stack := newUndo(sched)
	for i := 0; i < 30; i++ {
		stack.Push(fix(strings.Repeat("w", i+1), sched.Time()))
	}
	if stack.Count() != 20 {
		t.Fatalf("count %d", stack.Count())
	}
	if last, _ := stack.Last(); len(last.OriginalWord) != 30 {
		t.Fatal("newest must be kept")
	}
}

func TestClipboardBeforeExpiresWithTheRest(t *testing.T) {
	sched := &fakeScheduler{}
	stack := newUndo(sched)
	before := "qalaisyz"
	f := fix(before, sched.Time())
	f.ClipboardBefore = &before
	stack.Push(f)
	sched.Advance(121)
	if _, ok := stack.PopLast(); ok {
		t.Fatal("the previous clipboard text must not outlive two minutes either")
	}
}

// MARK: - P3: the clipboard rules

func flag(name string, value ...byte) core.ClipboardFlag {
	return core.ClipboardFlag{Name: name, Data: value}
}

func TestClipboardFlagFormats(t *testing.T) {
	cases := []struct {
		name  string
		flags []core.ClipboardFlag
		want  bool
	}{
		{"nothing", nil, false},
		{"exclude, any data", []core.ClipboardFlag{flag(core.FormatExcludeFromMonitor, 1)}, true},
		{"exclude, empty data", []core.ClipboardFlag{flag(core.FormatExcludeFromMonitor)}, true},
		{"history 0", []core.ClipboardFlag{flag(core.FormatCanIncludeInHistory, 0, 0, 0, 0)}, true},
		{"history 1", []core.ClipboardFlag{flag(core.FormatCanIncludeInHistory, 1, 0, 0, 0)}, false},
		{"cloud 0", []core.ClipboardFlag{flag(core.FormatCanUploadToCloud, 0, 0, 0, 0)}, true},
		{"cloud 1", []core.ClipboardFlag{flag(core.FormatCanUploadToCloud, 1, 0, 0, 0)}, false},
		{"history unreadable counts as 0", []core.ClipboardFlag{flag(core.FormatCanIncludeInHistory)}, true},
		{"one allowing flag does not cancel another", []core.ClipboardFlag{
			flag(core.FormatCanIncludeInHistory, 1, 0, 0, 0), flag(core.FormatCanUploadToCloud, 0, 0, 0, 0)}, true},
		{"names are case-insensitive", []core.ClipboardFlag{flag("excludeclipboardcontentfrommonitorprocessing", 1)}, true},
		{"unrelated format", []core.ClipboardFlag{flag("HTML Format", 0, 0, 0, 0)}, false},
	}
	for _, c := range cases {
		if got := core.ClipboardSensitive(c.flags); got != c.want {
			t.Errorf("%s: got %v", c.name, got)
		}
	}
	if len(core.ClipboardFlagFormats) != 3 {
		t.Fatal("the three formats of the specification")
	}
}

func TestClipboardTextLimitIsTenThousandCharacters(t *testing.T) {
	if core.MaxClipboardText != 10_000 {
		t.Fatal("limit")
	}
	if core.ExceedsClipboardLimit(strings.Repeat("a", 10_000)) {
		t.Fatal("exactly 10,000 is fine")
	}
	if !core.ExceedsClipboardLimit(strings.Repeat("a", 10_001)) {
		t.Fatal("10,001 is too long")
	}
	if core.ExceedsClipboardLimit(strings.Repeat("я", 10_000)) {
		t.Fatal("characters, not bytes: 10,000 two-byte letters are fine")
	}
	if !core.ExceedsClipboardLimit(strings.Repeat("😀", 10_001)) {
		t.Fatal("10,001 emoji are too long")
	}
}

// MARK: - P5: wiping the learned vocabulary

func TestResetLearnedVocabularyWipesEntriesAndRotatesTheSalt(t *testing.T) {
	learner := core.NewLearner(nil)
	changed := 0
	learner.OnChange(func() { changed++ })
	learner.Kept("Aidar", core.LangEN)
	learner.Kept("Aidar", core.LangEN)
	learner.Confirmed("ghbdtn", core.LangRU)
	learner.Rejected("instagram")
	if learner.Counts().Total() != 3 {
		t.Fatalf("setup: %+v", learner.Counts())
	}
	oldHash := learner.Hash("ghbdtn")
	before := changed

	learner.ResetLearnedVocabulary()

	if learner.Counts().Total() != 0 {
		t.Fatal("entries remain")
	}
	if _, ok := learner.Decision("ghbdtn"); ok || learner.IsKnown("Aidar", core.LangEN) {
		t.Fatal("something is still known")
	}
	if learner.Hash("ghbdtn") == oldHash {
		t.Fatal("the salt must be replaced")
	}
	if changed != before+1 {
		t.Fatal("the change callback must fire so the store writes the empty state")
	}
	data, err := learner.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "ghbdtn") || strings.Contains(string(data), oldHashHex(oldHash)) {
		t.Fatal("old hash still in the encoded store")
	}
	// the old name still works
	learner.Confirmed("abc", core.LangEN)
	learner.ForgetAll()
	if learner.Counts().Total() != 0 {
		t.Fatal("ForgetAll")
	}
}

func oldHashHex(hash uint64) string {
	const digits = "0123456789abcdef"
	out := make([]byte, 16)
	for i := 15; i >= 0; i-- {
		out[i] = digits[hash&0xF]
		hash >>= 4
	}
	return string(out)
}

func TestLearnedStoreIsHashedNotTextAndTheHashIsGuessableWithTheSalt(t *testing.T) {
	learner := core.NewLearner(nil)
	learner.Confirmed("hello", core.LangEN)
	data, err := learner.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "hello") {
		t.Fatal("a word is stored as text")
	}
	// The salt is in the same file, FNV-1a is not cryptographic: a guess with the salt
	// confirms the word. The documentation says exactly that, so the test pins the facts.
	if learner.Hash("hello") == learner.Hash("world") {
		t.Fatal("distinct words, distinct hashes")
	}
	if !strings.Contains(string(data), oldHashHex(learner.Hash("hello"))) {
		t.Fatal("the stored entry is the salted hash anyone with the file can recompute")
	}
}

func TestNoSourceCallsTheHashesOneWay(t *testing.T) {
	words := regexp.MustCompile(`(?i)one-way|irreversib|cannot be (reversed|recovered|reconstructed)|необратим`)
	root := filepath.Join(golden.ModuleRoot(t), "internal")
	var offenders []string
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, _ := os.ReadFile(path)
		for i, line := range strings.Split(string(data), "\n") {
			if words.MatchString(line) {
				offenders = append(offenders, fmt.Sprintf("%s:%d: %s", filepath.Base(path), i+1, strings.TrimSpace(line)))
			}
		}
		return nil
	})
	if len(offenders) > 0 {
		t.Fatalf("FNV-1a is not a one-way function:\n%s", strings.Join(offenders, "\n"))
	}
}
