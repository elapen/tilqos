package core

// Port of Apply/Replacement.swift.

import (
	"strings"
	"unicode/utf16"
)

// Replacement is an edit to apply at the caret: remove DeletedText from behind it, put
// InsertText there instead.
//
// Holding the removed text rather than just a count pays off: backspaces need a character
// count, and undo needs the original characters back.
type Replacement struct {
	// DeletedText is what is on screen now and is about to go away.
	DeletedText string
	InsertText  string
}

// DeleteCount is the number of Backspace presses required — one per Character.
func (r Replacement) DeleteCount() int { return CharCount(r.DeletedText) }

// DeleteUTF16Count is the deleted length in UTF-16 code units.
func (r Replacement) DeleteUTF16Count() int { return len(utf16.Encode([]rune(r.DeletedText))) }

func (r Replacement) IsEmpty() bool { return r.DeletedText == "" && r.InsertText == "" }

// Inverse is the edit that puts things back the way they were.
func (r Replacement) Inverse() Replacement {
	return Replacement{DeletedText: r.InsertText, InsertText: r.DeletedText}
}

// WordCorrection is a word the detector wants re-rendered under a different layout.
type WordCorrection struct {
	// Lo, Hi index the recorded strokes (half-open).
	Lo, Hi int
	// TargetLayoutID is the layout the word should be rendered under.
	TargetLayoutID string
}

func (c WordCorrection) Contains(index int) bool { return index >= c.Lo && index < c.Hi }

func correctionAt(corrections []WordCorrection, index int) (WordCorrection, bool) {
	for _, correction := range corrections {
		if correction.Contains(index) {
			return correction, true
		}
	}
	return WordCorrection{}, false
}

func minLower(corrections []WordCorrection) (int, bool) {
	if len(corrections) == 0 {
		return 0, false
	}
	low := corrections[0].Lo
	for _, correction := range corrections[1:] {
		if correction.Lo < low {
			low = correction.Lo
		}
	}
	return low, true
}

func maxUpper(corrections []WordCorrection) (int, bool) {
	if len(corrections) == 0 {
		return 0, false
	}
	high := corrections[0].Hi
	for _, correction := range corrections[1:] {
		if correction.Hi > high {
			high = correction.Hi
		}
	}
	return high, true
}

// PlanReplacement turns a set of word corrections into a single minimal edit
// (Swift: ReplacementPlanner.plan). Returns false when there is nothing to do.
//
// Two rules matter here. Separators are never re-rendered — a period the user typed is a
// period they meant, even though the same key yields `ю` on the Russian layout. And the
// common prefix is preserved, so correcting `с2леметс3з` to `сәлеметсіз` deletes nine
// characters rather than ten.
func PlanReplacement(recorded []RecordedStroke, corrections []WordCorrection, resolve Resolver) (Replacement, bool) {
	start, ok := minLower(corrections)
	if !ok || start >= len(recorded) {
		return Replacement{}, false
	}

	var displayed, corrected strings.Builder
	for index := start; index < len(recorded); index++ {
		entry := recorded[index]
		source := resolve(entry.LayoutID)
		if source == nil {
			return Replacement{}, false
		}
		asTyped, _ := source.Text(entry.Stroke)
		displayed.WriteString(asTyped)

		// Only strokes inside a correction change layout; everything between them —
		// spaces, commas, digits the user meant — stays exactly as typed.
		if correction, found := correctionAt(corrections, index); found {
			if target := resolve(correction.TargetLayoutID); target != nil {
				if text, ok := target.Text(entry.Stroke); ok {
					corrected.WriteString(text)
				} else {
					corrected.WriteString(asTyped)
				}
				continue
			}
		}
		corrected.WriteString(asTyped)
	}

	before, after := displayed.String(), corrected.String()
	if CanonEqual(before, after) {
		return Replacement{}, false
	}
	shared := CommonPrefixCount(before, after)
	replacement := Replacement{
		DeletedText: DropFirstChars(before, shared),
		InsertText:  DropFirstChars(after, shared),
	}
	if replacement.IsEmpty() {
		return Replacement{}, false
	}
	return replacement, true
}

// CommonPrefixCount is `a.commonPrefix(with: b).count`: shared leading Characters.
func CommonPrefixCount(a, b string) int {
	ia, ib := graphemeIter{s: a}, graphemeIter{s: b}
	count := 0
	for {
		ca, cb := ia.next(), ib.next()
		if ca == "" || cb == "" || !CanonEqual(ca, cb) {
			return count
		}
		count++
	}
}

// RewrittenTail returns the strokes that replace the tail once a correction is applied, so
// the buffer can keep matching the screen without waiting to see its own output come back.
func RewrittenTail(recorded []RecordedStroke, corrections []WordCorrection, start int) []RecordedStroke {
	if start < 0 || start > len(recorded) {
		return nil
	}
	tail := make([]RecordedStroke, 0, len(recorded)-start)
	for offset, entry := range recorded[start:] {
		if correction, found := correctionAt(corrections, start+offset); found {
			entry.LayoutID = correction.TargetLayoutID
		}
		tail = append(tail, entry)
	}
	return tail
}
