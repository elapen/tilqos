package translit_test

import (
	"math"
	"reflect"
	"strings"
	"testing"

	"tilqos/internal/core"
	"tilqos/internal/golden"
	"tilqos/internal/translit"
)

type readingRecord struct {
	Lang string `json:"lang"`
	Text string `json:"text"`
	Cost string `json:"cost"`
}

type translitRecord struct {
	Input               string          `json:"input"`
	Direction           string          `json:"direction"`
	ToCyrillicPractical string          `json:"toCyrillicPractical"`
	ToCyrillicOfficial  string          `json:"toCyrillicOfficial"`
	ToLatinPractical    string          `json:"toLatinPractical"`
	ToLatinOfficial     string          `json:"toLatinOfficial"`
	Readings            []readingRecord `json:"readings"`
	LatinOnly           bool            `json:"latinOnly"`
}

// TestTranslitParity compares every direction and scheme, the raw readings with their
// costs (exactly), direction detection and the brand list against the Swift dump.
func TestTranslitParity(t *testing.T) {
	models := golden.Models(t)
	tr := translit.New(models, nil, nil)
	file := golden.Load[translitRecord](t, "translit.json")
	for _, r := range file.Records {
		if got := translit.DirectionFor(r.Input).String(); got != r.Direction {
			t.Errorf("DirectionFor(%q) = %s, Swift %s", r.Input, got, r.Direction)
		}
		checks := []struct{ name, got, want string }{
			{"toCyrillic practical", tr.ToCyrillic(r.Input, translit.Practical), r.ToCyrillicPractical},
			{"toCyrillic official", tr.ToCyrillic(r.Input, translit.Official2021), r.ToCyrillicOfficial},
			{"toLatin practical", tr.ToLatin(r.Input, translit.Practical), r.ToLatinPractical},
			{"toLatin official", tr.ToLatin(r.Input, translit.Official2021), r.ToLatinOfficial},
		}
		for _, c := range checks {
			if c.got != c.want {
				t.Errorf("%s(%q) = %q, Swift %q", c.name, r.Input, c.got, c.want)
			}
		}
		readings := tr.Readings(r.Input)
		if len(readings) != len(r.Readings) {
			t.Errorf("Readings(%q): %d, Swift %d", r.Input, len(readings), len(r.Readings))
		} else {
			for i, reading := range readings {
				want := r.Readings[i]
				if string(reading.Lang) != want.Lang || reading.Text != want.Text || !sameFloat(reading.Cost, golden.F(t, want.Cost)) {
					t.Errorf("Readings(%q)[%d] = %s %q %v, Swift %s %q %s", r.Input, i, reading.Lang, reading.Text, reading.Cost, want.Lang, want.Text, want.Cost)
				}
			}
		}
		if got := translit.IsLatinOnly(r.Input, nil); got != r.LatinOnly {
			t.Errorf("IsLatinOnly(%q) = %v, Swift %v", r.Input, got, r.LatinOnly)
		}
	}
	t.Logf("%d transliteration inputs compared", len(file.Records))
}

func sameFloat(a, b float64) bool {
	if math.IsNaN(a) || math.IsNaN(b) {
		return math.IsNaN(a) && math.IsNaN(b)
	}
	return a == b
}

type learnedRecord struct {
	Never     []string          `json:"never"`
	Overrides map[string]string `json:"overrides"`
	Input     string            `json:"input"`
	Output    string            `json:"output"`
}

func TestLearnedWordsParity(t *testing.T) {
	models := golden.Models(t)
	for _, r := range golden.Load[learnedRecord](t, "translit-learned.json").Records {
		tr := translit.New(models, r.Overrides, r.Never)
		if got := tr.ToCyrillic(r.Input, translit.Practical); got != r.Output {
			t.Errorf("never=%v overrides=%v: ToCyrillic(%q) = %q, Swift %q", r.Never, r.Overrides, r.Input, got, r.Output)
		}
	}
}

type changedRecord struct {
	Before  string   `json:"before"`
	After   string   `json:"after"`
	Changed []string `json:"changed"`
}

func TestChangedWordsParity(t *testing.T) {
	tr := translit.New(golden.Models(t), nil, nil)
	for _, r := range golden.Load[changedRecord](t, "translit-changed.json").Records {
		if got := tr.ChangedWords(r.Before, r.After); !reflect.DeepEqual(got, r.Changed) {
			t.Errorf("ChangedWords(%q, %q) = %q, Swift %q", r.Before, r.After, got, r.Changed)
		}
	}
}

// MARK: - The Swift TransliteratorTests, ported

var sample = []struct{ latin, cyrillic string }{
	{"qalaisyz", "қалайсыз"}, {"qai", "қай"}, {"jylgysyz", "жылғысыз"}, {"kim", "кім"},
	{"qalai", "қалай"}, {"dese", "десе"}, {"magan", "маған"}, {"baribir", "бәрібір"},
	{"eken", "екен"}, {"siz", "сіз"}, {"bilmeisiz", "білмейсіз"}, {"goi", "ғой"},
	{"menin", "менің"}, {"ishimde", "ішімде"}, {"bolyp", "болып"}, {"jatqanyn", "жатқанын"},
	{"aitqan", "айтқан"}, {"edi", "еді"}, {"koz", "көз"}, {"kore", "көре"},
	{"uirenedi", "үйренеді"}, {"dep", "деп"}, {"alde", "әлде"}, {"olai", "олай"},
	{"aitpady", "айтпады"}, {"ony", "оны"}, {"bilmeidi", "білмейді"}, {"ekenmin", "екенмін"},
	{"jylqy", "жылқы"}, {"qysta", "қыста"}, {"jazda", "жазда"}, {"qoigan", "қойған"},
	{"apai", "апай"}, {"sanyrau", "саңырау"}, {"qalgan", "қалған"}, {"qaharman", "қаһарман"},
	{"ulken", "үлкен"}, {"tulga", "тұлға"}, {"bolamyn", "боламын"}, {"desenizder", "десеңіздер"},
	{"kop", "көп"}, {"enbek", "еңбек"}, {"etu", "ету"}, {"kerek", "керек"},
}

func TestCyrillicToPracticalMatchesTheUsersSpelling(t *testing.T) {
	tr := translit.New(golden.Models(t), nil, nil)
	for _, entry := range sample {
		if got := tr.ToLatin(entry.cyrillic, translit.Practical); got != entry.latin {
			t.Errorf("%s: expected %s, got %s", entry.cyrillic, entry.latin, got)
		}
	}
	for in, want := range map[string]string{"Ұрғашы": "Urgashy", "Шерхан": "Sherhan", "ҚАЗАҚСТАН": "QAZAQSTAN"} {
		if got := tr.ToLatin(in, translit.Practical); got != want {
			t.Errorf("ToLatin(%s) = %s, want %s", in, got, want)
		}
	}
}

func TestOfficialAlphabetRoundTrips(t *testing.T) {
	tr := translit.New(golden.Models(t), nil, nil)
	cyrillic := "аәбвгғдежзиклқлмнңоөпрстуұүфхшыі"
	if got := tr.ToCyrillic(tr.ToLatin(cyrillic, translit.Official2021), translit.Official2021); got != cyrillic {
		t.Fatalf("round trip: %s", got)
	}
	if got := tr.ToLatin("әғңөұүш", translit.Official2021); got != "äğñöūüş" {
		t.Fatalf("decreed letters: %s", got)
	}
	if got := tr.ToCyrillic("qalaisyz", translit.Official2021); got != "қалайсыз" {
		t.Fatalf("i versus yod: %s", got)
	}
}

func TestPracticalDecodingAccuracy(t *testing.T) {
	tr := translit.New(golden.Models(t), nil, nil)
	correct := 0
	var misses []string
	for _, entry := range sample {
		if got := tr.ToCyrillic(entry.latin, translit.Practical); got == entry.cyrillic {
			correct++
		} else {
			misses = append(misses, entry.latin+" → "+got)
		}
	}
	rate := float64(correct) / float64(len(sample))
	t.Logf("practical Latin → Cyrillic: %d/%d (%.0f%%) misses: %s", correct, len(sample), rate*100, strings.Join(misses, ", "))
	if rate <= 0.75 {
		t.Fatal("decoding regressed below 75% of the sample")
	}
}

func TestHarmonyAndMixedText(t *testing.T) {
	tr := translit.New(golden.Models(t), nil, nil)
	for in, want := range map[string]string{
		"ulken": "үлкен", "tulga": "тұлға", "koz": "көз", "bolyp": "болып", "magan": "маған",
		"qaharman": "қаһарман", "Sherhan": "Шерхан", "v zale": "в зале", "(Пример)": "(Пример)",
		"zzxq": "zzxq", "whatsapp": "whatsapp", "Instagram": "Instagram",
	} {
		if got := tr.ToCyrillic(in, translit.Practical); got != want {
			t.Errorf("ToCyrillic(%q) = %q, want %q", in, got, want)
		}
	}
	for _, word := range []string{"Zametki", "Spisok", "Pokupki", "Papka", "kofe", "Poisk", "Pochta"} {
		if out := tr.ToCyrillic(word, translit.Practical); core.ContainsKazakhSpecific(out) {
			t.Errorf("%s → %s contains Kazakh-only letters", word, out)
		}
	}
	if translit.DirectionFor("qalaisyz aga") != translit.ToCyrillic || translit.DirectionFor("қалайсыз аға") != translit.ToLatin {
		t.Fatal("direction detection")
	}
}
