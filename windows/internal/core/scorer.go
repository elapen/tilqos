package core

// Port of Detect/WordScorer.swift.

import (
	"math"
	"strings"
	"sync"
)

// Reading is a run of keystrokes read under one layout, with everything the detector weighs.
type Reading struct {
	Lang     Lang
	LayoutID string
	// Lo, Hi are the strokes this reading covers (half-open).
	Lo, Hi int
	// Text is everything the strokes produce under this layout, punctuation included.
	Text string
	// Core is Text without leading and trailing punctuation — the word itself.
	Core string
	// Length is the number of Characters in Core — `i'm` is three long, like `его`.
	Length int
	// HasOddEdges: punctuation glued to the word that text in this script does not have: a
	// leading comma, a trailing backslash or bracket. `to\` is not `to` followed by
	// punctuation; it is `ещё` typed on ABC, where `ё` sits on the backslash key.
	HasOddEdges bool
	// IsClean: the core is a single word — nothing but letters, digits, and at most inner
	// hyphens or apostrophes. A reading with a comma in the middle (`nt,t`) is not something
	// a person types in that language.
	IsClean   bool
	IsNumeric bool
	// IsKnown: a real word of the language — in the lexicon, derived from it, or taught by
	// the user.
	IsKnown bool
	// IsCore: among the language's most frequent words.
	IsCore bool
	// Cost is the trigram cost in bits per symbol, penalties included; +Inf when unscorable.
	Cost float64
	// InteriorJunk counts off-alphabet characters wedged between letters — `с2лем`, `nt,t`.
	InteriorJunk int
}

// LearnedDecisionKind is a standing decision the user made about one on-screen word.
type LearnedDecisionKind uint8

const (
	// DecisionConvert: the user accepted this conversion, or asked for it — do it without
	// asking next time.
	DecisionConvert LearnedDecisionKind = iota + 1
	// DecisionKeep: the user undid it, erased it, or ignored the offer repeatedly — leave
	// the word alone.
	DecisionKeep
)

// LearnedDecision is Swift's LearnedDecision: .convert(to:) or .keep.
type LearnedDecision struct {
	Kind LearnedDecisionKind
	// Lang is the target of a DecisionConvert.
	Lang Lang
}

// LearnedVocabulary is what the user has taught the app, as the detector sees it. See
// Learner.
type LearnedVocabulary interface {
	// Version changes whenever anything learned changes, so cached judgements can be told
	// apart.
	Version() int
	// IsKnown: words the user keeps typing and keeping in a language are words, whatever
	// the dictionary says.
	IsKnown(word string, lang Lang) bool
	// Decision is a standing decision about converting this exact on-screen word, if the user
	// made one.
	Decision(typed string) (LearnedDecision, bool)
}

const (
	// NumericCost is assigned to an all-digit reading.
	//
	// Digits are not letters of any script, so no trigram model can say anything about
	// `25`. Without this, the reverse Kazakh mistake — typing `25` while the Kazakh layout
	// is active and getting `әғ` — could never be detected. The value sits below any real
	// word's cost, since a bare number is unambiguous.
	NumericCost = 2.0

	// MorphologyBonus is subtracted from a Kazakh candidate that is a well-formed inflected
	// word. Smaller than the transliterator's bonus, because here it competes with a
	// confidence threshold that was calibrated on trigram costs alone.
	MorphologyBonus = 0.6
)

// singleLetterWords are the only one-letter words each language has. A single letter is a
// word only if it is one of these — whatever else a dictionary lists.
var singleLetterWords = map[Lang]StringSet{
	LangEN: NewStringSet("a", "i"),
	LangRU: NewStringSet("я", "а", "и", "в", "с", "к", "у", "о"),
	LangKK: NewStringSet("а", "е", "о"),
}

// LatinOnlyWords stay in Latin: brands, apps and platform names (TranslitTables.latinOnlyWords,
// shared by the detector and the transliterator). A list rather than a model, because
// measurement showed a model cannot do this job.
var LatinOnlyWords = func() map[string]bool {
	out := map[string]bool{}
	for _, word := range []string{
		// Apps and platforms
		"instagram", "whatsapp", "telegram", "tiktok", "youtube", "facebook", "twitter",
		"linkedin", "snapchat", "viber", "zoom", "skype", "discord", "reddit", "spotify",
		"netflix", "github", "gitlab", "figma", "notion", "slack", "trello", "jira", "canva",
		"google", "gmail", "chatgpt", "claude",
		// Hardware and software
		"iphone", "ipad", "macbook", "imac", "airpods", "android", "windows", "macos",
		"linux", "chrome", "safari", "firefox", "wifi", "bluetooth", "airdrop", "icloud",
		// Point of sale and delivery systems
		"iiko", "drinkit", "poster", "rkeeper", "syrve", "wolt", "glovo", "bolt", "uber",
		// Payments
		"visa", "mastercard", "paypal", "stripe", "applepay",
		// File and web jargon
		"email", "url", "api", "pdf", "html", "css", "sql", "json", "http", "https",
	} {
		out[word] = true
	}
	return out
}()

// WordScorer renders a keystroke run under every candidate layout and prices each reading.
type WordScorer struct {
	models     *Models
	morphology *Morphology
	lexicons   map[Lang]*Lexicon
	memo       analysisMemo
}

// NewWordScorer takes the lexicons and the morphology out of models once. The Swift scorer
// reads them from its ModelStore at every use and keys its memo by the store's load
// generation, because the app builds its detector while the models are still loading in the
// background: a copy taken then stayed empty for good (a bug long since fixed). Models is never
// partly loaded — LoadModels returns it whole or not at all, winapp.Run decodes it before
// engine.New and quits without it — and it never changes afterwards, so the copy is the same
// data at every use.
func NewWordScorer(models *Models) *WordScorer {
	scorer := &WordScorer{models: models, morphology: models.KazakhMorphology(), lexicons: map[Lang]*Lexicon{}}
	for _, lang := range AllLangs {
		if lexicon := models.Lexicon(lang); lexicon != nil {
			scorer.lexicons[lang] = lexicon
		}
	}
	return scorer
}

// HasLexicons reports whether any word lexicon is loaded.
func (s *WordScorer) HasLexicons() bool { return len(s.lexicons) > 0 }

// IsNumeric reports a non-empty all-digit text.
func IsNumeric(text string) bool {
	return text != "" && AllChars(text, IsNumberChar)
}

// MARK: - Readings

// Reading reads recorded[lo:hi] under layout.
//
// priced false skips the trigram cost (left at +Inf) for callers that decide on the lexicon
// alone — pulling neighbours into a fix does, and it runs on every one of them.
func (s *WordScorer) Reading(recorded []RecordedStroke, lo, hi int, layout *KeyMapTable, lang Lang,
	learned LearnedVocabulary, priced bool) Reading {
	var text strings.Builder
	for index := lo; index < hi; index++ {
		if produced, ok := layout.Text(recorded[index].Stroke); ok {
			text.WriteString(produced)
		}
	}
	analysis := s.analyze(text.String(), lang, learned, priced)
	return Reading{
		Lang: lang, LayoutID: layout.SourceID, Lo: lo, Hi: hi, Text: text.String(),
		Core: analysis.core, Length: analysis.length, HasOddEdges: analysis.hasOddEdges,
		IsClean: analysis.isClean, IsNumeric: analysis.isNumeric, IsKnown: analysis.isKnown,
		IsCore: analysis.isCore, Cost: analysis.cost, InteriorJunk: analysis.interiorJunk,
	}
}

// analysis is everything about a reading that depends on its text alone.
type analysis struct {
	core         string
	length       int
	hasOddEdges  bool
	isClean      bool
	isNumeric    bool
	isKnown      bool
	isCore       bool
	cost         float64
	interiorJunk int
	priced       bool
}

type memoKey struct {
	lang    Lang
	version int
	text    string
}

// analysisMemo is Swift's Memo: every word end re-reads the words before it, so the same few
// texts come back again and again. Bounded, and keyed by the learned vocabulary's version so
// a word the user teaches takes effect at once.
type analysisMemo struct {
	mu      sync.Mutex
	entries map[memoKey]analysis
}

const memoCapacity = 2048

func (m *analysisMemo) value(key memoKey) (analysis, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.entries[key]
	return value, ok
}

func (m *analysisMemo) store(value analysis, key memoKey) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.entries == nil || len(m.entries) >= memoCapacity {
		m.entries = make(map[memoKey]analysis, memoCapacity)
	}
	m.entries[key] = value
}

func learnedVersion(learned LearnedVocabulary) int {
	if learned == nil {
		return -1
	}
	return learned.Version()
}

func (s *WordScorer) analyze(text string, lang Lang, learned LearnedVocabulary, priced bool) analysis {
	key := memoKey{lang: lang, version: learnedVersion(learned), text: text}
	if cached, ok := s.memo.value(key); ok && (cached.priced || !priced) {
		return cached
	}

	core := CoreOf(text)
	numeric := IsNumeric(core)
	known, frequent := true, false
	if !numeric {
		known, frequent = s.lookup(core, lang, learned)
	}
	cost := math.Inf(1)
	switch {
	case numeric:
		cost = NumericCost
	case priced:
		cost, _ = s.TrigramCost(core, lang, known)
	}
	result := analysis{
		core: core, length: CharCount(core), hasOddEdges: HasOddEdges(text), isClean: IsCleanWord(core),
		isNumeric: numeric, isKnown: known, isCore: frequent, cost: cost,
		interiorJunk: InteriorJunk(core), priced: priced || numeric,
	}
	s.memo.store(result, key)
	return result
}

// lookup gives known and frequent in one pass over the lexicon.
func (s *WordScorer) lookup(word string, lang Lang, learned LearnedVocabulary) (bool, bool) {
	if word == "" {
		return false, false
	}
	if CharCount(word) == 1 {
		isWord := singleLetterWords[lang].Contains(Lower(word))
		return isWord, isWord
	}
	if lexicon := s.lexicons[lang]; lexicon != nil {
		if known, frequent := lexicon.Lookup(word); known {
			return true, frequent
		}
	}
	return s.IsKnown(word, lang, learned), false
}

// InteriorJunk counts characters inside a word that no word of the language has there: a
// digit or a symbol between two letters, or a hyphen next to a lone letter (`к-к` is `көк`
// typed on the Russian layout, where `ө` sits on the hyphen key). A hyphen between real
// parts, as in `кто-то`, and an apostrophe, as in `don't`, are not junk.
func InteriorJunk(core string) int {
	characters := Graphemes(core)
	if len(characters) <= 2 {
		return 0
	}
	junk := 0
	for index := 1; index < len(characters)-1; index++ {
		character := characters[index]
		if IsLetterChar(character) || character == "'" || character == "’" {
			continue
		}
		if !IsLetterChar(characters[index-1]) || !IsLetterChar(characters[index+1]) {
			continue
		}
		if character == "-" {
			left := 0
			for i := index - 1; i >= 0 && IsLetterChar(characters[i]); i-- {
				left++
			}
			right := 0
			for i := index + 1; i < len(characters) && IsLetterChar(characters[i]); i++ {
				right++
			}
			if left >= 2 && right >= 2 {
				continue
			}
		}
		junk++
	}
	return junk
}

// wordBounds are the grapheme indices of the first and last word characters; ok is false
// when there are none.
func wordBounds(characters []string) (first, last int, ok bool) {
	first = -1
	for index, character := range characters {
		if IsWordCharacter(character) {
			if first < 0 {
				first = index
			}
			last = index
		}
	}
	return first, last, first >= 0
}

// CoreOf is WordScorer.core(of:): the word with surrounding punctuation removed.
func CoreOf(text string) string {
	characters := Graphemes(text)
	first, last, ok := wordBounds(characters)
	if !ok {
		return ""
	}
	return strings.Join(characters[first:last+1], "")
}

// normalLeading and normalTrailing are the punctuation ordinary prose puts right before or
// right after a word.
var (
	normalLeading  = NewStringSet("(", "[", "«", "\"", "'", "“", "‘", "„", "¿", "¡")
	normalTrailing = NewStringSet(",", ".", "!", "?", ";", ":", ")", "]", "»", "\"", "'", "”", "’", "…")
)

// HasOddEdges: punctuation around the word that prose does not put there.
func HasOddEdges(text string) bool {
	characters := Graphemes(text)
	first, last, ok := wordBounds(characters)
	if !ok {
		return false
	}
	for _, character := range characters[:first] {
		if !normalLeading.Contains(character) {
			return true
		}
	}
	for _, character := range characters[last+1:] {
		if !normalTrailing.Contains(character) {
			return true
		}
	}
	return false
}

// IsCleanWord is WordScorer.isClean: nothing but word characters and inner hyphens, and
// never two hyphens in a row.
func IsCleanWord(core string) bool {
	if core == "" {
		return false
	}
	return AllChars(core, func(c string) bool { return IsWordCharacter(c) || c == "-" }) &&
		!strings.Contains(core, "--")
}

// IsKnown reports whether the word exists. A hyphenated word counts when the dictionary has
// it whole (`x-ray`), or when every part is a word of two letters or more (`кто-то`) — `к-к`
// is not a word because `к` is.
func (s *WordScorer) IsKnown(word string, lang Lang, learned LearnedVocabulary) bool {
	if word == "" {
		return false
	}
	if s.isKnownWhole(word, lang, learned) {
		return true
	}
	parts := splitOnChar(word, "-", false)
	if len(parts) <= 1 {
		return false
	}
	for _, part := range parts {
		if CharCount(part) < 2 {
			return false
		}
	}
	for _, part := range parts {
		if !s.isKnownWhole(part, lang, learned) {
			return false
		}
	}
	return true
}

func (s *WordScorer) isKnownWhole(word string, lang Lang, learned LearnedVocabulary) bool {
	if CharCount(word) == 1 {
		return singleLetterWords[lang].Contains(Lower(word))
	}
	if lexicon := s.lexicons[lang]; lexicon != nil && (lexicon.Contains(word) || lexicon.IsCore(word)) {
		return true
	}
	if lang == LangEN && LatinOnlyWords[Canon(Lower(word))] {
		return true
	}
	return learned != nil && learned.IsKnown(word, lang)
}

// IsCore reports whether the word is among the language's most frequent.
func (s *WordScorer) IsCore(word string, lang Lang) bool {
	if CharCount(word) == 1 {
		return singleLetterWords[lang].Contains(Lower(word))
	}
	lexicon := s.lexicons[lang]
	return lexicon != nil && lexicon.IsCore(word)
}

// TrigramCost is the mean trigram cost, hyphenated parts scored separately so the hyphen
// itself is not charged as junk. Returns the count of interior junk characters alongside.
//
// A Kazakh word the lexicon already knows gets the morphology bonus without the search: a
// known form is a stem or a stem plus a suffix, which is what the search would find.
func (s *WordScorer) TrigramCost(word string, lang Lang, known bool) (float64, int) {
	model := s.models.Model(lang)
	if model == nil {
		return math.Inf(1), 0
	}
	total, weight := 0.0, 0.0
	junk := 0
	for _, part := range splitOnChar(word, "-", true) {
		score, ok := model.Score(part)
		if !ok {
			continue
		}
		length := float64(score.Length)
		// The conversions stop the compiler fusing these into FMA instructions (arm64 does):
		// Swift rounds every product, and parity is exact.
		total += float64(score.PenalizedCost() * length)
		weight += length
		junk += score.InteriorUnknownCount
	}
	if weight <= 0 {
		return math.Inf(1), junk
	}
	cost := total / weight
	if lang == LangKK && (known || (s.morphology != nil && s.morphology.Decomposes(word, DefaultMorphologyDepth))) {
		cost -= MorphologyBonus
	}
	return cost, junk
}

// splitOnChar is Swift's String.split(separator:omittingEmptySubsequences:) for a
// one-Character separator: it compares Characters, so a separator carrying a combining mark
// is not a separator.
func splitOnChar(text, separator string, omitEmpty bool) []string {
	var parts []string
	var current strings.Builder
	it := graphemeIter{s: text}
	for c := it.next(); c != ""; c = it.next() {
		if c == separator {
			if current.Len() > 0 || !omitEmpty {
				parts = append(parts, current.String())
			}
			current.Reset()
			continue
		}
		current.WriteString(c)
	}
	if current.Len() > 0 || !omitEmpty {
		parts = append(parts, current.String())
	}
	return parts
}
