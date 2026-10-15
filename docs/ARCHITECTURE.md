# Architecture

Tilqos is a Swift Package (macOS menu-bar app, no third-party dependencies) plus a Go port of the same engine for Windows. The decision logic is deliberately free of AppKit, the event tap and the text-injection code so the regression corpus drives exactly the object that ships. How the detector decides is in [HOW_IT_WORKS.md](HOW_IT_WORKS.md); this page is a map of the code.

```
Package.swift                 targets: TilqosKit (library) / Tilqos (executable) / GenModel (tool) / TilqosKitTests
Sources/Tilqos/App.swift      the @main shell: creates NSApplication and AppController, nothing else
Sources/TilqosKit/            everything else
Sources/GenModel/main.swift   builds Resources/Models/*.bin from dictionaries (generate-models.sh)
Resources/                    Info.plist, AppIcon.icns, icon-1024.png, Models/*.bin and their NOTICE
wordlists/                    hand-written brand/term lists and frequency tiers compiled into the lexicons
Tests/TilqosKitTests/         262 tests, including the accuracy corpus, the privacy tests, and tests against the real system layouts
Tools/                        make-icon.swift, signing.sh (sourced by the build scripts), fetch-licenses.sh
LICENSES/                     verbatim upstream licence texts and READMEs for the language data
windows/                      the Go port (its own go.mod: module `tilqos`)
docs/                         this documentation and the GitHub Pages site
build.sh  make-dmg.sh  generate-models.sh  make-icon.sh
```

## The macOS app: `Sources/TilqosKit/`

| Directory / file | What lives there |
|---|---|
| `AppController.swift` | The menu-bar presence and start-up order: status item first (so the app is visibly up within milliseconds), then layouts, then language models in the background; first-run onboarding or the tutorial. |
| `Pipeline.swift` | Connects the tap to the detector to the injector and owns *when* to act: the side effects (hint panel, replacement, layout switch, undo stack, clipboard, hotkeys) and the learning signals only it can see. |
| `Input/` | `EventTap.swift` (the `CGEventTap`: key-down, flags-changed and mouse-down events; resets; secure-input check; a watchdog that re-arms a tap macOS disabled), `KeyStroke.swift` (a key code plus the modifiers that change its character, never the character), `KeyMapTable.swift` (key → character tables built at run time from the system's layouts with `UCKeyTranslate`), `InputSource.swift` (the installed layouts and the current one; rebuilt when you add or remove a layout). |
| `Detect/` | The engine. `Detector.swift` (readings, evidence ladder, confidence, segmentation, outcomes), `WordScorer.swift` (a reading's facts and cost), `TypingEngine.swift` (which key ends a word, when to evaluate, buffer upkeep after a fix, learning signals from what you do next), `StrokeBuffer.swift` (the recent strokes and the tokenizer), `Guards.swift` (what must never be rewritten), `TrigramModel.swift`, `Lexicon.swift`, `Morphology.swift`, `Alphabet.swift` (the data structures read from `Resources/Models`), `ModelStore.swift` (loads them off the start-up path). |
| `Translit/` | `Transliterator.swift` (Latin ⇄ Cyrillic Kazakh; beam search with vowel harmony; bilingual reading) and `TranslitScheme.swift` (letter tables for both schemes, brand list, short-word list). |
| `Apply/` | Making the edit. `TextReplacer.swift` (verify-then-replace; refuses to guess; written against protocols so it is tested with simulated misbehaving apps), `Injector.swift` (the real text field and keyboard, per-app strategy cache), `AXSupport.swift` (Accessibility wrappers, with messaging timeouts so a hung app cannot freeze the tap), `Replacement.swift` (delete-text/insert-text plan), `UndoStack.swift` (applied corrections), `SelectionClipboard.swift` (⌘C / paste-back for apps that hide their selection; restores the clipboard), `TextConverter.swift` (re-reading finished text as if typed on another layout), `CaretLocator.swift` (where the hint appears). |
| `Core/` | `Prefs.swift` (settings in `UserDefaults`), `AppRules.swift` (per-app mode, never-touched list, Tab-critical apps, per-app Cyrillic memory), `Hotkeys.swift`, `Learner.swift` (learned vocabulary as salted hashes), `Lang.swift`, `Permissions.swift` (the two TCC grants and the deep links to System Settings), `Log.swift` (shapes and counts only, never characters). |
| `UI/` | SwiftUI/AppKit: `StatusMenu.swift` and `StatusIcon.swift` (the menu and the `EN`/`РУ`/`ҚАЗ` keycap), `SuggestionPanel.swift` and `HintView.swift` (the non-activating hint card), `FixHUD.swift`, `Onboarding.swift`, `Tutorial.swift` (five steps with live practice fields driven by the real engine), `SettingsWindow.swift` and `Settings/*Pane.swift` (General, Correction, Languages, Transliteration, Apps, Exceptions, Hotkeys, Diagnostics). |

### Data flow for one word

1. `EventTap.process` (on the system's keystroke path, so it does almost nothing) classifies the event: our own injected event (passes), secure input on (passes, buffer dropped), mouse click / shortcut / navigation key (buffer reset), Backspace (drop the last stroke), hotkey (swallowed), otherwise a text stroke.
2. `Pipeline.handle` records the stroke through `TypingEngine.record` into the `StrokeBuffer`. If the stroke closed a word, the evaluation is deferred one run-loop turn so the separator reaches the app first.
3. `Pipeline.evaluate` reads the preceding text if needed (`precedingText`), then `TypingEngine.decide` runs `Detector.evaluate` and returns `none`, `offer` or `apply`.
4. `offer` shows `SuggestionPanel` and arms the accept key; `apply` calls `Injector.replace` → `TextReplacer.apply`. On success the buffer is rewritten to match the screen, the layout is switched, and the correction is recorded for undo and for the learner.

### Threading and time budget

Everything user-facing is main-thread by construction (the tap's run-loop source lives on the main run loop), which is why the package builds in Swift 5 language mode: Swift 6 strict concurrency cannot express "confined to the main thread" for the C callbacks of CoreGraphics/Carbon without unsafe shims. The tap callback must stay fast or macOS silently disables the tap, so heavy work (the evaluation, Accessibility calls, the pasteboard) happens after the callback returns. Language models are decoded on a background queue; until they load, the detector reports "not ready".

### Tests

`Tests/TilqosKitTests/` mixes unit tests (tokenizer, replacement planner, lexicon, trigram model, learner, transliterator), tests against the **real system layouts** (`KeyMapTableTests`, `ModelLoadingTests`, ...), the accuracy corpus (`Corpus.swift`, `CorpusTests.swift`), simulated misbehaving text fields (`TextReplacerTests`), micro-benchmarks (`PerformanceTests`) and UI snapshots (`UISnapshotTests`, opt-in). See [BUILDING.md](BUILDING.md#tests) for prerequisites.

## The data files: `Resources/Models/`

| File | Content |
|---|---|
| `en.bin`, `ru.bin`, `kk.bin` | letter-trigram cost cubes (28³, 43³, 43³ bytes plus a 20-byte header) |
| `en-lex.bin`, `ru-lex.bin`, `kk-lex.bin` | hunspell-style lexicons: hashed stems + affix-flag bitmasks + affix rules + hashed frequency tier |
| `kk-morph.bin` | hashed Kazakh stems and the suffix inventory for decomposition |
| `NOTICE` | the licence notice that must travel with them |

All are little-endian, magic-prefixed (`LSTG`, `LSLX`, `LSMO`; the magics predate the project's rename and were left alone so the files did not have to change) and produced by `Sources/GenModel` from upstream dictionaries; they are lossy derivatives of third-party data under their own licences ([THIRD_PARTY_NOTICES.md](../THIRD_PARTY_NOTICES.md)). On macOS they are bundle resources loaded at start-up; on Windows they are embedded in the executable with `go:embed`.

## The Windows port

The `windows/` directory is a line-by-line Go port of the engine, plus a native shell. Module `tilqos`, Go 1.25, standard library only (no cgo, no third-party modules, no `net`).

```
windows/
  cmd/tilqos/            main; rsrc_windows_*.syso (the exe icon)
  internal/core/         the engine: platform-independent port of Detect/ + Apply/ + Learner
  internal/translit/     port of Translit/
  internal/engine/       port of Pipeline.swift, behind a Platform interface (tested with a fake platform and document)
  internal/config/       config.json, learned.json, app rules, hotkeys, fix modes
  internal/models/       the embedded model copies and the drift check
  internal/modelsrc/     reads models from Resources/Models (or from git via TILQOS_MODELS_REV)
  internal/win32/        Win32 declarations: DLLs, constants, structs (sizes and offsets checked at compile time for amd64 and arm64)
  internal/winapp/       the Windows application: hooks, layout tables, Edit/RichEdit access, input, clipboard, tray, hint popup
  internal/iconkit/, tools/   icon generation (.ico, .syso) and model sync
  internal/golden/, testdata/golden/, parity/   the comparison against the Swift implementation
```

| Swift (`Sources/TilqosKit/`) | Go (`windows/internal/`) |
|---|---|
| `Detect/Alphabet.swift` | `core/alphabet.go` |
| `Detect/TrigramModel.swift` | `core/trigram.go` |
| `Detect/Morphology.swift` | `core/morphology.go` |
| `Detect/Lexicon.swift` | `core/lexicon.go` (reading and lookup; building stays in `GenModel`) |
| `Detect/ModelStore.swift` | `core/models.go`, `models/models.go` |
| `Detect/WordScorer.swift` | `core/scorer.go` |
| `Detect/Detector.swift` | `core/detector.go` |
| `Detect/TypingEngine.swift` | `core/typing.go` |
| `Detect/Guards.swift` | `core/guards.go` (+ `core/regex.go`: an ICU-compatible regular-expression engine, patterns copied verbatim) |
| `Detect/StrokeBuffer.swift` | `core/buffer.go`, `core/tokenizer.go` |
| `Input/KeyStroke.swift`, `Input/KeyMapTable.swift` | `core/keymap.go` (+ `winapp/layouts_windows.go` instead of TIS) |
| `Apply/Replacement.swift`, `Apply/UndoStack.swift`, `Apply/TextConverter.swift` | `core/replacement.go`, `core/undo.go`, `core/textconv.go` |
| `Apply/TextReplacer.swift` | `core/replacer.go` (+ `winapp/field_windows.go` instead of `AXTextField`, `winapp/inject_windows.go` instead of `SyntheticKeys`) |
| `Core/Learner.swift` | `core/learner.go` (+ `config/learned.go` instead of `UserDefaults`) |
| `Translit/*` | `translit/tables.go`, `translit/transliterator.go` |
| `Pipeline.swift` | `engine/engine.go` |
| `Core/Prefs.swift`, `Core/AppRules.swift`, `Core/Hotkeys.swift`, the sensitivity presets | `config/*.go` |

Swift string semantics that decisions depend on are reproduced explicitly in `core/text.go`: grapheme clusters for `String.count`, `Character.isLetter` / `isNumber`, full case mapping (`ß` → `SS`), canonical equivalence for model letters, Swift's `min`/`max` behaviour with NaN, and stable sorting. Floating-point products that Swift rounds separately are wrapped in an explicit `float64(...)` in Go so the compiler cannot fuse them into FMA instructions (which it does on ARM64); that is what lets the comparison with Swift be bit-exact.

Threads at run time: a pinned OS thread with its own message loop for the hooks (`WH_KEYBOARD_LL`, `WH_MOUSE_LL`, and a WinEvent hook for foreground and focus changes; the handlers only snapshot state and hand the event off without blocking), a goroutine for the engine (all logic in event order; field reads via `SendMessageTimeout` with a 100 ms limit; `SendInput`), the UI thread (hidden window, tray icon, menu, hint popup, clipboard) and a COM thread for MSAA.

### Keeping the port in step

Any change to the Swift algorithm must be ported to the matching Go file and the golden data regenerated on a Mac; the procedure is in [BUILDING.md](BUILDING.md#regenerating-the-golden-data-when-the-swift-algorithm-changes). The Go tests then report any divergence from Swift entry by entry.
