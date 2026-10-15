package core

// Port of Detect/TypingEngine.swift.

import (
	"slices"
	"strings"
	"time"
)

// TypingEngine is the keystroke-level decision logic: which key ends a word, what gets
// evaluated when, and how the buffer keeps describing the screen once a fix is in.
//
// Free of the hooks, the injector and the UI on purpose. The Windows engine performs the
// side effects this asks for; the regression corpus drives exactly the same object, so what
// is measured is what ships. Not safe for concurrent use.
type TypingEngine struct {
	Buffer   *StrokeBuffer
	Detector *Detector

	// Learner is where learning signals go. Nil in tests that measure detection alone.
	Learner *Learner
	// IsDictionaryWord says whether a word is in the shipped dictionaries. Only words that
	// are not get learned as the user's vocabulary — the rest are known already.
	IsDictionaryWord func(word string, lang Lang) bool
	// Resolve resolves a recorded stroke's layout, to read back what is on screen.
	Resolve Resolver

	// Scheduler drives the idle reset: a real timer that empties the buffer Buffer.IdleReset
	// seconds after the last key, without waiting for the next one. Nil (tests that measure
	// detection alone) means no timer.
	Scheduler Scheduler
	// Dispatch hands work to the goroutine that owns the engine: the timer fires on a
	// goroutine of its own, and the engine is not safe for concurrent use. Nil runs the work
	// right there (single-goroutine tests).
	Dispatch func(f func())
	// OnIdleReset runs, on the engine's goroutine, after the idle timer emptied the buffer.
	OnIdleReset func()
	// OnReset runs whenever the buffer is emptied, whatever the reason, so whoever caches text
	// read from the field (the engine) drops it too.
	OnReset func()

	idleTimer Timer
	idleToken uint64
	lastKeyAt float64

	recentFix *recentFix
	lastWord  *lastWord
	// lastCandidates are the layouts the last keystroke was judged against, for reading a
	// kept word under each.
	lastCandidates []*KeyMapTable
	// lowWater is the lowest buffer length since the last word ended: below a word's end
	// means it was edited.
	lowWater int
	// countedThisSession: words already counted this session, so a word repeated in one
	// sentence counts once.
	countedThisSession map[string]bool
	// session advances whenever the buffer starts over (a reset or an idle pause), so callers
	// can cache what they read about the text in front of it.
	session int
}

// Session identifies the current buffer session: it changes whenever the buffer starts over.
func (e *TypingEngine) Session() int { return e.session }

// DecisionKind is what to do after a word ends.
type DecisionKind int

const (
	DecideNone DecisionKind = iota
	// DecideApply: rewrite it now.
	DecideApply
	// DecideOffer: show it, and let the user accept it.
	DecideOffer
)

// Decision is the engine's verdict on a word end.
type Decision struct {
	Kind    DecisionKind
	Outcome *Outcome
}

// TypingSettings shape a decision, gathered by the caller once per evaluation.
type TypingSettings struct {
	CandidateLayouts []*KeyMapTable
	Resolve          Resolver
	MinWordLength    int
	Exceptions       StringSet
	AutoThreshold    float64
	SuggestThreshold float64
	// MayApply: automatic rewriting allowed here; otherwise even confident fixes are offered.
	MayApply bool
	Learned  LearnedVocabulary
}

// NewTypingSettings fills in the Swift defaults.
func NewTypingSettings(candidates []*KeyMapTable, resolve Resolver) TypingSettings {
	return TypingSettings{
		CandidateLayouts: candidates, Resolve: resolve, MinWordLength: DefaultMinWordLength,
		Exceptions: StringSet{}, AutoThreshold: DefaultAutoThreshold,
		SuggestThreshold: DefaultSuggestThreshold, MayApply: true,
	}
}

// WordEnd says a word just ended, and whether it ended a sentence too.
type WordEnd struct {
	SentenceEnd bool
}

// RejectionWindow is how long after a fix erasing it and retyping the original still reads
// as "no", in seconds.
const RejectionWindow = 20.0

// recentFix is the last automatic fix, watched for the user taking it back.
type recentFix struct {
	originals []string
	// wordEnd is where the corrected words end in the buffer; deleting below this erases
	// into them.
	wordEnd int
	time    float64
	erased  bool
}

// lastWord is the last finished word, waiting to see whether it is kept.
type lastWord struct {
	text string
	lang Lang
	end  int
	// strokes are the keys that produced it, to read it under the other layouts.
	strokes []KeyStroke
}

// VocabularyMinimumLength is the shortest word the app will learn as the user's own.
// Shorter strings are too often a fragment, an abbreviation or a slip to be worth
// remembering.
const VocabularyMinimumLength = 4

// NewTypingEngine uses a default buffer (256 strokes, 5 s idle reset) when buffer is nil.
func NewTypingEngine(detector *Detector, buffer *StrokeBuffer) *TypingEngine {
	if buffer == nil {
		buffer = NewStrokeBuffer(0, 0)
	}
	return &TypingEngine{Buffer: buffer, Detector: detector, lowWater: maxInt, countedThisSession: map[string]bool{}}
}

const maxInt = int(^uint(0) >> 1)

// MARK: - Keystrokes

// Record records a stroke. ok is true when it closed a word and an evaluation should follow.
//
// A key ends a word only if it is not a letter on any candidate layout — see EndsWord. The
// price is that `word,` on ABC is judged at the following space rather than at the comma,
// which nobody can see.
func (e *TypingEngine) Record(stroke KeyStroke, layout *KeyMapTable, time float64, candidates []*KeyMapTable) (WordEnd, bool) {
	e.lastCandidates = candidates
	e.noteActivity()
	if e.Buffer.Append(stroke, time, layout.SourceID) {
		// Idle reset: a new session, and nothing before it is on screen any more.
		e.forgetSession()
	}
	if !EndsWord(stroke, layout, candidates) {
		return WordEnd{}, false
	}
	e.noteWordEnd(time)
	produced, _ := layout.Text(stroke)
	return WordEnd{SentenceEnd: containsSentenceEnd(produced) || e.endsSentence(layout)}, true
}

// containsSentenceEnd compares Characters, as Swift's `".!?…\n".contains(character)` does.
func containsSentenceEnd(text string) bool {
	return AnyChar(text, func(c string) bool {
		return c == "." || c == "!" || c == "?" || c == "…" || c == "\n"
	})
}

// Backspace mirrors a Backspace keypress.
func (e *TypingEngine) Backspace() {
	e.Buffer.RemoveLast()
	e.lowWater = min(e.lowWater, e.Buffer.Count())
	if e.recentFix != nil && e.Buffer.Count() < e.recentFix.wordEnd {
		e.recentFix.erased = true
	}
	if !e.Buffer.IsEmpty() {
		e.noteActivity()
	}
}

// Reset: the buffer no longer describes the screen — click, Enter, app switch, a password
// field, lock, sleep, or five seconds of silence.
func (e *TypingEngine) Reset() {
	e.Buffer.Reset()
	e.forgetSession()
	e.cancelIdle()
	if e.OnReset != nil {
		e.OnReset()
	}
}

// MARK: - Idle timer

// noteActivity counts a key or a Backspace as typing and starts the timer if none is running.
//
// One timer for a whole burst of typing, not one per key: when it fires it measures the
// silence itself and, if keys came in since, sleeps for the remainder. The keystroke path
// therefore does no more than store a number.
func (e *TypingEngine) noteActivity() {
	if e.Scheduler == nil {
		return
	}
	e.lastKeyAt = e.Scheduler.Now()
	if e.idleTimer == nil {
		e.scheduleIdle(e.Buffer.IdleReset)
	}
}

func (e *TypingEngine) scheduleIdle(seconds float64) {
	e.idleToken++
	token, dispatch := e.idleToken, e.Dispatch
	e.idleTimer = e.Scheduler.AfterFunc(time.Duration(seconds*float64(time.Second)), func() {
		// On the timer's goroutine: touch nothing, hand over.
		run := func() { e.idleCheck(token) }
		if dispatch != nil {
			dispatch(run)
		} else {
			run()
		}
	})
}

// idleCheck runs on the engine's goroutine when the timer has fired. A timer that was called
// off meanwhile (a reset) has an outdated token and does nothing.
func (e *TypingEngine) idleCheck(token uint64) {
	if token != e.idleToken || e.idleTimer == nil || e.Scheduler == nil {
		return
	}
	e.idleTimer = nil
	if remaining := e.Buffer.IdleReset - (e.Scheduler.Now() - e.lastKeyAt); remaining > 0.001 {
		e.scheduleIdle(remaining)
		return
	}
	e.Reset()
	if e.OnIdleReset != nil {
		e.OnIdleReset()
	}
}

func (e *TypingEngine) cancelIdle() {
	if e.idleTimer != nil {
		e.idleTimer.Stop()
		e.idleTimer = nil
	}
	e.idleToken++ // a callback already on its way finds the token outdated
}

func (e *TypingEngine) forgetSession() {
	e.recentFix = nil
	e.lastWord = nil
	e.lowWater = maxInt
	clear(e.countedThisSession)
	e.session++
}

// MARK: - Learning from what the user does next

func (e *TypingEngine) noteWordEnd(time float64) {
	current, ok := e.lastTypedWord()
	if e.Learner == nil || !e.Learner.Enabled() || !ok {
		e.lastWord = nil
		return
	}

	// A fix erased and the original typed again: the fix was wrong.
	if fix := e.recentFix; fix != nil {
		if time-fix.time > RejectionWindow {
			e.recentFix = nil
		} else if fix.erased {
			for _, original := range fix.originals {
				if CanonEqual(Lower(original), Lower(current.text)) {
					e.Learner.Rejected(original)
					break
				}
			}
			e.recentFix = nil
		}
	}

	// The word before this one survived: kept. Only words nothing else can explain are worth
	// remembering — the user's names, slang and jargon.
	if previous := e.lastWord; previous != nil && e.lowWater >= previous.end &&
		!e.countedThisSession[Canon(Lower(previous.text))] &&
		e.mayLearnAsVocabulary(previous) {
		e.Learner.Kept(previous.text, previous.lang)
		e.countedThisSession[Canon(Lower(previous.text))] = true
	}

	e.lastWord = &current
	e.lowWater = e.Buffer.Count()
}

// mayLearnAsVocabulary says whether a word the user kept may be learned as their own.
//
// Only when no layout makes a dictionary word of the same keys. `yfgbcfk` is «написал» on the
// Russian layout: a word like that, left standing, is a mistake nobody caught — and learning
// it as the user's English would make the mistake permanent, since a known word is never
// corrected. The same check covers Russian typed on the Kazakh layout: the Russian layout
// renders those keys as the same letters, so `кароче` is looked up in Russian as well as
// Kazakh.
func (e *TypingEngine) mayLearnAsVocabulary(word *lastWord) bool {
	if CharCount(word.text) < VocabularyMinimumLength || e.IsDictionaryWord == nil {
		return false
	}
	if e.IsDictionaryWord(word.text, word.lang) {
		return false
	}
	for _, layout := range e.lastCandidates {
		if layout.Lang == LangNone {
			continue
		}
		if text := layout.Render(word.strokes); text != "" && e.IsDictionaryWord(text, layout.Lang) {
			return false
		}
	}
	return true
}

// lastTypedWord is the word just before the separator that ended it, as its own layout
// shows it.
func (e *TypingEngine) lastTypedWord() (lastWord, bool) {
	if e.Resolve == nil {
		return lastWord{}, false
	}
	recorded := e.Buffer.Recorded()
	index := len(recorded) - 1
	// Skip the separator run at the end.
	for index >= 0 && !e.isWordStroke(recorded[index]) {
		index--
	}
	end := index + 1
	var parts []string
	var strokes []KeyStroke
	// Swift's `Lang?`: the first language seen going backwards, then every other stroke
	// must agree with it (a layout without a language leaves it unset).
	lang := LangNone
	for index >= 0 && e.isWordStroke(recorded[index]) {
		table := e.Resolve(recorded[index].LayoutID)
		text := ""
		tableLang := LangNone
		if table != nil {
			text, _ = table.Text(recorded[index].Stroke)
			tableLang = table.Lang
		}
		parts = append(parts, text)
		strokes = append(strokes, recorded[index].Stroke)
		if lang == LangNone {
			lang = tableLang
		} else if lang != tableLang {
			return lastWord{}, false
		}
		index--
	}
	// Collected back to front.
	var text strings.Builder
	for i := len(parts) - 1; i >= 0; i-- {
		text.WriteString(parts[i])
	}
	if text.Len() == 0 || lang == LangNone {
		return lastWord{}, false
	}
	slices.Reverse(strokes)
	return lastWord{text: text.String(), lang: lang, end: end, strokes: strokes}, true
}

func (e *TypingEngine) isWordStroke(entry RecordedStroke) bool {
	table := e.Resolve(entry.LayoutID)
	if table == nil {
		return false
	}
	text, _ := table.Text(entry.Stroke)
	return IsWordText(text)
}

// endsSentence: `word.` then space — the period was typed before the space that closed the
// word. (Rendered on the layout of the closing stroke, as the Swift engine does.)
func (e *TypingEngine) endsSentence(layout *KeyMapTable) bool {
	recorded := e.Buffer.Recorded()
	if len(recorded) < 2 {
		return false
	}
	previous := recorded[len(recorded)-2]
	text, _ := layout.Text(previous.Stroke)
	return containsSentenceEnd(text)
}

// MARK: - Deciding

// Decide looks at the last word first; at a sentence end, a wider look for a run that
// slipped past, which is only ever offered — silently rewriting several words at once is
// far too surprising.
func (e *TypingEngine) Decide(end WordEnd, settings TypingSettings, precedingText string) Decision {
	if e.Buffer.IsEmpty() {
		return Decision{}
	}
	context := e.MakeContext(settings, precedingText)
	action := e.Detector.Evaluate(context, ScopeLastWord)
	switch action.Kind {
	case ActionFix:
		if settings.MayApply {
			return Decision{Kind: DecideApply, Outcome: action.Outcome}
		}
		return Decision{Kind: DecideOffer, Outcome: action.Outcome}
	case ActionSuggest:
		return Decision{Kind: DecideOffer, Outcome: action.Outcome}
	}
	if !end.SentenceEnd {
		return Decision{}
	}
	if tail := e.Detector.Evaluate(context, ScopeTail); tail.Outcome != nil {
		return Decision{Kind: DecideOffer, Outcome: tail.Outcome}
	}
	return Decision{}
}

// ManualFix is the fix-last-word hotkey: the best reading of the last word, whatever the
// thresholds say.
func (e *TypingEngine) ManualFix(settings TypingSettings) *Outcome {
	if e.Buffer.IsEmpty() {
		return nil
	}
	context := e.MakeContext(settings, "")
	context.Manual = true
	context.MinWordLength = 1
	return e.Detector.Evaluate(context, ScopeLastWord).Outcome
}

// MakeContext builds the detector context for the current buffer.
func (e *TypingEngine) MakeContext(settings TypingSettings, precedingText string) Context {
	exceptions := settings.Exceptions
	if exceptions == nil {
		exceptions = StringSet{}
	}
	return Context{
		Recorded:         e.Buffer.Snapshot(),
		CandidateLayouts: settings.CandidateLayouts,
		Resolve:          settings.Resolve,
		MinWordLength:    settings.MinWordLength,
		Exceptions:       exceptions,
		AutoThreshold:    settings.AutoThreshold,
		SuggestThreshold: settings.SuggestThreshold,
		MaxWords:         DefaultMaxWords,
		PrecedingText:    precedingText,
		Learned:          settings.Learned,
	}
}

// MARK: - After a fix

// DidApply keeps the buffer describing the screen, so the next word is judged against
// corrected text rather than against what was there before.
//
// Automatic fixes are watched for a while afterwards: erasing one and typing the original
// again is the user saying no.
func (e *TypingEngine) DidApply(outcome *Outcome, automatic bool, time float64) {
	start, ok := minLower(outcome.Corrections)
	if !ok || start > e.Buffer.Count() {
		return
	}
	e.Buffer.ReplaceTail(start, RewrittenTail(e.Buffer.Recorded(), outcome.Corrections, start))
	// The corrected word is a dictionary word; nothing to learn from keeping it.
	e.lastWord = nil
	e.recentFix = nil
	if automatic {
		originals := make([]string, len(outcome.Changes))
		for i, change := range outcome.Changes {
			originals[i] = change.Typed
		}
		wordEnd := e.Buffer.Count()
		if len(outcome.WordRanges) > 0 {
			wordEnd = outcome.WordRanges[0][1]
			for _, r := range outcome.WordRanges[1:] {
				wordEnd = max(wordEnd, r[1])
			}
		}
		e.recentFix = &recentFix{originals: originals, wordEnd: wordEnd, time: time}
	}
}

// ScreenText is the text the buffer says is on screen, for verifying against the text field.
func (e *TypingEngine) ScreenText(resolve Resolver) string {
	var text strings.Builder
	for _, entry := range e.Buffer.Recorded() {
		if table := resolve(entry.LayoutID); table != nil {
			produced, _ := table.Text(entry.Stroke)
			text.WriteString(produced)
		}
	}
	return text.String()
}
