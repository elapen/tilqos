package core_test

// Parity with Lexicon.swift and WordScorer.swift, plus the LexiconTests expectations.

import (
	"encoding/base64"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"tilqos/internal/core"
	"tilqos/internal/golden"
)

type lexiconQueryRecord struct {
	W        string `json:"w"`
	Contains bool   `json:"contains"`
	Core     bool   `json:"core"`
	Lookup   []bool `json:"lookup"`
}

type syntheticLexiconRecord struct {
	Name    string               `json:"name"`
	Lang    string               `json:"lang"`
	Data    string               `json:"data"`
	Stems   int                  `json:"stems"`
	Rules   int                  `json:"rules"`
	Cores   int                  `json:"cores"`
	Queries []lexiconQueryRecord `json:"queries"`
}

func syntheticLexicons(t *testing.T) map[string]*core.Lexicon {
	t.Helper()
	out := map[string]*core.Lexicon{}
	for _, record := range golden.Load[syntheticLexiconRecord](t, "lexicon-synthetic.json").Records {
		data, err := base64.StdEncoding.DecodeString(record.Data)
		if err != nil {
			t.Fatalf("%s: %v", record.Name, err)
		}
		lexicon, err := core.DecodeLexicon(data)
		if err != nil {
			t.Fatalf("%s: DecodeLexicon: %v", record.Name, err)
		}
		out[record.Name] = lexicon
	}
	return out
}

// TestSyntheticLexiconParity decodes lexicons the Swift builder encoded from the LexiconTests
// dictionary (every rule kind visible) and asks them what Swift asked.
func TestSyntheticLexiconParity(t *testing.T) {
	lexicons := syntheticLexicons(t)
	for _, record := range golden.Load[syntheticLexiconRecord](t, "lexicon-synthetic.json").Records {
		lexicon := lexicons[record.Name]
		if string(lexicon.Lang) != record.Lang || lexicon.StemCount() != record.Stems ||
			lexicon.RuleCount() != record.Rules || lexicon.CoreCount() != record.Cores {
			t.Errorf("%s: lang %s stems %d rules %d core %d, Swift %s %d %d %d", record.Name, lexicon.Lang,
				lexicon.StemCount(), lexicon.RuleCount(), lexicon.CoreCount(), record.Lang, record.Stems, record.Rules, record.Cores)
		}
		for _, query := range record.Queries {
			known, frequent := lexicon.Lookup(query.W)
			if lexicon.Contains(query.W) != query.Contains || lexicon.IsCore(query.W) != query.Core ||
				known != query.Lookup[0] || frequent != query.Lookup[1] {
				t.Errorf("%s: %q contains=%v core=%v lookup=%v/%v, Swift %v %v %v", record.Name, query.W,
					lexicon.Contains(query.W), lexicon.IsCore(query.W), known, frequent, query.Contains, query.Core, query.Lookup)
			}
		}
	}
}

// The LexiconTests expectations on the synthetic dictionary, stated directly.
func TestLexiconAnalyzer(t *testing.T) {
	lexicons := syntheticLexicons(t)
	plain, extra := lexicons["plain"], lexicons["extra"]
	expect := func(lexicon *core.Lexicon, word string, want bool, why string) {
		t.Helper()
		if got := lexicon.Contains(word); got != want {
			t.Errorf("Contains(%q) = %v, want %v (%s)", word, got, want, why)
		}
	}
	for _, word := range []string{"happy", "try", "toy", "play", "боль", "стол"} {
		expect(plain, word, true, "stems are words")
	}
	expect(plain, "happ", false, "not a stem")
	expect(plain, "", false, "empty")
	expect(plain, "tries", true, "y → ies after a consonant")
	expect(plain, "toys", true, "plain s after a vowel")
	expect(plain, "tryies", false, "strip must apply")
	expect(plain, "toies", false, "the condition [^aeiou]y must reject toy")
	expect(plain, "tried", false, "try has no rule producing tried")
	expect(plain, "played", true, "ed after a non-e")
	expect(plain, "tryed", true, "the literal rule output")
	expect(plain, "болю", true, "ь → ю")
	expect(plain, "столом", true, "0 → ом after [^ь]")
	expect(plain, "столю", false, "condition ь")
	expect(plain, "больом", false, "condition [^ь]")
	expect(plain, "happies", false, "happy has only U")
	expect(plain, "happys", false, "happy has only U")
	expect(plain, "unhappy", true, "prefix")
	expect(plain, "unplay", true, "prefix")
	expect(plain, "unplays", true, "U and S cross")
	expect(plain, "unplayed", false, "D is not cross-product")
	expect(plain, "untoy", false, "toy has no U")

	normalization := lexicons["normalization"]
	for _, word := range []string{"еще", "ЕЩЁ", "don\u2019t"} {
		expect(normalization, word, true, "case, ё and apostrophes fold")
	}

	expect(extra, "iPhone", true, "extra words")
	if !extra.IsCore("try") || extra.IsCore("play") {
		t.Errorf("core tier: try %v play %v", extra.IsCore("try"), extra.IsCore("play"))
	}
	if known, frequent := extra.Lookup("toy"); !known || !frequent {
		t.Errorf("Lookup(toy) = %v %v", known, frequent)
	}
	if known, frequent := extra.Lookup("toys"); !known || frequent {
		t.Errorf("Lookup(toys) = %v %v", known, frequent)
	}
}

func TestLexiconGarbageIsRejected(t *testing.T) {
	if _, err := core.DecodeLexicon([]byte{1, 2, 3}); err == nil {
		t.Error("three bytes decoded")
	}
	if _, err := core.DecodeLexicon(make([]byte, 64)); err == nil {
		t.Error("zeros decoded")
	}
	var plain string
	for _, record := range golden.Load[syntheticLexiconRecord](t, "lexicon-synthetic.json").Records {
		if record.Name == "plain" {
			plain = record.Data
		}
	}
	data, _ := base64.StdEncoding.DecodeString(plain)
	if _, err := core.DecodeLexicon(data[:len(data)-10]); err == nil {
		t.Error("truncated lexicon decoded")
	}
}

type lexiconWordRecord struct {
	W     string `json:"w"`
	Norm  string `json:"norm"`
	Hash  uint32 `json:"hash"`
	EN    []bool `json:"en"`
	RU    []bool `json:"ru"`
	KK    []bool `json:"kk"`
	Known []bool `json:"known"`
	Core  []bool `json:"core"`
}

// TestShippedLexiconParity asks the shipped lexicons about every word the dump knows.
func TestShippedLexiconParity(t *testing.T) {
	models := golden.Models(t)
	scorer := core.NewWordScorer(models)
	file := golden.Load[lexiconWordRecord](t, "lexicon.json")
	for _, lang := range core.AllLangs {
		lexicon := models.Lexicon(lang)
		got := fmt.Sprintf("%d,%d,%d", lexicon.StemCount(), lexicon.RuleCount(), lexicon.CoreCount())
		if got != file.Header[string(lang)] {
			t.Errorf("%s lexicon stems,rules,core = %s, Swift %s", lang, got, file.Header[string(lang)])
		}
	}
	if file.Header["langs"] != "en,ru,kk" {
		t.Fatalf("language order %q", file.Header["langs"])
	}
	for _, record := range file.Records {
		if got := string(core.NormalizeWord(record.W)); got != record.Norm {
			t.Errorf("NormalizeWord(%q) = %q, Swift %q", record.W, got, record.Norm)
		}
		if got := core.LexiconHash(core.NormalizeWord(record.W)); got != record.Hash {
			t.Errorf("LexiconHash(%q) = %d, Swift %d", record.W, got, record.Hash)
		}
		for i, lang := range core.AllLangs {
			want := [][]bool{record.EN, record.RU, record.KK}[i]
			known, frequent := models.Lexicon(lang).Lookup(record.W)
			if known != want[0] || frequent != want[1] {
				t.Errorf("%s.Lookup(%q) = %v/%v, Swift %v", lang, record.W, known, frequent, want)
			}
			if got := scorer.IsKnown(record.W, lang, nil); got != record.Known[i] {
				t.Errorf("scorer.IsKnown(%q, %s) = %v, Swift %v", record.W, lang, got, record.Known[i])
			}
			if got := scorer.IsCore(record.W, lang); got != record.Core[i] {
				t.Errorf("scorer.IsCore(%q, %s) = %v, Swift %v", record.W, lang, got, record.Core[i])
			}
		}
	}
	t.Logf("%d words × 3 lexicons compared", len(file.Records))
}

// The shipped-lexicon expectations of LexiconTests.
func TestShippedLexicons(t *testing.T) {
	models := golden.Models(t)
	check := func(lang core.Lang, words []string, want bool, what string) {
		t.Helper()
		for _, word := range words {
			if got := models.Lexicon(lang).Contains(word); got != want {
				t.Errorf("%s %s: Contains(%q) = %v", lang, what, word, got)
			}
		}
	}
	check(core.LangRU, []string{"еще", "ещё", "сделал", "сделала", "людей", "книгу", "где", "тот", "тут",
		"мне", "пришёл", "было", "тебе", "люблю", "хорошо"}, true, "inflected forms")
	for _, word := range []string{"еще", "как", "что", "это", "где"} {
		if !models.Lexicon(core.LangRU).IsCore(word) {
			t.Errorf("ru core: %q", word)
		}
	}
	check(core.LangEN, []string{"the", "played", "running", "don't", "unhappy", "telegram", "iphone", "email",
		"github", "afk", "lol"}, true, "forms and terms")
	check(core.LangKK, []string{"сәлеметсіз", "қазақ", "жақсы", "бұл", "және", "көп", "үшін", "мектептер"}, true, "forms")
	check(core.LangEN, []string{"ghbdtn", "rfr", "xnj", "ntcn", "ltkf", "cgfcb,j", "vyt", "ytn"}, false, "garble")
	check(core.LangRU, []string{"руддщ", "цщкдв", "лунищфкв", "еуые", "нщг", "тще"}, false, "garble")

	scorer := core.NewWordScorer(models)
	for word, want := range map[string]bool{"кто-то": true, "из-за": true, "к-к": false, "-те": false} {
		if got := scorer.IsKnown(word, core.LangRU, nil); got != want {
			t.Errorf("hyphenated %q: %v, want %v", word, got, want)
		}
	}
}

// MARK: - Readings

type readingFieldsRecord struct {
	Layout  string `json:"layout"`
	Lang    string `json:"lang"`
	Text    string `json:"text"`
	Core    string `json:"core"`
	Length  int    `json:"length"`
	Odd     bool   `json:"odd"`
	Clean   bool   `json:"clean"`
	Numeric bool   `json:"numeric"`
	Known   bool   `json:"known"`
	IsCore  bool   `json:"isCore"`
	Cost    string `json:"cost"`
	Junk    int    `json:"junk"`
}

type readingsRecord struct {
	Text     string                `json:"text"`
	On       string                `json:"on"`
	Readings []readingFieldsRecord `json:"readings"`
}

// TestReadingParity compares WordScorer.reading field for field: every text of the dump,
// typed on the first layout that can type it, read under all three layouts.
func TestReadingParity(t *testing.T) {
	models := golden.Models(t)
	tables, _, _ := golden.Tables(t)
	scorer := core.NewWordScorer(models)
	file := golden.Load[readingsRecord](t, "readings.json")
	compared := 0
	for _, record := range file.Records {
		recorded, ok := golden.Record(tables, []golden.Segment{{Text: record.Text, On: record.On, Active: record.On}})
		if !ok {
			t.Errorf("cannot type %q on %s", record.Text, record.On)
			continue
		}
		for _, want := range record.Readings {
			table := tables[want.Layout]
			got := scorer.Reading(recorded, 0, len(recorded), table, table.Lang, nil, true)
			gotFields := readingFieldsRecord{
				Layout: want.Layout, Lang: string(got.Lang), Text: got.Text, Core: got.Core, Length: got.Length,
				Odd: got.HasOddEdges, Clean: got.IsClean, Numeric: got.IsNumeric, Known: got.IsKnown,
				IsCore: got.IsCore, Cost: want.Cost, Junk: got.InteriorJunk,
			}
			if !reflect.DeepEqual(gotFields, want) || !sameFloat(got.Cost, golden.F(t, want.Cost)) {
				t.Errorf("reading of %q (typed on %s) under %s:\n  Go    %+v cost=%v\n  Swift %+v", record.Text, record.On,
					want.Layout, gotFields, got.Cost, want)
			}
			compared++
		}
	}
	t.Logf("%d readings compared", compared)
}

// MARK: - Learner hashes

type learnerHashRecord struct {
	W    string `json:"w"`
	Hash string `json:"hash"`
}

func TestLearnerHashParity(t *testing.T) {
	file := golden.Load[learnerHashRecord](t, "learner.json")
	salt := make([]byte, 0, 8)
	for i := 0; i+2 <= len(file.Header["salt"]); i += 2 {
		value, err := strconv.ParseUint(file.Header["salt"][i:i+2], 16, 8)
		if err != nil {
			t.Fatalf("salt: %v", err)
		}
		salt = append(salt, byte(value))
	}
	learner := core.NewLearner(nil)
	learner.SetSaltForTesting(salt)
	for _, record := range file.Records {
		if got := fmt.Sprintf("%016x", learner.Hash(record.W)); got != record.Hash {
			t.Errorf("Hash(%q) = %s, Swift %s", record.W, got, record.Hash)
		}
	}
}

// MARK: - Replacement helpers

type chunksRecord struct {
	Text   string   `json:"text"`
	Limit  int      `json:"limit"`
	Chunks []string `json:"chunks"`
}

type capitalRecord struct {
	Screen     string `json:"screen"`
	Deleted    string `json:"deleted"`
	Insert     string `json:"insert"`
	OutDeleted string `json:"outDeleted"`
	OutInsert  string `json:"outInsert"`
}

func TestChunksAndCapitalParity(t *testing.T) {
	for _, record := range golden.Load[chunksRecord](t, "chunks.json").Records {
		got := core.Chunks(record.Text, record.Limit)
		if strings.Join(got, "\x00") != strings.Join(record.Chunks, "\x00") || len(got) != len(record.Chunks) {
			t.Errorf("Chunks(%q, %d) = %q, Swift %q", record.Text, record.Limit, got, record.Chunks)
		}
	}
	for _, record := range golden.Load[capitalRecord](t, "capital.json").Records {
		got := core.CarryingCapital(record.Screen, core.Replacement{DeletedText: record.Deleted, InsertText: record.Insert})
		if got.DeletedText != record.OutDeleted || got.InsertText != record.OutInsert {
			t.Errorf("CarryingCapital(%q, %q→%q) = %q→%q, Swift %q→%q", record.Screen, record.Deleted, record.Insert,
				got.DeletedText, got.InsertText, record.OutDeleted, record.OutInsert)
		}
	}
}
