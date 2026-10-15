package translit

// Port of Translit/Transliterator.swift.

import (
	"sort"
	"strings"

	"tilqos/internal/core"
)

// Direction is which way to convert.
type Direction int

const (
	ToLatin Direction = iota
	ToCyrillic
)

func (d Direction) String() string {
	if d == ToLatin {
		return "toLatin"
	}
	return "toCyrillic"
}

const (
	// beamWidth: how many partial readings to keep while decoding a word.
	beamWidth = 12
	// harmonyPenalty is charged when a word mixes front and back vowels.
	harmonyPenalty = 2.2
	// hehPenalty is charged for choosing һ, which is correct in only a handful of words.
	hehPenalty = 4.0
	// engPenalty is charged for choosing ң over н.
	engPenalty = 1.0
	// initialGPenalty is charged for г at the start of a word.
	initialGPenalty = 1.6
	// morphologyBonus is subtracted from a Kazakh reading that decomposes into real morphemes.
	morphologyBonus = 1.0
	// kazakhBias is added to the Kazakh reading before comparing: the two models are not
	// calibrated against each other (see TranslitCalibrationTests in the Swift suite).
	kazakhBias = 0.4
	// leaveAloneThreshold: above this cost per symbol a word is implausible in both
	// languages and is left in Latin.
	leaveAloneThreshold = 5.0
	// shortWordLength: one- and two-letter words skip the threshold entirely.
	shortWordLength = 2
)

// Transliterator converts Kazakh between Cyrillic and Latin.
type Transliterator struct {
	models     *core.Models
	morphology *core.Morphology
	// overrides are user corrections, consulted before anything else (lowercased Latin →
	// Cyrillic).
	overrides map[string]string
	// neverConvert are words the user has rejected, kept in Latin.
	neverConvert core.StringSet
}

// New builds a transliterator. overrides and neverConvert may be nil.
func New(models *core.Models, overrides map[string]string, neverConvert []string) *Transliterator {
	normalized := make(map[string]string, len(overrides))
	for latin, cyrillic := range overrides {
		normalized[core.Canon(latin)] = cyrillic
	}
	return &Transliterator{
		models:       models,
		morphology:   models.KazakhMorphology(),
		overrides:    normalized,
		neverConvert: core.NewStringSet(neverConvert...),
	}
}

// ChangedWords lists the words in before that the conversion changed, as their original
// spellings. Used to learn from an undo: whichever words changed are the ones the user
// rejected.
func (t *Transliterator) ChangedWords(before, after string) []string {
	isWord := func(c string) bool { return core.IsLetterChar(c) || c == "'" }
	source := core.SplitWords(before, isWord)
	result := core.SplitWords(after, isWord)
	if len(source) != len(result) {
		return []string{}
	}
	changed := []string{}
	for i := range source {
		if !core.CanonEqual(source[i], result[i]) {
			changed = append(changed, source[i])
		}
	}
	return changed
}

// DirectionFor guesses which way the user wants to go by looking at what the text already is.
func DirectionFor(text string) Direction {
	cyrillic, latin := 0, 0
	for _, c := range core.Graphemes(core.Lower(text)) {
		if !core.IsLetterChar(c) {
			continue
		}
		if _, ok := CyrillicToPractical[core.Canon(c)]; ok {
			cyrillic++
		} else if _, ok := OfficialToCyrillic[core.Canon(c)]; ok || core.IsASCIIChar(c) {
			latin++
		}
	}
	if cyrillic > latin {
		return ToLatin
	}
	return ToCyrillic
}

// Convert converts in the given direction.
func (t *Transliterator) Convert(text string, scheme Scheme, direction Direction) string {
	if direction == ToLatin {
		return t.ToLatin(text, scheme)
	}
	return t.ToCyrillic(text, scheme)
}

// MARK: - Cyrillic to Latin

// ToLatin is a plain substitution in either scheme.
func (t *Transliterator) ToLatin(text string, scheme Scheme) string {
	table := CyrillicToPractical
	if scheme == Official2021 {
		table = CyrillicToOfficial
	}
	var result strings.Builder
	result.Grow(len(text))
	for _, c := range core.Graphemes(text) {
		mapped, ok := table[core.Canon(core.Lower(c))]
		if !ok {
			result.WriteString(c)
			continue
		}
		// Restore the case of the original letter across however many Latin letters it
		// became, so Ш becomes Sh rather than SH or sh.
		if core.IsUppercaseChar(c) {
			result.WriteString(capitalizeFirst(mapped))
		} else {
			result.WriteString(mapped)
		}
	}
	return result.String()
}

func capitalizeFirst(text string) string {
	first := core.FirstChar(text)
	if first == "" {
		return text
	}
	return core.Upper(first) + text[len(first):]
}

// MARK: - Latin to Cyrillic

// ToCyrillic works word by word: harmony and the language model are properties of a word,
// not of a whole passage.
func (t *Transliterator) ToCyrillic(text string, scheme Scheme) string {
	var result, word strings.Builder
	result.Grow(len(text))
	flush := func() {
		if word.Len() > 0 {
			result.WriteString(t.convertWord(word.String(), scheme))
			word.Reset()
		}
	}
	for _, c := range core.Graphemes(text) {
		if core.IsLetterChar(c) || c == "'" || c == "’" {
			word.WriteString(c)
		} else {
			flush()
			result.WriteString(c)
		}
	}
	flush()
	return result.String()
}

func (t *Transliterator) convertWord(word string, scheme Scheme) string {
	lower := core.Lower(word)
	var converted string
	if scheme == Official2021 {
		converted = t.decodeOfficial(lower)
	} else {
		converted = t.decodePractical(lower)
	}
	return restoreCase(word, converted)
}

// restoreCase applies the original word's capitalisation. Length can change (`sh` → `ш`),
// so this works at word level rather than per character.
func restoreCase(original, converted string) string {
	first := core.FirstChar(original)
	if first == "" || !core.IsUppercaseChar(first) {
		return converted
	}
	var letters []string
	for _, c := range core.Graphemes(original) {
		if core.IsLetterChar(c) {
			letters = append(letters, c)
		}
	}
	// ALL CAPS stays all caps; a single leading capital stays a single leading capital.
	if len(letters) > 1 {
		allUpper := true
		for _, c := range letters {
			if !core.IsUppercaseChar(c) {
				allUpper = false
				break
			}
		}
		if allUpper {
			return core.Upper(converted)
		}
	}
	return capitalizeFirst(converted)
}

// MARK: - Official decoding

// decodeOfficial is deterministic apart from `i`, which the standard uses for both и and й.
func (t *Transliterator) decodeOfficial(word string) string {
	characters := core.Graphemes(word)
	var result strings.Builder
	mappedVowel := func(c string) bool {
		mapped, ok := OfficialToCyrillic[core.Canon(c)]
		return ok && IsVowel(mapped)
	}
	for position, c := range characters {
		mapped, ok := OfficialToCyrillic[core.Canon(c)]
		if !ok {
			result.WriteString(c)
			continue
		}
		if c == "i" {
			// и becomes й between a vowel and a consonant, which is where Kazakh diphthongs
			// put it: `ai` → ай, `oi` → ой.
			previousIsVowel := position > 0 && mappedVowel(characters[position-1])
			nextIsVowel := position+1 < len(characters) && mappedVowel(characters[position+1])
			if previousIsVowel && !nextIsVowel {
				result.WriteString("й")
			} else {
				result.WriteString("и")
			}
			continue
		}
		result.WriteString(mapped)
	}
	return applyHehExceptions(result.String())
}

// MARK: - Practical decoding

type beam struct {
	text           string
	cost           float64
	previous       int
	beforePrevious int
	sawFrontVowel  bool
	sawBackVowel   bool
	steps          int
}

type variant struct {
	lang         core.Lang
	options      map[string][]string
	digraphs     map[string][]string
	kazakhRules  bool
	variantLabel string
}

var (
	kazakhVariant  = variant{core.LangKK, PracticalToCyrillic, PracticalDigraphs, true, "kk"}
	russianVariant = variant{core.LangRU, PracticalRussianToCyrillic, PracticalRussianDigraphs, false, "ru"}
)

// Reading is one decoded spelling and its mean cost per symbol.
type Reading struct {
	Lang core.Lang
	Text string
	Cost float64
}

func (t *Transliterator) decodePractical(word string) string {
	// The user's own corrections win, then the built-in frequency list.
	if pinned, ok := t.overrides[core.Canon(word)]; ok {
		return pinned
	}
	// Brands and anything the user has rejected stay exactly as typed.
	if IsLatinOnly(word, t.neverConvert) {
		return word
	}
	if known, ok := PracticalWordOverrides[word]; ok {
		return known
	}

	// Read the word as Kazakh and as Russian, then keep whichever fits better. `q` settles
	// it outright: in this spelling q is always қ, and Russian written in Latin has no use
	// for the letter.
	isDefinitelyKazakh := false
	for _, c := range core.Graphemes(word) {
		if c == "q" {
			isDefinitelyKazakh = true
			break
		}
	}

	var best *Reading
	for _, v := range []variant{kazakhVariant, russianVariant} {
		reading, ok := t.decode(word, v)
		if !ok {
			continue
		}
		if v.kazakhRules && !isDefinitelyKazakh {
			reading.Cost += kazakhBias
		}
		if best == nil || reading.Cost < best.Cost {
			r := reading
			best = &r
		}
	}
	if best == nil {
		return t.fallbackDecode(word)
	}
	// Neither language claims it: almost certainly a name or a foreign word.
	if core.CharCount(word) > shortWordLength && best.Cost > leaveAloneThreshold {
		return word
	}
	return best.Text
}

// Readings returns both readings of a word with their costs (no language bias applied).
// Used to calibrate the bias against labelled words.
func (t *Transliterator) Readings(word string) []Reading {
	var out []Reading
	for _, v := range []variant{kazakhVariant, russianVariant} {
		if reading, ok := t.decode(core.Lower(word), v); ok {
			out = append(out, reading)
		}
	}
	return out
}

func (t *Transliterator) decode(word string, v variant) (Reading, bool) {
	model := t.models.Model(v.lang)
	if model == nil {
		return Reading{}, false
	}

	units := split(word, v)
	boundary := model.BoundarySymbol()
	beams := []beam{{previous: boundary, beforePrevious: boundary}}

	for _, unit := range units {
		next := make([]beam, 0, len(beams)*len(unit.options))
		for _, b := range beams {
			atWordStart := b.text == ""
			for _, option := range unit.options {
				// ң never begins a Kazakh word, so that reading is not merely unlikely.
				if v.kazakhRules && option == "ң" && atWordStart {
					continue
				}
				symbol, ok := model.Symbol(option)
				if !ok {
					// Not a Cyrillic letter — carry it through without scoring it.
					extended := b
					extended.text += option
					next = append(next, extended)
					continue
				}

				extended := b
				extended.cost += model.Cost(b.beforePrevious, b.previous, symbol)
				extended.text += option
				extended.beforePrevious = b.previous
				extended.previous = symbol
				extended.steps++

				if v.kazakhRules {
					switch {
					case FrontVowels[option]:
						if b.sawBackVowel {
							extended.cost += harmonyPenalty
						}
						extended.sawFrontVowel = true
					case BackVowels[option]:
						if b.sawFrontVowel {
							extended.cost += harmonyPenalty
						}
						extended.sawBackVowel = true
					case FrontConsonants[option]:
						if b.sawBackVowel {
							extended.cost += harmonyPenalty
						}
					case BackConsonants[option]:
						if b.sawFrontVowel {
							extended.cost += harmonyPenalty
						}
					}
					if option == "һ" {
						extended.cost += hehPenalty
					}
					if option == "ң" {
						extended.cost += engPenalty
					}
					if option == "г" && atWordStart {
						extended.cost += initialGPenalty
					}
				}
				next = append(next, extended)
			}
		}
		// Swift's sort is stable, and ties are common here, so the order must be too.
		sort.SliceStable(next, func(i, j int) bool { return next[i].cost < next[j].cost })
		if len(next) > beamWidth {
			next = next[:beamWidth]
		}
		beams = next
	}

	// Word endings carry a lot of information in an agglutinative language, so close the
	// word properly rather than stopping mid-context.
	for i := range beams {
		beams[i].cost += model.Cost(beams[i].beforePrevious, beams[i].previous, boundary)
		beams[i].steps++
	}

	// Re-rank on morphology: a reading that decomposes into real morphemes is far stronger
	// evidence than any trigram score.
	candidates := make([]Reading, 0, len(beams))
	for _, b := range beams {
		if b.steps <= 0 {
			continue
		}
		text := b.text
		if v.kazakhRules {
			text = applyHehExceptions(text)
		}
		// Normalised, so a long word is not judged worse for being long.
		cost := b.cost / float64(b.steps)
		if v.kazakhRules && t.morphology != nil {
			if _, suffixes, ok := t.morphology.Decomposition(text, core.DefaultMorphologyDepth); ok {
				depth := len(suffixes) - 1
				if depth < 0 {
					depth = 0
				}
				// A shallow decomposition is stronger evidence, and it suppresses accidental
				// chains: `екенмің` only decomposes as `екен + м + ің`.
				cost -= morphologyBonus / (1.0 + 0.6*float64(depth))
			}
		}
		candidates = append(candidates, Reading{Lang: v.lang, Text: text, Cost: cost})
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].Cost < candidates[j].Cost })
	if len(candidates) == 0 {
		return Reading{}, false
	}
	return candidates[0], true
}

// applyHehExceptions rewrites х to һ in the few words that genuinely take it.
func applyHehExceptions(word string) string {
	if !strings.Contains(word, "х") && !strings.Contains(word, "һ") {
		return word
	}
	normalized := strings.ReplaceAll(word, "һ", "х")
	for _, candidate := range HehWords {
		if strings.ReplaceAll(candidate, "һ", "х") == normalized {
			return candidate
		}
	}
	return normalized
}

func (t *Transliterator) fallbackDecode(word string) string {
	var result strings.Builder
	for _, unit := range split(word, kazakhVariant) {
		if len(unit.options) > 0 {
			result.WriteString(unit.options[0])
		} else {
			result.WriteString(unit.source)
		}
	}
	return result.String()
}

type unit struct {
	source  string
	options []string
}

// split cuts a Latin word into units, matching digraphs before single letters so `sh` is
// one unit rather than an `s` followed by an `h`.
func split(word string, v variant) []unit {
	characters := core.Graphemes(word)
	var units []unit
	index := 0
	for index < len(characters) {
		matched := false
		// Longest digraph first: shch before sh.
		for length := 4; length >= 2; length-- {
			if index+length > len(characters) {
				continue
			}
			candidate := joinChars(characters[index : index+length])
			if options, ok := v.digraphs[candidate]; ok {
				units = append(units, unit{source: candidate, options: options})
				index += length
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		c := characters[index]
		options, ok := v.options[core.Canon(c)]
		if !ok {
			options = []string{c}
		}
		units = append(units, unit{source: c, options: options})
		index++
	}
	return units
}
