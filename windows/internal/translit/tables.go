// Package translit is the port of Sources/TilqosKit/Translit: Kazakh Latin ⇄
// Cyrillic transliteration, with the practical (ASCII) direction decoded by a beam search
// over the Kazakh and Russian trigram models, vowel harmony and morphology.
//
//	TranslitScheme.swift → tables.go
//	Transliterator.swift → transliterator.go
package translit

import (
	"tilqos/internal/core"
	"tilqos/internal/i18n"
)

// Scheme is which Latin spelling of Kazakh to use.
type Scheme string

const (
	// Official2021 is the alphabet adopted by presidential decree in 2021: 31 letters using
	// ä ğ ñ ö ū ü ş and the Turkish-style dotted/dotless i pair.
	Official2021 Scheme = "official2021"
	// Practical is the plain-ASCII spelling people type in chat. Lossy by construction, so
	// reading it back needs a language model.
	Practical Scheme = "practical"
)

func (s Scheme) DisplayName() string {
	if s == Official2021 {
		return i18n.T(i18n.SchemeOfficial)
	}
	return i18n.T(i18n.SchemePractical)
}

func (s Scheme) Explanation() string {
	if s == Official2021 {
		return i18n.T(i18n.SchemeOfficialDetail)
	}
	return i18n.T(i18n.SchemePracticalDetail)
}

// CyrillicToOfficial is Cyrillic → official 2021 Latin. The letters that exist only in
// Russian loanwords get the conventional fallbacks so mixed text survives.
var CyrillicToOfficial = map[string]string{
	"а": "a", "ә": "ä", "б": "b", "в": "v", "г": "g", "ғ": "ğ", "д": "d", "е": "e",
	"ж": "j", "з": "z", "и": "i", "й": "i", "к": "k", "қ": "q", "л": "l", "м": "m",
	"н": "n", "ң": "ñ", "о": "o", "ө": "ö", "п": "p", "р": "r", "с": "s", "т": "t",
	"у": "u", "ұ": "ū", "ү": "ü", "ф": "f", "х": "h", "һ": "h", "ш": "ş", "ы": "y",
	"і": "ı",
	// Outside the standard.
	"ё": "io", "ц": "ts", "ч": "ch", "щ": "şş", "ъ": "", "ь": "", "э": "e",
	"ю": "iu", "я": "ia",
}

// OfficialToCyrillic is official 2021 Latin → Cyrillic; single-valued apart from `i`
// (и/й) and `h` (х/һ).
var OfficialToCyrillic = map[string]string{
	"a": "а", "ä": "ә", "b": "б", "v": "в", "g": "г", "ğ": "ғ", "d": "д", "e": "е",
	"j": "ж", "z": "з", "i": "и", "ı": "і", "k": "к", "q": "қ", "l": "л", "m": "м",
	"n": "н", "ñ": "ң", "o": "о", "ö": "ө", "p": "п", "r": "р", "s": "с", "t": "т",
	"u": "у", "ū": "ұ", "ü": "ү", "f": "ф", "h": "х", "ş": "ш", "y": "ы",
}

// CyrillicToPractical is Cyrillic → practical ASCII Latin. Deliberately many-to-one.
var CyrillicToPractical = map[string]string{
	"а": "a", "ә": "a", "б": "b", "в": "v", "г": "g", "ғ": "g", "д": "d", "е": "e",
	"ж": "j", "з": "z", "и": "i", "й": "i", "к": "k", "қ": "q", "л": "l", "м": "m",
	"н": "n", "ң": "n", "о": "o", "ө": "o", "п": "p", "р": "r", "с": "s", "т": "t",
	"у": "u", "ұ": "u", "ү": "u", "ф": "f", "х": "h", "һ": "h", "ш": "sh", "ы": "y",
	"і": "i", "ё": "io", "ц": "ts", "ч": "ch", "щ": "sh", "ъ": "", "ь": "",
	"э": "e", "ю": "iu", "я": "ia",
}

// PracticalToCyrillic lists every Cyrillic letter a practical Latin letter could stand
// for, best guess first. This is where the ambiguity lives.
var PracticalToCyrillic = map[string][]string{
	"a": {"а", "ә"}, "b": {"б"}, "c": {"с"}, "d": {"д"}, "e": {"е"}, "f": {"ф"},
	"g": {"г", "ғ"}, "h": {"х", "һ"}, "i": {"і", "и", "й"}, "j": {"ж"}, "k": {"к"},
	"l": {"л"}, "m": {"м"}, "n": {"н", "ң"}, "o": {"о", "ө"}, "p": {"п"}, "q": {"қ"},
	"r": {"р"}, "s": {"с"}, "t": {"т"}, "u": {"ұ", "ү", "у"}, "v": {"в"}, "w": {"у"},
	"x": {"х"}, "y": {"ы", "й"}, "z": {"з"},
}

// PracticalDigraphs are multi-letter sequences matched before single letters.
var PracticalDigraphs = map[string][]string{
	"sh": {"ш"}, "ch": {"ч"}, "shch": {"щ"},
}

// PracticalRussianToCyrillic is Latin → Cyrillic for Russian words typed in Latin. Real
// writing code-switches, and reading a Russian word such as `Zametki` under Kazakh rules puts
// Kazakh-only letters into it.
var PracticalRussianToCyrillic = map[string][]string{
	"a": {"а"}, "b": {"б"}, "c": {"ц", "к"}, "d": {"д"}, "e": {"е", "э"}, "f": {"ф"},
	"g": {"г"}, "h": {"х"}, "i": {"и", "й"}, "j": {"ж", "й"}, "k": {"к"}, "l": {"л"},
	"m": {"м"}, "n": {"н"}, "o": {"о"}, "p": {"п"}, "q": {"к"}, "r": {"р"}, "s": {"с"},
	"t": {"т"}, "u": {"у", "ю"}, "v": {"в"}, "w": {"в"}, "x": {"х"}, "y": {"ы", "й"},
	"z": {"з"},
}

var PracticalRussianDigraphs = map[string][]string{
	"sh": {"ш"}, "ch": {"ч"}, "zh": {"ж"}, "kh": {"х"}, "ts": {"ц"}, "shch": {"щ"},
	"ya": {"я"}, "yu": {"ю"}, "yo": {"ё"},
}

// Kazakh vowel harmony: a native word draws its vowels from one row, not both.
var (
	FrontVowels   = set("ә", "е", "ө", "ү", "і")
	BackVowels    = set("а", "о", "ұ", "ы")
	NeutralVowels = set("и", "у")
	// ғ and қ belong to back-vowel words, г and к to front-vowel ones.
	BackConsonants  = set("ғ", "қ")
	FrontConsonants = set("г", "к")
)

func set(items ...string) map[string]bool {
	out := make(map[string]bool, len(items))
	for _, item := range items {
		out[item] = true
	}
	return out
}

func IsVowel(c string) bool { return FrontVowels[c] || BackVowels[c] || NeutralVowels[c] }

// PracticalWordOverrides are short, high-frequency words spelled out explicitly: a
// character model has almost nothing to work with in a three-letter word.
var PracticalWordOverrides = map[string]string{
	"goi": "ғой", "gana": "ғана", "da": "да", "de": "де", "ta": "та", "te": "те",
	"ma": "ма", "me": "ме", "ba": "ба", "be": "бе", "pa": "па", "pe": "пе",
	"ne": "не", "ol": "ол", "bul": "бұл", "sol": "сол", "osy": "осы", "onda": "онда",
	"men": "мен", "sen": "сен", "biz": "біз", "siz": "сіз", "olar": "олар",
	"kim": "кім", "qai": "қай", "qalai": "қалай", "eken": "екен", "edi": "еді",
	"dep": "деп", "dese": "десе", "bar": "бар", "joq": "жоқ", "jane": "және",
	"ushin": "үшін", "keiin": "кейін", "endi": "енді", "ote": "өте", "oz": "өз",
	"ozi": "өзі", "aga": "аға", "apai": "апай", "koz": "көз", "kop": "көп",
	"alde": "әлде", "baribir": "бәрібір", "kerek": "керек", "sonda": "сонда",
	"sondyqtan": "сондықтан", "birak": "бірақ", "biraq": "бірақ", "tek": "тек",
}

// LatinOnlyWords stay in Latin: brands, apps and platform names. A list rather than a
// model, because measurement showed a model cannot do this job. The list lives in core,
// because the detector uses it too (a brand is an English word).
var LatinOnlyWords = core.LatinOnlyWords

// IsLatinOnly is true when a word is a Latin-only name, allowing for a short attached
// suffix so `instagramda` (instagram + да) is recognised too.
func IsLatinOnly(word string, extra core.StringSet) bool {
	lower := core.Lower(word)
	if LatinOnlyWords[lower] || extra.Contains(lower) {
		return true
	}
	characters := core.Graphemes(lower)
	count := len(characters)
	// A known name plus up to four letters of Kazakh or Russian case ending.
	for length := count - 1; length >= 4; length-- {
		if count-length > 4 {
			continue
		}
		base := joinChars(characters[:length])
		if LatinOnlyWords[base] || extra.Contains(base) {
			return true
		}
	}
	return false
}

// HehWords are the words where һ is correct rather than х.
var HehWords = []string{
	"қаһарман", "гауһар", "жауһар", "шаһар", "аһ", "уһ", "жиһаз", "жиһан",
	"қаһар", "маһаббат", "шаһзада", "аллаһ", "таһарат",
}

func joinChars(characters []string) string {
	n := 0
	for _, c := range characters {
		n += len(c)
	}
	out := make([]byte, 0, n)
	for _, c := range characters {
		out = append(out, c...)
	}
	return string(out)
}
