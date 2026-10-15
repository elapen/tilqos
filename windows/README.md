<img src="../Resources/icon-1024.png" width="96" align="left" hspace="16" vspace="4">

# Tilqos for Windows (experimental)

Fixes text typed in the wrong keyboard layout and switches the layout, for **English, Russian and Kazakh**. The same engine as the macOS app, ported to Go and checked against the Swift implementation on golden data; it lives in the notification area. Windows 10 (1607 or later) and Windows 11, x64 and ARM64.

<br clear="left">

> **Status: experimental.** The port is built, tested and cross-compiled **on a Mac**. The decision engine is verified against Swift (see "Verification"), but the parts that only exist on Windows (the low-level hooks, `SendInput`, the Edit/RichEdit field access, the clipboard, the tray icon, the hint popup) have **never been run on real Windows by the author**; the manual checklist at the end of this page is a to-do list, not a record of results. The executable is **unsigned**. Please try it and report what you see; see [CONTRIBUTING.md](../CONTRIBUTING.md).

```
ghbdtn       →  привет         Latin instead of Cyrillic
руддщ        →  hello          and the other way round
[jhjij       →  хорошо         punctuation keys inside a word
с2леметс3з   →  сәлеметсіз     Kazakh typed on the Russian layout: letters became digits
мне әғ лет   →  мне 25 лет     digits typed on the Kazakh layout
еще, тот, her                  real words are never touched
```

How the detector decides is described once, for both platforms, in [docs/HOW_IT_WORKS.md](../docs/HOW_IT_WORKS.md); privacy in [docs/PRIVACY.md](../docs/PRIVACY.md); the code map in [docs/ARCHITECTURE.md](../docs/ARCHITECTURE.md#the-windows-port). This page covers what is specific to Windows.

## Build

You need only **Go 1.25+**: standard library, no cgo, no third-party modules; the build downloads nothing.

**On macOS or Linux** (cross-compilation):

```bash
cd windows
./build.sh          # models → icons → tests → vet → dist/Tilqos-amd64.exe and -arm64.exe → zip archives
./build.sh fast     # the same without the tests
VERSION=1.0.0 ./build.sh     # override the version (default: `var Version` in internal/winapp/app_windows.go)
```

**On Windows** (PowerShell):

```powershell
cd windows
powershell -ExecutionPolicy Bypass -File .\build.ps1        # or .\build.ps1 -Fast
```

**Manually:**

```bash
go generate ./...   # copies ../Resources/Models and the licence texts (LICENSE, THIRD_PARTY_NOTICES.md) into internal/{models,licenses}/embedded and checks the icons
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-H windowsgui -s -w" -o dist/Tilqos-amd64.exe ./cmd/tilqos
```

Output in `dist/`: `Tilqos-amd64.exe`, `Tilqos-arm64.exe`, and `Tilqos-<version>-Windows-x64.zip` / `Tilqos-<version>-Windows-arm64.zip` (each with `Tilqos.exe`, `LICENSE`, `THIRD_PARTY_NOTICES.md`, `NOTICE` and `LICENSES/`; the archives need `zip` on macOS/Linux, `Compress-Archive` on Windows).

The language models come from `../Resources/Models`, the single source shared with the macOS app: trigram models `en.bin`, `ru.bin`, `kk.bin`, Kazakh morphology `kk-morph.bin`, and word-form lexicons `en-lex.bin`, `ru-lex.bin`, `kk-lex.bin` (about 2 MB together). For `//go:embed` the build copies them into `internal/models/embedded/` (the copies are git-ignored), and `TestEmbeddedModelsMatchSource` **fails** if the copies are missing or differ from the original (the same goes for the embedded licence texts and `TestEmbeddedLicensesMatchSource`: the executable carries `LICENSE` and `THIRD_PARTY_NOTICES.md` inside, reachable from the tray menu under *About → Licenses…*). If a branch is built before the model files exist in it, `TILQOS_MODELS_REV=<git revision> ./build.sh` (or `go run ./tools/syncmodels -rev <revision>`) takes the missing files from git; without that variable a missing file is an error and is never silently replaced.

The icons (`internal/winapp/assets/*.ico`) and the exe icon resources (`cmd/tilqos/rsrc_windows_*.syso`) are generated from `../Resources/icon-1024.png` by `tools/mkico` and are committed; a test fails if they are stale.

## Install and run

1. Unzip `Tilqos-<version>-Windows-x64.zip` (Intel/AMD) or `-arm64.zip` (Snapdragon and other ARM), and put `Tilqos.exe` somewhere permanent, for example `%LOCALAPPDATA%\Programs\Tilqos\Tilqos.exe`.
2. Run it. The file is unsigned, so **SmartScreen may warn**: *More info → Run anyway*. No administrator rights are needed (and they are not recommended).
3. The tray icon appears (possibly behind the ^ arrow; drag it onto the taskbar). On first run there is a short balloon with the hotkeys.
4. In the tray menu you can switch on *Start with Windows* (a value in `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`).

You need at least two of the three layouts: Settings → Time & language → Language & region → add English (United States), Russian, Kazakh (Kazakh keyboard). Tilqos uses the layouts that are installed and re-reads them when the set changes.

Uninstall: turn off *Start with Windows* in the menu, choose *Exit*, delete the exe and the folder `%LOCALAPPDATA%\Tilqos`.

Security software may flag an **unsigned** program with a global keyboard hook; that is why the source is open and the build is reproducible from this repository.

## What it does

- **Auto-fix** of a word closed by a space or punctuation when confidence is high, and a **hint** at the caret when it is medium: accept with **Tab** (in terminals and code editors: **Insert**). Any other key dismisses the hint; **Backspace** too, because it changes the text under the hint; after 4 seconds it disappears by itself.
- **How bold to be:** three modes in the menu, as in the macOS settings: **Cautious** (0.93 / 0.55: only obvious mistakes are fixed automatically), **Balanced** (0.80 / 0.45, the default), **Bold** (0.55 / 0.30). The first number is the automatic threshold, the second the hint threshold.
- At the end of a sentence, a wider look at the whole tail (up to 24 words, two passes: sure fixes first, then the rest with the corrected neighbours) — offered as a hint only.
- After a fix it **switches the layout**. When the text cannot tell Russian from Kazakh, the *Languages* menu decides: "Auto + remember per app" (the Cyrillic layout you used in that app), "Prefer Kazakh" or "Prefer Russian".
- **Checks the text before replacing** where Windows lets it read the field (standard Edit and RichEdit controls; see the table below). If the text before the caret is not what it was about to delete (the app autocorrected, the caret moved), it **does nothing** and forgets the buffer. If the app capitalised the first letter, the replacement is capitalised too (`Ghbdtn` → `Привет`).
- **Learns from your actions** (the *Learning* menu): an undone fix, or a fix that is erased and the original typed again, means the word is never fixed again; a hint dismissed twice with **Esc** is no longer offered (a hint you merely typed past, left with the arrows or Enter, or that timed out means nothing: it was most likely not noticed); a hint accepted (Tab/Insert) or a word fixed with Pause is fixed without asking next time (but among text in another language, only offered); a word of 4 or more letters that no dictionary has (name, slang, term), typed and left in two separate sessions, becomes "your word", but only if the same keys give a dictionary word on no layout. Only salted 64-bit hashes are stored, at most 3,000 entries (older ones are evicted), in `learned.json`; *Reset learned words…* erases everything and changes the salt. Details and the honest limits of hashing: [docs/PRIVACY.md](../docs/PRIVACY.md).
- **Never touches**: URLs, e-mail addresses, paths, `snake_case`/`camelCase`, calls like `foo(`, ALL-CAPS acronyms, words after `$` `` ` `` `#` `\` `~` `@` `%` `&`, words shorter than 3 letters, words on the exceptions list.
- **Undo right after a fix** adds each changed word to the exceptions (`wordExceptions`); undoing a transliteration adds the changed words to "do not convert".
- **Kazakh transliteration** Latin ⇄ Cyrillic (the official 2021 scheme and the practical ASCII scheme decoded with a language model). What is converted, in order: (1) **what you just typed**, if nothing is selected (type a line in Latin, press Alt+Scroll Lock); in Edit/RichEdit fields the words are taken from the screen so the app's autocorrect does not interfere; (2) otherwise the **selection**, through the clipboard, which is restored afterwards; (3) otherwise the **clipboard**. If there is nothing to convert, a short system sound plays.
- **Fix selection** and **convert clipboard** for text that was never typed on your keyboard.
- The tray menu: on/off, smart mode, default mode (auto-fix / suggestions only), how boldly to correct, languages and the Russian-or-Kazakh choice, learning (with *Reset learned words…* and *Clear all exception words…*), "Don’t correct in *app.exe*" and "Suggestions only in *app.exe*" for the current app, fix, undo, transliteration and scheme, layout switching, *Start with Windows*, the settings folder, a shortcut reference, *Interface language*, *About* (privacy summary and licence texts), exit. The menu and messages are in **English, Russian or Kazakh**: they follow the Windows display language unless you choose one in the menu (`uiLanguage`). The Kazakh and Russian texts have had no native review.

## Hotkeys

| Key | Action |
|---|---|
| `Pause` | fix the last word (even a real word: you asked, and it is remembered); **right after a fix, undo it** |
| `Shift+Pause` | fix the selected text |
| `Alt+Pause` | undo the last correction |
| `Alt+Scroll Lock` | transliterate what you just typed, the selection or the clipboard |
| `Tab` / `Insert` | accept a hint (Insert in terminals, VS Code, JetBrains, Excel...) |

Why not `⌥⇧F` / `⌥⇧Z` as on macOS: **Alt+Shift is Windows' standard layout switch**, so Alt+Shift+letter would switch the language at the same time (the same problem as Alt+Shift+F in VS Code), and Ctrl+Alt+letter is AltGr, i.e. character input, on many European layouts. So the keys are the ones people know from Punto Switcher: **Pause/Break** and **Scroll Lock**, which almost no modern application uses. On laptops without them they are usually reachable with Fn (Fn+B, Fn+P, Fn+K, depending on the model).

"Pause right after a fix undoes it" means that when the program got it wrong you press the same key and everything is restored, and the word is remembered as an exception. Once you keep typing, Pause again means "fix the last word".

`Alt` and `Win` in combinations do not open the window menu or the Start menu on release: the program injects an unassigned mask key (as AutoHotkey does). The hotkeys work while correction is switched off. Any of them can be changed or disabled in `config.json` (section `hotkeys`, for example `"undoLastFix": "Ctrl+Alt+Z"`, an empty string for none); also available are `forceEnglish`, `forceRussian`, `forceKazakh`, `convertClipboard` and `toggleEnabled`.

## Differences from the macOS version

| | macOS | Windows |
|---|---|---|
| Checking before a replacement | Accessibility API in almost every app | standard **Edit** and **RichEdit** controls (Notepad, WordPad, dialogs, WinForms/Delphi, many classic programs): the text before the caret is read with `EM_GETSEL` + `WM_GETTEXT` (up to 20,000 characters in the field), waiting up to 32 ms for the space to arrive. Browsers, Electron, Office and terminals have no such field: the keystroke buffer is trusted, as on macOS without Accessibility |
| Replacing text | Accessibility (select exactly the replaced text, write, read back), else Delete + Unicode events of at most 20 characters | in Edit/RichEdit, `EM_SETSEL` + `EM_REPLACESEL` with checks before and after (Ctrl+Z in the field itself undoes the fix); an app where that went wrong gets keystrokes from then on. Otherwise `SendInput`: N × Backspace + the text as Unicode events (chunks of at most 20 UTF-16 units, never splitting a character), **in one call** so your own keystrokes cannot land in the middle. Inserting without deleting is never sent |
| You press a key while a fix is being checked | cannot happen: the tap waits | the program handles its own messages before its input queue, so a key could fall between them: the number of the last keystroke is checked before the selection and before the write, and the fix is **cancelled without touching anything**; the buffer stays correct |
| Selection | Accessibility, or ⌘C/⌘V where the app does not show it | always through the clipboard: Ctrl+C → convert → Ctrl+V, the previous clipboard content restored |
| Context before the caret | Accessibility | from an Edit/RichEdit field, once per buffer session, only the last three words, only in memory |
| Layout tables | `UCKeyTranslate`, by key code | `ToUnicodeEx` for each installed layout, by **scan code** (the physical key); the numeric keypad has separate codes for NumLock |
| Tab with no hint on screen | goes into the buffer as a separator | **resets the buffer** (it inserts a character that is not in the tables) |
| F1 to F24, dead keys, Insert | F keys go into the buffer | reset the buffer |
| Input generated by other programs (on-screen keyboard, a password manager's auto-type) | not applicable | never read, only resets the buffer |
| Password fields | Secure Event Input, entirely | standard Windows fields (`ES_PASSWORD`) only; see the limitations |
| Accepting a hint in terminals | ⌥Space | Insert (Alt+Space opens the window menu on Windows) |
| Hotkeys | ⌥⇧… | Pause / Shift+Pause / Alt+Pause / Alt+Scroll Lock |
| Settings and learning | Settings window, UserDefaults | tray menu, `config.json` and `learned.json`; the menu shows only counts of what was learned |
| Selection in terminals | works (through Accessibility; ⌘C/⌘V are not used there) | Ctrl+C is "interrupt" in terminals, so the selection is not read; transliteration works on what you just typed or on the clipboard |

The recognition algorithm is the same: the same dictionaries, models, confidence levels (0.95 / 0.9 / 0.6), context weight, short-word and hyphen rules, guards.

## Limitations

- **The checks before a replacement exist only in standard Edit and RichEdit controls.** In browsers, Electron apps, Office and terminals Tilqos relies on the keystroke buffer: if the app changed the word itself (autocorrect) before the space arrived, a fix may delete the wrong characters (the same holds for transliteration of what you just typed). In Word it is sensible to switch autocorrect off or set the app to hints only.
- **Elevated windows.** Windows (UIPI) does not deliver keystrokes from elevated windows to an ordinary process and does not accept its input there. Tilqos does nothing in such windows (it recognises them up front and sends no input). Running Tilqos itself as administrator lifts the limit but is not recommended.
- **Games, full-screen apps, Raw Input/DirectInput.** A hint cannot be seen over exclusive full-screen mode, and synthetic input may be ignored. Add the game to the apps where Tilqos is off (*Don’t correct in …*).
- **Remote desktops and virtual machines** (mstsc, AnyDesk, TeamViewer, VMware, VirtualBox, Hyper-V) are off by default: the keys go to another machine with an unknown layout. You can switch them on in the menu.
- **Input generated by another program** is never read (it could be a password from a password manager): on-screen and touch keyboards, another computer's keyboard through Mouse without Borders, Synergy or Barrier, AutoHotkey remaps. Such keystrokes only reset the buffer.
- **Apps with their own text engine.** Almost all understand Unicode input, but not all report the caret position for the hint: first the system caret, then MSAA, otherwise the hint appears next to the mouse pointer.
- **Password fields in browsers and non-standard programs are not recognised**: the `ES_PASSWORD` flag exists only on standard Windows controls. Correction triggers only on a space or punctuation, so a password of letters and digits ended with Enter is not touched, but a password with punctuation in the middle could in theory be "fixed". Password managers (KeePass, KeePassXC, 1Password, Bitwarden and others) and the system credential prompts are always off.
- **Classic console (conhost, cmd.exe)** does not always report the layout of its window correctly; hints only by default in terminals. Windows Terminal works more reliably.
- **Layout switching** is done by sending `WM_INPUTLANGCHANGEREQUEST` to the window and its input field and checking the result. Apps that ignore it (some consoles and games) will not switch: the fix is applied, the layout stays (a line in the log).
- **Chinese, Japanese, Korean and other IME** layouts: no buffer is kept for them.
- **The clipboard** is temporarily replaced when working with a selection. All formats stored as memory blocks are restored (text, HTML, RTF, files, `CF_DIB` pictures); a clipboard over 32 MB is not borrowed at all, and OLE objects and GDI handles (`CF_BITMAP`, metafiles) cannot be saved (synthesised formats such as `CF_BITMAP` from `CF_DIB` come back by themselves). A selection copied by the application may stay in the clipboard history (Win+V): the application puts it there; Tilqos's own temporary text is marked "not for history".
- **Dead keys** (US-International and similar): a character typed through them is not tracked; the buffer is reset.
- "Kazakh language with a Russian keyboard" (HKL `0419043F`) counts as Kazakh: the language comes from the input language, as on macOS.
- 64-bit Windows only (x64, ARM64), Windows 10 1607 or later. UI Automation is not used (in pure Go it is a large COM interface that could not be verified here); adding it is the next step for checking text in browsers.

## Privacy on Windows

The promises are the same as on macOS ([docs/PRIVACY.md](../docs/PRIVACY.md)); what is specific:

- The keystroke buffer is in memory only, at most 256 key codes, emptied by a timer 5 seconds after the last key (the setting can only shorten it), on app or window switch, on Enter, Tab, Esc and the arrow keys, on mouse clicks, on Ctrl/Alt/Win combinations, in standard password fields, and when the session is locked or disconnected or the PC suspends. Undo keeps each correction for at most 2 minutes and is wiped on the same events and when another window comes to the front.
- The text of a field before the caret is read only if the field is a standard Edit/RichEdit control (and not a password field): to verify a replacement and for context (the last three words, once per buffer session). It stays in memory and is never logged.
- Clipboard use is limited to the actions that need it. Content flagged with the Windows formats `ExcludeClipboardContentFromMonitorProcessing`, `CanIncludeInClipboardHistory` = 0 or `CanUploadToCloudClipboard` = 0 is never read, converted or overwritten (Tilqos does not even send Ctrl+C then), text over 10,000 characters is ignored, and a clipboard over 32 MB is not borrowed at all. Tilqos's own temporary text is marked with the same three formats.
- **All files are in `%LOCALAPPDATA%\Tilqos`** (not the roaming profile): `config.json` (settings, per-app rules, the exception words in plain text, transliteration overrides) and `learned.json` (salted 64-bit hashes and small numbers, not words; each install has its own salt; not cryptographically irreversible, see [docs/PRIVACY.md](../docs/PRIVACY.md)). File modes in the code have no meaning on NTFS: access follows the inherited ACL of the folder.
- **Logging is off by default.** If you switch on `diagnostics` (or start with `TILQOS_DEBUG=1`), `%LOCALAPPDATA%\Tilqos\tilqos.log` (rotated at 512 KiB, with one `.old` copy) is written; until you switch it on, no log file or folder is created. It holds lengths, languages, decisions and failure reasons, **never typed characters, clipboard content, window titles or executable names** (those appear as `<private>`). `TILQOS_DEBUG=1` switches it on for that run only and is not saved into `config.json`.
- Input from other programs is never read. Password fields (standard controls only) and password managers are skipped.
- No network, no telemetry.
- Builds from before the first release kept their files in `%APPDATA%\Tilqos` (the roaming profile); on first start those files are moved to `%LOCALAPPDATA%\Tilqos` and the old folder, including its log, is deleted.
- All of the above is covered by tests of the platform-independent code (see "Verification"); the Windows-only parts (the hook thread's password-field pre-check, the lock/disconnect/suspend messages, the clipboard flag reads, the folder migration) are compiled and vetted for x64 and ARM64 but **have never been executed on real Windows**.

## Settings: `%LOCALAPPDATA%\Tilqos\config.json`

Changed from the menu. Edit by hand only while the program is not running (*Exit* in the menu, edit, start): the running program rewrites the file at the next settings change. Wrong values are corrected on load, and an unreadable file is set aside as `config.json.bad`.

| Field | Default | |
|---|---|---|
| `enabled`, `smartMode` | `true` | the main switch; automatic recognition |
| `defaultFixMode` | `"auto"` | `auto` / `suggest` / `off` |
| `autoThreshold`, `suggestThreshold` | `0.8`, `0.45` | confidence for auto-fix and for a hint; the menu modes are 0.93/0.55, 0.80/0.45, 0.55/0.30, other values show as "Custom" |
| `minWordLength` | `3` | shorter words are not touched (not less than 2) |
| `enabledLanguages` | `["en","ru","kk"]` | |
| `cyrillicPreference` | `"automatic"` | `automatic` (per-app memory) / `preferKazakh` / `preferRussian` |
| `acceptKey` | `"tab"` | `tab` (Insert in terminals and editors) or `insert` |
| `suggestionTimeoutSeconds`, `bufferIdleResetSeconds` | `4`, `5` | the idle reset is limited to 1 to 5 seconds: a value outside that range is replaced by 5 |
| `rememberLayoutPerApp` | `true` | remember the Cyrillic layout per app |
| `playSound`, `diagnostics` | `false` | a sound on a fix; the log file (no text) |
| `wordExceptions` | `[]` | words never to fix |
| `appRules` | `{}` | `"chrome.exe": "off"` / `"suggest"` / `"auto"` |
| `hotkeys` | see above | action → key |
| `translitScheme`, `translitOverrides`, `translitNeverConvert` | `"practical"`, `{}`, `[]` | transliteration |
| `uiLanguage` | `"system"` | language of the menus and messages: `system` (follow Windows), `en`, `ru`, `kk` |

What is learned lives next to it in `learned.json` (hashes; saved at most every 2 seconds and at exit; an unreadable file is set aside as `learned.json.bad` and learning starts again). The `schema` field is the version of the rules the entries were learned under (like `learningSchema` on macOS): a file without it, or with an older one, loses everything except explicit acceptances at start-up and is rewritten at once.

## Layout of the code, and porting changes from Swift

```
windows/
  cmd/tilqos/         main; rsrc_windows_*.syso (the exe icon)
  internal/core/      the engine (platform-independent: port of Detect + Apply + Learner)
  internal/translit/  transliteration (port of Translit/*)
  internal/engine/    the pipeline (port of Pipeline.swift) behind a Platform interface
  internal/config/    config.json, learned.json, app rules, hotkeys, modes
  internal/models/    embedded models (copies of ../Resources/Models) and the drift check
  internal/modelsrc/  reads models from Resources/Models (or from git with TILQOS_MODELS_REV)
  internal/win32/     Win32 declarations: DLLs, constants, structs with size checks
  internal/winapp/    the Windows app: hooks, tables, Edit/RichEdit, input, clipboard, tray, hint
  internal/iconkit, tools/   icon and .syso generation; model sync
  internal/golden, testdata/golden/, parity/   data and tools for the comparison with Swift, the corpus
```

The Swift file ↔ Go file table, the thread model, and the way Swift string semantics and floating-point behaviour are reproduced are in [docs/ARCHITECTURE.md](../docs/ARCHITECTURE.md#the-windows-port). To port a change of the algorithm:

1. Port the change to the matching Go file.
2. On a Mac with the ABC, Russian and Kazakh layouts, run `windows/parity/regen-golden.sh` ([docs/BUILDING.md](../docs/BUILDING.md#regenerating-the-golden-data-when-the-swift-algorithm-changes)).
3. `go test ./...`: any divergence from Swift shows up entry by entry.

## Verification

`go test ./...` runs 248 top-level tests (on macOS, Linux and Windows). What they establish, and what they do not:

- **Comparison with Swift** on data dumped from the real Swift implementation, the real macOS layouts and the real models and dictionaries: the regression corpus (all **8,971 runs** through the Go `TypingEngine` as the Swift `HarnessEngine` drives the Swift engine: the screen, every fix and every hint equal to Swift in every run); **7,476 detector sessions, 0 mismatches**; **835 learning scenarios, 0 differences**, hash for hash; 13,011 `WordScorer` readings; 4,248 words × 3 lexicons; 1,941 words × 3 trigram models (costs to the bit); 164 Unicode-semantics strings; 178 guard cases; the text converter and transliteration inputs.
- Ported Swift tests (detector, lexicon, learner, text replacer with a fake field that misbehaves like real apps, tokenizer, replacement planner, trigram model, transliterator), and the whole pipeline (`engine`) with a fake platform and document.
- The corpus on **Windows layouts**, built from scan codes as KBDUS/KBDRU/KBDKAZ have them. **Those tables are hand-written approximations** (on Windows the real ones come from `ToUnicodeEx`), so this checks that the scan-code indexing, the keypad handling and the engine agree with each other, not that they match real Windows layouts:

  | | macOS layouts (comparison with Swift) | Windows layout fixtures |
  |---|---|---|
  | Correct words changed automatically | **0 of 1,858** | **0 of 1,857** (one word with « » cannot be typed there) |
  | Wrong-layout words fixed automatically | 240 of 249 (96.4 %) | 240 of 249 (96.4 %) |
  | Wrong-layout sentences converted completely | 152 of 152 | 152 of 152 |
  | Mixed text and context | 34 of 34 | 34 of 34 |

  These figures are in-sample, like the macOS ones ([docs/ACCURACY.md](../docs/ACCURACY.md)).
- Icons: the `.syso` **links into real x64 and ARM64 exes** and the PE resource tree is checked the way Windows reads it. Models: the embedded copies match the source.
- `GOOS=windows GOARCH=amd64|arm64 go vet ./...` and `go build` are clean. The sizes and offsets of all structures passed to Win32 are checked **at compile time** for both architectures (`internal/win32/types.go`): a wrong layout does not build.

The hook thread, `SendInput`, the Edit/RichEdit controls, the clipboard, the tray and the popup **cannot be exercised on macOS**; the checklist below exists for them.

## Manual test checklist (please help)

Preparation: English (US), Russian and Kazakh layouts installed; Tilqos running; Notepad open. "Type X on layout Y" means press the keys with which X is typed on Y. Reports to [Issues](https://github.com/elapen/tilqos/issues) (Windows version, x64 or ARM64, the app, what happened). Never paste text you actually typed.

1. **Start.** The tray icon; on first run a balloon with the hotkeys. The menu shows the keyboard hook as working. A second start of the exe says it is already running.
2. **Latin → Cyrillic.** On EN type `ghbdtn` and a space → `привет `, the layout became Russian, the next word is typed in Russian.
3. **Cyrillic → Latin.** On RU type the keys of the word `keyboard` and a space → `keyboard `, the layout is EN.
4. **Punctuation inside a word.** On EN type the keys of `хорошо` and `было` (you get `[jhjij ,skj`): both words are fixed whole.
5. **Kazakh on Russian.** On RU type the keys of `сәлеметсіз` as on the Kazakh layout, then a space → `с2леметс3з` is fixed to `сәлеметсіз`, the layout is Kazakh.
6. **Digits on Kazakh.** Type `мне ` (RU), switch to Kazakh, type `25` on the number row (you get `әғ`), then ` лет` and a space → `әғ` becomes `25`.
7. **Must not be touched:** `еще`, `тот`, `тут` (RU), `her`, `tot`, `the` (EN), `hello world`, `привет мир`, `Астана` on the Kazakh layout, `ok`, `NASA`, `Telegram` inside Russian text, `user@example.com`, `https://github.com`, `user_name`, `getUserName()`, `$PATH`.
8. **Hint.** On EN type the keys of `тут` and a space (you get `nen`) → a hint `nen → тут` with a Tab badge at the caret. Tab accepts (the Tab is not typed into the document). Repeat and press any letter: the hint disappears, the letter is typed. Repeat and wait 4 seconds: it disappears. Repeat and press Backspace: it disappears. The hint does not take focus; a click through it reaches the window under it.
9. **Replacement through the field.** In Notepad (Windows 10: Edit, Windows 11: RichEdit), after auto-fix `ghbdtn ` → `привет ` press Ctrl+Z: Notepad itself returns `ghbdtn ` (the replacement was done through the field, not by keystrokes). In a browser the same fix is done by keystrokes.
10. **Pause.** Type `ghbdtn` (no space), press Pause → `привет`. Right after an auto-fix of `ghbdtn ` press Pause → `ghbdtn ` is back, the layout as before; type `ghbdtn ` again → no longer fixed (an exception in `config.json`). Remove it from `wordExceptions`.
11. **Learning.** (a) After the auto-fix `ghbdtn ` → `привет ` erase the word with Backspace, switch to EN and type `ghbdtn ` again: it stays as it is, and later too. (b) Dismiss the hint `nen → тут` twice with Esc: a third time there is none. If you just keep typing or wait, it appears again. (c) Accept a hint with Tab: next time `nen ` is fixed at once. The *Learning* menu shows counts; *Forget all learned* zeroes them.
12. **Modes.** Menu → *How boldly to correct* → *Cautious*: `rfr ` (the keys of `как` on EN) is now offered as a hint, not fixed; *Balanced*: fixed again.
13. **Russian or Kazakh.** *Languages* menu → *Prefer Kazakh*: after fixing `ghbdtn ` the Kazakh layout is switched on; *Prefer Russian*: the Russian one.
14. **Undo unavailable.** In a browser fix a word, type one more letter, press Alt+Pause → a notice that undo is unavailable, the text untouched. Alt+Pause right after a fix is an undo; the window menu (Alt) does not open.
15. **Selection.** Copy something to the clipboard. On EN type `Ghbdtn rfr ltkf`, select it, Shift+Pause → `Привет как дела`, the layout is Russian. Ctrl+V pastes your earlier clipboard content (it was restored).
16. **Transliteration.** On EN type `qalaisyz aga` and, selecting nothing, press Alt+Scroll Lock → `қалайсыз аға`, the layout is Cyrillic; Alt+Pause brings back `qalaisyz aga`. The same in a browser (there, by keystrokes). Select `qalaisyz aga` with the mouse, Alt+Scroll Lock → `қалайсыз аға`. Copy `сәлеметсіз бе`, click an empty spot (selection cleared) and press Alt+Scroll Lock → the clipboard holds `salemetsiz be`; Alt+Pause restores it. With nothing typed or selected and a picture in the clipboard, Alt+Scroll Lock gives a short system sound; so does Shift+Pause without a selection.
17. **Resets.** Type `ghbd`, click elsewhere in the text, type `tn ` → nothing is fixed. The same with Ctrl+A, the arrows, Enter, Alt+Tab.
18. **Backspace.** Type `ghbdtnx`, Backspace, a space → `привет `.
19. **Caps Lock and Shift.** With Caps Lock: `GHBDTN ` is not touched (an acronym). `Ghbdtn ` → `Привет `.
20. **Apps.** Menu → *Don’t correct in “notepad.exe”*: no fixes in Notepad, but in WordPad there are. In Windows Terminal hints only by default, accepted with Insert; Tab works as usual there. A selection plus Shift+Pause in a terminal does not send Ctrl+C.
21. **Browser and Office.** Chrome/Edge (the search field, a Google document), Word, Telegram Desktop: the fix and the hint at the caret (or at the mouse pointer if the app does not report the caret).
22. **Passwords.** A password field in a standard window (for example a Win32 dialog): nothing is fixed. KeePassXC: always off (the menu item for it says so).
23. **Administrator.** Open Notepad "as administrator": no fixes, the program does not hang, no input errors in the log.
24. **Layouts.** Add or remove a layout in Settings, go back to Notepad: it works without a restart (the log, if switched on, says layouts changed). US-International layout: dead keys do not break the text.
25. **DPI and monitors.** A second monitor with a different scale (100 % / 150 %): the hint is sharp, the right size, at the caret on both monitors. Light and dark app themes.
26. **Tray.** Restart Explorer (Task Manager → Windows Explorer → Restart): the icon comes back. Switch off from the menu: the icon is grey, no fixes; the hotkeys still work.
27. **Autostart.** Switch it on in the menu, sign out and in: the program is running.
28. **Load.** Type fast for a few minutes, also with CPU load: typing does not lag, the hook does not drop out (the menu says it is working; with logging on, "watchdog: hooks reinstalled" may appear).
29. **ARM64.** Items 2 to 9 on a Windows on ARM device with `Tilqos-arm64.exe`.
30. **Log and files.** With `diagnostics` off no log file is created. With it on, `%LOCALAPPDATA%\Tilqos\tilqos.log` contains no typed characters; `learned.json` contains only hexadecimal hashes and numbers.
