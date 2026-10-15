package core

// Port of Detect/ModelStore.swift, minus the loading thread: decoding the ~2 MB of embedded
// models and lexicons takes a few tens of milliseconds, so the Windows app does it before
// installing the hooks.

import "fmt"

// Models holds the trigram models, the word lexicons and the Kazakh morphology. Immutable
// once built, so it is safe to share between goroutines.
type Models struct {
	byLang     map[Lang]*TrigramModel
	lexicons   map[Lang]*Lexicon
	morphology *Morphology
}

// ModelFileNames are the files LoadModels expects, as shipped in Resources/Models.
var ModelFileNames = []string{
	"en.bin", "ru.bin", "kk.bin", "kk-morph.bin", "en-lex.bin", "ru-lex.bin", "kk-lex.bin",
}

// LoadModels decodes model files by name (ModelFileNames). Missing or malformed files are
// errors: unlike the Swift app, which degrades to "no model" or "no lexicon", the Windows
// build ships its models inside the executable, so a missing one is a packaging bug.
func LoadModels(files map[string][]byte) (*Models, error) {
	models := &Models{byLang: map[Lang]*TrigramModel{}, lexicons: map[Lang]*Lexicon{}}
	for _, lang := range AllLangs {
		name := string(lang) + ".bin"
		data, ok := files[name]
		if !ok {
			return nil, fmt.Errorf("no model for %s (%s)", lang, name)
		}
		model, err := DecodeTrigramModel(data)
		if err != nil {
			return nil, fmt.Errorf("%s model rejected: %w", lang, err)
		}
		if model.Lang != lang {
			return nil, fmt.Errorf("%s contains a %s model", name, model.Lang)
		}
		models.byLang[lang] = model

		// `.bin` like every other data file, because that is what the build bundles.
		lexName := string(lang) + "-lex.bin"
		lexData, ok := files[lexName]
		if !ok {
			return nil, fmt.Errorf("no lexicon for %s (%s)", lang, lexName)
		}
		lexicon, err := DecodeLexicon(lexData)
		if err != nil {
			return nil, fmt.Errorf("%s lexicon rejected: %w", lang, err)
		}
		if lexicon.Lang != lang {
			return nil, fmt.Errorf("%s contains a %s lexicon", lexName, lexicon.Lang)
		}
		models.lexicons[lang] = lexicon
	}
	data, ok := files["kk-morph.bin"]
	if !ok {
		return nil, fmt.Errorf("no Kazakh morphology (kk-morph.bin)")
	}
	morphology, err := DecodeMorphology(data)
	if err != nil {
		return nil, fmt.Errorf("morphology rejected: %w", err)
	}
	models.morphology = morphology
	return models, nil
}

// NewModels assembles a store from already-decoded parts (tests). lexicons may be nil: the
// detector then falls back to the trigram models alone, as the Swift app does without them.
func NewModels(byLang map[Lang]*TrigramModel, lexicons map[Lang]*Lexicon, morphology *Morphology) *Models {
	if lexicons == nil {
		lexicons = map[Lang]*Lexicon{}
	}
	return &Models{byLang: byLang, lexicons: lexicons, morphology: morphology}
}

// Lexicon returns the language's word forms, or nil.
func (m *Models) Lexicon(lang Lang) *Lexicon {
	if m == nil {
		return nil
	}
	return m.lexicons[lang]
}

// Model returns the model for a language, or nil.
func (m *Models) Model(lang Lang) *TrigramModel {
	if m == nil {
		return nil
	}
	return m.byLang[lang]
}

// KazakhMorphology is the suffix-stripping data. Only Kazakh has it: it is the language
// whose forms a character model cannot judge, being agglutinative.
func (m *Models) KazakhMorphology() *Morphology {
	if m == nil {
		return nil
	}
	return m.morphology
}

// AvailableLanguages lists the loaded languages in AllLangs order.
func (m *Models) AvailableLanguages() []Lang {
	var out []Lang
	for _, lang := range AllLangs {
		if m.Model(lang) != nil {
			out = append(out, lang)
		}
	}
	return out
}
