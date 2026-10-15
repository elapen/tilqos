package core

// Port of Detect/Alphabet.swift.

// AlphabetKind is serialised into model files, so the values are fixed.
type AlphabetKind uint8

const (
	AlphabetLatin    AlphabetKind = 0
	AlphabetCyrillic AlphabetKind = 1
)

// Alphabet is the letters a script's trigram model is built over, plus a word-boundary
// symbol.
//
// One alphabet per script rather than per language: Russian and Kazakh share the same
// 42-letter Cyrillic inventory, and using one inventory for both means their models are
// directly comparable index for index.
type Alphabet struct {
	Kind       AlphabetKind
	Characters []string
	lookup     map[string]int
}

// LatinAlphabet is 26 letters plus the apostrophe, so English contractions stay one word.
var LatinAlphabet = newAlphabet(AlphabetLatin, "abcdefghijklmnopqrstuvwxyz'")

// CyrillicAlphabet is the 33 Russian letters followed by the 9 Kazakh-specific ones:
// ә ғ қ ң ө ұ ү һ і.
var CyrillicAlphabet = newAlphabet(AlphabetCyrillic,
	"абвгдеёжзийклмнопрстуфхцчшщъыьэюя"+
		"әғқңөұүһі")

// kazakhSpecific are the nine letters Kazakh adds to the Russian inventory.
//
// Their presence is positive evidence that Cyrillic text is Kazakh rather than Russian.
// Their absence is not evidence of anything, since the two alphabets otherwise coincide.
var kazakhSpecific = map[string]bool{
	"ә": true, "ғ": true, "қ": true, "ң": true, "ө": true,
	"ұ": true, "ү": true, "һ": true, "і": true,
}

// IsKazakhSpecific reports whether a Character is one of the nine Kazakh-only letters.
func IsKazakhSpecific(c string) bool { return kazakhSpecific[c] }

// ContainsKazakhSpecific is Alphabet.containsKazakhSpecific.
func ContainsKazakhSpecific(text string) bool {
	return AnyChar(Lower(text), IsKazakhSpecific)
}

// AlphabetForKind is Alphabet.forKind.
func AlphabetForKind(kind AlphabetKind) *Alphabet {
	if kind == AlphabetCyrillic {
		return CyrillicAlphabet
	}
	return LatinAlphabet
}

// AlphabetForScript is Alphabet.forScript.
func AlphabetForScript(script Script) *Alphabet {
	if script == ScriptCyrillic {
		return CyrillicAlphabet
	}
	return LatinAlphabet
}

func newAlphabet(kind AlphabetKind, characters string) *Alphabet {
	a := &Alphabet{Kind: kind, Characters: Graphemes(characters), lookup: map[string]int{}}
	for index, c := range a.Characters {
		a.lookup[c] = index
	}
	return a
}

func (a *Alphabet) LetterCount() int { return len(a.Characters) }

// SymbolCount is letters plus the boundary marker — the dimension of the trigram cube.
func (a *Alphabet) SymbolCount() int { return len(a.Characters) + 1 }

// Boundary is the index of the word-boundary symbol; it also stands in for anything
// off-alphabet.
func (a *Alphabet) Boundary() int { return len(a.Characters) }

// Index returns the symbol index of one Character. Canonically equivalent spellings
// ("е\u0308" for "ё") resolve like they do in Swift.
func (a *Alphabet) Index(c string) (int, bool) {
	if index, ok := a.lookup[c]; ok {
		return index, true
	}
	index, ok := a.lookup[Canon(c)]
	return index, ok
}

// Encode maps a word to symbol indices.
//
// Characters outside the alphabet become boundaries and are counted separately — that is
// what makes `с2леметс3з` score badly as Russian without being rejected outright: the digit
// splits the word and costs it a penalty.
func (a *Alphabet) Encode(word string) (symbols []int, unknown int) {
	lowered := Lower(word)
	symbols = make([]int, 0, len(lowered))
	it := graphemeIter{s: lowered}
	for c := it.next(); c != ""; c = it.next() {
		if index, ok := a.Index(c); ok {
			symbols = append(symbols, index)
		} else {
			symbols = append(symbols, a.Boundary())
			unknown++
		}
	}
	return symbols, unknown
}

// Covers reports whether every character is a letter of this alphabet.
func (a *Alphabet) Covers(word string) bool {
	return AllChars(Lower(word), func(c string) bool {
		_, ok := a.Index(c)
		return ok
	})
}
