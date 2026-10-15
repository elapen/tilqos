package core

// Port of Detect/Guards.swift.
//
// This is where most of the app's perceived quality lives. A missed correction is a shrug;
// a mangled URL or a rewritten identifier in a code editor is the reason people uninstall
// utilities like this one. When in doubt, protect.

// SkipReason says why a word must never be rewritten. Values match the Swift raw values.
type SkipReason string

const (
	SkipNone           SkipReason = ""
	SkipTooShort       SkipReason = "tooShort"
	SkipException      SkipReason = "exception"
	SkipAllDigits      SkipReason = "allDigits"
	SkipAcronym        SkipReason = "acronym"
	SkipEmail          SkipReason = "email"
	SkipURL            SkipReason = "url"
	SkipPath           SkipReason = "path"
	SkipCodeIdentifier SkipReason = "codeIdentifier"
	SkipSigil          SkipReason = "sigil"
	SkipMixedLayouts   SkipReason = "mixedLayouts"
)

// sigils are separator characters that mark whatever follows as machine text rather than
// prose.
var sigils = map[string]bool{"$": true, "`": true, "#": true, "\\": true, "~": true, "@": true, "%": true, "&": true}

type guardPattern struct {
	reason SkipReason
	re     *icuRegex
}

// guardPatterns are the Swift patterns, verbatim, in the same order.
var guardPatterns = func() []guardPattern {
	sources := []struct {
		reason  SkipReason
		pattern string
	}{
		{SkipEmail, `[\w.+-]+@[\w-]+\.[\w.-]+`},
		{SkipURL, `[a-zA-Z][a-zA-Z0-9+.\-]*://\S+`},
		{SkipURL, `\b(?:www\.)?[\w-]+\.(?:com|net|org|io|dev|app|co|ru|kz|uk|de|fr|me|ai|sh|xyz|info|biz|edu|gov)\b`},
		{SkipPath, `(?:^|\s)(?:~|\.{1,2})?/[\w./~-]+`},
		{SkipPath, `\\[\w\\.-]+`},
		// snake_case, camelCase, and anything that looks like a call.
		{SkipCodeIdentifier, `\b\w+(?:_\w+)+\b`},
		{SkipCodeIdentifier, `\b[a-z][a-z0-9]*(?:[A-Z][a-z0-9]*)+\b`},
		{SkipCodeIdentifier, `\b\w+\(`},
	}
	patterns := make([]guardPattern, 0, len(sources))
	for _, source := range sources {
		patterns = append(patterns, guardPattern{source.reason, mustCompileICU(source.pattern)})
	}
	return patterns
}()

// StringSet is a set of strings compared the way Swift compares them (canonical
// equivalence for the letters that matter).
type StringSet map[string]struct{}

// NewStringSet builds a set from items.
func NewStringSet(items ...string) StringSet {
	set := make(StringSet, len(items))
	for _, item := range items {
		set[Canon(item)] = struct{}{}
	}
	return set
}

func (s StringSet) Contains(item string) bool {
	if len(s) == 0 {
		return false
	}
	_, ok := s[Canon(item)]
	return ok
}

func (s StringSet) Add(item string) { s[Canon(item)] = struct{}{} }

// ProtectedTokens maps word-token indices to the reason their *surroundings* put them off
// limits.
//
// Only context-dependent reasons live here — `user@example.com` tokenizes into six
// innocent-looking words and only the joined text reveals it is an address. Checks that
// need nothing but the word itself belong in WordLevelReason.
func ProtectedTokens(tokens []Token) map[int]SkipReason {
	result := map[int]SkipReason{}

	// Reconstruct the line, recording each token's span so match ranges map back onto
	// tokens. Spans are in runes here and UTF-16 units in Swift; overlap is the same.
	var text []rune
	spans := make([][2]int, len(tokens))
	for index, token := range tokens {
		start := len(text)
		text = append(text, []rune(token.Displayed)...)
		spans[index] = [2]int{start, len(text)}
	}

	for index, token := range tokens {
		if !token.IsWord || index == 0 {
			continue
		}
		// A word introduced by $ ` # \ ~ @ % & is machine text.
		if previous := tokens[index-1]; !previous.IsWord {
			if last := LastChar(previous.Displayed); last != "" && sigils[last] {
				result[index] = SkipSigil
			}
		}
	}

	if len(text) == 0 {
		return result
	}
	for _, pattern := range guardPatterns {
		for _, match := range pattern.re.FindAll(text) {
			for index, span := range spans {
				if !tokens[index].IsWord {
					continue
				}
				if _, taken := result[index]; taken {
					continue
				}
				if rangesOverlap(span, match) {
					result[index] = pattern.reason
				}
			}
		}
	}
	return result
}

// rangesOverlap is Swift's Range.overlaps: empty ranges overlap nothing.
func rangesOverlap(a, b [2]int) bool {
	if a[0] >= a[1] || b[0] >= b[1] {
		return false
	}
	return !(b[1] <= a[0] || a[1] <= b[0])
}

// WordLevelReason holds the checks that need only the word itself.
//
// allowNumeric exists because an all-digit token is the one case where a very short word
// carries a decisive signal: `әғ` rendering as `25` is a certain mistake, not a statistical
// guess, so the length floor must not hide it.
func WordLevelReason(token Token, minLength int, exceptions StringSet, allowNumeric bool) SkipReason {
	word := token.Displayed

	if AllChars(word, IsNumberChar) {
		// Already a plain number; nothing about it can be a layout mistake.
		return SkipAllDigits
	}
	if exceptions.Contains(Lower(word)) {
		return SkipException
	}
	length := CharCount(word)
	if !allowNumeric && length < minLength {
		return SkipTooShort
	}
	// FYI, HTTP, ГОСТ — initialisms are not words the models know.
	if length > 1 && AnyChar(word, IsLetterChar) && CanonEqual(word, Upper(word)) &&
		!CanonEqual(word, Lower(word)) {
		return SkipAcronym
	}
	return SkipNone
}
