package core

// Port of Apply/TextReplacer.swift: replaces the text right before the caret, and refuses
// to guess. Platform-neutral; the Windows app supplies the text field (standard Edit and
// RichEdit controls, read and edited through window messages) and the keyboard (SendInput).

import (
	"time"
	"unicode/utf16"
)

// Range16 is a range in UTF-16 code units — NSRange, and what Edit controls count in.
type Range16 struct {
	Location, Length int
}

// TextFieldAccess is the focused text field, as far as the platform lets us see and touch
// it. An interface so the replacement logic can be tested against simulated apps —
// including the ones that misbehave, which is the whole point.
type TextFieldAccess interface {
	// SelectedRange is the selection; a zero-length range is the caret. ok is false when
	// the app will not say.
	SelectedRange() (Range16, bool)
	SetSelectedRange(r Range16) bool
	// StringIn is the text in a range; ok is false when the app will not say.
	StringIn(r Range16) (string, bool)
	SelectedText() (string, bool)
	SetSelectedText(text string) bool
	CanSetSelectedRange() bool
	CanSetSelectedText() bool
}

// KeystrokeSender types on the user's behalf.
type KeystrokeSender interface {
	// SendKeys presses Backspace deleting times, then types inserting. False when the
	// events could not be sent. (Swift has two calls, sendBackspaces and sendText, always
	// made back to back; one call lets Windows send both in one uninterruptible SendInput.)
	SendKeys(deleting int, inserting string) bool
}

// ReplaceStrategy is how an edit was made.
type ReplaceStrategy string

const (
	// StrategyAccessibility: through the text field itself (on Windows, EM_REPLACESEL on a
	// standard Edit or RichEdit control), which keeps the control's own undo.
	StrategyAccessibility ReplaceStrategy = "accessibility"
	// StrategySynthetic: Backspace presses and Unicode key events.
	StrategySynthetic ReplaceStrategy = "synthetic"
)

// ReplaceResult is what happened to one replacement.
type ReplaceResult int

const (
	// ReplaceApplied: done. Inserted is what went in — the planned text, with the capital
	// the host app gave the word carried over.
	ReplaceApplied ReplaceResult = iota
	// ReplaceMismatch: the screen does not show what we meant to replace. Nothing was
	// touched; the buffer must be thrown away.
	ReplaceMismatch
	// ReplaceFailed: the app refused, or left the text in a state we cannot vouch for.
	// Nothing more is done: another attempt on text we cannot see would be a guess.
	ReplaceFailed
)

// ReplaceOutcome is TextReplacer.Outcome.
type ReplaceOutcome struct {
	Result   ReplaceResult
	Strategy ReplaceStrategy
	Inserted string
	Reason   string
	// Abandoned: the user typed on before anything was touched (see TextReplacer.Current),
	// so the text is exactly as the buffer says. Only with ReplaceFailed.
	Abandoned bool
}

func replaceApplied(strategy ReplaceStrategy, inserted string) ReplaceOutcome {
	return ReplaceOutcome{Result: ReplaceApplied, Strategy: strategy, Inserted: inserted}
}

func replaceFailed(reason string) ReplaceOutcome {
	return ReplaceOutcome{Result: ReplaceFailed, Reason: reason}
}

func replaceAbandoned() ReplaceOutcome {
	return ReplaceOutcome{Result: ReplaceFailed, Reason: "typing continued", Abandoned: true}
}

// TextReplacer replaces the text right before the caret.
//
// The failure this exists to prevent is the correction appearing next to the wrong text
// instead of replacing it. That happens when an edit is made without confirming what it
// acts on. So every step is checked wherever the app lets us check it:
//
//  1. Before anything is touched, the text before the caret must be the text about to be
//     deleted. If the app lets us read it and it is not, nothing is done at all — the host
//     app autocorrected, or the caret moved, and the buffer no longer describes the screen.
//  2. The field route writes only once the selection is confirmed to cover exactly that
//     text, and reads the result back. If the app inserted without deleting, the damage is
//     visible and bounded, and it is repaired with keystrokes.
//  3. Otherwise keystrokes: Backspace presses, then the text.
type TextReplacer struct {
	// Field is nil when there is no readable focused control.
	Field TextFieldAccess
	Keys  KeystrokeSender
	// Wait sleeps the calling goroutine. Injected so tests run instantly.
	Wait func(time.Duration)
	// PreferAccessibility: try the field route first. False where it already failed.
	PreferAccessibility bool
	// Verify: read the text to confirm it before deleting.
	Verify bool
	// AccessibilityUnreliable is set when the field route misbehaved in a way that says to
	// stop using it here.
	AccessibilityUnreliable bool
	// Current, when set, says whether the edit may still be made: false once the user has
	// typed another key. (Windows-only: on macOS the event tap waits for the edit; on Windows
	// the user's keystrokes keep arriving, and an application processes our window messages
	// before its queued input — a key pressed meanwhile would land between two of them and
	// replace the selection or end up after the caret.) Checked before anything is touched
	// and before the write; the edit is abandoned with the text as it was.
	Current func() bool
}

func (r *TextReplacer) current() bool { return r.Current == nil || r.Current() }

// How long to wait for the app to catch up: with the keystroke that triggered the fix — it
// may not have inserted the separator yet when we first look — and with field changes some
// apps apply asynchronously. Short on purpose: 4 × 8 ms at most per wait.
const (
	ReplacerSettleAttempts = 5
	ReplacerSettleInterval = 8 * time.Millisecond
)

// UTF16Len is a string's length in UTF-16 code units.
func UTF16Len(text string) int {
	n := 0
	for _, r := range text {
		n += utf16.RuneLen(r)
	}
	return n
}

// Apply makes the edit.
func (r *TextReplacer) Apply(replacement Replacement) ReplaceOutcome {
	if replacement.IsEmpty() {
		return replaceFailed("empty replacement")
	}

	// 1. What is actually in front of the caret.
	var caret Range16
	hasCaret := false
	confirmed := false
	if r.Verify && r.Field != nil {
		check := r.confirmText(replacement.DeletedText)
		switch check.kind {
		case confirmMatches:
			caret, hasCaret, confirmed = check.at, true, true
			replacement = CarryingCapital(check.onScreen, replacement)
		case confirmDiffers:
			return ReplaceOutcome{Result: ReplaceMismatch}
		case confirmSelection:
			// Backspace presses would remove the selection instead of our characters.
			return replaceFailed("text is selected")
		case confirmUnreadable:
			caret, hasCaret = check.at, check.hasAt
		}
	}

	// 2. The field route: only on confirmed text, only when the app lets us set both.
	if confirmed && r.PreferAccessibility && r.Field != nil && hasCaret &&
		r.Field.CanSetSelectedRange() && r.Field.CanSetSelectedText() {
		result, reason := r.replaceViaField(replacement, caret)
		switch result {
		case fieldDone:
			return replaceApplied(StrategyAccessibility, replacement.InsertText)
		case fieldAbandoned:
			return replaceAbandoned()
		case fieldInterrupted:
			return replaceFailed(reason)
		case fieldUntouched:
			// The app would not take the edit and the text is as it was, so keystrokes can
			// still do the job.
			r.AccessibilityUnreliable = true
		case fieldRepaired:
			r.AccessibilityUnreliable = true
			return replaceApplied(StrategySynthetic, replacement.InsertText)
		default:
			r.AccessibilityUnreliable = true
			return replaceFailed(reason)
		}
	}

	// 3. Keystrokes.
	if !r.current() {
		return replaceAbandoned()
	}
	if r.sendKeys(replacement.DeleteCount(), replacement.InsertText) {
		return replaceApplied(StrategySynthetic, replacement.InsertText)
	}
	return replaceFailed("could not post key events")
}

// MARK: - Confirming the text

type confirmKind int

const (
	confirmMatches confirmKind = iota
	confirmDiffers
	confirmSelection
	// confirmUnreadable: the app will not say. at is the caret if it would at least report
	// that.
	confirmUnreadable
)

type confirmation struct {
	kind     confirmKind
	at       Range16
	hasAt    bool
	onScreen string
}

func (r *TextReplacer) wait() {
	if r.Wait != nil {
		r.Wait(ReplacerSettleInterval)
	}
}

func (r *TextReplacer) confirmText(expected string) confirmation {
	length := UTF16Len(expected)
	sawText, sawSelection := false, false
	var caret Range16
	hasCaret := false
	for attempt := 0; attempt < ReplacerSettleAttempts; attempt++ {
		if attempt > 0 {
			r.wait()
		}
		selection, ok := r.Field.SelectedRange()
		if !ok {
			return confirmation{kind: confirmUnreadable, at: caret, hasAt: hasCaret}
		}
		caret, hasCaret = selection, true
		// A selection can be the app still catching up; give it a moment.
		sawSelection = selection.Length > 0
		if sawSelection {
			continue
		}
		if selection.Location < length {
			sawText = true
			continue
		}
		before, ok := r.Field.StringIn(Range16{Location: selection.Location - length, Length: length})
		if !ok {
			return confirmation{kind: confirmUnreadable, at: selection, hasAt: true}
		}
		sawText = true
		// The host app may have capitalised the word (sentence start); the letters are still
		// ours.
		if CanonEqual(before, expected) || CanonEqual(Lower(before), Lower(expected)) {
			return confirmation{kind: confirmMatches, at: selection, hasAt: true, onScreen: before}
		}
	}
	if sawSelection {
		return confirmation{kind: confirmSelection}
	}
	if sawText {
		return confirmation{kind: confirmDiffers}
	}
	return confirmation{kind: confirmUnreadable, at: caret, hasAt: hasCaret}
}

// CarryingCapital: when the app capitalised the first letter of what we are replacing, the
// replacement gets a capital too — `Ghbdtn` becomes `Привет`, not `привет`.
func CarryingCapital(onScreen string, replacement Replacement) Replacement {
	shown, planned, first := FirstChar(onScreen), FirstChar(replacement.DeletedText), FirstChar(replacement.InsertText)
	if CanonEqual(onScreen, replacement.DeletedText) || shown == "" || planned == "" ||
		!IsUppercaseChar(shown) || IsUppercaseChar(planned) || first == "" || !IsLetterChar(first) {
		return Replacement{DeletedText: onScreen, InsertText: replacement.InsertText}
	}
	return Replacement{DeletedText: onScreen, InsertText: Upper(first) + DropFirstChars(replacement.InsertText, 1)}
}

// MARK: - The field route

type fieldResult int

const (
	fieldDone fieldResult = iota
	// fieldUntouched: nothing changed; safe to fall back.
	fieldUntouched
	// fieldRepaired: the app inserted without deleting; fixed with keystrokes.
	fieldRepaired
	fieldUnknown
	// fieldAbandoned: the user typed on; nothing changed, and nothing more may be done.
	fieldAbandoned
	// fieldInterrupted: the user typed on while our selection was up and it could not be
	// taken down in time; the text is unknown, through no fault of the control.
	fieldInterrupted
)

func (r *TextReplacer) replaceViaField(replacement Replacement, caret Range16) (fieldResult, string) {
	field := r.Field
	length := replacement.DeleteUTF16Count()
	start := caret.Location - length
	target := Range16{Location: start, Length: length}
	restore := Range16{Location: caret.Location}
	caretIsRestored := func() (bool, bool) {
		now, ok := field.SelectedRange()
		if !ok {
			return false, false
		}
		return now == restore, true
	}

	// Select exactly the characters to replace, and see that the app really did.
	if !r.current() {
		return fieldAbandoned, ""
	}
	field.SetSelectedRange(target)
	if !r.settle(func() (bool, bool) { return selectionIs(field, target, replacement.DeletedText) }) {
		// Put the caret back, and make sure it is back before anything else happens: an app
		// applying the selection late would otherwise turn the first Backspace into "delete
		// the whole selection".
		field.SetSelectedRange(restore)
		if r.settle(caretIsRestored) {
			return fieldUntouched, ""
		}
		return fieldUnknown, "selection did not settle"
	}

	// A key pressed while the selection was up would replace it: take the selection down
	// and leave the text as it was.
	if !r.current() {
		field.SetSelectedRange(restore)
		if r.settle(caretIsRestored) {
			return fieldAbandoned, ""
		}
		// The key may already have landed on the selection: the text is not what the buffer
		// says any more, though the control did nothing wrong.
		return fieldInterrupted, "typing continued over the selection"
	}

	// The return value is not trusted either way: some apps report an error for a write
	// they performed, others report success for one they ignored. The text decides.
	field.SetSelectedText(replacement.InsertText)

	insertLength := UTF16Len(replacement.InsertText)
	written := Range16{Location: start, Length: insertLength}
	expectedCaret := Range16{Location: start + insertLength}
	if r.settle(func() (bool, bool) {
		now, ok := field.SelectedRange()
		if !ok {
			return false, false
		}
		if now != expectedCaret {
			return false, true
		}
		text, ok := field.StringIn(written)
		return ok && CanonEqual(text, replacement.InsertText), true
	}) {
		return fieldDone, ""
	}

	// Not what we asked for. Work out from the screen what happened.
	if now, ok := field.SelectedRange(); ok && now == target {
		if text, ok := field.StringIn(target); ok && CanonEqual(text, replacement.DeletedText) {
			// The write was ignored and our selection is still up. Take it down again, and
			// the text is exactly as it was.
			field.SetSelectedRange(restore)
			if r.settle(caretIsRestored) {
				return fieldUntouched, ""
			}
			return fieldUnknown, "selection did not clear"
		}
	}
	now, ok := field.SelectedRange()
	if !ok || now.Length != 0 || now.Location < start {
		return fieldUnknown, "result unreadable"
	}
	region, ok := field.StringIn(Range16{Location: start, Length: now.Location - start})
	if !ok {
		return fieldUnknown, "result unreadable"
	}
	if CanonEqual(region, replacement.DeletedText) {
		// The write was ignored outright: the text is untouched and the caret where it
		// started, so keystrokes can do the whole job.
		if now.Location == caret.Location {
			return fieldUntouched, ""
		}
		return fieldUnknown, "caret moved"
	}
	// Inserted without deleting, or any other arrangement of our own two strings.
	// Everything from start to the caret is ours: remove it and type the replacement.
	if CanonEqual(region, replacement.DeletedText+replacement.InsertText) ||
		CanonEqual(region, replacement.InsertText+replacement.DeletedText) {
		if r.sendKeys(CharCount(region), replacement.InsertText) {
			return fieldRepaired, ""
		}
		return fieldUnknown, "repair failed"
	}
	return fieldUnknown, "unexpected result"
}

// selectionIs: ok false when the app will not answer, which ends the wait.
func selectionIs(field TextFieldAccess, r Range16, text string) (bool, bool) {
	selection, ok := field.SelectedRange()
	if !ok {
		return false, false
	}
	if selection != r {
		return false, true
	}
	// Where the app reports the selected text it must agree too; otherwise the range
	// read-back plus the text confirmed a moment ago will do.
	if selected, ok := field.SelectedText(); ok {
		return CanonEqual(selected, text), true
	}
	if inRange, ok := field.StringIn(r); ok {
		return CanonEqual(inRange, text), true
	}
	return true, true
}

// settle polls condition for as long as an app may take to apply a change asynchronously. A
// condition that cannot be answered (answered false) ends the wait.
func (r *TextReplacer) settle(condition func() (value, answered bool)) bool {
	for attempt := 0; attempt < ReplacerSettleAttempts; attempt++ {
		if attempt > 0 {
			r.wait()
		}
		value, answered := condition()
		if !answered {
			return false
		}
		if value {
			return true
		}
	}
	return false
}

// MARK: - Keystrokes

func (r *TextReplacer) sendKeys(deleting int, inserting string) bool {
	if r.Keys == nil {
		return false
	}
	if deleting <= 0 && inserting == "" {
		return true
	}
	return r.Keys.SendKeys(deleting, inserting)
}

// Chunks splits text into pieces of at most limit UTF-16 units without breaking a
// character. (macOS carries at most 20 units per Unicode key event; the Windows sender
// builds its SendInput batch chunk by chunk — see internal/winapp.)
func Chunks(text string, limit int) []string {
	var result []string
	current := ""
	units := 0
	it := graphemeIter{s: text}
	for character := it.next(); character != ""; character = it.next() {
		size := UTF16Len(character)
		if units+size > limit && current != "" {
			result = append(result, current)
			current = ""
			units = 0
		}
		current += character
		units += size
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

// ChunkLimit is the chunk size the Swift injector uses.
const ChunkLimit = 20
