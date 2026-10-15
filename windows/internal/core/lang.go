// Package core is the platform-neutral heart of Tilqos: the port of the Swift
// detector (Sources/TilqosKit/Detect), the replacement logic and undo stack
// (Sources/TilqosKit/Apply), the learner (Core/Learner.swift) and the keystroke/table
// abstractions they work on.
//
// Every file mirrors one Swift file of the same name and keeps its structure, constants
// and comments, so a change to the Swift algorithm can be ported by diffing file for file:
//
//	Alphabet.swift      → alphabet.go
//	TrigramModel.swift  → trigram.go
//	Morphology.swift    → morphology.go
//	Lexicon.swift       → lexicon.go (decoding and lookup; the builder stays in GenModel)
//	ModelStore.swift    → models.go
//	WordScorer.swift    → scorer.go
//	Detector.swift      → detector.go
//	TypingEngine.swift  → typing.go
//	Learner.swift       → learner.go (persistence: internal/config/learned.go)
//	Guards.swift        → guards.go (+ regex.go: ICU-compatible pattern matching)
//	StrokeBuffer.swift  → buffer.go, tokenizer.go
//	KeyStroke.swift, KeyMapTable.swift → keymap.go (platform-neutral part)
//	Replacement.swift, ReplacementPlanner.swift → replacement.go
//	TextReplacer.swift  → replacer.go
//	UndoStack.swift     → undo.go
//	TextConverter.swift → textconv.go
//	Lang.swift          → lang.go
package core

// Script is the writing system. The first cascade of the detector decides only this.
type Script int

const (
	ScriptNone Script = iota
	ScriptLatin
	ScriptCyrillic
)

// Lang is one of the three languages the detector models, or LangNone.
//
// Layouts whose language is outside this set still get key tables built (so rendering
// and manual conversion work), but they are not scoring candidates.
type Lang string

const (
	LangNone Lang = ""
	LangEN   Lang = "en"
	LangRU   Lang = "ru"
	LangKK   Lang = "kk"
)

// AllLangs is Swift's Lang.allCases, in declaration order.
var AllLangs = []Lang{LangEN, LangRU, LangKK}

func (l Lang) Script() Script {
	switch l {
	case LangEN:
		return ScriptLatin
	case LangRU, LangKK:
		return ScriptCyrillic
	}
	return ScriptNone
}

// Badge is the two-to-three character layout badge.
func (l Lang) Badge() string {
	switch l {
	case LangEN:
		return "EN"
	case LangRU:
		return "РУ"
	case LangKK:
		return "ҚАЗ"
	}
	return ""
}

func (l Lang) DisplayName() string {
	switch l {
	case LangEN:
		return "English"
	case LangRU:
		return "Русский"
	case LangKK:
		return "Қазақша"
	}
	return ""
}

// LangFromCode maps a language code ("en", "RU", …) onto a modelled language.
func LangFromCode(code string) Lang {
	switch Lower(code) {
	case "en":
		return LangEN
	case "ru":
		return LangRU
	case "kk":
		return LangKK
	}
	return LangNone
}
