# Changelog

All notable changes to Tilqos. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions follow [SemVer](https://semver.org/).

## [1.0.0] - 2026-10-16

First public release. Tilqos grew out of a private project (LangSwitcher); this is its first release under the MIT license.

### Added
- Native macOS menu-bar app (Swift, macOS 14 or later, no third-party dependencies) that fixes text typed in the wrong keyboard layout and switches the layout, for **English, Russian and Kazakh**; DMG and Homebrew cask (`elapen/tilqos`).
- Automatic correction when the detector is sure, and a **hint at the cursor** (`было → стало`, accept with Tab, `⌥Space` in terminals and editors) when it is not.
- Detection built on evidence rather than on character statistics alone: word-form lexicons (stems plus hunspell affix rules, 48,392 English, 139,035 Russian and 53,991 Kazakh stems), a frequency tier for short words, Kazakh suffix morphology (54,063 stems, 360 suffixes, three morphemes deep), character-trigram models as the last resort, context from neighbouring words and from the text field, pull-in of short neighbours, a wider scan at sentence ends, and guards for URLs, paths, e-mail addresses, code identifiers and acronyms.
- Kazakh as a first-class language: keystroke (not character) buffering, the nine Kazakh letters that become digits on the Russian layout, and "Russian typed on the Kazakh layout looks right, so leave it" ([docs/KAZAKH.md](docs/KAZAKH.md)).
- Layout switching after a fix, with a setting and per-app memory for choosing between the Russian and Kazakh layouts.
- Three sensitivity presets (Cautious 0.93/0.55, Balanced 0.80/0.45, Bold 0.55/0.30) and fine-tuning of both thresholds and of the minimum word length.
- Per-app modes (auto, suggest only, off), always-off password managers, suggest-only terminals.
- Hotkeys: fix the last word `⌥⇧F`, fix the selection `⌥⇧S`, undo `⌥⇧Z`, convert the clipboard `⌥⇧V`, transliterate `⌥⇧T`, switch layout `⌥⇧1/2/3`, toggle `⌥⇧L`; all rebindable.
- Learning from your actions (undo, erase-and-retype, accepted and dismissed hints, your own vocabulary), stored as salted hashes, with switches to turn it off and to forget.
- Kazakh transliteration, Latin to Cyrillic and back: the official 2021 alphabet and the practical ASCII spelling, a beam-search decoder with vowel harmony, mixed Kazakh/Russian text, a brand list.
- Settings window (General, Correction, Languages, Transliteration, Apps, Exceptions, Shortcuts, Diagnostics, About), welcome window with permission walkthrough, five-step tutorial with live practice fields, menu-bar icon states (`EN` / `РУ` / `ҚАЗ`, dimmed, warning).
- Interface and documentation in English, Russian and Kazakh.
- **Privacy behaviour** (see [docs/PRIVACY.md](docs/PRIVACY.md)): the keystroke buffer lives in memory only and is cleared by a timer after 5 seconds without typing, on app switch, on navigation keys, in password fields and on lock or sleep; undo keeps only the most recent correction(s) for at most 2 minutes; the clipboard is touched only by the actions that need it, is restored, never read when flagged concealed or transient, and text over 10,000 characters is ignored; no typed text, clipboard content or window titles are logged (diagnostic logging is off by default: only failures of the app are written; app identifiers are redacted); no network access, telemetry or updater; nothing is read or logged while secure input is active; actions to clear the exception words and to reset the learned words.
- Experimental **Windows port** (Go, standard library only, x64 and ARM64, unsigned): the same engine, a tray icon, hotkeys on `Pause` / `Scroll Lock`, hints with Tab or Insert. **Built and tested on macOS only; never run on real Windows.**
- Test suites: 262 Swift tests (including an accuracy corpus that reads the real macOS layouts and tests for every privacy promise) and 248 Go tests, including bit-for-bit comparison with the Swift implementation on golden data.
- Build scripts for the app, the DMG (ad-hoc signed by default, notarization supported but not exercised), the language models (`generate-models.sh`) and the Windows port.
- Licences and notices for the dictionary-derived data in `THIRD_PARTY_NOTICES.md` and `LICENSES/`, shipped with every build.

### Known limitations
- Builds are ad-hoc signed and not notarized: macOS blocks the first launch, and the Accessibility permission must be re-approved after every update ([docs/PERMISSIONS.md](docs/PERMISSIONS.md)).
- Developed and tested on macOS 27 only; the declared minimum is macOS 14. Apple silicon DMG; Intel untested.
- The accuracy numbers are in-sample and the Kazakh part of the corpus is small ([docs/ACCURACY.md](docs/ACCURACY.md)).
- Learned words are salted 64-bit hashes that are not cryptographically irreversible; exception words are plain text ([docs/PRIVACY.md](docs/PRIVACY.md)).
- Password fields are recognised through macOS Secure Event Input only (on Windows only standard Win32 password controls).
