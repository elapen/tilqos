package config

import (
	"encoding/json"
	"testing"

	"tilqos/internal/i18n"
)

func TestUILanguageDefaultsToSystem(t *testing.T) {
	if got := Default().UILanguage; got != string(i18n.System) {
		t.Errorf("default UILanguage = %q, want %q", got, i18n.System)
	}
}

func TestUILanguageIsNormalized(t *testing.T) {
	for in, want := range map[string]string{
		"": "system", "system": "system", "en": "en", "ru": "ru", "kk": "kk",
		"klingon": "system", "RU": "system", "de": "system",
	} {
		c := Default()
		c.UILanguage = in
		c.Normalize()
		if c.UILanguage != want {
			t.Errorf("UILanguage %q normalized to %q, want %q", in, c.UILanguage, want)
		}
	}
}

// A config.json from before the setting existed has no such field: it must read as "system".
func TestOlderConfigWithoutUILanguageFollowsTheSystem(t *testing.T) {
	c := Default()
	if err := json.Unmarshal([]byte(`{"version":1,"enabled":true}`), &c); err != nil {
		t.Fatal(err)
	}
	c.Normalize()
	if c.UILanguage != "system" {
		t.Errorf("UILanguage = %q", c.UILanguage)
	}
}

// The names the menu shows follow the language set for the process.
func TestDisplayNamesFollowTheLanguage(t *testing.T) {
	defer i18n.Set(i18n.EN)
	for lang, want := range map[i18n.Lang]string{i18n.EN: "Off", i18n.RU: "Выключено", i18n.KK: "Өшірулі"} {
		i18n.Set(lang)
		if got := ModeOff.DisplayName(); got != want {
			t.Errorf("%s: ModeOff = %q, want %q", lang, got, want)
		}
	}
	i18n.Set(i18n.KK)
	if got := ActionForceRussian.DisplayName(); got != "Русский тіліне ауыстыру" {
		t.Errorf("ActionForceRussian = %q", got)
	}
}
