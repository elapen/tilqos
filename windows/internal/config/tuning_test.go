package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tilqos/internal/core"
)

func TestSensitivityPresets(t *testing.T) {
	cfg := Default()
	if cfg.Sensitivity() != SensitivityBalanced {
		t.Fatalf("the default is balanced, got %s", cfg.Sensitivity())
	}
	for _, preset := range SensitivityPresets {
		cfg.SetSensitivity(preset)
		if cfg.Sensitivity() != preset {
			t.Errorf("%s does not round-trip", preset)
		}
	}
	cfg.SetSensitivity(SensitivityCautious)
	if cfg.AutoThreshold != 0.93 || cfg.SuggestThreshold != 0.55 {
		t.Errorf("cautious %v/%v", cfg.AutoThreshold, cfg.SuggestThreshold)
	}
	cfg.SetSensitivity(SensitivityBold)
	if cfg.AutoThreshold != 0.55 || cfg.SuggestThreshold != 0.30 {
		t.Errorf("bold %v/%v", cfg.AutoThreshold, cfg.SuggestThreshold)
	}
	cfg.AutoThreshold = 0.7
	if cfg.Sensitivity() != SensitivityCustom {
		t.Error("hand-set thresholds are custom")
	}
	cfg.SetSensitivity(SensitivityCustom)
	if cfg.AutoThreshold != 0.7 {
		t.Error("custom changes nothing")
	}
}

func TestCyrillicPreferenceIsNormalized(t *testing.T) {
	cfg := Default()
	if cfg.CyrillicPreference != CyrillicAutomatic {
		t.Fatalf("default %q", cfg.CyrillicPreference)
	}
	cfg.CyrillicPreference = "kazakh please"
	cfg.Normalize()
	if cfg.CyrillicPreference != CyrillicAutomatic {
		t.Fatalf("an unknown value falls back to automatic, got %q", cfg.CyrillicPreference)
	}
	cfg.CyrillicPreference = CyrillicPreferKazakh
	cfg.Normalize()
	if cfg.CyrillicPreference != CyrillicPreferKazakh {
		t.Fatal("a valid value survives")
	}
}

func TestLearnedWordsSurviveARestartAsHashesOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Tilqos", "learned.json")
	store, err := OpenLearner(path)
	if err != nil {
		t.Fatal(err)
	}
	store.Learner().Confirmed("ghbdtn", core.LangRU)
	store.Learner().Kept("Айдар", core.LangRU)
	store.Learner().Kept("айдар", core.LangRU)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if text := strings.ToLower(string(data)); strings.Contains(text, "ghbdtn") || strings.Contains(text, "айдар") {
		t.Fatalf("plaintext on disk: %s", data)
	}
	if !strings.Contains(string(data), `"schema": 2`) {
		t.Fatalf("the rules the words were learned under are stored: %s", data)
	}
	reopened, err := OpenLearner(path)
	if err != nil {
		t.Fatal(err)
	}
	if decision, _ := reopened.Learner().Decision("ghbdtn"); decision.Kind != core.DecisionConvert {
		t.Fatalf("decision lost: %+v", decision)
	}
	if !reopened.Learner().IsKnown("АЙДАР", core.LangRU) {
		t.Fatal("vocabulary lost")
	}
}

// learned.json from the first Windows version has no schema: what its looser rules learned
// is dropped when the file is opened, explicit acceptances stay, and the file is rewritten at
// once, before anything new is learned.
func TestLearnedWordsFromTheOldRulesAreDroppedOnOpen(t *testing.T) {
	old := core.NewLearner(nil)
	old.Confirmed("rfhjxt", core.LangRU)
	old.Ignored("ghbdtn")
	old.Ignored("ghbdtn")
	old.Kept("nfrjq", core.LangEN)
	data, err := old.Encode()
	if err != nil {
		t.Fatal(err)
	}
	var file map[string]any
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatal(err)
	}
	delete(file, "schema") // as the first version wrote it
	if data, err = json.Marshal(file); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "learned.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := OpenLearner(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if upgrade, ok := store.Learner().Upgraded(); !ok || upgrade.Before != 3 || upgrade.Kept != 1 {
		t.Fatalf("upgrade %+v %v", upgrade, ok)
	}
	if _, ok := store.Learner().Decision("ghbdtn"); ok {
		t.Fatal("offers that merely went by no longer count")
	}
	if decision, _ := store.Learner().Decision("rfhjxt"); decision.Kind != core.DecisionConvert {
		t.Fatalf("an explicit acceptance stays: %+v", decision)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	reread, err := core.DecodeLearner(written, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, again := reread.Upgraded(); again || reread.Counts().Total() != 1 {
		t.Fatalf("the file must already hold the upgraded store: %s", written)
	}
}

func TestUnreadableLearnedWordsAreSetAside(t *testing.T) {
	path := filepath.Join(t.TempDir(), "learned.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := OpenLearner(path)
	if err == nil || store.Learner() == nil {
		t.Fatal("a broken file is reported, and learning starts fresh")
	}
	if _, statErr := os.Stat(path + ".bad"); statErr != nil {
		t.Fatalf("the broken file must be kept aside: %v", statErr)
	}
}
