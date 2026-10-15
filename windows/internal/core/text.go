package core

// Swift-compatible text semantics.
//
// The Swift implementation works on `Character`s — extended grapheme clusters — and relies
// on a handful of standard-library behaviours that differ from Go's defaults:
//
//   - `String.count`, `dropFirst`, `suffix` and iteration are per grapheme cluster;
//   - `Character.isLetter` is the Unicode Alphabetic property of the first scalar
//     (Go's unicode.IsLetter is only category L);
//   - `Character.isNumber` is "has a Numeric_Type" of the first scalar;
//   - `lowercased()` / `uppercased()` use the *full* case mappings (ß → SS, İ → i\u0307),
//     where Go's unicode.ToUpper/ToLower use the simple ones;
//   - `Character` and `String` equality is canonical equivalence, so "е\u0308" == "ё".
//
// Everything in this package that the Swift code does per Character goes through these
// helpers, so the port reproduces Swift's decisions and not merely similar ones. The
// golden parity tests (see ../../testdata/golden) exercise each of them.
//
// Known, documented deviations (none reachable from the en/ru/kk keyboard layouts):
//   - Han ideographs with a numeric value: only the common ones are treated as numbers;
//   - canonical equivalence is implemented for the letters the models and the
//     transliteration tables actually use (see composePairs), not for all of Unicode;
//   - grapheme segmentation follows UAX #29 without the Indic conjunct rule (GB9c) and
//     with an approximate Extended_Pictographic table.

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// MARK: - Grapheme clusters

type gbClass uint8

const (
	gbOther gbClass = iota
	gbCR
	gbLF
	gbControl
	gbExtend
	gbZWJ
	gbRegional
	gbPrepend
	gbSpacingMark
	gbL
	gbV
	gbT
	gbLV
	gbLVT
)

func graphemeClass(r rune) gbClass {
	switch {
	case r < 0x80:
		switch {
		case r == '\r':
			return gbCR
		case r == '\n':
			return gbLF
		case r < 0x20 || r == 0x7F:
			return gbControl
		}
		return gbOther
	case r == 0x200D:
		return gbZWJ
	case r >= 0x1F1E6 && r <= 0x1F1FF:
		return gbRegional
	case r >= 0x1F3FB && r <= 0x1F3FF: // emoji modifiers are Extend
		return gbExtend
	case r >= 0x1100 && r <= 0x115F, r >= 0xA960 && r <= 0xA97C:
		return gbL
	case r >= 0x1160 && r <= 0x11A7, r >= 0xD7B0 && r <= 0xD7C6:
		return gbV
	case r >= 0x11A8 && r <= 0x11FF, r >= 0xD7CB && r <= 0xD7FB:
		return gbT
	case r >= 0xAC00 && r <= 0xD7A3:
		if (r-0xAC00)%28 == 0 {
			return gbLV
		}
		return gbLVT
	}
	if isGraphemeExtend(r) {
		return gbExtend
	}
	switch r {
	case 0x0600, 0x0601, 0x0602, 0x0603, 0x0604, 0x0605, 0x06DD, 0x070F, 0x0890, 0x0891,
		0x08E2, 0x110BD, 0x110CD:
		return gbPrepend
	case 0x0E33, 0x0EB3:
		return gbSpacingMark
	}
	if unicode.Is(unicode.Mc, r) {
		return gbSpacingMark
	}
	if unicode.Is(unicode.Cc, r) || unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r) ||
		unicode.Is(unicode.Cf, r) {
		return gbControl
	}
	return gbOther
}

func isGraphemeExtend(r rune) bool {
	return unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) ||
		unicode.Is(unicode.Other_Grapheme_Extend, r)
}

// isExtendedPictographic approximates the Extended_Pictographic property, which Go's
// unicode package does not carry. It only matters for emoji ZWJ sequences (rule GB11).
func isExtendedPictographic(r rune) bool {
	switch {
	case r == 0x00A9, r == 0x00AE, r == 0x203C, r == 0x2049, r == 0x2122, r == 0x2139:
		return true
	case r >= 0x2194 && r <= 0x2199, r >= 0x21A9 && r <= 0x21AA, r >= 0x231A && r <= 0x231B:
		return true
	case r == 0x2328, r == 0x2388, r == 0x23CF, r >= 0x23E9 && r <= 0x23F3,
		r >= 0x23F8 && r <= 0x23FA, r == 0x24C2, r >= 0x25AA && r <= 0x25AB, r == 0x25B6,
		r == 0x25C0, r >= 0x25FB && r <= 0x25FE:
		return true
	case r >= 0x2600 && r <= 0x27BF:
		return true
	case r >= 0x2934 && r <= 0x2935, r >= 0x2B05 && r <= 0x2B07, r >= 0x2B1B && r <= 0x2B1C,
		r == 0x2B50, r == 0x2B55, r == 0x3030, r == 0x303D, r == 0x3297, r == 0x3299:
		return true
	case r >= 0x1F000 && r <= 0x1F0FF, r >= 0x1F10D && r <= 0x1F10F, r == 0x1F12F,
		r >= 0x1F16C && r <= 0x1F171, r >= 0x1F17E && r <= 0x1F17F, r == 0x1F18E,
		r >= 0x1F191 && r <= 0x1F19A, r >= 0x1F1AD && r <= 0x1F1E5:
		return true
	case r >= 0x1F201 && r <= 0x1F3FA, r >= 0x1F400 && r <= 0x1FAFF, r >= 0x1FC00 && r <= 0x1FFFD:
		return true
	}
	return false
}

// shouldBreak reports whether a grapheme boundary falls between prev and next, given the
// state of the cluster built so far.
func shouldBreak(prev, next gbClass, riRun int, pictSeqZWJ bool, nextRune rune) bool {
	switch {
	case prev == gbCR && next == gbLF: // GB3
		return false
	case prev == gbControl || prev == gbCR || prev == gbLF: // GB4
		return true
	case next == gbControl || next == gbCR || next == gbLF: // GB5
		return true
	case prev == gbL && (next == gbL || next == gbV || next == gbLV || next == gbLVT): // GB6
		return false
	case (prev == gbLV || prev == gbV) && (next == gbV || next == gbT): // GB7
		return false
	case (prev == gbLVT || prev == gbT) && next == gbT: // GB8
		return false
	case next == gbExtend || next == gbZWJ: // GB9
		return false
	case next == gbSpacingMark: // GB9a
		return false
	case prev == gbPrepend: // GB9b
		return false
	case pictSeqZWJ && prev == gbZWJ && isExtendedPictographic(nextRune): // GB11
		return false
	case prev == gbRegional && next == gbRegional: // GB12, GB13
		return riRun%2 == 0
	}
	return true
}

// graphemeIter walks a string one extended grapheme cluster at a time without allocating.
type graphemeIter struct {
	s   string
	pos int
}

// next returns the next cluster, or "" at the end.
func (it *graphemeIter) next() string {
	if it.pos >= len(it.s) {
		return ""
	}
	start := it.pos
	r, size := utf8.DecodeRuneInString(it.s[it.pos:])
	it.pos += size
	prev := graphemeClass(r)
	// Consecutive regional indicators at the end of the cluster (GB12/13).
	riRun := 0
	if prev == gbRegional {
		riRun = 1
	}
	// pictRun: the cluster ends in "Extended_Pictographic Extend*".
	// pictZWJ: the cluster ends in "Extended_Pictographic Extend* ZWJ" (GB11).
	pictRun := isExtendedPictographic(r)
	pictZWJ := false
	for it.pos < len(it.s) {
		nr, nsize := utf8.DecodeRuneInString(it.s[it.pos:])
		next := graphemeClass(nr)
		if shouldBreak(prev, next, riRun, pictZWJ, nr) {
			break
		}
		it.pos += nsize
		if next == gbRegional {
			riRun++
		} else {
			riRun = 0
		}
		switch next {
		case gbZWJ:
			pictZWJ = pictRun
			pictRun = false
		case gbExtend:
			if prev == gbZWJ {
				pictRun = false
			}
			pictZWJ = false
		default:
			pictRun = isExtendedPictographic(nr)
			pictZWJ = false
		}
		prev = next
	}
	return it.s[start:it.pos]
}

// Graphemes splits s into extended grapheme clusters — Swift's Characters.
func Graphemes(s string) []string {
	var out []string
	it := graphemeIter{s: s}
	for c := it.next(); c != ""; c = it.next() {
		out = append(out, c)
	}
	return out
}

// CharCount is Swift's String.count.
func CharCount(s string) int {
	if isASCIIString(s) {
		// No ASCII pair joins except CR LF.
		return len(s) - strings.Count(s, "\r\n")
	}
	n := 0
	it := graphemeIter{s: s}
	for c := it.next(); c != ""; c = it.next() {
		n++
	}
	return n
}

// DropFirstChars is Swift's String(s.dropFirst(n)).
func DropFirstChars(s string, n int) string {
	it := graphemeIter{s: s}
	for i := 0; i < n; i++ {
		if it.next() == "" {
			return ""
		}
	}
	return s[it.pos:]
}

// LastChar is Swift's s.last, or "" for an empty string.
func LastChar(s string) string {
	last := ""
	it := graphemeIter{s: s}
	for c := it.next(); c != ""; c = it.next() {
		last = c
	}
	return last
}

// FirstChar is Swift's s.first, or "" for an empty string.
func FirstChar(s string) string {
	it := graphemeIter{s: s}
	return it.next()
}

func isASCIIString(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// MARK: - Character properties (first scalar, like Swift)

func firstRune(c string) rune {
	r, _ := utf8.DecodeRuneInString(c)
	return r
}

// isAlphabetic is the Unicode Alphabetic property: L + Nl + Other_Alphabetic, with the
// Other_Lowercase/Other_Uppercase letters that Lowercase/Uppercase add on top.
func isAlphabetic(r rune) bool {
	if r < 0x80 {
		return (r|0x20) >= 'a' && (r|0x20) <= 'z'
	}
	return unicode.IsLetter(r) || unicode.Is(unicode.Nl, r) ||
		unicode.Is(unicode.Other_Alphabetic, r) || unicode.Is(unicode.Other_Lowercase, r) ||
		unicode.Is(unicode.Other_Uppercase, r)
}

// hanNumeric lists the common Han ideographs that carry a Unihan numeric value, which
// Swift counts as numbers. Not exhaustive; unreachable from the supported layouts.
var hanNumeric = map[rune]bool{
	0x4E00: true, 0x4E03: true, 0x4E07: true, 0x4E09: true, 0x4E5D: true, 0x4E8C: true,
	0x4E94: true, 0x4EBF: true, 0x5104: true, 0x5146: true, 0x516B: true, 0x516D: true,
	0x5341: true, 0x5343: true, 0x56DB: true, 0x767E: true, 0x842C: true, 0x96F6: true,
	0x5EFF: true, 0x5345: true, 0x534C: true, 0x58F9: true, 0x8CB3: true, 0x53C3: true,
	0x8086: true, 0x4F0D: true, 0x9678: true, 0x67D2: true, 0x634C: true, 0x7396: true,
	0x62FE: true, 0x4F70: true, 0x4EDF: true, 0x5169: true, 0x4E24: true, 0x8D30: true,
	0x9646: true, 0x53C1: true,
}

// hasNumericType is Swift's `properties.numericType != nil`.
func hasNumericType(r rune) bool {
	if r < 0x80 {
		return r >= '0' && r <= '9'
	}
	return unicode.IsNumber(r) || hanNumeric[r]
}

// IsLetterChar is Swift's Character.isLetter.
func IsLetterChar(c string) bool { return c != "" && isAlphabetic(firstRune(c)) }

// IsNumberChar is Swift's Character.isNumber.
func IsNumberChar(c string) bool { return c != "" && hasNumericType(firstRune(c)) }

// IsASCIIChar is Swift's Character.isASCII (CR LF counts as ASCII).
func IsASCIIChar(c string) bool {
	if c == "\r\n" {
		return true
	}
	return len(c) == 1 && c[0] < 0x80
}

// IsWhitespaceChar is Swift's Character.isWhitespace: the first scalar has the Unicode
// White_Space property, which is exactly what unicode.IsSpace tests. Line breaks count.
func IsWhitespaceChar(c string) bool { return c != "" && unicode.IsSpace(firstRune(c)) }

// IsNewlineChar is Swift's Character.isNewline: LF, VT, FF, CR (so CR LF too), NEL, and the
// line and paragraph separators.
func IsNewlineChar(c string) bool {
	if c == "" {
		return false
	}
	switch r := firstRune(c); {
	case r >= 0x0A && r <= 0x0D, r == 0x85, r == 0x2028, r == 0x2029:
		return true
	}
	return false
}

func isUppercaseProp(r rune) bool {
	return unicode.Is(unicode.Lu, r) || unicode.Is(unicode.Other_Uppercase, r)
}

func isLowercaseProp(r rune) bool {
	return unicode.Is(unicode.Ll, r) || unicode.Is(unicode.Other_Lowercase, r)
}

func isCasedProp(r rune) bool {
	return isUppercaseProp(r) || isLowercaseProp(r) || unicode.Is(unicode.Lt, r)
}

func singleRune(c string) (rune, bool) {
	r, size := utf8.DecodeRuneInString(c)
	return r, size == len(c) && c != ""
}

// IsUppercaseChar is Swift's Character.isUppercase.
func IsUppercaseChar(c string) bool {
	if r, ok := singleRune(c); ok && isUppercaseProp(r) {
		return true
	}
	return Upper(c) == c && isCasedChar(c)
}

func isCasedChar(c string) bool {
	if r, ok := singleRune(c); ok && isCasedProp(r) {
		return true
	}
	return Upper(c) != c || Lower(c) != c
}

// AllChars reports whether every Character of s satisfies pred (true for "").
func AllChars(s string, pred func(string) bool) bool {
	it := graphemeIter{s: s}
	for c := it.next(); c != ""; c = it.next() {
		if !pred(c) {
			return false
		}
	}
	return true
}

// AnyChar reports whether some Character of s satisfies pred.
func AnyChar(s string, pred func(string) bool) bool {
	it := graphemeIter{s: s}
	for c := it.next(); c != ""; c = it.next() {
		if pred(c) {
			return true
		}
	}
	return false
}

// MARK: - Case mapping (full mappings, like Swift)

// specialUpper holds SpecialCasing.txt's unconditional uppercase mappings that expand to
// more than one scalar. Everything else uses the simple mapping.
var specialUpper = map[rune]string{
	0x00DF: "SS", 0x0149: "ʼN", 0x01F0: "J\u030C", 0x0390: "Ι\u0308\u0301",
	0x03B0: "Υ\u0308\u0301", 0x0587: "ԵՒ", 0x1E96: "H\u0331", 0x1E97: "T\u0308",
	0x1E98: "W\u030A", 0x1E99: "Y\u030A", 0x1E9A: "Aʾ", 0x1F50: "Υ\u0313",
	0x1F52: "Υ\u0313\u0300", 0x1F54: "Υ\u0313\u0301", 0x1F56: "Υ\u0313\u0342",
	0x1FB6: "Α\u0342", 0x1FC6: "Η\u0342", 0x1FD2: "Ι\u0308\u0300",
	0x1FD3: "Ι\u0308\u0301", 0x1FD6: "Ι\u0342", 0x1FD7: "Ι\u0308\u0342",
	0x1FE2: "Υ\u0308\u0300", 0x1FE3: "Υ\u0308\u0301", 0x1FE4: "Ρ\u0313",
	0x1FE6: "Υ\u0342", 0x1FE7: "Υ\u0308\u0342", 0x1FF6: "Ω\u0342",
	0x1FB3: "ΑΙ", 0x1FBC: "ΑΙ", 0x1FC3: "ΗΙ", 0x1FCC: "ΗΙ",
	0x1FF3: "ΩΙ", 0x1FFC: "ΩΙ", 0x1FB2: "ᾺΙ", 0x1FB4: "ΆΙ",
	0x1FC2: "ῊΙ", 0x1FC4: "ΉΙ", 0x1FF2: "ῺΙ", 0x1FF4: "ΏΙ",
	0x1FB7: "Α\u0342Ι", 0x1FC7: "Η\u0342Ι", 0x1FF7: "Ω\u0342Ι",
	0xFB00: "FF", 0xFB01: "FI", 0xFB02: "FL", 0xFB03: "FFI", 0xFB04: "FFL", 0xFB05: "ST",
	0xFB06: "ST", 0xFB13: "ՄՆ", 0xFB14: "ՄԵ", 0xFB15: "ՄԻ",
	0xFB16: "ՎՆ", 0xFB17: "ՄԽ",
}

func upperRune(b *strings.Builder, r rune) {
	if s, ok := specialUpper[r]; ok {
		b.WriteString(s)
		return
	}
	// Greek with ypogegrammeni: ᾀ → ἈΙ and friends.
	if r >= 0x1F80 && r <= 0x1FAF {
		base := [...]rune{0x1F08, 0x1F08, 0x1F28, 0x1F28, 0x1F68, 0x1F68}[(r-0x1F80)/8]
		b.WriteRune(base + (r-0x1F80)%8)
		b.WriteRune(0x0399)
		return
	}
	b.WriteRune(unicode.ToUpper(r))
}

// Upper is Swift's String.uppercased().
func Upper(s string) string {
	if isASCIIString(s) {
		return strings.ToUpper(s)
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		upperRune(&b, r)
	}
	return b.String()
}

// Lower is Swift's String.lowercased(). The only unconditional multi-scalar lowercase
// mapping is İ → i\u0307.
func Lower(s string) string {
	if isASCIIString(s) {
		return strings.ToLower(s)
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == 0x0130 {
			b.WriteString("i\u0307")
			continue
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// MARK: - Canonical equivalence (for the letters that matter)

// composePairs maps base + combining mark to the precomposed letter, for every letter the
// alphabets and transliteration tables contain that has a canonical decomposition.
var composePairs = map[[2]rune]rune{
	{'е', 0x0308}: 'ё', {'Е', 0x0308}: 'Ё', {'и', 0x0306}: 'й', {'И', 0x0306}: 'Й',
	{'і', 0x0308}: 'ї', {'І', 0x0308}: 'Ї', {'у', 0x0306}: 'ў', {'У', 0x0306}: 'Ў',
	{'a', 0x0308}: 'ä', {'A', 0x0308}: 'Ä', {'o', 0x0308}: 'ö', {'O', 0x0308}: 'Ö',
	{'u', 0x0308}: 'ü', {'U', 0x0308}: 'Ü', {'g', 0x0306}: 'ğ', {'G', 0x0306}: 'Ğ',
	{'n', 0x0303}: 'ñ', {'N', 0x0303}: 'Ñ', {'u', 0x0304}: 'ū', {'U', 0x0304}: 'Ū',
	{'s', 0x0327}: 'ş', {'S', 0x0327}: 'Ş', {'I', 0x0307}: 'İ', {'e', 0x0301}: 'é',
	{'E', 0x0301}: 'É', {'a', 0x0301}: 'á', {'i', 0x0308}: 'ï', {'e', 0x0300}: 'è',
}

// Canon composes the decomposed spellings in composePairs, so strings that Swift treats
// as equal compare equal here. Cheap when there is nothing to compose.
func Canon(s string) string {
	needs := false
	for _, r := range s {
		if r >= 0x0300 && r <= 0x036F {
			needs = true
			break
		}
	}
	if !needs {
		return s
	}
	runes := []rune(s)
	out := make([]rune, 0, len(runes))
	for i := 0; i < len(runes); i++ {
		if i+1 < len(runes) {
			if composed, ok := composePairs[[2]rune{runes[i], runes[i+1]}]; ok {
				out = append(out, composed)
				i++
				continue
			}
		}
		out = append(out, runes[i])
	}
	return string(out)
}

// CanonEqual is Swift's String == for the purposes of this app.
func CanonEqual(a, b string) bool {
	return a == b || Canon(a) == Canon(b)
}

// MARK: - Swift numeric helpers

// swiftMin is Swift's min(x, y): `y < x ? y : x`. Differs from math.Min on NaN.
func swiftMin(x, y float64) float64 {
	if y < x {
		return y
	}
	return x
}

// swiftMax is Swift's max(x, y): `y >= x ? y : x`.
func swiftMax(x, y float64) float64 {
	if y >= x {
		return y
	}
	return x
}
