package models_test

import (
	"bytes"
	"testing"

	"tilqos/internal/core"
	"tilqos/internal/golden"
	"tilqos/internal/models"
)

// TestEmbeddedModelsMatchSource fails — never skips — when the embedded copies are
// missing or differ from Resources/Models, the single source of truth.
func TestEmbeddedModelsMatchSource(t *testing.T) {
	embedded, err := models.Files()
	if err != nil {
		t.Fatalf("%v", err)
	}
	for _, name := range core.ModelFileNames {
		if !bytes.Equal(golden.ModelFile(t, name), embedded[name]) {
			t.Errorf("embedded %s differs from Resources/Models/%s — run `go generate ./...`", name, name)
		}
	}
}

func TestEmbeddedModelsLoad(t *testing.T) {
	loaded, err := models.Load()
	if err != nil {
		t.Fatalf("%v", err)
	}
	if got := loaded.AvailableLanguages(); len(got) != 3 || loaded.KazakhMorphology() == nil {
		t.Fatalf("languages %v", got)
	}
	for _, lang := range core.AllLangs {
		if loaded.Lexicon(lang) == nil {
			t.Errorf("no %s lexicon", lang)
		}
	}
}
