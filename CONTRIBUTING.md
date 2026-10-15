# Contributing to Tilqos

Thank you for helping! Tilqos is deliberately small: a Swift package with no third-party dependencies, a Go port that uses only the standard library, and data files generated from open dictionaries. Please keep it that way (no new dependencies without a very good reason). Because the app reads keystrokes, there is one more rule that outranks style: **nothing you add may keep, log or send more than [docs/PRIVACY.md](docs/PRIVACY.md) says.**

## Where help is most welcome

- **Windows testing.** The Go port is built and tested on a Mac and has never been run on real Windows by the author. Even "it starts and the tray icon appears" is useful. The checklist is at the end of [windows/README.md](windows/README.md); please report which Windows version, architecture and apps you tried.
- **Kazakh review.** The Kazakh text (the UI strings, [README.kk.md](README.kk.md), [docs/KAZAKH.md](docs/KAZAKH.md)) was written without a professional reviewer. If you are a native speaker, corrections to wording, terminology and spelling are valuable: see [Kazakh review](#kazakh-review).
- **Notarization.** Release builds are ad-hoc signed and not notarized, so macOS blocks the first launch and users must re-approve the Accessibility permission after every update. A notarized, Developer ID signed build would fix both. The build scripts already contain the notarization path ([docs/BUILDING.md](docs/BUILDING.md#signing)); it has not been exercised.
- **More languages.** See [Adding a language](#adding-a-language). Open a *Language request* issue first.
- **Linux.** Nobody has tried. A port would need an input layer (X11 / Wayland / libinput are quite different from each other) and layout tables; the engine in `windows/internal/core` is platform-independent Go.
- **Real-world bug reports and app compatibility.** Which apps does it misbehave in, and how? (Without pasting your text: see [Reporting bugs](#reporting-bugs).)
- **A harder accuracy corpus.** The published numbers are in-sample ([docs/ACCURACY.md](docs/ACCURACY.md)). Realistic sentences, with the layout mistakes people actually make, from people who write in all three languages, would make them honest. Only contribute text you are free to publish and that contains nothing private.
- **Performance and memory measurements** on your hardware (nothing in the repository currently publishes CPU or RAM figures).

## Development setup

You need macOS 14 or later and a Swift 6.0+ toolchain (Xcode 16+ or the Command Line Tools). For the Windows port, Go 1.25+.

```bash
git clone https://github.com/elapen/tilqos && cd tilqos
swift build                      # library + app + GenModel
swift test                       # 262 tests; 6 UI-snapshot tests skip unless TILQOS_SNAPSHOTS is set
./build.sh run                   # build, sign and launch build/Tilqos.app through LaunchServices
(cd windows && go test ./...)    # 248 top-level tests, runs on macOS/Linux/Windows
```

**Test prerequisite: enable the ABC, Russian and Kazakh keyboard layouts** (System Settings → Keyboard → Input Sources). The tests read the real system layouts, not fixtures. Without them most of the Swift suite is *skipped, not failed*, so a green `swift test` on a machine without the Kazakh layout proves very little; please make sure the numbers in the summary say what you expect (262 run, 6 skipped).

Launch Tilqos through LaunchServices (`./build.sh run`, Finder, `open -a`), not from a shell: macOS credits the permission request to the process that started the app, and from a terminal it never appears in the Accessibility list. After a rebuild without a signing certificate, macOS asks for the permission again ([docs/PERMISSIONS.md](docs/PERMISSIONS.md)); an *Apple Development* certificate in your keychain (a free Apple ID is enough) is picked up automatically and avoids that.

The code map is in [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md); how the detector decides is in [docs/HOW_IT_WORKS.md](docs/HOW_IT_WORKS.md); build, signing, model generation and golden-data regeneration are in [docs/BUILDING.md](docs/BUILDING.md).

### Two things that are easy to get wrong

1. **Settings storage in tests.** Never use `UserDefaults.standard`, `UserDefaults()` or a named suite in a test: a named suite leaves a plist in `~/Library/Preferences` that nothing cleans up. Use `ScratchDefaults` (a `UserDefaults` backed by a temporary directory). `ScratchDefaultsTests` fails if a test source breaks this rule.
2. **The two implementations must agree.** If you change the detection algorithm in Swift, port it to the matching Go file under `windows/internal/` and regenerate the golden data on a Mac with the ABC, Russian and Kazakh layouts: `windows/parity/regen-golden.sh`, then `go test ./...` ([docs/BUILDING.md](docs/BUILDING.md#regenerating-the-golden-data-when-the-swift-algorithm-changes)). Never edit `windows/testdata/golden/*.json` by hand. If you cannot run the regeneration, say so in the pull request and keep the change out of the algorithm.

## Adding a language

An outline, not a recipe: the engine currently has exactly two scripts (Latin, Cyrillic) and three languages (`Lang` in `Sources/TilqosKit/Core/Lang.swift`), and the data formats assume it. Opening a *Language request* issue first lets us agree whether and how your language fits. The pieces:

1. **A dictionary under a licence we can ship.** Hunspell (`.dic` + `.aff`) is what the pipeline reads. Permissive or weak-copyleft licences (BSD, MIT, MPL, LGPL) are fine as long as the notices can travel with the app; strong copyleft or "non-commercial" terms are not. Record the source, the checksum and the licence in [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) and add the upstream README and licence text to `LICENSES/`.
2. **The language itself:** a `Lang` case (script, badge, display name, the system language code), plus its alphabet if it uses letters the shared Cyrillic or Latin alphabets lack. Growing an alphabet changes the trigram cube size and therefore the binary model format and the Go decoder. A language that fits the existing 42-letter Cyrillic or 26-letter Latin alphabets is far easier.
3. **Models:** extend `generate-models.sh` to fetch the dictionary (and pin its checksum), add `wordlists/<lang>-extra.txt` (brands, terms, colloquial words) and `wordlists/<lang>-core.txt` (the most frequent words, including inflected forms: this frequency tier decides the short-word cases), run the script, commit the generated `<lang>.bin` and `<lang>-lex.bin`. A language with rich morphology (as Kazakh) may also need its own suffix data.
4. **Keyboard layouts:** tables are built from whatever layouts the system has, so the work is mostly to check which layouts users of your language really use and what each key produces. Document the layout facts (see [docs/KAZAKH.md](docs/KAZAKH.md) for the shape of that document).
5. **The corpus:** add frequent words, correctly typed sentences, and wrong-layout words and sentences to `Tests/TilqosKitTests/Corpus.swift`. A language without corpus coverage cannot be accepted: the corpus is what enforces "never rewrite a correct word".
6. **Everything user-facing:** UI strings in every supported language, README and docs.
7. **The Go port:** `windows/internal/core` (`lang.go`, `alphabet.go`, `models.go`), the embedded model list, and regenerated golden data.

## UI strings and translations

User-facing text goes through the localisation layer, in **English, Russian and Kazakh**. Never add a hard-coded string to a view.

- **macOS app:** add the key to `Resources/en.lproj/Localizable.strings` first (English is the source language and the fallback; keys are semantic, like `general.enabled.title`), then translate it in `ru.lproj` and `kk.lproj`. Use `%@` / `%d`, or positional `%1$@` / `%2$@` where word order may change; plural keys end in `.one`, `.few`, `.many`, `.other`. Read strings in code through `L10n` (`Sources/TilqosKit/UI/L10n.swift`). `L10nTests` fails if a key is missing in a language, a value is empty or an untranslated copy, the placeholders differ between languages, a source file uses a key that does not exist, or a plural category is missing. `InfoPlist.strings` and the installer text (`Resources/Installer/Install.{en,ru,kk}.txt`) follow the same rule.
- **Windows tray app:** `windows/internal/i18n` (typed keys in `keys.go`, texts in `en.go`, `ru.go`, `kk.go`); its tests fail when a language lacks a key or the format verbs differ.
- If you can write only one language, say so in the pull request and the others will be filled in. Check a light and a dark window, and remember that Russian and Kazakh strings are usually longer than English ones, so layouts must not clip them. `TILQOS_SNAPSHOTS=<dir> swift test --filter UISnapshotTests` renders the windows to PNG.

## Kazakh review

The Kazakh strings and documents are written to be idiomatic, but have had no professional review. To suggest a correction:

- open a pull request with the change, **or**
- open an issue (any template; say that it is about the Kazakh text) saying which text, what is wrong and what it should be.

The Kazakh UI texts are in `Resources/kk.lproj/Localizable.strings` and `windows/internal/i18n/kk.go`, and the file header says they need native review. Terminology we would like a native speaker's opinion on: *орналасым* for "keyboard layout" (colloquially *раскладка*), *болдырмау* for "undo", *ұсыныс* for "hint/suggestion", *ерекшеліктер* for "exceptions", *қолданба* for "app", and the Kazakh names of the macOS settings. Please say if a text sounds like a translation rather than something a Kazakh speaker would write.

## Pull requests

1. Open an issue first for anything bigger than a small fix, so we agree on the direction.
2. Keep the change focused and match the surrounding style (concise code; comments say *why*, not *what*).
3. Add or update a test when you touch the detector, the replacer, the learner, the transliterator or anything privacy-relevant. Tests stay offline and deterministic, and use real system layouts where the behaviour depends on them.
4. Anything that changes **what is kept in memory, written to disk, logged or sent** must update [docs/PRIVACY.md](docs/PRIVACY.md) and its tests in the same pull request. No network code, ever. No logging of typed text, clipboard content or window titles, not even at debug level.
5. If you change accuracy-relevant constants, include the `CorpusTests` report before and after.
6. Do not commit secrets, signing material, personal data, real e-mail addresses or text that someone typed. Fixtures use invented names and text.
7. By contributing you agree that your work is released under the MIT license (the dictionary-derived data files keep their own licences, see [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)).

## Reporting bugs

Use the bug template. The useful facts are: macOS version and chip, Tilqos version (Settings → General), how you installed it, the app you typed in, which keyboard layouts are enabled, and a **made-up example** of the words involved.

**Never paste text you actually typed, clipboard contents or window titles.** Tilqos's log contains only lengths, languages, confidences and decisions, so it is safe to attach after you have read it. By default only failures are logged; to include the per-word trail, run `defaults write io.github.elapen.tilqos diagnosticsEnabled -bool true`, restart Tilqos, reproduce the problem, and use `log show --last 10m --predicate 'subsystem == "io.github.elapen.tilqos"'` (or `log stream --level debug …` while you reproduce it). Do not report security or privacy problems in a public issue: see [SECURITY.md](SECURITY.md).

By taking part you agree to follow the [Code of Conduct](CODE_OF_CONDUCT.md).
