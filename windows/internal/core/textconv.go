package core

// Port of Apply/TextConverter.swift.

import "strings"

// ConvertText re-reads existing text as if it had been typed on a different layout.
//
// This is a guess by nature: finished text carries no record of which keys produced it,
// so the source layout has to be assumed. Characters the source layout cannot produce are
// passed through untouched, which keeps punctuation, emoji and a third language intact.
func ConvertText(text string, source, target *KeyMapTable) string {
	var result strings.Builder
	result.Grow(len(text))
	it := graphemeIter{s: text}
	for c := it.next(); c != ""; c = it.next() {
		stroke, ok := source.Stroke(c)
		if ok {
			if produced, ok := target.Text(stroke); ok {
				result.WriteString(produced)
				continue
			}
		}
		result.WriteString(c)
	}
	return result.String()
}

// Conversion is one reading of a text under another layout.
type Conversion struct {
	Layout *KeyMapTable
	Text   string
}

// ConversionCandidates lists every reading of text, one per candidate layout, excluding
// no-ops.
func ConversionCandidates(text string, source *KeyMapTable, layouts []*KeyMapTable) []Conversion {
	var out []Conversion
	for _, layout := range layouts {
		if layout.SourceID == source.SourceID {
			continue
		}
		converted := ConvertText(text, source, layout)
		if CanonEqual(converted, text) {
			continue
		}
		out = append(out, Conversion{Layout: layout, Text: converted})
	}
	return out
}

// PassageCost is the mean per-word cost of a passage under one language, for ranking
// conversions. ok is false when the passage has no scorable word.
func PassageCost(text string, lang Lang, models *Models) (float64, bool) {
	model := models.Model(lang)
	if model == nil {
		return 0, false
	}
	total := 0.0
	counted := 0
	for _, word := range SplitWords(text, IsWordCharacter) {
		if IsNumeric(word) {
			total += NumericCost
			counted++
		} else if score, ok := model.Score(word); ok {
			total += score.PenalizedCost()
			counted++
		}
	}
	if counted == 0 {
		return 0, false
	}
	return total / float64(counted), true
}

// SplitWords is Swift's `text.split(whereSeparator: { !isWord($0) })`: maximal non-empty
// runs of Characters satisfying isWord.
func SplitWords(text string, isWord func(string) bool) []string {
	var words []string
	start, pos := -1, 0
	it := graphemeIter{s: text}
	for c := it.next(); c != ""; c = it.next() {
		if isWord(c) {
			if start < 0 {
				start = pos
			}
		} else if start >= 0 {
			words = append(words, text[start:pos])
			start = -1
		}
		pos = it.pos
	}
	if start >= 0 {
		words = append(words, text[start:])
	}
	return words
}

// BestConversion is the best re-reading of text, or false when leaving it alone scores at
// least as well.
func BestConversion(text string, source *KeyMapTable, layouts []*KeyMapTable, models *Models) (Conversion, bool) {
	if source.Lang == LangNone {
		return Conversion{}, false
	}
	bestCost, ok := PassageCost(text, source.Lang, models)
	if !ok {
		return Conversion{}, false
	}
	var best Conversion
	found := false
	for _, candidate := range ConversionCandidates(text, source, layouts) {
		if candidate.Layout.Lang == LangNone {
			continue
		}
		cost, ok := PassageCost(candidate.Text, candidate.Layout.Lang, models)
		if !ok || !(cost < bestCost) {
			continue
		}
		bestCost = cost
		best = candidate
		found = true
	}
	return best, found
}
