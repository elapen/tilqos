package core_test

// Parity with the Swift implementation, record for record, against the golden data in
// windows/testdata/golden (dumped by windows/parity/GoldenDumpTests.swift from the real
// Swift code, the real macOS layouts and the real models).
//
// Doubles are compared exactly: the port performs the same integer sums and the same
// divisions in the same order, so any difference is a real divergence.

import (
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"tilqos/internal/core"
	"tilqos/internal/golden"
)

func sameFloat(a, b float64) bool {
	if math.IsNaN(a) || math.IsNaN(b) {
		return math.IsNaN(a) && math.IsNaN(b)
	}
	return a == b
}

// MARK: - Unicode semantics

type charRecord struct {
	C            string `json:"c"`
	IsLetter     bool   `json:"isLetter"`
	IsNumber     bool   `json:"isNumber"`
	IsUppercase  bool   `json:"isUppercase"`
	IsWord       bool   `json:"isWord"`
	IsASCII      bool   `json:"isASCII"`
	IsWhitespace bool   `json:"isWhitespace"`
	IsNewline    bool   `json:"isNewline"`
}

type stringRecord struct {
	S     string       `json:"s"`
	Count int          `json:"count"`
	Lower string       `json:"lower"`
	Upper string       `json:"upper"`
	Chars []charRecord `json:"chars"`
}

func TestUnicodeParity(t *testing.T) {
	file := golden.Load[stringRecord](t, "unicode.json")
	for _, record := range file.Records {
		if got := core.CharCount(record.S); got != record.Count {
			t.Errorf("CharCount(%q) = %d, Swift %d", record.S, got, record.Count)
		}
		if got := core.Lower(record.S); got != record.Lower {
			t.Errorf("Lower(%q) = %q, Swift %q", record.S, got, record.Lower)
		}
		if got := core.Upper(record.S); got != record.Upper {
			t.Errorf("Upper(%q) = %q, Swift %q", record.S, got, record.Upper)
		}
		clusters := core.Graphemes(record.S)
		want := make([]string, len(record.Chars))
		for i, c := range record.Chars {
			want[i] = c.C
		}
		if !reflect.DeepEqual(clusters, want) && !(len(clusters) == 0 && len(want) == 0) {
			t.Errorf("Graphemes(%q) = %q, Swift %q", record.S, clusters, want)
			continue
		}
		for _, c := range record.Chars {
			if got := core.IsLetterChar(c.C); got != c.IsLetter {
				t.Errorf("IsLetterChar(%q) = %v, Swift %v", c.C, got, c.IsLetter)
			}
			if got := core.IsNumberChar(c.C); got != c.IsNumber {
				t.Errorf("IsNumberChar(%q) = %v, Swift %v", c.C, got, c.IsNumber)
			}
			if got := core.IsUppercaseChar(c.C); got != c.IsUppercase {
				t.Errorf("IsUppercaseChar(%q) = %v, Swift %v", c.C, got, c.IsUppercase)
			}
			if got := core.IsWordCharacter(c.C); got != c.IsWord {
				t.Errorf("IsWordCharacter(%q) = %v, Swift %v", c.C, got, c.IsWord)
			}
			if got := core.IsASCIIChar(c.C); got != c.IsASCII {
				t.Errorf("IsASCIIChar(%q) = %v, Swift %v", c.C, got, c.IsASCII)
			}
			if got := core.IsWhitespaceChar(c.C); got != c.IsWhitespace {
				t.Errorf("IsWhitespaceChar(%q) = %v, Swift %v", c.C, got, c.IsWhitespace)
			}
			if got := core.IsNewlineChar(c.C); got != c.IsNewline {
				t.Errorf("IsNewlineChar(%q) = %v, Swift %v", c.C, got, c.IsNewline)
			}
		}
	}
	t.Logf("%d strings compared", len(file.Records))
}

// MARK: - Tables

func TestReverseMapParity(t *testing.T) {
	tables, _, records := golden.Tables(t)
	for _, record := range records {
		table := tables[record.Short]
		for character, want := range record.Reverse {
			stroke, ok := table.Stroke(character)
			if !ok || int(stroke.Key) != want[0] || int(stroke.Mods) != want[1] {
				t.Errorf("%s: Stroke(%q) = %v %v, Swift key %d mods %d",
					record.Short, character, stroke, ok, want[0], want[1])
			}
		}
		if table.ReverseLen() != len(record.Reverse) {
			t.Errorf("%s: reverse map has %d characters, Swift %d", record.Short, table.ReverseLen(), len(record.Reverse))
		}
	}
}

// TestWordStrokeParity checks ProducesWordCharacter for every key and modifier state against
// KeyMapTable.producesWordCharacter.
func TestWordStrokeParity(t *testing.T) {
	tables, _, records := golden.Tables(t)
	for _, record := range records {
		table := tables[record.Short]
		if len(record.Word) != table.KeyCount()*core.VariantCount {
			t.Fatalf("%s: %d word flags, want %d", record.Short, len(record.Word), table.KeyCount()*core.VariantCount)
		}
		for index, flag := range record.Word {
			stroke := core.KeyStroke{Key: uint16(index / core.VariantCount), Mods: core.ModState(index % core.VariantCount)}
			if got := table.ProducesWordCharacter(stroke); got != (flag == '1') {
				t.Errorf("%s: ProducesWordCharacter(%+v) = %v, Swift %c", record.Short, stroke, got, flag)
			}
		}
	}
}

func TestRussianAndKazakhDifferOnlyOnTheNumberRow(t *testing.T) {
	tables, _, _ := golden.Tables(t)
	got := tables["ru"].DifferingKeyCodes(tables["kk"], core.TypingVariants)
	want := []uint16{10, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 50, 93}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("differing key codes %v, want %v", got, want)
	}
}

// MARK: - Per-word model outputs

type scoreRecord struct {
	Mean      string `json:"mean"`
	Interior  int    `json:"interior"`
	Unknown   int    `json:"unknown"`
	Length    int    `json:"length"`
	Penalized string `json:"penalized"`
}

type encodeRecord struct {
	Symbols []int `json:"symbols"`
	Unknown int   `json:"unknown"`
	Covers  bool  `json:"covers"`
}

type decompRecord struct {
	Stem     string   `json:"stem"`
	Suffixes []string `json:"suffixes"`
}

type wordRecord struct {
	W          string        `json:"w"`
	Latin      encodeRecord  `json:"latin"`
	Cyrillic   encodeRecord  `json:"cyrillic"`
	EN         *scoreRecord  `json:"en"`
	RU         *scoreRecord  `json:"ru"`
	KK         *scoreRecord  `json:"kk"`
	Hash       uint32        `json:"hash"`
	IsStem     bool          `json:"isStem"`
	Decomp     *decompRecord `json:"decomp"`
	Decomp1    bool          `json:"decomp1"`
	ContainsKK bool          `json:"containsKK"`
	Numeric    bool          `json:"numeric"`
}

func TestWordParity(t *testing.T) {
	models := golden.Models(t)
	file := golden.Load[wordRecord](t, "words.json")
	morphology := models.KazakhMorphology()
	if got := strconv.Itoa(morphology.StemCount()); got != file.Header["stemCount"] {
		t.Errorf("stem count %s, Swift %s", got, file.Header["stemCount"])
	}
	if got := strconv.Itoa(morphology.SuffixCount()); got != file.Header["suffixCount"] {
		t.Errorf("suffix count %s, Swift %s", got, file.Header["suffixCount"])
	}

	checkEncode := func(word string, alphabet *core.Alphabet, want encodeRecord) {
		symbols, unknown := alphabet.Encode(word)
		if len(symbols) == 0 {
			symbols = []int{}
		}
		if !reflect.DeepEqual(symbols, want.Symbols) || unknown != want.Unknown {
			t.Errorf("Encode(%q) kind %d = %v/%d, Swift %v/%d", word, alphabet.Kind, symbols, unknown, want.Symbols, want.Unknown)
		}
		if got := alphabet.Covers(word); got != want.Covers {
			t.Errorf("Covers(%q) kind %d = %v, Swift %v", word, alphabet.Kind, got, want.Covers)
		}
	}
	checkScore := func(word string, lang core.Lang, want *scoreRecord) {
		score, ok := models.Model(lang).Score(word)
		if want == nil {
			if ok {
				t.Errorf("%s.Score(%q) = %+v, Swift nil", lang, word, score)
			}
			return
		}
		if !ok {
			t.Errorf("%s.Score(%q) = nil, Swift %+v", lang, word, *want)
			return
		}
		if !sameFloat(score.MeanCost, golden.F(t, want.Mean)) || score.InteriorUnknownCount != want.Interior ||
			score.UnknownCount != want.Unknown || score.Length != want.Length ||
			!sameFloat(score.PenalizedCost(), golden.F(t, want.Penalized)) {
			t.Errorf("%s.Score(%q) = %v/%d/%d/%d/%v, Swift %s/%d/%d/%d/%s", lang, word,
				score.MeanCost, score.InteriorUnknownCount, score.UnknownCount, score.Length, score.PenalizedCost(),
				want.Mean, want.Interior, want.Unknown, want.Length, want.Penalized)
		}
	}

	for _, record := range file.Records {
		word := record.W
		checkEncode(word, core.LatinAlphabet, record.Latin)
		checkEncode(word, core.CyrillicAlphabet, record.Cyrillic)
		checkScore(word, core.LangEN, record.EN)
		checkScore(word, core.LangRU, record.RU)
		checkScore(word, core.LangKK, record.KK)
		if got := core.MorphologyHash(word); got != record.Hash {
			t.Errorf("MorphologyHash(%q) = %d, Swift %d", word, got, record.Hash)
		}
		if got := morphology.IsStem(word); got != record.IsStem {
			t.Errorf("IsStem(%q) = %v, Swift %v", word, got, record.IsStem)
		}
		stem, suffixes, ok := morphology.Decomposition(word, core.DefaultMorphologyDepth)
		switch {
		case ok != (record.Decomp != nil):
			t.Errorf("Decomposition(%q) ok = %v, Swift %v", word, ok, record.Decomp != nil)
		case ok && (stem != record.Decomp.Stem || !reflect.DeepEqual(suffixes, record.Decomp.Suffixes)):
			t.Errorf("Decomposition(%q) = %q %q, Swift %q %q", word, stem, suffixes, record.Decomp.Stem, record.Decomp.Suffixes)
		}
		if got := morphology.Decomposes(word, 1); got != record.Decomp1 {
			t.Errorf("Decomposes(%q, 1) = %v, Swift %v", word, got, record.Decomp1)
		}
		if got := core.ContainsKazakhSpecific(word); got != record.ContainsKK {
			t.Errorf("ContainsKazakhSpecific(%q) = %v, Swift %v", word, got, record.ContainsKK)
		}
		if got := core.IsNumeric(word); got != record.Numeric {
			t.Errorf("IsNumeric(%q) = %v, Swift %v", word, got, record.Numeric)
		}
	}
	t.Logf("%d words × 3 models compared", len(file.Records))
}

// MARK: - Confidence

type confidenceRecord struct {
	Gap        string `json:"gap"`
	Confidence string `json:"confidence"`
}

func TestConfidenceParity(t *testing.T) {
	file := golden.Load[confidenceRecord](t, "confidence.json")
	for _, record := range file.Records {
		got := core.Confidence(golden.F(t, record.Gap))
		if !sameFloat(got, golden.F(t, record.Confidence)) {
			t.Errorf("Confidence(%s) = %v, Swift %s", record.Gap, got, record.Confidence)
		}
	}
}

// MARK: - Tokenizer and guards

type tokenRecord struct {
	IsWord    bool   `json:"isWord"`
	Lo        int    `json:"lo"`
	Hi        int    `json:"hi"`
	Displayed string `json:"displayed"`
}

type reasonRecord struct {
	Token    int     `json:"token"`
	Plain    *string `json:"plain"`
	Numeric  *string `json:"numeric"`
	Min2     *string `json:"min2"`
	Excepted *string `json:"excepted"`
}

type guardRecord struct {
	Segs          []golden.Segment  `json:"segs"`
	Tokens        []tokenRecord     `json:"tokens"`
	Protected     map[string]string `json:"protected"`
	Reasons       []reasonRecord    `json:"reasons"`
	InProgress    *int              `json:"inProgress"`
	LastCompleted *int              `json:"lastCompleted"`
}

func reasonString(reason core.SkipReason) *string {
	if reason == core.SkipNone {
		return nil
	}
	text := string(reason)
	return &text
}

func equalOptional(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func show(s *string) string {
	if s == nil {
		return "nil"
	}
	return *s
}

func TestGuardsAndTokenizerParity(t *testing.T) {
	tables, resolve, _ := golden.Tables(t)
	file := golden.Load[guardRecord](t, "guards.json")
	for _, record := range file.Records {
		recorded, ok := golden.Record(tables, record.Segs)
		if !ok {
			t.Errorf("cannot type %+v", record.Segs)
			continue
		}
		tokens := core.Tokenize(recorded, resolve)
		gotTokens := make([]tokenRecord, len(tokens))
		for i, token := range tokens {
			gotTokens[i] = tokenRecord{token.IsWord, token.Lo, token.Hi, token.Displayed}
		}
		if !reflect.DeepEqual(gotTokens, record.Tokens) && !(len(gotTokens) == 0 && len(record.Tokens) == 0) {
			t.Errorf("%+v: tokens %+v, Swift %+v", record.Segs, gotTokens, record.Tokens)
			continue
		}

		protected := core.ProtectedTokens(tokens)
		gotProtected := map[string]string{}
		for index, reason := range protected {
			gotProtected[strconv.Itoa(index)] = string(reason)
		}
		if !reflect.DeepEqual(gotProtected, record.Protected) {
			t.Errorf("%q: protected %v, Swift %v", joinSegs(record.Segs), gotProtected, record.Protected)
		}

		for _, want := range record.Reasons {
			token := tokens[want.Token]
			checks := []struct {
				name string
				got  core.SkipReason
				want *string
			}{
				{"plain", core.WordLevelReason(token, 3, core.NewStringSet(), false), want.Plain},
				{"numeric", core.WordLevelReason(token, 3, core.NewStringSet(), true), want.Numeric},
				{"min2", core.WordLevelReason(token, 2, core.NewStringSet(), false), want.Min2},
				{"excepted", core.WordLevelReason(token, 3, core.NewStringSet("ghbdtn", "привет", "hello"), false), want.Excepted},
			}
			for _, check := range checks {
				if got := reasonString(check.got); !equalOptional(got, check.want) {
					t.Errorf("%q token %q %s: %s, Swift %s", joinSegs(record.Segs), token.Displayed, check.name, show(got), show(check.want))
				}
			}
		}

		gotInProgress := -1
		if word, ok := core.WordInProgress(tokens); ok {
			gotInProgress = indexOfToken(tokens, word)
		}
		if want := optionalIndex(record.InProgress); gotInProgress != want {
			t.Errorf("%q: WordInProgress %d, Swift %d", joinSegs(record.Segs), gotInProgress, want)
		}
		gotLast := -1
		if word, ok := core.LastCompletedWord(tokens); ok {
			gotLast = indexOfToken(tokens, word)
		}
		if want := optionalIndex(record.LastCompleted); gotLast != want {
			t.Errorf("%q: LastCompletedWord %d, Swift %d", joinSegs(record.Segs), gotLast, want)
		}
	}
	t.Logf("%d guard cases compared", len(file.Records))
}

func indexOfToken(tokens []core.Token, token core.Token) int {
	for i, candidate := range tokens {
		if candidate.Lo == token.Lo && candidate.Hi == token.Hi {
			return i
		}
	}
	return -1
}

func optionalIndex(value *int) int {
	if value == nil {
		return -1
	}
	return *value
}

func joinSegs(segments []golden.Segment) string {
	var b strings.Builder
	for _, segment := range segments {
		b.WriteString(segment.Text)
	}
	return b.String()
}

// MARK: - Detector

type correctionRecord struct {
	Lo     int    `json:"lo"`
	Hi     int    `json:"hi"`
	Layout string `json:"layout"`
}

type changeRecord struct {
	Typed      string `json:"typed"`
	Corrected  string `json:"corrected"`
	TypedLang  string `json:"typedLang"`
	TargetLang string `json:"targetLang"`
}

type outcomeRecord struct {
	Kind         string             `json:"kind"`
	Confidence   *string            `json:"confidence"`
	Before       *string            `json:"before"`
	After        *string            `json:"after"`
	Deleted      *string            `json:"deleted"`
	Insert       *string            `json:"insert"`
	Corrections  []correctionRecord `json:"corrections"`
	TargetLang   *string            `json:"targetLang"`
	TargetLayout *string            `json:"targetLayout"`
	Changes      []changeRecord     `json:"changes"`
	WordRanges   [][]int            `json:"wordRanges"`
}

type sessionRecord struct {
	Segs       []golden.Segment `json:"segs"`
	Layouts    []string         `json:"layouts"`
	MinLen     int              `json:"minLen"`
	Exceptions []string         `json:"exceptions"`
	Scope      string           `json:"scope"`
	Manual     bool             `json:"manual"`
	Auto       string           `json:"auto"`
	Suggest    string           `json:"suggest"`
	Preceding  *string          `json:"preceding"`
	Taught     bool             `json:"taught"`
	Result     outcomeRecord    `json:"result"`
}

// taughtVocabulary is the dump's Taught class: fixed decisions and user words, keyed by the
// lowercased word.
type taughtVocabulary struct {
	decisions map[string]core.LearnedDecision
	known     map[core.Lang]core.StringSet
	version   int
}

func (v *taughtVocabulary) Version() int { return v.version }

func (v *taughtVocabulary) IsKnown(word string, lang core.Lang) bool {
	return v.known[lang].Contains(core.Lower(word))
}

func (v *taughtVocabulary) Decision(typed string) (core.LearnedDecision, bool) {
	decision, ok := v.decisions[core.Canon(core.Lower(typed))]
	return decision, ok
}

// parseTaught rebuilds the Taught set from the detector.json header.
func parseTaught(t *testing.T, header map[string]string) *taughtVocabulary {
	t.Helper()
	taught := &taughtVocabulary{decisions: map[string]core.LearnedDecision{}, known: map[core.Lang]core.StringSet{}}
	version, err := strconv.Atoi(header["taughtVersion"])
	if err != nil {
		t.Fatalf("taughtVersion: %v", err)
	}
	taught.version = version
	for _, item := range strings.Split(header["taughtDecisions"], ";") {
		word, decision, ok := strings.Cut(item, "=")
		if !ok {
			t.Fatalf("bad taught decision %q", item)
		}
		if decision == "keep" {
			taught.decisions[core.Canon(word)] = core.LearnedDecision{Kind: core.DecisionKeep}
		} else {
			taught.decisions[core.Canon(word)] = core.LearnedDecision{Kind: core.DecisionConvert, Lang: core.Lang(decision)}
		}
	}
	for _, item := range strings.Split(header["taughtKnown"], ";") {
		lang, words, ok := strings.Cut(item, "=")
		if !ok {
			t.Fatalf("bad taught words %q", item)
		}
		taught.known[core.Lang(lang)] = core.NewStringSet(strings.Fields(words)...)
	}
	return taught
}

func describeAction(action core.Action) string {
	if action.Outcome == nil {
		return action.Kind.String()
	}
	o := action.Outcome
	return fmt.Sprintf("%s conf=%v %q→%q del=%q ins=%q corr=%v lang=%s layout=%s changes=%v words=%v", action.Kind,
		o.Confidence, o.Before, o.After, o.Replacement.DeletedText, o.Replacement.InsertText, o.Corrections,
		o.TargetLang, o.TargetLayoutID, o.Changes, o.WordRanges)
}

func describeRecord(r outcomeRecord) string {
	if r.Kind == "none" {
		return "none"
	}
	return fmt.Sprintf("%s conf=%s %q→%q del=%q ins=%q corr=%v lang=%s layout=%s changes=%v words=%v", r.Kind,
		show(r.Confidence), show(r.Before), show(r.After), show(r.Deleted), show(r.Insert), r.Corrections,
		show(r.TargetLang), show(r.TargetLayout), r.Changes, r.WordRanges)
}

func actionMatches(t *testing.T, action core.Action, want outcomeRecord) bool {
	if action.Kind.String() != want.Kind {
		return false
	}
	if action.Outcome == nil {
		return want.Kind == "none"
	}
	o := action.Outcome
	if want.Confidence == nil || !sameFloat(o.Confidence, golden.F(t, *want.Confidence)) {
		return false
	}
	if show(want.Before) != o.Before || show(want.After) != o.After ||
		show(want.Deleted) != o.Replacement.DeletedText || show(want.Insert) != o.Replacement.InsertText ||
		show(want.TargetLang) != string(o.TargetLang) || show(want.TargetLayout) != o.TargetLayoutID {
		return false
	}
	if len(o.Corrections) != len(want.Corrections) || len(o.Changes) != len(want.Changes) ||
		len(o.WordRanges) != len(want.WordRanges) {
		return false
	}
	for i, correction := range o.Corrections {
		w := want.Corrections[i]
		if correction.Lo != w.Lo || correction.Hi != w.Hi || correction.TargetLayoutID != w.Layout {
			return false
		}
	}
	for i, change := range o.Changes {
		w := want.Changes[i]
		if change.Typed != w.Typed || change.Corrected != w.Corrected || string(change.TypedLang) != w.TypedLang ||
			string(change.TargetLang) != w.TargetLang {
			return false
		}
	}
	for i, r := range o.WordRanges {
		w := want.WordRanges[i]
		if len(w) != 2 || r[0] != w[0] || r[1] != w[1] {
			return false
		}
	}
	return true
}

func TestDetectorParity(t *testing.T) {
	models := golden.Models(t)
	tables, resolve, _ := golden.Tables(t)
	detector := core.NewDetector(models)
	file := golden.Load[sessionRecord](t, "detector.json")
	taught := parseTaught(t, file.Header)

	counts := map[string]int{}
	mismatches := 0
	for _, record := range file.Records {
		recorded, ok := golden.Record(tables, record.Segs)
		if !ok {
			t.Errorf("cannot type %+v", record.Segs)
			continue
		}
		layouts := make([]*core.KeyMapTable, len(record.Layouts))
		for i, short := range record.Layouts {
			layouts[i] = tables[short]
		}
		context := core.NewContext(recorded, layouts, resolve)
		context.MinWordLength = record.MinLen
		context.Exceptions = core.NewStringSet(record.Exceptions...)
		context.AutoThreshold = golden.F(t, record.Auto)
		context.SuggestThreshold = golden.F(t, record.Suggest)
		context.Manual = record.Manual
		if record.Preceding != nil {
			context.PrecedingText = *record.Preceding
		}
		if record.Taught {
			context.Learned = taught
		}
		scope := core.ScopeTail
		if record.Scope == "lastWord" {
			scope = core.ScopeLastWord
		}
		action := detector.Evaluate(context, scope)
		counts[record.Result.Kind]++
		if !actionMatches(t, action, record.Result) {
			mismatches++
			if mismatches <= 40 {
				t.Errorf("%q (%s, layouts %v, manual %v, preceding %q, taught %v, thresholds %s/%s):\n  Go    %s\n  Swift %s",
					joinSegs(record.Segs), record.Scope, record.Layouts, record.Manual, show(record.Preceding),
					record.Taught, record.Auto, record.Suggest, describeAction(action), describeRecord(record.Result))
			}
		}
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var summary []string
	for _, key := range keys {
		summary = append(summary, fmt.Sprintf("%s=%d", key, counts[key]))
	}
	if mismatches > 0 {
		t.Errorf("%d of %d detector sessions differ from Swift", mismatches, len(file.Records))
	}
	t.Logf("%d detector sessions compared (%s), %d mismatches", len(file.Records), strings.Join(summary, " "), mismatches)
}

// MARK: - Text converter

type convertRecord struct {
	Text string `json:"text"`
	From string `json:"from"`
	To   string `json:"to"`
	Out  string `json:"out"`
}

type bestRecord struct {
	Text   string  `json:"text"`
	From   string  `json:"from"`
	Layout *string `json:"layout"`
	Out    *string `json:"out"`
}

type costRecord struct {
	Text string  `json:"text"`
	Lang string  `json:"lang"`
	Cost *string `json:"cost"`
}

func TestTextConverterParity(t *testing.T) {
	models := golden.Models(t)
	tables, _, _ := golden.Tables(t)
	all := []*core.KeyMapTable{tables["abc"], tables["ru"], tables["kk"]}

	for _, record := range golden.Load[convertRecord](t, "textconv-convert.json").Records {
		if got := core.ConvertText(record.Text, tables[record.From], tables[record.To]); got != record.Out {
			t.Errorf("ConvertText(%q, %s→%s) = %q, Swift %q", record.Text, record.From, record.To, got, record.Out)
		}
	}
	for _, record := range golden.Load[bestRecord](t, "textconv-best.json").Records {
		best, ok := core.BestConversion(record.Text, tables[record.From], all, models)
		switch {
		case ok != (record.Layout != nil):
			t.Errorf("BestConversion(%q from %s) found=%v, Swift %v", record.Text, record.From, ok, record.Layout != nil)
		case ok && (best.Layout.SourceID != *record.Layout || best.Text != *record.Out):
			t.Errorf("BestConversion(%q from %s) = %s %q, Swift %s %q", record.Text, record.From, best.Layout.SourceID, best.Text, *record.Layout, *record.Out)
		}
	}
	for _, record := range golden.Load[costRecord](t, "textconv-cost.json").Records {
		cost, ok := core.PassageCost(record.Text, core.Lang(record.Lang), models)
		switch {
		case ok != (record.Cost != nil):
			t.Errorf("PassageCost(%q, %s) ok=%v, Swift %v", record.Text, record.Lang, ok, record.Cost != nil)
		case ok && !sameFloat(cost, golden.F(t, *record.Cost)):
			t.Errorf("PassageCost(%q, %s) = %v, Swift %s", record.Text, record.Lang, cost, *record.Cost)
		}
	}
}
