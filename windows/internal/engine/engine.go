// Package engine is the port of Pipeline.swift: it connects keystrokes to the typing engine
// to the text replacer, and owns the side effects — the suggestion, the replacement, the
// layout switch, the undo stack — and the learning signals only it can see: which offers
// were taken and which went by.
//
// The decisions themselves live in core.TypingEngine, which the corpus tests drive directly.
// Everything Windows-specific — hooks, SendInput, the Edit-control field, the clipboard, the
// popup — sits behind Platform, so this package is unit-tested on any OS. The engine is
// single-threaded: every method must be called from one goroutine (the Windows app feeds it
// from a channel).
package engine

import (
	"strings"
	"sync/atomic"
	"time"
	"unicode"

	"tilqos/internal/config"
	"tilqos/internal/core"
	"tilqos/internal/i18n"
	"tilqos/internal/translit"
)

// KeyClass is the platform's classification of a key.
type KeyClass int

const (
	// KeyText may produce text; the engine looks it up in the layout table.
	KeyText KeyClass = iota
	KeyBackspace
	// KeyNavigation moves the caret or commits the line (Enter, arrows, Home/End,
	// PageUp/PageDown, Delete, Insert). Each invalidates the buffer.
	KeyNavigation
	// KeyEscape is Escape: like the navigation keys it ends the line of thought, and it is
	// also the user's explicit "no" to a suggestion on screen.
	KeyEscape
	// KeyTab is Tab when it did not accept a suggestion.
	KeyTab
	// KeyModifier is Shift, Ctrl, Alt, Win, Caps/Num/Scroll Lock: ignored.
	KeyModifier
	// KeyOther is any other key that types nothing (F-keys, media keys, Apps…).
	KeyOther
)

// KeyEvent is one key press as the hook saw it.
type KeyEvent struct {
	Class KeyClass
	// Key is the physical key code the layout tables are indexed by.
	Key            uint16
	Shift, Caps    bool
	Ctrl, Alt, Win bool
	// LayoutID is the layout active in the foreground thread when the key was pressed.
	LayoutID string
	Window   uintptr
	// Time is in seconds on a monotonic clock.
	Time float64
	// Seq is the hook's sequence number for this key press.
	Seq uint64
	// Foreign marks input injected by another program (an on-screen keyboard, a password
	// manager's auto-type): the screen changed in a way we cannot follow.
	Foreign bool
}

// Suggestion is what the popup shows.
type Suggestion struct {
	ID          uint64
	Before      string
	After       string
	AcceptLabel string
	Timeout     time.Duration
	Window      uintptr
}

// Platform is everything the engine needs from the operating system.
type Platform interface {
	// Tables lists the installed layouts in system order; Table resolves an ID.
	Tables() []*core.KeyMapTable
	Table(id string) *core.KeyMapTable
	// CurrentLayoutID is the layout of the given window's input thread, read live.
	CurrentLayoutID(window uintptr) string
	SelectLayout(window uintptr, id string) bool

	// AppName is the lowercased executable name behind a window.
	AppName(window uintptr) string
	IsPasswordField(window uintptr) bool
	// CanInject is false when synthetic input cannot reach the window (an elevated
	// process, a secure desktop).
	CanInject(window uintptr) bool

	// FocusedField is the window's focused text control when its text can be read and
	// edited directly (a standard Edit or RichEdit control), or nil. It is how a
	// replacement is verified before anything is deleted.
	FocusedField(window uintptr) core.TextFieldAccess
	// Keystrokes types on the user's behalf. expectSeq, when non-zero, is the newest key
	// press the edit was planned against: the sender refuses once the hook has seen another.
	Keystrokes(expectSeq uint64) core.KeystrokeSender
	// KeySeq is the sequence number of the newest key press the hook has seen.
	KeySeq() uint64
	// ReplayAccept re-sends an accept key the hook swallowed for a suggestion that was
	// already gone, so the keystroke is not lost.
	ReplayAccept(key config.AcceptKey)

	ShowSuggestion(s Suggestion)
	HideSuggestion()
	// SetAcceptKey tells the hook which key to swallow while a suggestion is up.
	SetAcceptKey(key config.AcceptKey, active bool)

	// CopySelection copies the selection through the clipboard; restore puts the
	// clipboard back. ok is false when nothing was selected.
	CopySelection() (text string, restore func(), ok bool)
	// Paste replaces the selection with text via the clipboard, then calls restore.
	Paste(text string, restore func()) bool
	ReadClipboard() (string, bool)
	WriteClipboard(text string) bool

	Notify(title, message string)
	// PlaySound is the sound of a fix; Beep says that a command had nothing to act on.
	PlaySound()
	Beep()
}

// LogPrivate is what stands in the log for a value that comes from the user's machine and is
// not a number — the name of the application in front, say. The counterpart of the unified
// log's <private>.
const LogPrivate = "<private>"

// Logger receives diagnostics. The engine never logs typed text, clipboard content or window
// titles: only lengths, languages, decisions and counts. (engine/privacy_test.go reads every
// call of Printf in this package and refuses new ones that name text.)
type Logger interface {
	Printf(format string, args ...any)
}

type discard struct{}

func (discard) Printf(string, ...any) {}

// Status is a snapshot for the tray menu, safe to read from any goroutine.
type Status struct {
	App      string
	Mode     config.FixMode
	CanUndo  bool
	Pending  bool
	Language core.Lang
}

// Engine is the pipeline.
type Engine struct {
	platform Platform
	store    *config.Store
	models   *core.Models
	detector *core.Detector
	typing   *core.TypingEngine
	learner  *core.Learner
	undo     *core.UndoStack
	log      Logger
	now      func() time.Time
	sleep    func(time.Duration)

	// The clock and timers behind the privacy promises (see SetScheduler).
	sched     core.Scheduler
	dispatch  func(func())
	undoTimer core.Timer
	undoToken uint64

	cfg        config.Config
	exceptions core.StringSet

	window uintptr
	app    string
	mode   config.FixMode

	// pending is a correction offered and awaiting the accept key.
	pending       *core.Outcome
	pendingWindow uintptr
	pendingID     uint64

	// generation advances with everything that changes the text or moves the caret, so
	// an undo can tell whether the screen still looks the way the fix left it.
	generation   uint64
	afterDeadKey bool
	lastKeyTime  float64

	// The text in front of the buffer is read from the field at most once per buffer session
	// (core.TypingEngine.Session: every reset and idle pause starts a new one).
	contextRead    bool
	contextText    string
	contextSession int

	// fieldUnreliable: applications whose text control misbehaved when edited directly;
	// their next corrections go straight to keystrokes.
	fieldUnreliable map[string]bool

	status atomic.Pointer[Status]
}

// New builds an engine. models may be nil (detection then stays off); learner may be nil (an
// in-memory learner is used, nothing persists).
func New(platform Platform, store *config.Store, models *core.Models, learner *core.Learner, log Logger) *Engine {
	if log == nil {
		log = discard{}
	}
	if learner == nil {
		learner = core.NewLearner(nil)
	}
	e := &Engine{
		platform:        platform,
		store:           store,
		models:          models,
		learner:         learner,
		undo:            core.NewUndoStack(20),
		log:             log,
		now:             time.Now,
		sleep:           time.Sleep,
		fieldUnreliable: map[string]bool{},
	}
	if models != nil {
		e.detector = core.NewDetector(models)
	}
	e.typing = core.NewTypingEngine(e.detector, core.NewStrokeBuffer(256, store.Get().BufferIdleReset))
	e.typing.OnIdleReset = e.onIdleReset
	// Whatever empties the buffer also drops the text read from the field in front of it.
	e.typing.OnReset = func() { e.contextRead, e.contextText = false, "" }
	// The undo list measures its two minutes on the engine's clock, whatever SetClock says later.
	e.undo.Now = func() time.Time { return e.now() }
	e.typing.Learner = learner
	e.typing.Resolve = platform.Table
	if e.detector != nil {
		e.typing.IsDictionaryWord = e.detector.IsDictionaryWord
	}
	e.RefreshConfig()
	e.publish()
	return e
}

// SetClock replaces the wall clock (tests).
func (e *Engine) SetClock(now func() time.Time) { e.now = now }

// SetScheduler turns on the timers the privacy promises rest on: the keystroke buffer is
// emptied Config.BufferIdleReset (at most five) seconds after the last key, and every correction
// is wiped from the undo list two minutes after it was made, in both cases by the clock and not
// by the next call. Timers fire on goroutines of their own; dispatch must run its argument on
// the goroutine that owns the engine (the Windows app posts it to the engine's event channel).
// A nil dispatch runs it in place, which is right for single-goroutine tests. Without a
// scheduler there are no timers: entries still expire when they are looked at.
func (e *Engine) SetScheduler(s core.Scheduler, dispatch func(func())) {
	e.sched, e.dispatch = s, dispatch
	e.typing.Scheduler, e.typing.Dispatch = s, dispatch
}

// onIdleReset runs when the idle timer has emptied the keystroke buffer. The text read from
// the field goes with it; an offer on screen stays, it has its own, shorter, timeout.
func (e *Engine) onIdleReset() {
	e.afterDeadKey = false
	e.log.Printf("buffer cleared: idle")
}

// Wipe forgets everything the engine holds about what was typed or corrected: the buffer and
// the words derived from it, the text read from the field, the offer on screen and the undo
// list. The app calls it on screen lock, sleep and sign-out.
func (e *Engine) Wipe(reason string) {
	e.reset(reason, false)
	e.clearUndo()
	e.publish()
}

// clearUndo wipes the undo list and its timer.
func (e *Engine) clearUndo() {
	e.undo.Clear()
	e.stopUndoTimer()
}

func (e *Engine) stopUndoTimer() {
	if e.undoTimer != nil {
		e.undoTimer.Stop()
		e.undoTimer = nil
	}
	e.undoToken++
}

// pushUndo records a fix and makes sure a timer will wipe it when its two minutes are up.
func (e *Engine) pushUndo(fix core.AppliedFix) {
	e.undo.Push(fix)
	e.armUndoTimer()
}

// armUndoTimer aims one timer at the oldest entry; when it fires everything due goes and it
// aims at the next one.
func (e *Engine) armUndoTimer() {
	if e.sched == nil || e.undoTimer != nil {
		return
	}
	when, ok := e.undo.NextExpiry()
	if !ok {
		return
	}
	e.undoToken++
	token, dispatch := e.undoToken, e.dispatch
	wait := max(when.Sub(e.now()), 0)
	e.undoTimer = e.sched.AfterFunc(wait, func() {
		run := func() { e.undoExpired(token) }
		if dispatch != nil {
			dispatch(run)
		} else {
			run()
		}
	})
}

// undoExpired runs on the engine's goroutine when the undo timer has fired.
func (e *Engine) undoExpired(token uint64) {
	if token != e.undoToken || e.undoTimer == nil {
		return
	}
	e.undoTimer = nil
	dropped := e.undo.DropExpired()
	e.armUndoTimer()
	if dropped > 0 {
		e.publish()
	}
}

// SetSleep replaces the sleep used while waiting for an application to catch up (tests).
func (e *Engine) SetSleep(sleep func(time.Duration)) { e.sleep = sleep }

// Learner is what the engine learns from; the tray reads and clears it.
func (e *Engine) Learner() *core.Learner { return e.learner }

// RefreshConfig re-reads the settings snapshot. Call after every config change.
func (e *Engine) RefreshConfig() {
	e.cfg = e.store.Get()
	e.exceptions = core.NewStringSet(e.cfg.WordExceptions...)
	e.typing.Buffer.SetIdleReset(e.cfg.BufferIdleReset)
	e.mode = e.cfg.ModeFor(e.app)
	if !e.cfg.Enabled {
		e.Reset("disabled")
	}
	e.publish()
}

// Status returns the latest status snapshot.
func (e *Engine) Status() Status {
	if status := e.status.Load(); status != nil {
		return *status
	}
	return Status{}
}

func (e *Engine) publish() {
	status := &Status{App: e.app, Mode: e.mode, Pending: e.pending != nil}
	if fix, ok := e.undo.Last(); ok {
		status.CanUndo = fix.ClipboardBefore != nil || e.undoable(fix)
	}
	if table := e.platform.Table(e.platform.CurrentLayoutID(e.window)); table != nil {
		status.Language = table.Lang
	}
	e.status.Store(status)
}

// bumpGeneration records that the text or the caret changed. An offered undo expires with
// it, so the status is republished when one was on offer.
func (e *Engine) bumpGeneration() {
	e.generation++
	if status := e.status.Load(); status != nil && status.CanUndo {
		e.publish()
	}
}

// BufferLen is the number of buffered keystrokes (tests and diagnostics).
func (e *Engine) BufferLen() int { return e.typing.Buffer.Count() }

// ScreenText is what the buffer says is on screen (tests).
func (e *Engine) ScreenText() string { return e.typing.ScreenText(e.platform.Table) }

// UndoCount is the number of fixes that can be taken back.
func (e *Engine) UndoCount() int { return e.undo.Count() }

// PendingOutcome is the suggestion on screen, if any.
func (e *Engine) PendingOutcome() *core.Outcome { return e.pending }

// MARK: - Context

// Foreground records a new foreground window. The caret is now somewhere else entirely.
//
// Idempotent for the current window: Windows posts the foreground event, while key presses
// arrive synchronously, so after Alt+Tab the first keys typed in the new window can reach
// the engine (carrying that window) before the event does. Resetting again then would
// throw away the start of the first word.
func (e *Engine) Foreground(window uintptr) {
	if window == e.window && e.app != "" {
		return
	}
	e.window = window
	e.app = e.platform.AppName(window)
	e.mode = e.cfg.ModeFor(e.app)
	e.Reset("appSwitch")
	// The caret is somewhere else entirely, and so is the text a correction was made in.
	e.clearUndo()
	e.publish()
}

// Reset throws the buffer away: the text it describes is no longer what is in front of
// the caret. An offer on screen goes with it, without counting as ignored.
func (e *Engine) Reset(reason string) { e.reset(reason, false) }

// reset: ignored says the reset was Escape, the one deliberate "no" to an offer on screen,
// which counts against offering the same word again. Enter, the arrows, a click or an app
// switch only mean the user went elsewhere.
func (e *Engine) reset(reason string, ignored bool) {
	e.typing.Reset()
	e.afterDeadKey = false
	e.bumpGeneration()
	e.dismissPending(ignored)
	// Not for a password field: when one was in front is not written down.
	if reason == "disabled" {
		e.log.Printf("buffer cleared: %s", reason)
	}
}

// MARK: - Keystroke path

// HandleKey processes one key press. The hook has already passed it to the application.
func (e *Engine) HandleKey(ev KeyEvent) {
	if ev.Class == KeyModifier {
		return
	}
	if ev.Window != e.window {
		e.Foreground(ev.Window)
	}
	e.bumpGeneration()
	e.lastKeyTime = ev.Time

	if ev.Foreign {
		e.Reset("foreignInput")
		return
	}

	switch ev.Class {
	case KeyBackspace:
		if ev.Ctrl || ev.Alt || ev.Win {
			e.Reset("shortcut")
			return
		}
		// The text under an offer just changed, so the offer no longer describes it —
		// accepting it now would replace characters that are not there. Not counted as
		// ignoring it: the user may be fixing that very word by hand.
		e.dismissPending(false)
		if !e.cfg.Enabled || e.mode == config.ModeOff {
			e.typing.Reset()
			return
		}
		e.typing.Backspace()
		return
	case KeyEscape:
		if ev.Ctrl || ev.Alt || ev.Win {
			// Ctrl+Esc opens Start, Alt+Esc switches windows: shortcuts, not an answer.
			e.Reset("shortcut")
			return
		}
		// Escape is the one deliberate "no" to an offer on screen, and like the navigation
		// keys it ends the line of thought.
		e.reset("escape", true)
		return
	case KeyNavigation:
		// Enter, the arrows: the caret moved or the line was committed. An offer left on
		// screen through them only means the user went elsewhere.
		e.Reset("navigationKey")
		return
	case KeyTab:
		// Tab inserts a tab or moves focus; the tables have no text for it, so keeping it
		// would leave the buffer one character short of the screen. Typed past an offer, it
		// leaves the word as it is, like any other key.
		e.Reset("nonTextKey")
		return
	case KeyOther:
		e.Reset("nonTextKey")
		return
	}

	// Read the layout captured by the hook. A stroke attributed to the wrong layout is worse
	// than useless: it reads as correct text.
	layout := e.platform.Table(ev.LayoutID)
	var mods core.ModState
	if ev.Shift {
		mods |= core.ModShift
	}
	if ev.Caps {
		mods |= core.ModCaps
	}
	if ev.Ctrl || ev.Alt || ev.Win {
		// Ctrl+Alt is AltGr on Windows: text if the layout has a third level here. Anything
		// else is a shortcut, not text — whatever it does (paste, undo, select-all), the
		// buffer stops matching the screen.
		if !(ev.Ctrl && ev.Alt && !ev.Win) || layout == nil {
			e.Reset("shortcut")
			return
		}
		mods |= core.ModOption
		if _, ok := layout.Text(core.KeyStroke{Key: ev.Key, Mods: mods}); !ok {
			e.Reset("shortcut")
			return
		}
	}

	// Typing on past an offer leaves the word as it is this time, and says nothing more:
	// people type on because they did not see the offer at least as often as because they
	// meant the word. Counting that as a refusal once taught the app to stop fixing ordinary
	// words («написал» typed on ABC) for good.
	e.dismissPending(false)

	if !e.cfg.Enabled || e.mode == config.ModeOff {
		e.typing.Reset()
		return
	}
	if e.platform.IsPasswordField(ev.Window) {
		// The one case where correctness means doing nothing at all.
		if !e.typing.Buffer.IsEmpty() {
			e.Reset("passwordField")
		}
		return
	}
	if layout == nil {
		// An input method, or a layout installed since the tables were built.
		e.Reset("unknownLayout")
		return
	}

	stroke := core.KeyStroke{Key: ev.Key, Mods: mods}
	if layout.IsDeadKey(stroke) {
		// The next key composes with this one into a character we cannot predict.
		e.Reset("deadKey")
		e.afterDeadKey = true
		return
	}
	if e.afterDeadKey {
		e.afterDeadKey = false
		e.Reset("deadKey")
		return
	}
	if _, ok := layout.Text(stroke); !ok {
		// The key types nothing on this layout, so the screen did not change either.
		return
	}

	end, wordEnded := e.typing.Record(stroke, layout, ev.Time, e.candidateTables())

	// Note which Cyrillic layout this app is written in; it is the tie-breaker when the
	// letters cannot distinguish Russian from Kazakh.
	if layout.Lang.Script() == core.ScriptCyrillic {
		e.rememberCyrillic(layout.SourceID)
	}

	if !wordEnded || !e.cfg.SmartMode || e.detector == nil {
		return
	}
	e.evaluate(end, ev.Seq)
}

// HandleMouse: a click moved the caret somewhere the buffer knows nothing about.
func (e *Engine) HandleMouse() {
	if !e.cfg.Enabled {
		return
	}
	e.Reset("mouseClick")
}

// HandleFocusChange: keyboard focus moved inside the foreground window.
func (e *Engine) HandleFocusChange() { e.Reset("focusChange") }

// MARK: - Deciding

func (e *Engine) candidateTables() []*core.KeyMapTable {
	langs := e.cfg.Languages()
	var out []*core.KeyMapTable
	for _, table := range e.platform.Tables() {
		if table.Lang != core.LangNone && langs[table.Lang] {
			out = append(out, table)
		}
	}
	return out
}

func (e *Engine) settings(mayApply bool) core.TypingSettings {
	settings := core.NewTypingSettings(e.candidateTables(), e.platform.Table)
	settings.MinWordLength = e.cfg.MinWordLength
	settings.Exceptions = e.exceptions
	settings.AutoThreshold = e.cfg.AutoThreshold
	settings.SuggestThreshold = e.cfg.SuggestThreshold
	settings.MayApply = mayApply
	if e.learner.Enabled() {
		settings.Learned = e.learner
	}
	return settings
}

func (e *Engine) evaluate(end core.WordEnd, seq uint64) {
	if e.typing.Buffer.IsEmpty() {
		return
	}
	decision := e.typing.Decide(end, e.settings(e.mode == config.ModeAuto), e.precedingText())
	e.trace(decision)
	switch decision.Kind {
	case core.DecideApply:
		e.apply(decision.Outcome, seq, true)
	case core.DecideOffer:
		e.offer(decision.Outcome)
	}
}

// precedingText is the last few words in front of the buffer, from the text field itself,
// so the first word after a click still has context. Read at most once per buffer session,
// only when the buffer holds too little context of its own and the field can be read (an
// Edit or RichEdit control), and reduced at once to three words. It never leaves memory
// and is never logged.
func (e *Engine) precedingText() string {
	if e.contextRead && e.contextSession == e.typing.Session() {
		return e.contextText
	}
	e.contextRead, e.contextSession, e.contextText = true, e.typing.Session(), ""

	// Plenty of context in the buffer already: do not touch the app.
	if e.typing.Buffer.Count() >= 24 {
		return ""
	}
	field := e.platform.FocusedField(e.window)
	if field == nil {
		return ""
	}
	caret, ok := field.SelectedRange()
	if !ok || caret.Length != 0 {
		return ""
	}
	end := caret.Location - core.UTF16Len(e.typing.ScreenText(e.platform.Table))
	if end <= 0 {
		return ""
	}
	start := max(0, end-160)
	before, ok := field.StringIn(core.Range16{Location: start, Length: end - start})
	if !ok {
		return ""
	}
	words := strings.FieldsFunc(before, unicode.IsSpace)
	if len(words) > 3 {
		words = words[len(words)-3:]
	}
	e.contextText = strings.Join(words, " ")
	return e.contextText
}

func (e *Engine) trace(decision core.Decision) {
	if !e.cfg.Diagnostics {
		return
	}
	if decision.Outcome == nil {
		e.log.Printf("detect: none (buffer %d)", e.typing.Buffer.Count())
		return
	}
	o := decision.Outcome
	kind := "offer"
	if decision.Kind == core.DecideApply {
		kind = "apply"
	}
	e.log.Printf("detect: %s confidence=%.2f words=%d chars=%d→%d lang=%s",
		kind, o.Confidence, len(o.Changes), core.CharCount(o.Before), core.CharCount(o.After), o.TargetLang)
}

// MARK: - Offering

func (e *Engine) effectiveAcceptKey() config.AcceptKey {
	// Tab is load-bearing in terminals and editors, so the secondary binding takes over
	// there rather than stealing a completion keystroke.
	if e.cfg.AcceptKey == config.AcceptTab && config.IsTabCritical(e.app) {
		return config.AcceptInsert
	}
	return e.cfg.AcceptKey
}

// AcceptLabel is the key name shown in the popup.
func AcceptLabel(key config.AcceptKey) string {
	if key == config.AcceptInsert {
		return "Ins"
	}
	return "Tab"
}

func (e *Engine) offer(outcome *core.Outcome) {
	e.pendingID++
	e.pending = outcome
	e.pendingWindow = e.window
	key := e.effectiveAcceptKey()
	// Arm the hook before the popup appears, so a quick accept is never missed.
	e.platform.SetAcceptKey(key, true)
	e.platform.ShowSuggestion(Suggestion{
		ID:          e.pendingID,
		Before:      outcome.Before,
		After:       outcome.After,
		AcceptLabel: AcceptLabel(key),
		Timeout:     time.Duration(e.cfg.SuggestionTimeout * float64(time.Second)),
		Window:      e.window,
	})
	e.publish()
}

// dismissPending takes the offer down. ignored only when the user turned it down with
// Escape, which counts against offering the same word again; typing on past it or letting it
// time out says nothing.
func (e *Engine) dismissPending(ignored bool) {
	outcome := e.pending
	if outcome == nil {
		return
	}
	e.pending = nil
	e.pendingWindow = 0
	e.platform.SetAcceptKey("", false)
	e.platform.HideSuggestion()
	if ignored {
		for _, change := range outcome.Changes {
			e.learner.Ignored(change.Typed)
		}
	}
	e.publish()
}

// HandleAccept is called when the hook swallowed the accept key for a suggestion.
func (e *Engine) HandleAccept(seq uint64) {
	outcome := e.pending
	if outcome == nil {
		// The suggestion was already gone when the key arrived; give the key back.
		e.platform.ReplayAccept(e.effectiveAcceptKey())
		return
	}
	window := e.pendingWindow
	e.dismissPending(false)
	// The user may have moved on while the popup was up.
	if window != e.window {
		return
	}
	if e.apply(outcome, seq, false) {
		// Accepted: next time, without asking.
		for _, change := range outcome.Changes {
			e.learner.Confirmed(change.Typed, change.TargetLang)
		}
	}
}

// HandleSuggestionTimeout is called when the popup timed out on its own. Not a "no": the user
// may simply not have looked.
func (e *Engine) HandleSuggestionTimeout(id uint64) {
	if e.pending != nil && id == e.pendingID {
		e.dismissPending(false)
	}
}

// MARK: - Applying

// replace makes an edit through core.TextReplacer: verified against the field where the
// field can be read, by keystrokes otherwise. seq is the key press the edit was planned
// against (0: whatever is newest now); the edit is abandoned, untouched, once the user has
// pressed another key.
func (e *Engine) replace(r core.Replacement, seq uint64) core.ReplaceOutcome {
	// Every correction replaces text that was typed; inserting without deleting is exactly
	// the failure (text next to the old text) this must never cause.
	if r.DeleteUTF16Count() == 0 {
		return core.ReplaceOutcome{Result: core.ReplaceFailed, Reason: "nothing to delete"}
	}
	expect := seq
	if expect == 0 {
		expect = e.platform.KeySeq()
	}
	field := e.platform.FocusedField(e.window)
	replacer := core.TextReplacer{
		Keys:                e.platform.Keystrokes(expect),
		Wait:                e.sleep,
		PreferAccessibility: !e.fieldUnreliable[e.app],
		Verify:              true,
		Current:             func() bool { return e.platform.KeySeq() == expect },
	}
	if field != nil {
		replacer.Field = field
	}
	outcome := replacer.Apply(r)
	if outcome.Result == core.ReplaceMismatch && e.platform.KeySeq() != expect {
		// The text differs because the user's own next key landed before we looked: the
		// buffer will follow it as soon as that key reaches the engine.
		outcome = core.ReplaceOutcome{Result: core.ReplaceFailed, Reason: "typing continued", Abandoned: true}
	}
	// Remember an application whose control misbehaved, so the next correction there goes
	// straight to keystrokes. A text mismatch is not the application's fault.
	if replacer.AccessibilityUnreliable && e.app != "" && !e.fieldUnreliable[e.app] {
		e.fieldUnreliable[e.app] = true
		// Which application is the user's business: it is logged as a private value.
		e.log.Printf("direct edit unreliable in an application (%s), using keystrokes", LogPrivate)
	}
	switch {
	case outcome.Result == core.ReplaceMismatch:
		e.log.Printf("screen does not match the buffer (%d chars); nothing done", r.DeleteCount())
	case outcome.Result == core.ReplaceFailed && !outcome.Abandoned:
		e.log.Printf("replacement not applied: %s", outcome.Reason)
	}
	return outcome
}

// apply makes a fix and returns whether it reached the screen.
func (e *Engine) apply(outcome *core.Outcome, seq uint64, automatic bool) bool {
	if len(outcome.Corrections) == 0 {
		return false
	}
	// If the user typed on after the key that triggered this, the text before the caret is
	// no longer what the replacement expects: deleting now would eat the wrong characters.
	// Nothing was touched and the buffer still follows the screen, so it is kept.
	if seq != 0 && e.platform.KeySeq() != seq {
		e.log.Printf("fix skipped: typing continued before it could be applied")
		return false
	}
	if !e.platform.CanInject(e.window) {
		e.log.Printf("fix skipped: the window does not accept synthetic input")
		return false
	}
	previousLayoutID := e.platform.CurrentLayoutID(e.window)

	result := e.replace(outcome.Replacement, seq)
	if result.Result != core.ReplaceApplied {
		if result.Abandoned {
			// A key arrived before anything was touched: the buffer still follows the screen,
			// and that key is on its way to the engine.
			e.log.Printf("fix skipped: typing continued before it could be applied")
			return false
		}
		// Whatever happened, the buffer can no longer be trusted to describe the screen.
		e.typing.Reset()
		return false
	}

	e.typing.DidApply(outcome, automatic, e.lastKeyTime)
	e.switchLayout(outcome)

	typedWords := make([]string, len(outcome.Changes))
	for i, change := range outcome.Changes {
		typedWords[i] = change.Typed
	}
	e.pushUndo(core.AppliedFix{
		Kind:             core.FixLayout,
		Replacement:      core.Replacement{DeletedText: outcome.Replacement.DeletedText, InsertText: result.Inserted},
		App:              e.app,
		Time:             e.now(),
		OriginalWord:     outcome.Before,
		CorrectedWord:    outcome.After,
		PreviousLayoutID: previousLayoutID,
		Strategy:         string(result.Strategy),
		TypedWords:       typedWords,
		Window:           e.window,
		Generation:       e.generation,
	})
	if e.cfg.PlaySound {
		e.platform.PlaySound()
	}
	how := "accepted"
	if automatic {
		how = "automatic"
	}
	e.log.Printf("fixed %d→%d chars via %s, lang=%s, confidence=%.2f, %s",
		core.CharCount(outcome.Before), core.CharCount(outcome.After), result.Strategy, outcome.TargetLang,
		outcome.Confidence, how)
	e.publish()
	return true
}

// switchLayout switches the keyboard so what the user types next lands in the right
// language. When the corrected text carries no Kazakh-specific letter, Russian and Kazakh
// are indistinguishable from the text alone. Then the user's preference decides — a fixed
// language if they chose one, otherwise the Cyrillic layout this app was last written in —
// over whatever the models marginally preferred.
func (e *Engine) switchLayout(outcome *core.Outcome) {
	targetID := outcome.TargetLayoutID
	if outcome.TargetLang.Script() == core.ScriptCyrillic && !core.ContainsKazakhSpecific(outcome.After) {
		switch e.cfg.CyrillicPreference {
		case config.CyrillicPreferKazakh:
			if table := e.firstTable(core.LangKK); table != nil {
				targetID = table.SourceID
			}
		case config.CyrillicPreferRussian:
			if table := e.firstTable(core.LangRU); table != nil {
				targetID = table.SourceID
			}
		default:
			if remembered := e.cfg.RememberedCyrillic(e.app); remembered != "" {
				if table := e.platform.Table(remembered); table != nil && table.Lang.Script() == core.ScriptCyrillic {
					targetID = remembered
				}
			}
		}
	}
	if targetID == e.platform.CurrentLayoutID(e.window) {
		return
	}
	e.platform.SelectLayout(e.window, targetID)
	if table := e.platform.Table(targetID); table != nil && table.Lang.Script() == core.ScriptCyrillic {
		e.rememberCyrillic(targetID)
	}
}

func (e *Engine) firstTable(lang core.Lang) *core.KeyMapTable {
	for _, table := range e.platform.Tables() {
		if table.Lang == lang {
			return table
		}
	}
	return nil
}

func (e *Engine) rememberCyrillic(layoutID string) {
	if e.cfg.RememberedCyrillic(e.app) == layoutID || !e.cfg.RememberLayoutPerApp || e.app == "" {
		return
	}
	e.updateConfig(func(c *config.Config) { c.RememberCyrillic(layoutID, e.app) })
}

func (e *Engine) updateConfig(fn func(*config.Config)) {
	if err := e.store.Update(fn); err != nil {
		e.log.Printf("saving settings failed: %v", err)
	}
	e.RefreshConfig()
}

// MARK: - Actions

// blockedByPasswordField: while a password field is in front, the commands that read text or
// the clipboard do nothing at all. (Typing into it never reaches the engine as text; this
// covers the hotkeys and the tray menu.)
func (e *Engine) blockedByPasswordField() bool {
	if !e.platform.IsPasswordField(e.window) {
		return false
	}
	// A beep, and no log line: when a password field was in front is not written down.
	e.platform.Beep()
	return true
}

// tooLong: clipboard and selection text over core.MaxClipboardText characters is ignored —
// not converted, not kept.
func (e *Engine) tooLong(text string) bool {
	if !core.ExceedsClipboardLimit(text) {
		return false
	}
	e.nothing("text is too long; left alone")
	return true
}

// Perform runs a hotkey or menu action. seq is the key press that triggered it (0 for
// menu items).
func (e *Engine) Perform(action config.Action, seq uint64) {
	switch action {
	case config.ActionToggleEnabled:
		e.updateConfig(func(c *config.Config) { c.Enabled = !c.Enabled })
	case config.ActionUndoLastFix:
		e.UndoLastFix(seq)
	case config.ActionFixLastWord:
		e.FixLastWordOrUndo(seq)
	case config.ActionFixSelection:
		e.FixSelection()
	case config.ActionConvertClipboard:
		e.ConvertClipboard()
	case config.ActionTransliterate:
		e.Transliterate(nil, seq)
	case config.ActionForceEnglish:
		e.selectLang(core.LangEN)
	case config.ActionForceRussian:
		e.selectLang(core.LangRU)
	case config.ActionForceKazakh:
		e.selectLang(core.LangKK)
	}
	e.publish()
}

func (e *Engine) selectLang(lang core.Lang) bool {
	if table := e.firstTable(lang); table != nil {
		return e.platform.SelectLayout(e.window, table.SourceID)
	}
	return false
}

// FixLastWordOrUndo is the Pause key, Punto style: right after a fix (nothing typed since)
// it takes the fix back; otherwise it corrects the last word.
func (e *Engine) FixLastWordOrUndo(seq uint64) {
	if fix, ok := e.undo.Last(); ok && fix.Kind == core.FixLayout && e.undoable(fix) {
		e.UndoLastFix(seq)
		return
	}
	e.FixLastWord(seq)
}

// FixLastWord is the manual correction: it acts even when confidence is below the
// automatic threshold, and even on a real word, because the user asked for it explicitly.
// That makes it a lesson too.
func (e *Engine) FixLastWord(seq uint64) {
	if e.typing.Buffer.IsEmpty() || e.detector == nil || e.blockedByPasswordField() {
		return
	}
	e.dismissPending(false)
	outcome := e.typing.ManualFix(e.settings(true))
	if outcome == nil {
		return
	}
	if e.apply(outcome, seq, false) {
		for _, change := range outcome.Changes {
			e.learner.Confirmed(change.Typed, change.TargetLang)
		}
	}
}

// undoable: where the text cannot be read back, the inverse can only be typed blind, which
// is safe only while nothing has happened since the fix.
func (e *Engine) undoable(fix core.AppliedFix) bool {
	return fix.Generation == e.generation && fix.Window == e.window
}

// UndoLastFix takes the newest fix back, and learns from it when it was undone promptly.
func (e *Engine) UndoLastFix(seq uint64) {
	if e.blockedByPasswordField() {
		return
	}
	// Nothing to take back, or the two minutes are up: a quiet no-op.
	fix, ok := e.undo.Last()
	if !ok {
		return
	}
	if fix.ClipboardBefore == nil && !e.undoable(fix) {
		e.platform.Notify(i18n.T(i18n.NotifyUndoUnavailableTitle), i18n.T(i18n.NotifyUndoUnavailableText))
		return
	}
	e.undo.PopLast()
	wasRecent := e.now().Sub(fix.Time) <= core.DefaultFreshWindow

	if fix.ClipboardBefore != nil {
		// The change was made to the clipboard, so put the clipboard back.
		e.platform.WriteClipboard(*fix.ClipboardBefore)
	} else if result := e.replace(fix.Replacement.Inverse(), seq); result.Result != core.ReplaceApplied {
		if result.Abandoned {
			e.log.Printf("undo skipped: typing continued")
			return
		}
		// The fixed text is no longer where we left it; undoing blindly would damage
		// whatever is there now.
		e.log.Printf("undo skipped: the corrected text is not at the caret")
		e.Reset("undoFailed")
		return
	}
	if fix.PreviousLayoutID != "" {
		e.platform.SelectLayout(e.window, fix.PreviousLayoutID)
	}

	// Undoing straight away means we got it wrong, and what to remember depends on what
	// was undone.
	if wasRecent {
		switch fix.Kind {
		case core.FixLayout:
			// Each changed word goes on the visible exceptions list, where the user can see
			// it and take it off again.
			words := fix.TypedWords
			if len(words) == 0 {
				words = []string{fix.OriginalWord}
			}
			var kept []string
			for _, word := range words {
				if word != "" {
					kept = append(kept, word)
					e.learner.ForgetWord(word)
				}
			}
			e.updateConfig(func(c *config.Config) {
				for _, word := range kept {
					c.AddWordException(word)
				}
			})
			e.log.Printf("learned %d word exception(s) from undo", len(kept))
		case core.FixTransliteration:
			if e.models != nil {
				rejected := e.transliterator().ChangedWords(fix.OriginalWord, fix.CorrectedWord)
				e.updateConfig(func(c *config.Config) { c.AddNeverConvert(rejected) })
				e.log.Printf("learned %d never-convert word(s) from undo", len(rejected))
			}
		}
	}
	e.Reset("undo")
}

// lineCopyApps copy the whole current line on Ctrl+C when nothing is selected. Pasting a
// conversion of that back would insert a duplicate line instead of replacing anything.
var lineCopyApps = map[string]bool{
	"code.exe": true, "code - insiders.exe": true, "vscodium.exe": true, "cursor.exe": true,
	"windsurf.exe": true, "devenv.exe": true, "sublime_text.exe": true, "idea64.exe": true,
	"pycharm64.exe": true, "webstorm64.exe": true, "clion64.exe": true, "rider64.exe": true,
	"goland64.exe": true, "phpstorm64.exe": true, "datagrip64.exe": true, "rustrover64.exe": true,
	"studio64.exe": true,
}

// nothing says that nothing could be done with a beep, rather than silently doing nothing.
func (e *Engine) nothing(reason string) {
	e.log.Printf("%s", reason)
	e.platform.Beep()
}

// copySelection reads the selection, treating an editor's copied line as "no selection".
// Terminals are never sent Ctrl+C: there it interrupts the running program.
func (e *Engine) copySelection() (string, func(), bool) {
	if config.IsTerminal(e.app) {
		return "", nil, false
	}
	text, restore, ok := e.platform.CopySelection()
	if ok && lineCopyApps[e.app] && strings.HasSuffix(text, "\n") {
		return "", restore, false
	}
	return text, restore, ok
}

// FixSelection converts the selected text in place (read through the clipboard, which is
// restored). Used for text pasted from someone else, where there are no keystrokes.
func (e *Engine) FixSelection() {
	source := e.platform.Table(e.platform.CurrentLayoutID(e.window))
	if source == nil || e.models == nil || e.blockedByPasswordField() {
		return
	}
	text, restore, ok := e.copySelection()
	if !ok || text == "" {
		if restore != nil {
			restore()
		}
		e.nothing("no selection to convert")
		return
	}
	if e.tooLong(text) {
		if restore != nil {
			restore()
		}
		return
	}
	best, found := core.BestConversion(text, source, e.candidateTables(), e.models)
	if !found {
		restore()
		e.nothing("selection needs no fix")
		return
	}
	if !e.platform.Paste(best.Text, restore) {
		e.log.Printf("selection replacement failed")
		return
	}
	e.platform.SelectLayout(e.window, best.Layout.SourceID)
	e.Reset("selectionConverted")
}

// ConvertClipboard converts the clipboard contents in place.
func (e *Engine) ConvertClipboard() {
	if e.blockedByPasswordField() {
		return
	}
	source := e.platform.Table(e.platform.CurrentLayoutID(e.window))
	if source == nil || e.models == nil {
		return
	}
	// The platform has already refused a clipboard that is flagged as sensitive (it then
	// reports nothing to read); text over the limit is ignored here, whatever the platform.
	text, ok := e.platform.ReadClipboard()
	if !ok || text == "" || e.tooLong(text) {
		return
	}
	if best, found := core.BestConversion(text, source, e.candidateTables(), e.models); found {
		e.platform.WriteClipboard(best.Text)
	}
}

// MARK: - Transliteration

func (e *Engine) transliterator() *translit.Transliterator {
	return translit.New(e.models, e.cfg.TranslitOverrides, e.cfg.TranslitNeverConvert)
}

func (e *Engine) scheme() translit.Scheme {
	if e.cfg.TranslitScheme == string(translit.Official2021) {
		return translit.Official2021
	}
	return translit.Practical
}

// Transliterate rewrites text between Latin and Cyrillic Kazakh. direction nil means "work it
// out from the text"; seq is the key press that asked for it (0 for the menu). What it
// rewrites, in order:
//
//  1. The text just typed, unless a selection is on screen: nobody selects what they have
//     just written before asking for it in the other alphabet. Where the field can be read
//     (Edit, RichEdit) the words are taken from the screen, so the application's own
//     autocorrection does not break it; elsewhere the keystrokes are all there is.
//  2. The selection, copied with Ctrl+C and pasted back, the clipboard restored.
//  3. The clipboard itself, when nothing is selected.
//
// A beep when there is nothing to do. The Swift app first converts a selection it can read
// through accessibility; here a selection is always taken through the clipboard, so a
// selection seen in an Edit or RichEdit control only makes the typed text step aside.
func (e *Engine) Transliterate(direction *translit.Direction, seq uint64) {
	if e.models == nil || e.blockedByPasswordField() {
		return
	}
	switch e.transliterateTyped(direction, seq) {
	case typedDone, typedAbandoned:
		return
	}

	// A selection: copy it, convert it, paste it back.
	text, restore, ok := e.copySelection()
	if !ok || text == "" {
		if restore != nil {
			restore()
		}
		// Nothing selected at all: the documented fallback, the clipboard itself.
		e.TransliterateClipboard(direction)
		return
	}
	if e.tooLong(text) {
		if restore != nil {
			restore()
		}
		return
	}
	resolved := resolveDirection(direction, text)
	converted := e.transliterator().Convert(text, e.scheme(), resolved)
	if core.CanonEqual(converted, text) {
		restore()
		e.nothing("nothing to transliterate in the selection")
		return
	}
	if !e.platform.Paste(converted, restore) {
		e.log.Printf("selection rejected the transliteration")
		return
	}
	e.Reset("transliterated")
	e.switchLayoutForScript(scriptFor(resolved))
	// The pasted text sits right before the caret, so typing the inverse undoes it —
	// provided it can be retyped.
	if typeableBlind(text) {
		e.pushUndo(core.AppliedFix{
			Kind:          core.FixTransliteration,
			Replacement:   core.Replacement{DeletedText: text, InsertText: converted},
			App:           e.app,
			Time:          e.now(),
			OriginalWord:  text,
			CorrectedWord: converted,
			Strategy:      "clipboard",
			Window:        e.window,
			Generation:    e.generation,
		})
	}
	e.publish()
}

// typedResult is how converting the text just typed went.
type typedResult int

const (
	// typedNone: nothing typed, a selection on screen, or the text could not be replaced —
	// the selection and the clipboard come next.
	typedNone typedResult = iota
	// typedDone: converted where it stands.
	typedDone
	// typedAbandoned: the user typed on before anything was touched. Nothing else is tried;
	// the keys on their way belong to the text as it is.
	typedAbandoned
)

// transliterateTyped converts the text typed since the buffer last started, where it stands,
// through the same verified replacement as a layout fix.
func (e *Engine) transliterateTyped(direction *translit.Direction, seq uint64) typedResult {
	keyed := e.typing.ScreenText(e.platform.Table)
	if !core.AnyChar(keyed, core.IsLetterChar) {
		return typedNone
	}
	field := e.platform.FocusedField(e.window)
	var caret core.Range16
	caretKnown := false
	if field != nil {
		if caret, caretKnown = field.SelectedRange(); caretKnown && caret.Length > 0 {
			// A selection on screen is what the user is looking at.
			return typedNone
		}
	}
	if seq != 0 && e.platform.KeySeq() != seq {
		e.log.Printf("transliteration skipped: typing continued")
		return typedAbandoned
	}
	if !e.platform.CanInject(e.window) {
		return typedNone
	}

	// The direction comes from what the user typed, never from the rest of the line.
	resolved := resolveDirection(direction, keyed)
	// What is on screen, where the field will say: the application may have autocorrected a
	// Latin word, and the letters on screen are the ones to convert. Otherwise the keystrokes
	// are all there is.
	typed := keyed
	if caretKnown {
		if tail, ok := screenTail(field, caret, wordCount(keyed), core.UTF16Len(keyed)*2+32); ok {
			typed = tail
		}
	}
	converted := e.transliterator().Convert(typed, e.scheme(), resolved)
	if core.CanonEqual(converted, typed) {
		return typedNone
	}

	// Only the part that changes is retyped; the caret stays at the end.
	shared := core.CommonPrefixCount(typed, converted)
	plan := core.Replacement{
		DeletedText: core.DropFirstChars(typed, shared),
		InsertText:  core.DropFirstChars(converted, shared),
	}
	result := e.replace(plan, seq)
	if result.Result != core.ReplaceApplied {
		if result.Abandoned {
			e.log.Printf("transliteration skipped: typing continued")
			return typedAbandoned
		}
		// Whatever happened, the buffer can no longer be trusted to describe the screen.
		e.Reset("transliterationFailed")
		return typedNone
	}

	e.Reset("transliterated")
	e.switchLayoutForScript(scriptFor(resolved))
	if typeableBlind(plan.DeletedText) {
		e.pushUndo(core.AppliedFix{
			Kind:          core.FixTransliteration,
			Replacement:   core.Replacement{DeletedText: plan.DeletedText, InsertText: result.Inserted},
			App:           e.app,
			Time:          e.now(),
			OriginalWord:  typed,
			CorrectedWord: converted,
			Strategy:      string(result.Strategy),
			Window:        e.window,
			Generation:    e.generation,
		})
	}
	e.log.Printf("transliterated %d→%d typed chars via %s", core.CharCount(plan.DeletedText),
		core.CharCount(result.Inserted), result.Strategy)
	e.publish()
	return typedDone
}

// screenTail is the last `words` words before the caret with whatever follows them, read
// from the field — false where it cannot be read. Stops at a line break: the words typed
// since the buffer started are all on the current line.
func screenTail(field core.TextFieldAccess, caret core.Range16, words, maxLength int) (string, bool) {
	if words <= 0 || caret.Length != 0 || caret.Location <= 0 {
		return "", false
	}
	start := max(0, caret.Location-maxLength)
	before, ok := field.StringIn(core.Range16{Location: start, Length: caret.Location - start})
	if !ok {
		return "", false
	}
	characters := core.Graphemes(before)
	seen, inWord, cut := 0, false, len(characters)
	for index := len(characters) - 1; index >= 0; index-- {
		character := characters[index]
		if core.IsNewlineChar(character) {
			break
		}
		if core.IsWhitespaceChar(character) {
			if inWord {
				inWord = false
				if seen == words {
					break
				}
			}
		} else if !inWord {
			inWord = true
			seen++
		}
		cut = index
	}
	if seen != words {
		return "", false
	}
	tail := strings.Join(characters[cut:], "")
	return tail, tail != ""
}

// wordCount is Swift's `text.split(whereSeparator: \.isWhitespace).count`.
func wordCount(text string) int {
	count, inWord := 0, false
	for _, character := range core.Graphemes(text) {
		if core.IsWhitespaceChar(character) {
			inWord = false
		} else if !inWord {
			inWord = true
			count++
		}
	}
	return count
}

// resolveDirection is the direction asked for, or else the one the text itself suggests.
func resolveDirection(direction *translit.Direction, text string) translit.Direction {
	if direction != nil {
		return *direction
	}
	return translit.DirectionFor(text)
}

// scriptFor is the script a transliteration writes. The layout follows it: after writing
// Cyrillic, the next keystroke should be Cyrillic.
func scriptFor(direction translit.Direction) core.Script {
	if direction == translit.ToCyrillic {
		return core.ScriptCyrillic
	}
	return core.ScriptLatin
}

// typeableBlind says whether an undo may retype this text where the field cannot be read
// back: control characters cannot be typed blind — Enter may send a message, Tab moves the
// focus.
func typeableBlind(text string) bool { return !strings.ContainsAny(text, "\r\n\t") }

// TransliterateClipboard converts the clipboard contents.
func (e *Engine) TransliterateClipboard(direction *translit.Direction) {
	if e.models == nil || e.blockedByPasswordField() {
		return
	}
	text, ok := e.platform.ReadClipboard()
	if !ok || text == "" {
		e.nothing("nothing to transliterate")
		return
	}
	if e.tooLong(text) {
		return
	}
	resolved := resolveDirection(direction, text)
	converted := e.transliterator().Convert(text, e.scheme(), resolved)
	if core.CanonEqual(converted, text) || !e.platform.WriteClipboard(converted) {
		return
	}
	before := text
	e.pushUndo(core.AppliedFix{
		Kind:            core.FixTransliteration,
		ClipboardBefore: &before,
		Replacement:     core.Replacement{DeletedText: text, InsertText: converted},
		App:             e.app,
		Time:            e.now(),
		OriginalWord:    text,
		CorrectedWord:   converted,
		Strategy:        "clipboard",
		Window:          e.window,
		Generation:      e.generation,
	})
	e.publish()
}

// switchLayoutForScript moves to a layout of the given script, preferring the one this app
// was last written in.
func (e *Engine) switchLayoutForScript(script core.Script) {
	if script == core.ScriptCyrillic {
		if remembered := e.cfg.RememberedCyrillic(e.app); remembered != "" && e.platform.Table(remembered) != nil {
			e.platform.SelectLayout(e.window, remembered)
			return
		}
	}
	for _, table := range e.platform.Tables() {
		if table.Lang.Script() == script {
			e.platform.SelectLayout(e.window, table.SourceID)
			return
		}
	}
}

// SetAppMode sets (ok=true) or clears the rule for the current app.
func (e *Engine) SetAppMode(app string, mode config.FixMode, ok bool) {
	e.updateConfig(func(c *config.Config) { c.SetMode(app, mode, ok) })
}
