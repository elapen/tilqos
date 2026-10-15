<div align="center">

<img src="Resources/icon-1024.png" width="112" alt="Tilqos icon">

# Tilqos

**Fixes text typed in the wrong keyboard layout, and switches the layout for you.**<br>
English, Russian and Kazakh. A native macOS menu-bar app: `ghbdtn` becomes `привет`, `с2леметс3з` becomes `сәлеметсіз`.<br>
Everything runs on your Mac: no cloud, no account, no network, no telemetry.

[![Release](https://img.shields.io/github/v/release/elapen/tilqos?color=4a5cff)](https://github.com/elapen/tilqos/releases/latest)
[![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)
[![Platforms](https://img.shields.io/badge/platforms-macOS%2014%2B%20%C2%B7%20Windows%20%28experimental%29-lightgrey)](#install)
[![Русский](https://img.shields.io/badge/README-%D0%A0%D1%83%D1%81%D1%81%D0%BA%D0%B8%D0%B9-blue)](README.ru.md)
[![Қазақша](https://img.shields.io/badge/README-%D2%9A%D0%B0%D0%B7%D0%B0%D2%9B%D1%88%D0%B0-blue)](README.kk.md)

<img src="docs/images/hero.png" width="860" alt="Tilqos in suggest mode: a hint card at the cursor offering to turn ghbdtn into привет, plus the English and Kazakh examples">

</div>

*Tilqos* (Kazakh «Тіл қос», "switch the language on") is an independent open-source project. It is similar in purpose to Punto Switcher and is **not affiliated with, or endorsed by, Yandex or Punto Switcher**.

## Why Tilqos

You start a sentence, glance up, and the screen says `ghbdtn rfr ltkf` instead of `привет как дела`. Tilqos notices, rewrites the words, switches the layout, and lets you carry on typing.

It is for people who type in several of **English, Russian and Kazakh** on a Mac every day: bilingual and trilingual writers, students, developers, anyone in Kazakhstan or the Russian-speaking world who keeps hitting the wrong layout.

**Kazakh is a first-class language here, not an add-on.** On a Mac, the Russian and Kazakh layouts agree on all 33 Russian letters and differ only on the number row (plus a few edge keys), where Kazakh puts its nine extra letters and Russian puts digits:

| Key | ABC | Russian | Kazakh |
|---|---|---|---|
| `2` | `2` | `2` | **ә** |
| `3` | `3` | `3` | **і** |
| `4` | `4` | `4` | **ң** |
| `5` | `5` | `5` | **ғ** |
| `=` | `=` | `=` | **һ** |
| `9` | `9` | `9` | **ұ** |
| `-` | `-` | `-` | **ө** |
| `8` | `8` | `8` | **ү** |
| `0` | `0` | `0` | **қ** |

(Verified against the real macOS layouts by the test suite; full table in [docs/KAZAKH.md](docs/KAZAKH.md).) Three things follow, and the whole design leans on them:

1. **Latin ↔ Cyrillic garbles the whole word:** `привет` ↔ `ghbdtn`. Easy to recognise.
2. **Kazakh typed on the Russian layout breaks only the nine Kazakh letters.** Seven of them turn into digits and two into `=` and `-`: `сәлеметсіз` → `с2леметс3з`, `қазақ` → `0аза0`. A digit inside a Cyrillic word is a very strong signal.
3. **Russian typed on the Kazakh layout looks right** (`Астана` stays `Астана`): nothing to fix, no layout to switch. That removes the biggest source of false corrections.

On the Kazakh layout the digits live on Option, so Tilqos remembers **key presses, not characters**; a character-to-character table cannot represent that without loss.

<div align="center">
<img src="docs/images/demo.gif" width="760" alt="Animation: ghbdtn is typed, a hint appears at the cursor, Tab turns it into привет and the layout switches">
</div>

<sub>The animation is an illustration of the flow, not a screen recording, and it shows **suggest mode** (the default for terminals and for low-confidence words). In Auto mode a word the detector is sure about (confidence above the preset threshold) is replaced as soon as you type the space, without a card.</sub>

## What it does

- **Fixes it for you, or offers.** When the detector is sure it replaces the word as soon as you type the space or punctuation; when it is not, it shows a small card at the cursor (`было → стало`) and **Tab** accepts it. Any other key dismisses it and the word stays as typed.
- **Switches the layout** after a fix, so the next word comes out right. If a corrected word has none of the nine Kazakh letters, Russian and Kazakh look the same; Tilqos then follows your setting or the Cyrillic layout you last used in that app.
- **Never fixes a word that is a real word.** `еще`, `тот`, `her` are left alone. URLs, paths, e-mail addresses, `snake_case` and `camelCase`, calls like `foo(`, ALL-CAPS acronyms and words after `$ # @ \` are never touched.
- **Menu-bar icon** shows the current layout as a keycap (`EN`, `РУ`, `ҚАЗ`), dimmed when switched off, and a warning triangle when a permission is missing. The menu says in words whether it is working, has the on/off switch, "Don't correct in *this app*" and one-click undo of the last correction.
- **Per-app control.** Auto, suggest-only or off for each app (Settings → Apps). Password managers and the login window are always off; terminals are suggest-only by default, with `⌥Space` to accept because Tab is needed there.
- **Presets: Cautious, Balanced, Bold** (automatic / hint thresholds 0.93/0.55, 0.80/0.45, 0.55/0.30), with both thresholds and the minimum word length adjustable.
- **Undo** with `⌥⇧Z`; undoing right away teaches it to leave that word alone.
- **Learning** from what you do: undone fixes, words you erase and retype, hints you accept or dismiss, and words you keep that no dictionary knows. Stored as hashes, can be switched off, forgotten word by word or all at once.
- **Kazakh transliteration** Latin ⇄ Cyrillic (`⌥⇧T`): the official 2021 alphabet or the practical ASCII chat spelling (`qalaisyz` → `қалайсыз`), decoded with vowel harmony and Kazakh morphology, mixed Kazakh/Russian text included.
- **Fix selection / convert clipboard** for text that was never typed on your keyboard.
- **Interface in English, Russian and Kazakh**: it follows the system language, or you can pick one in Settings → General. Open at login is optional and off by default.

| Hotkey | Action |
|---|---|
| `⌥⇧F` | fix the last word (even a real word; you asked) |
| `⌥⇧S` | fix the selection |
| `⌥⇧Z` | undo the last correction |
| `⌥⇧V` | convert the clipboard |
| `⌥⇧T` | transliterate Latin ⇄ Cyrillic |
| `⌥⇧1` / `⌥⇧2` / `⌥⇧3` | switch to English / Russian / Kazakh |
| `⌥⇧L` | switch Tilqos on or off (works while it is off) |
| `Tab` | accept a hint (`⌥Space` in terminals and code editors) |

All of them can be rebound in Settings → Shortcuts.

## Screenshots

| | |
|---|---|
| <img src="docs/images/onboarding.png" alt="Welcome window that walks through granting the Accessibility permission"> | <img src="docs/images/settings.png" alt="Settings window, Correction pane with the Cautious, Balanced and Bold presets"> |
| **A welcome window** walks through the permission, then a five-step tutorial lets you practise on the real engine | **Settings** in the style of System Settings: presets in words, thresholds if you want them |
| <img src="docs/images/hint.png" alt="The hint card shown in suggest mode: ghbdtn to привет with the Tab key"> | |
| **The hint card** appears at the cursor and never steals focus | |

## Install

| Platform | Get it | Notes |
|---|---|---|
| **macOS 14+**, Apple silicon | [`Tilqos-1.0.0-macOS-arm64.dmg`](https://github.com/elapen/tilqos/releases/latest) | Drag to Applications. Not notarized: see "First launch" below. |
| macOS via Homebrew | `brew install --cask elapen/tap/tilqos` | The same app, installed by the cask; the same first-launch notes apply. |
| From source | `git clone https://github.com/elapen/tilqos && cd tilqos && ./build.sh install` | Needs a Swift 6.0+ toolchain (Xcode 16+). Installs to `/Applications`. Intel Macs are untested. |
| **Windows 10/11**, *experimental* | [`Tilqos-1.0.0-Windows-x64.zip` or `-arm64.zip`](https://github.com/elapen/tilqos/releases/latest) | **Never run on real Windows by the author.** Unsigned. See [Windows](#windows-experimental). |

Release notes list the SHA-256 checksum of every file. Tilqos has **no updater**: watch the [Releases page](https://github.com/elapen/tilqos/releases).

*Compatibility:* macOS 14 is the declared minimum (`Info.plist`, `Package.swift`), but Tilqos was developed and tested on macOS 27 only; macOS 14 to 26 are unverified.

### First launch (Gatekeeper)

Release builds are **ad-hoc signed and not notarized**, so macOS blocks the first launch:

1. Open the DMG and drag **Tilqos** to **Applications**, then open it.
2. When macOS refuses, on **macOS 14**: Control-click the app → **Open** → **Open**. On **macOS 15 and later** that shortcut is gone: open **System Settings → Privacy & Security**, scroll to the message about Tilqos and press **Open Anyway**.
3. If you prefer Terminal and trust the download: `xattr -dr com.apple.quarantine /Applications/Tilqos.app`.

### Permissions

macOS makes you grant an app that watches the keyboard its access by hand.

- **Accessibility** is the one to grant (System Settings → Privacy & Security → Accessibility → switch **Tilqos** on; the welcome window opens it for you). Tilqos needs it to see key presses, to swallow the key that accepts a hint (so Tab is not also typed), to read the text before the caret, and to replace the wrong text.
- **Input Monitoring** is an optional fallback: it lets Tilqos *read* keystrokes, so it can detect and offer hints but cannot apply them. If Accessibility is on, macOS may not list Tilqos under Input Monitoring at all, and that is normal.
- Nothing else is requested: no Screen Recording, no Full Disk Access, no network.

> **After an update, re-approve the permission.** macOS ties a grant to the app's code identity, and an ad-hoc signature is different for every build. After you install a newer Tilqos the old entry can look switched on but no longer apply. Quit Tilqos, remove the **Tilqos** entry in Privacy & Security → Accessibility (**−**) or run `tccutil reset Accessibility io.github.elapen.tilqos`, open Tilqos again and grant it again. Details, and what to do when nothing happens: [docs/PERMISSIONS.md](docs/PERMISSIONS.md).

Launch Tilqos from Finder, the Dock or `open -a Tilqos`, not from a shell: macOS credits the permission request to the process that started the app.

You need at least two of the **ABC (or US), Russian and Kazakh** layouts added in System Settings → Keyboard → Input Sources; Tilqos switches only among the layouts you have.

## How it decides

Evidence, strongest first (details and every number in [docs/HOW_IT_WORKS.md](docs/HOW_IT_WORKS.md)):

1. **Is what you typed a real word?** Then it is never rewritten automatically.
2. **Is the other reading a real word?** Layout garble is not a word in any language (`ghbdtn`); its conversion is (`привет`). Brands and terms are the opposite. That asymmetry is what makes correcting safe. "Real word" means word forms from the hunspell dictionaries (stems plus affix rules, not just headwords), your own vocabulary and a brand list.
3. **Frequency, for short words.** A three-letter fix is accepted only if it lands on one of the language's most frequent words (`ult` → `где`, yes; `afk` → `фал`, no).
4. **Context.** The script of neighbouring words, and of the text before the caret, tilts a close call but never decides it.
5. **Structure.** A digit or hyphen inside a Cyrillic word (`с2лем`) is Kazakh on the Russian layout whatever a dictionary says. Kazakh suffix morphology (`десеңіздер` = `десе` + `ңіз` + `дер`) backs it up.
6. **Character trigram models last,** for words no dictionary has, and then only as a hint, never automatically.

A word ends only at a key that is not a letter on *any* of your layouts, so `,skj` (`было`) and `[jhjij` (`хорошо`) are not cut in half.

## Accuracy

On the project's own regression corpus (`swift test --filter CorpusTests`), with default settings:

| | |
|---|---|
| Correctly typed words changed | **0 of 1,858** |
| Correctly typed sentences changed | **0 of 132** |
| Words typed on the wrong layout, fixed automatically | **240 of 249 (96.4 %)**; 6 more offered as hints |
| Sentences typed on the wrong layout, converted completely and correctly | **152 of 152** |
| Mixed-script and context cases | **34 of 34** |

**Read this before you trust those numbers.** The corpus was written by the author and the decision constants were tuned against it, so these are *in-sample* results, not a held-out benchmark. The Kazakh part is small (for example 20 Kazakh sentences per direction). The corpus is everyday vocabulary; jargon, code, names and heavily mixed chat will do worse. The simulation drives the real detector with the real macOS layouts but not real apps. They reproduce with the commands in [docs/ACCURACY.md](docs/ACCURACY.md); nothing is claimed about other products, and no CPU or memory figures are published because nothing in the repository measures them.

## Privacy

Tilqos has to watch every key you press to notice a wrong layout, so here is exactly what it does. [docs/PRIVACY.md](docs/PRIVACY.md) has the details and the limits.

- **The typed-word buffer lives only in memory.** It is cleared after 5 seconds without typing (by a timer), when you switch apps, on Return, Tab and Escape-type boundaries, in password fields, and when the Mac locks or sleeps.
- **Undo keeps only the most recent correction(s)**, in memory, for at most 2 minutes; they are wiped on lock or sleep, quit and app switch.
- **The clipboard is touched only by the actions that need it** (fix selection, transliterate, convert clipboard). Your previous contents are restored. Content flagged concealed or transient (password managers) is never read. Text over 10,000 characters is ignored.
- **No typed text, clipboard content or window titles are ever logged.**
- **Stored locally:** settings, per-app exclusions, the exception words you add (plain text) and the learned vocabulary as **salted 64-bit FNV-1a hashes**. Those hashes are *not* cryptographically irreversible: someone who can read the file could guess short, common words. Settings has *Clear exception words* and *Reset learned words*.
- **No network access, no telemetry, no updater.**
- **Nothing is read or logged while secure input is active** (macOS password fields and anything else that turns Secure Event Input on). Fields that do not turn it on are not protected by that mechanism.

Limits: other software with the same permission sees what Tilqos sees; the OS can write crash dumps containing memory; the builds are not notarized, so check the checksums or build from source. Privacy bugs are security bugs: see [SECURITY.md](SECURITY.md).

## Windows (experimental)

`windows/` is a Go port of the same engine (standard library only, x64 and ARM64). It lives in the tray, uses `Pause`, `Shift+Pause`, `Alt+Pause` and `Alt+Scroll Lock` as hotkeys (Alt+Shift is Windows' layout switch), accepts hints with Tab or Insert, and checks the text in standard Edit/RichEdit fields before replacing it.

**Status: experimental.** It is built and tested on a Mac (cross-compiled), it produces the same decisions as the Swift engine on all the golden data, but its hooks, `SendInput`, clipboard, tray and popup code has **never been run on real Windows by the author**. The executable is unsigned (SmartScreen will warn). Password fields in browsers are not recognised there. Please test it and report what you see: [windows/README.md](windows/README.md) has the build steps, the differences from macOS, the limitations and a manual test checklist.

```bash
cd windows && ./build.sh        # needs Go 1.25+; macOS or Linux (cross-build) or: .\build.ps1 on Windows
```

## FAQ

**Is it a keylogger?** It reads every key in order to do its job, which is why this page, [docs/PRIVACY.md](docs/PRIVACY.md) and the source exist. It keeps the typed-word buffer in memory only, logs no text, and has no network code.
**It stopped working after an update.** Re-approve the Accessibility permission (see above).
**It changed a word I typed correctly.** `⌥⇧Z` straight away: that word is added to the exceptions and it will not happen again. Choose the *Cautious* preset if it keeps happening, and open an issue.
**It did nothing.** Check [docs/PERMISSIONS.md](docs/PERMISSIONS.md#when-nothing-seems-to-happen): permission for *this* build, at least two layouts installed, the app's mode, a password field in focus.
**Why isn't it notarized?** Notarization needs an Apple Developer ID certificate; the build scripts support it but it has not been exercised. Help is welcome.
**More languages?** Possibly; see [CONTRIBUTING.md](CONTRIBUTING.md#adding-a-language) and the *Language request* issue form.
More in [docs/FAQ.md](docs/FAQ.md).

## Documentation

[How it decides](docs/HOW_IT_WORKS.md) · [Accuracy](docs/ACCURACY.md) · [Kazakh](docs/KAZAKH.md) · [Privacy](docs/PRIVACY.md) · [Permissions](docs/PERMISSIONS.md) · [Building](docs/BUILDING.md) · [Architecture](docs/ARCHITECTURE.md) · [FAQ](docs/FAQ.md) · [Changelog](CHANGELOG.md)

## Contributing

Bug reports, Windows testing, Kazakh and Russian text review, accuracy corpora, notarization help and new languages are all welcome: [CONTRIBUTING.md](CONTRIBUTING.md). Ideas and questions go to [Discussions](https://github.com/elapen/tilqos/discussions). The Kazakh text in particular has had no professional review: native speakers, please tell us what to fix.

## Credits and licences

Tilqos's own code is [MIT](LICENSE). The language data in `Resources/Models/` is a **machine-generated, lossy derivative of third-party dictionaries and keeps their licences**: the hunspell Kazakh dictionary (MPL 1.1 / LGPL 2.1+ / GPL 2.0+, by Alexey Lipchansky, Akmaral Mussayeva, László Németh, Rail Aliev and others), the LibreOffice Russian dictionary (Alexander I. Lebedev, BSD-style licence), the LibreOffice US English dictionary derived from SCOWL (Kevin Atkinson) and Ispell (Geoff Kuenning), and letter statistics from `/usr/share/dict/words`. Full credits, the description of modifications, where the source is, and the Go licence for the Windows build: [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md); verbatim upstream licence texts: [`LICENSES/`](LICENSES). The macOS app links only Apple system frameworks.
