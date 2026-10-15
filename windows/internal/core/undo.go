package core

// Port of Apply/UndoStack.swift.

import "time"

// FixKind says what kind of change a fix was, because undoing them teaches different
// things: undoing a layout fix means "never correct this word", undoing a transliteration
// means "never convert this word".
type FixKind string

const (
	FixLayout          FixKind = "layoutFix"
	FixTransliteration FixKind = "transliteration"
)

// AppliedFix is a correction that was applied, with everything needed to take it back.
type AppliedFix struct {
	Kind FixKind
	// ClipboardBefore is the clipboard contents before the change, when the change was
	// made to the clipboard rather than to a text field.
	ClipboardBefore *string
	Replacement     Replacement
	// App identifies the application the fix was made in (bundle ID on macOS, lowercased
	// executable name on Windows).
	App  string
	Time time.Time
	// OriginalWord is the text as it looked on screen before the fix. Feeds the "never
	// correct this word" action and learning from undo.
	OriginalWord  string
	CorrectedWord string
	// PreviousLayoutID is the layout active before we switched, so undo restores that too.
	PreviousLayoutID string
	Strategy         string
	// TypedWords is each changed word as it was typed. A fix can span several words
	// (`z nt,z` → `я тебя`), and undoing it rejects each of them, not the span as one string.
	TypedWords []string
	// Window and Generation let the Windows engine check that nothing happened since the
	// fix before it types the inverse where the text cannot be read back to verify it.
	Window     uintptr
	Generation uint64
}

// DefaultUndoLifetime is how long a correction can still be taken back.
const DefaultUndoLifetime = 2 * time.Minute

// UndoStack holds recent corrections, newest last.
//
// Bounded on purpose: this is a safety net for "that was wrong, put it back", not an edit
// history, and it holds text the user typed. So it is also short-lived: an entry is gone
// Lifetime (two minutes) after the correction. The engine runs a timer that calls
// DropExpired, so entries are wiped by the clock rather than by the next call, and every
// accessor refuses an expired entry even if the timer is late. Clear wipes everything at once
// (lock, sleep, quit, another window in front).
type UndoStack struct {
	Limit    int
	Lifetime time.Duration
	// Now is the clock the lifetime is measured on (tests replace it).
	Now     func() time.Time
	entries []AppliedFix
}

func NewUndoStack(limit int) *UndoStack {
	if limit <= 0 {
		limit = 20
	}
	return &UndoStack{Limit: limit, Lifetime: DefaultUndoLifetime, Now: time.Now}
}

func (u *UndoStack) now() time.Time {
	if u.Now != nil {
		return u.Now()
	}
	return time.Now()
}

// expired: a fix made exactly Lifetime ago is gone, so "two minutes" is a ceiling.
func (u *UndoStack) expired(fix AppliedFix, now time.Time) bool {
	return u.Lifetime > 0 && !now.Before(fix.Time.Add(u.Lifetime))
}

// DropExpired removes every entry past its lifetime and returns how many went.
func (u *UndoStack) DropExpired() int {
	now := u.now()
	kept := u.entries[:0]
	dropped := 0
	for _, fix := range u.entries {
		if u.expired(fix, now) {
			dropped++
			continue
		}
		kept = append(kept, fix)
	}
	// Do not leave the dropped words behind in the backing array.
	for i := len(kept); i < len(u.entries); i++ {
		u.entries[i] = AppliedFix{}
	}
	u.entries = kept
	return dropped
}

// NextExpiry is when the oldest entry expires, if there is one.
func (u *UndoStack) NextExpiry() (time.Time, bool) {
	u.DropExpired()
	if len(u.entries) == 0 || u.Lifetime <= 0 {
		return time.Time{}, false
	}
	return u.entries[0].Time.Add(u.Lifetime), true
}

// Last returns the newest fix that has not expired.
func (u *UndoStack) Last() (AppliedFix, bool) {
	u.DropExpired()
	if len(u.entries) == 0 {
		return AppliedFix{}, false
	}
	return u.entries[len(u.entries)-1], true
}

func (u *UndoStack) IsEmpty() bool { return u.Count() == 0 }
func (u *UndoStack) Count() int {
	u.DropExpired()
	return len(u.entries)
}

func (u *UndoStack) Push(fix AppliedFix) {
	u.DropExpired()
	u.entries = append(u.entries, fix)
	if excess := len(u.entries) - u.Limit; excess > 0 {
		u.entries = append(u.entries[:0], u.entries[excess:]...)
	}
}

func (u *UndoStack) PopLast() (AppliedFix, bool) {
	fix, ok := u.Last()
	if ok {
		u.entries[len(u.entries)-1] = AppliedFix{}
		u.entries = u.entries[:len(u.entries)-1]
	}
	return fix, ok
}

// DefaultFreshWindow is how recent an undo must be to count as "you got that wrong".
const DefaultFreshWindow = 8 * time.Second

// LastIsFresh is true when the newest fix is recent enough that undoing it should be read
// as "you got that wrong" and turned into a lasting exception.
func (u *UndoStack) LastIsFresh(window time.Duration, now time.Time) bool {
	last, ok := u.Last()
	if !ok {
		return false
	}
	return now.Sub(last.Time) <= window
}

func (u *UndoStack) Clear() {
	for i := range u.entries {
		u.entries[i] = AppliedFix{}
	}
	u.entries = u.entries[:0]
}
