package core

// Port of the Tokenizer half of Detect/StrokeBuffer.swift.

// Token is a run of word or separator strokes.
type Token struct {
	IsWord bool
	// Lo, Hi are indices into the recorded strokes (half-open).
	Lo, Hi int
	// Displayed is the text as it currently appears on screen.
	Displayed string
}

// Contains reports whether a stroke index falls inside the token.
func (t Token) Contains(index int) bool { return index >= t.Lo && index < t.Hi }

// Resolver resolves a layout ID to its table, or nil when the layout is gone.
type Resolver func(layoutID string) *KeyMapTable

// IsWordCharacter: word characters are letters, digits and the apostrophe. Digits count on
// purpose: a Kazakh word typed on the Russian layout comes out as `с2леметс3з`, and that
// has to stay one token for the detector to see the mistake.
func IsWordCharacter(c string) bool {
	return IsLetterChar(c) || IsNumberChar(c) || c == "'" || c == "’"
}

// Tokenize splits a keystroke run into words and separators. Strokes whose layout is gone
// are treated as separators, since we can no longer say what they produced.
func Tokenize(recorded []RecordedStroke, resolve Resolver) []Token {
	var tokens []Token
	index := 0
	for index < len(recorded) {
		start := index
		startIsWord, _ := classifyStroke(recorded[index], resolve)
		text := make([]byte, 0, 16)
		for index < len(recorded) {
			isWord, produced := classifyStroke(recorded[index], resolve)
			if isWord != startIsWord {
				break
			}
			text = append(text, produced...)
			index++
		}
		tokens = append(tokens, Token{IsWord: startIsWord, Lo: start, Hi: index, Displayed: string(text)})
	}
	return tokens
}

func classifyStroke(recorded RecordedStroke, resolve Resolver) (bool, string) {
	table := resolve(recorded.LayoutID)
	if table == nil {
		return false, ""
	}
	text, ok := table.Text(recorded.Stroke)
	if !ok {
		return false, ""
	}
	return text != "" && AllChars(text, IsWordCharacter), text
}

// WordInProgress is the word the caret sits at the end of, if the last thing typed was
// part of a word.
func WordInProgress(tokens []Token) (Token, bool) {
	if len(tokens) == 0 || !tokens[len(tokens)-1].IsWord {
		return Token{}, false
	}
	return tokens[len(tokens)-1], true
}

// LastCompletedWord is the most recent finished word — the one a just-typed space or
// comma closed.
func LastCompletedWord(tokens []Token) (Token, bool) {
	if len(tokens) == 0 || tokens[len(tokens)-1].IsWord {
		return Token{}, false
	}
	for i := len(tokens) - 2; i >= 0; i-- {
		if tokens[i].IsWord {
			return tokens[i], true
		}
	}
	return Token{}, false
}
