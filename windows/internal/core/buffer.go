package core

// Port of the StrokeBuffer half of Detect/StrokeBuffer.swift.

// StrokeBuffer is the recent keystrokes, in memory only.
//
// This is the most privacy-sensitive object in the app, so it is deliberately small, never
// written to disk, and thrown away at the first sign that it no longer describes what is on
// screen. Not safe for concurrent use: the engine owns it on one goroutine.
type StrokeBuffer struct {
	recorded []RecordedStroke
	// Capacity: long enough to hold a sentence or two, which is the span the
	// mixed-language segmenter reasons over; short enough that it is never a transcript.
	Capacity int
	// IdleReset is the idle time in seconds after which old text no longer counts.
	IdleReset float64
}

const (
	// MaxIdleReset is the longest idle period the buffer honours, in seconds. The app promises
	// that typed text does not outlive five seconds of silence, so a hand-edited setting
	// cannot stretch it.
	MaxIdleReset = 5.0
	// MinIdleReset is the shortest: below it the buffer would be wiped between two keys of one
	// word.
	MinIdleReset = 1.0
)

// ClampIdleReset pulls an idle period into [MinIdleReset, MaxIdleReset]; zero and negative
// values (unset) mean the default, five seconds.
func ClampIdleReset(seconds float64) float64 {
	if seconds <= 0 {
		return MaxIdleReset
	}
	return min(max(seconds, MinIdleReset), MaxIdleReset)
}

// NewStrokeBuffer uses the Swift defaults when given zero values (256 strokes, 5 s). The idle
// period is clamped to [MinIdleReset, MaxIdleReset].
func NewStrokeBuffer(capacity int, idleReset float64) *StrokeBuffer {
	if capacity <= 0 {
		capacity = 256
	}
	return &StrokeBuffer{recorded: make([]RecordedStroke, 0, capacity), Capacity: capacity, IdleReset: ClampIdleReset(idleReset)}
}

// SetIdleReset changes the idle period (clamped).
func (b *StrokeBuffer) SetIdleReset(seconds float64) { b.IdleReset = ClampIdleReset(seconds) }

func (b *StrokeBuffer) IsEmpty() bool { return len(b.recorded) == 0 }
func (b *StrokeBuffer) Count() int    { return len(b.recorded) }

// LastActivity is the time of the newest stroke, or 0.
func (b *StrokeBuffer) LastActivity() float64 {
	if len(b.recorded) == 0 {
		return 0
	}
	return b.recorded[len(b.recorded)-1].Time
}

// Recorded returns the buffer contents. The slice is owned by the buffer: read it, do not
// keep it across a mutation.
func (b *StrokeBuffer) Recorded() []RecordedStroke { return b.recorded }

// Snapshot returns a copy of the contents.
func (b *StrokeBuffer) Snapshot() []RecordedStroke {
	return append([]RecordedStroke(nil), b.recorded...)
}

// Append adds a stroke, first discarding the buffer if the user has been idle long enough
// that the old text is no longer in front of them. Returns true when an idle reset
// happened. This check is only the second line of defence: TypingEngine also runs a real
// timer that empties the buffer after IdleReset seconds, without waiting for the next key.
func (b *StrokeBuffer) Append(stroke KeyStroke, time float64, layoutID string) bool {
	didReset := false
	if n := len(b.recorded); n > 0 && time-b.recorded[n-1].Time > b.IdleReset {
		b.recorded = b.recorded[:0]
		didReset = true
	}
	b.recorded = append(b.recorded, RecordedStroke{Stroke: stroke, Time: time, LayoutID: layoutID})
	if excess := len(b.recorded) - b.Capacity; excess > 0 {
		b.recorded = append(b.recorded[:0], b.recorded[excess:]...)
	}
	return didReset
}

// RemoveLast mirrors a Backspace keypress so the buffer keeps tracking the visible text.
func (b *StrokeBuffer) RemoveLast() {
	if len(b.recorded) > 0 {
		b.recorded = b.recorded[:len(b.recorded)-1]
	}
}

// Reset forgets everything.
func (b *StrokeBuffer) Reset() {
	// Clear the old entries so no keystroke outlives the reset in the backing array.
	for i := range b.recorded {
		b.recorded[i] = RecordedStroke{}
	}
	b.recorded = b.recorded[:0]
}

// ReplaceTail drops the trailing strokes covered by a replacement just applied and
// substitutes the strokes the corrected text now stands for, so the buffer keeps matching
// the screen without a round trip through the hook.
func (b *StrokeBuffer) ReplaceTail(index int, strokes []RecordedStroke) {
	if index > len(b.recorded) || index < 0 {
		return
	}
	b.recorded = append(b.recorded[:index], strokes...)
}
