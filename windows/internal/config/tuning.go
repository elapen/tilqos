package config

import "tilqos/internal/i18n"

// Settings the detection rewrite added: the Cyrillic tie-breaker and the sensitivity
// presets (Sensitivity in SettingsWindow.swift).

// CyrillicPreference is which Cyrillic layout to prefer when the letters alone cannot tell
// Russian from Kazakh apart (all 33 Russian letters sit on the same keys in both layouts).
type CyrillicPreference string

const (
	// CyrillicAutomatic: no switch without a positive signal; remember the last Cyrillic
	// layout per application.
	CyrillicAutomatic     CyrillicPreference = "automatic"
	CyrillicPreferKazakh  CyrillicPreference = "preferKazakh"
	CyrillicPreferRussian CyrillicPreference = "preferRussian"
)

// AllCyrillicPreferences in menu order.
var AllCyrillicPreferences = []CyrillicPreference{CyrillicAutomatic, CyrillicPreferKazakh, CyrillicPreferRussian}

func (p CyrillicPreference) Valid() bool {
	return p == CyrillicAutomatic || p == CyrillicPreferKazakh || p == CyrillicPreferRussian
}

func (p CyrillicPreference) DisplayName() string {
	switch p {
	case CyrillicPreferKazakh:
		return i18n.T(i18n.CyrillicKazakh)
	case CyrillicPreferRussian:
		return i18n.T(i18n.CyrillicRussian)
	}
	return i18n.T(i18n.CyrillicAutomatic)
}

// Sensitivity is how eagerly to correct, in words instead of two thresholds.
//
// The detector reports confidence in tiers — 0.95 for a certain fix, 0.9 for a confident
// one, 0.6 for "only worth offering" — so each preset sits between tiers rather than on
// one. Balanced is the default; cautious lets only certain fixes happen by themselves; bold
// also applies what would otherwise only be offered. The values are the ones
// CorpusTests.testPresets measures.
type Sensitivity string

const (
	SensitivityCautious Sensitivity = "cautious"
	SensitivityBalanced Sensitivity = "balanced"
	SensitivityBold     Sensitivity = "bold"
	// SensitivityCustom: thresholds set by hand in config.json.
	SensitivityCustom Sensitivity = "custom"
)

// SensitivityPresets in menu order.
var SensitivityPresets = []Sensitivity{SensitivityCautious, SensitivityBalanced, SensitivityBold}

// Thresholds are the (auto, suggest) pair of a preset; ok is false for custom.
func (s Sensitivity) Thresholds() (auto, suggest float64, ok bool) {
	switch s {
	case SensitivityCautious:
		return 0.93, 0.55, true
	case SensitivityBalanced:
		return 0.80, 0.45, true
	case SensitivityBold:
		return 0.55, 0.30, true
	}
	return 0, 0, false
}

func (s Sensitivity) DisplayName() string {
	switch s {
	case SensitivityCautious:
		return i18n.T(i18n.SensitivityCautious)
	case SensitivityBalanced:
		return i18n.T(i18n.SensitivityBalanced)
	case SensitivityBold:
		return i18n.T(i18n.SensitivityBold)
	}
	return i18n.T(i18n.SensitivityCustom)
}

func (s Sensitivity) Explanation() string {
	switch s {
	case SensitivityCautious:
		return i18n.T(i18n.SensitivityExplainCautious)
	case SensitivityBalanced:
		return i18n.T(i18n.SensitivityExplainBalanced)
	case SensitivityBold:
		return i18n.T(i18n.SensitivityExplainBold)
	}
	return i18n.T(i18n.SensitivityExplainCustom)
}

// Sensitivity names the preset the thresholds match, or custom.
func (c Config) Sensitivity() Sensitivity {
	for _, preset := range SensitivityPresets {
		auto, suggest, _ := preset.Thresholds()
		if c.AutoThreshold == auto && c.SuggestThreshold == suggest {
			return preset
		}
	}
	return SensitivityCustom
}

// SetSensitivity applies a preset's thresholds; custom changes nothing.
func (c *Config) SetSensitivity(s Sensitivity) {
	if auto, suggest, ok := s.Thresholds(); ok {
		c.AutoThreshold, c.SuggestThreshold = auto, suggest
	}
}
