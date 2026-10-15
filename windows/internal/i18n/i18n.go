// Package i18n holds every text the Windows app shows to the user, in English, Russian and
// Kazakh (the counterpart of Sources/TilqosKit/UI/L10n.swift and Resources/*.lproj on macOS).
//
// Keys are typed constants, so a misspelt key does not compile, and the tests fail when a
// language has no text for a key or when two languages use different format verbs. English is
// the source language and the fallback.
//
// The package is platform-neutral and has no dependencies, so the tests run everywhere.
// Reading the Windows UI language is the caller's job (winapp passes GetUserDefaultUILanguage
// to Resolve).
package i18n

import (
	"fmt"
	"strings"
	"sync/atomic"
)

// Lang is an interface language.
type Lang string

const (
	EN Lang = "en"
	RU Lang = "ru"
	KK Lang = "kk"
)

// Languages in menu order.
var Languages = []Lang{EN, RU, KK}

// Setting is what the user chose in the config: a language, or "system" to follow Windows.
type Setting string

const (
	System    Setting = "system"
	SettingEN Setting = "en"
	SettingRU Setting = "ru"
	SettingKK Setting = "kk"
)

// Settings in menu order.
var Settings = []Setting{System, SettingEN, SettingRU, SettingKK}

// Valid reports whether s is one of the known settings.
func (s Setting) Valid() bool {
	for _, known := range Settings {
		if s == known {
			return true
		}
	}
	return false
}

// Pinned is the language a setting names, or false for "system" (and for anything unknown).
func (s Setting) Pinned() (Lang, bool) {
	switch s {
	case SettingEN:
		return EN, true
	case SettingRU:
		return RU, true
	case SettingKK:
		return KK, true
	}
	return EN, false
}

// Windows primary language identifiers (the low 10 bits of a LANGID).
const (
	primaryEnglish = 0x09
	primaryRussian = 0x19
	primaryKazakh  = 0x3F
)

// FromLangID maps a Windows LANGID (as GetUserDefaultUILanguage returns it) to an interface
// language: English, Russian or Kazakh by primary language, English for everything else.
func FromLangID(id uint16) Lang {
	switch id & 0x3FF {
	case primaryRussian:
		return RU
	case primaryKazakh:
		return KK
	case primaryEnglish:
		return EN
	}
	return EN
}

// Resolve is the language to use for a setting, given the Windows UI language.
func Resolve(setting Setting, systemLangID uint16) Lang {
	if lang, ok := setting.Pinned(); ok {
		return lang
	}
	return FromLangID(systemLangID)
}

var current atomic.Value // Lang

// Set chooses the language Text and Format use from now on. Safe from any goroutine.
func Set(lang Lang) {
	switch lang {
	case EN, RU, KK:
		current.Store(lang)
	default:
		current.Store(EN)
	}
}

// Current is the language in use (English until Set is called).
func Current() Lang {
	if lang, ok := current.Load().(Lang); ok {
		return lang
	}
	return EN
}

func table(lang Lang) *[numKeys]string {
	switch lang {
	case RU:
		return &russian
	case KK:
		return &kazakh
	}
	return &english
}

// TextIn is the text of key in lang; English when lang has none.
func TextIn(lang Lang, key Key) string {
	if key <= keyInvalid || key >= numKeys {
		return fmt.Sprintf("!key(%d)", int(key))
	}
	if text := table(lang)[key]; text != "" {
		return text
	}
	return english[key]
}

// T is the text of key in the current language.
func T(key Key) string { return TextIn(Current(), key) }

// F is T with fmt.Sprintf substitutions.
func F(key Key, args ...any) string { return fmt.Sprintf(T(key), args...) }

// FIn is TextIn with fmt.Sprintf substitutions.
func FIn(lang Lang, key Key, args ...any) string { return fmt.Sprintf(TextIn(lang, key), args...) }

// Verbs lists the format verbs of a text in argument order, for the tests: "%d", "%s", "%[2]s".
func Verbs(text string) []string {
	var verbs []string
	for i := 0; i < len(text); i++ {
		if text[i] != '%' {
			continue
		}
		j := i + 1
		for j < len(text) && strings.ContainsRune("[]0123456789+-# .", rune(text[j])) {
			j++
		}
		if j >= len(text) {
			break
		}
		if text[j] == '%' {
			i = j
			continue
		}
		verbs = append(verbs, text[i:j+1])
		i = j
	}
	return verbs
}
