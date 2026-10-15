package i18n

import (
	"fmt"
	"sort"
	"strings"
	"testing"
	"unicode"
)

func TestEveryKeyHasATextInEveryLanguage(t *testing.T) {
	for _, lang := range Languages {
		tbl := table(lang)
		for key := keyInvalid + 1; key < numKeys; key++ {
			if strings.TrimSpace(tbl[key]) == "" {
				t.Errorf("%s: key %d (English: %q) has no text", lang, key, english[key])
			}
		}
	}
}

// A format verb that differs between languages would crash Sprintf or print the wrong thing.
func TestFormatVerbsMatchAcrossLanguages(t *testing.T) {
	for key := keyInvalid + 1; key < numKeys; key++ {
		want := normalizedVerbs(english[key])
		for _, lang := range []Lang{RU, KK} {
			if got := normalizedVerbs(table(lang)[key]); fmt.Sprint(got) != fmt.Sprint(want) {
				t.Errorf("%s: key %d (%q): verbs %v, English has %v", lang, key, english[key], got, want)
			}
		}
	}
}

// normalizedVerbs gives each verb its argument position ("1s", "2d"), so a translation may
// reorder arguments with %[n]s.
func normalizedVerbs(text string) []string {
	var out []string
	next := 1
	for _, verb := range Verbs(text) {
		position := next
		if strings.HasPrefix(verb, "%[") {
			end := strings.Index(verb, "]")
			fmt.Sscanf(verb[2:end], "%d", &position)
			verb = "%" + verb[end+1:]
		}
		next++
		out = append(out, fmt.Sprintf("%d%s", position, verb[len(verb)-1:]))
	}
	sort.Strings(out)
	return out
}

func TestVerbsParser(t *testing.T) {
	cases := map[string][]string{
		"no verbs":                nil,
		"100%% sure":              nil,
		"%s and %d":               {"%s", "%d"},
		"%.2f / %.2f — %s":        {"%.2f", "%.2f", "%s"},
		"%[2]s before %[1]s":      {"%[2]s", "%[1]s"},
		"ends with a percent: 5%": nil,
	}
	for text, want := range cases {
		if got := Verbs(text); fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("Verbs(%q) = %v, want %v", text, got, want)
		}
	}
}

// Sprintf with the verbs of the English text must not produce %!v(MISSING) or EXTRA in any
// language: the arguments below match what the app passes.
func TestEveryTextFormatsCleanly(t *testing.T) {
	for _, lang := range Languages {
		for key := keyInvalid + 1; key < numKeys; key++ {
			args := argumentsFor(table(lang)[key])
			out := fmt.Sprintf(table(lang)[key], args...)
			if strings.Contains(out, "%!") {
				t.Errorf("%s: key %d formats badly: %q", lang, key, out)
			}
		}
	}
}

func argumentsFor(text string) []any {
	var args []any
	for _, verb := range Verbs(text) {
		switch verb[len(verb)-1] {
		case 'd':
			args = append(args, 3)
		case 'f':
			args = append(args, 0.5)
		default:
			args = append(args, "x")
		}
	}
	return args
}

func TestResolve(t *testing.T) {
	const (
		english  = 0x0409 // en-US
		britain  = 0x0809 // en-GB
		russian  = 0x0419 // ru-RU
		kazakh   = 0x043F // kk-KZ
		german   = 0x0407 // de-DE
		ukrainan = 0x0422 // uk-UA
	)
	cases := []struct {
		setting Setting
		id      uint16
		want    Lang
	}{
		{System, english, EN}, {System, britain, EN}, {System, russian, RU}, {System, kazakh, KK},
		{System, german, EN}, {System, ukrainan, EN}, {System, 0, EN},
		{SettingEN, russian, EN}, {SettingRU, english, RU}, {SettingKK, german, KK},
		{Setting("klingon"), russian, RU}, // unknown value: behaves like "system"
	}
	for _, c := range cases {
		if got := Resolve(c.setting, c.id); got != c.want {
			t.Errorf("Resolve(%q, %#x) = %s, want %s", c.setting, c.id, got, c.want)
		}
	}
}

func TestSettingsAreValid(t *testing.T) {
	for _, s := range Settings {
		if !s.Valid() {
			t.Errorf("%q is listed but not valid", s)
		}
	}
	if Setting("").Valid() || Setting("de").Valid() {
		t.Error("an unknown setting was accepted")
	}
}

func TestSetAndFallback(t *testing.T) {
	defer Set(EN)
	Set(RU)
	if got := T(MenuQuit); got != "Выход" {
		t.Errorf("Russian: %q", got)
	}
	Set(KK)
	if got := T(MenuQuit); got != "Шығу" {
		t.Errorf("Kazakh: %q", got)
	}
	Set(Lang("de"))
	if Current() != EN || T(MenuQuit) != "Exit" {
		t.Errorf("an unknown language must mean English, got %s / %q", Current(), T(MenuQuit))
	}
	if got := TextIn(Lang("de"), MenuQuit); got != "Exit" {
		t.Errorf("TextIn with an unknown language: %q", got)
	}
	if got := T(Key(-1)); !strings.HasPrefix(got, "!key") {
		t.Errorf("an invalid key must be visible, got %q", got)
	}
	if got := F(MenuLayout, "ABC"); got != "Layout: ABC" {
		t.Errorf("F: %q", got)
	}
}

func TestFromLangID(t *testing.T) {
	for id, want := range map[uint16]Lang{0x0409: EN, 0x0419: RU, 0x043F: KK, 0x0407: EN} {
		if got := FromLangID(id); got != want {
			t.Errorf("FromLangID(%#x) = %s, want %s", id, got, want)
		}
	}
}

// Kazakh must use its own letters and Russian must not be a copy of English: a cheap guard
// against pasting the wrong table.
func TestTablesAreTranslations(t *testing.T) {
	cyrillic := func(s string) bool {
		for _, r := range s {
			if unicode.Is(unicode.Cyrillic, r) {
				return true
			}
		}
		return false
	}
	kazakhOnly := "әғқңөұүһі"
	sawKazakhLetter := false
	untranslated := 0
	for key := keyInvalid + 1; key < numKeys; key++ {
		if !cyrillic(russian[key]) {
			untranslated++
		}
		if !cyrillic(kazakh[key]) && kazakh[key] != english[key] {
			t.Errorf("Kazakh key %d is neither Cyrillic nor the same as English: %q", key, kazakh[key])
		}
		if strings.ContainsAny(kazakh[key], kazakhOnly) {
			sawKazakhLetter = true
		}
	}
	// "Tilqos %s" and the like are the same everywhere.
	if untranslated > 3 {
		t.Errorf("%d Russian texts contain no Cyrillic", untranslated)
	}
	if !sawKazakhLetter {
		t.Error("no Kazakh text uses ә ғ қ ң ө ұ ү һ і")
	}
}

// The privacy text promises specific things; keep it from drifting from the specification.
func TestPrivacyTextMentionsTheLimits(t *testing.T) {
	for _, lang := range Languages {
		text := TextIn(lang, PrivacyText)
		for _, needle := range []string{"5", "2", "10", "64"} {
			if !strings.Contains(text, needle) {
				t.Errorf("%s privacy text lacks %q", lang, needle)
			}
		}
		for _, banned := range []string{"irreversible", "необратим", "қайтымсыз", "encrypted by"} {
			if strings.Contains(strings.ToLower(text), banned) {
				t.Errorf("%s privacy text must not claim %q", lang, banned)
			}
		}
	}
}
