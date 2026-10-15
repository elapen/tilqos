# Privacy

Tilqos has to see every key you press in order to notice that you are typing in the wrong keyboard layout. That is a lot of trust to ask for, so this page says exactly what the app does with those keys, what it keeps and for how long, what it writes to disk, and where these promises stop. Where a statement can be checked against a test or a place in the source, the table at the end says where.

## The promises

| | |
|---|---|
| **P1** | The typed-word buffer lives **only in memory**. It is cleared after **5 seconds without typing** (a timer, not "at the next key press"), when you switch apps, on Return, Tab, Escape and other navigation keys, in password fields, and when the Mac locks or goes to sleep. |
| **P2** | **Undo** keeps only the most recent correction(s), in memory, for **at most 2 minutes**. They are wiped when the Mac locks or sleeps, when you switch apps, and when the app quits. |
| **P3** | The **clipboard** is touched only by the actions that need it. Its previous contents are restored. Content flagged *concealed* or *transient* (password managers) is never read. Text longer than 10,000 characters is ignored. |
| **P4** | **No typed text, clipboard content or window titles are ever logged.** (On Windows the log is off by default and lives in `%LOCALAPPDATA%\Tilqos`.) |
| **P5** | Stored **locally**: settings, per-app exclusions, exception words you added (**plain text**), and the learned vocabulary as **salted 64-bit FNV-1a hashes**. Those hashes are *not* cryptographically irreversible: someone who can read the file can guess short, common words. Settings has *Clear exception words* and *Reset learned words*. |
| **P6** | **No network access, no telemetry, no updater.** |
| **P7** | **Nothing is read or logged while secure input is active.** |

Everything below is detail and the honest limits of these promises. If anything on this page and the behaviour of the app disagree, that is a bug, and because the app reads your keystrokes, a privacy bug is a security bug: see [SECURITY.md](../SECURITY.md).

## What Tilqos sees

- **macOS.** A system-wide event tap receives key-down events, modifier changes and mouse-button-down events from every app. It needs the Accessibility permission ([PERMISSIONS.md](PERMISSIONS.md)).
- **Windows (experimental).** Low-level keyboard and mouse hooks and a window-focus event hook.

For each text key it records the **physical key code, the modifiers and the layout that was active**, not the character. The text is reconstructed from those codes with your installed layouts when it is needed. Your Command and Control shortcuts are not text: they only reset the buffer. Keys that other programs inject (on Windows: auto-type from a password manager, an on-screen keyboard, remote-input tools) are never read; they only reset the buffer.

## What it keeps in memory

| What | How long |
|---|---|
| The **stroke buffer**: at most 256 recent key codes (with modifiers, timestamps and layout ids) | Emptied by a **timer** 5 seconds after the last key (not lazily at the next key; the period can be set between 1 and 5 seconds in the preferences, never longer than 5), when you switch apps, on Return / Enter / Tab / Escape, the arrow keys, Home, End, Page Up/Down and forward-delete, on a mouse click, on any Command or Control shortcut, in password fields (see P7), and when the Mac locks, starts the screen saver, sleeps, or switches user. Quitting wipes it too. Typing Backspace removes the last stroke. |
| The **last finished word** and the set of words typed since the last reset (so that a repeated word is counted once for learning), as text | Cleared with the buffer |
| **Text before the caret.** When the buffer is short (fewer than 24 strokes), up to 160 characters before the caret are read from the focused field through Accessibility, immediately cut down to the last three words, and used to judge which script you are writing in. Terminals and code editors are skipped. | Kept in memory until the buffer is cleared; never written to disk or the log |
| **Characters about to be replaced.** Before a correction, the app reads those characters from the focused field to check that they are what it thinks they are. Where an app cannot return just a range, the whole field value may be fetched for that moment (only if the field is shorter than 20,000 characters) and the slice cut out of it. | Not kept |
| The **undo list**: the most recent corrections (at most 20), as the original and corrected text, and the previous clipboard text if the correction was a clipboard conversion, for **Undo last correction** (`⌥⇧Z`) | Each entry at most 2 minutes, dropped by a timer; the whole list is wiped on lock, sleep and quit, and whenever another app or window comes to the front (on macOS that includes Tilqos's own Settings window) ([P2](#the-promises)) |
| A pending **hint**: the before and after text of the word it is offering | Until accepted, dismissed or timed out (4 seconds by default) |
| The app-to-method cache (which apps need keystrokes instead of Accessibility edits) | Until you quit; shown in Settings → Diagnostics |

None of this is written to disk or sent anywhere. (The operating system can write crash dumps and swap memory; see "Limits" below.)

## What it writes to disk

macOS: the app's `UserDefaults` domain, `~/Library/Preferences/io.github.elapen.tilqos.plist`. Windows: `config.json` and `learned.json` in `%LOCALAPPDATA%\Tilqos` (local to the machine, not the roaming profile; see [windows/README.md](../windows/README.md)). Pre-release builds that kept their files in `%APPDATA%\Tilqos` are migrated on first start and the old folder, including its log, is deleted.

| Data | Form |
|---|---|
| Settings: thresholds, hotkeys, enabled languages, accept key, hint timeout, transliteration scheme, and so on | numbers, switches, key codes |
| **Per-app rules** (apps you switched off, or set to suggest only) | bundle identifiers (macOS) / executable names (Windows) |
| **Exception words** (added by you, or by undoing a correction right after it) | **plain text**, lower-cased |
| Transliteration **overrides** you pinned, and **"do not convert"** words (from undoing a transliteration) | **plain text** |
| The **Cyrillic layout last used in each app** where you typed Cyrillic (for choosing Russian or Kazakh after a fix; Settings → Languages can turn it off) | bundle identifier → layout identifier |
| **Learned vocabulary** | **salted 64-bit FNV-1a hashes**, see below |
| Whether onboarding and the tutorial were completed, how many times the hint explained its keys | flags and a counter |

### The learned vocabulary, honestly

When you undo a correction, erase and retype a word, accept or dismiss a hint, or keep a word that no dictionary knows in two separate sessions, Tilqos remembers *something about that word* ([HOW_IT_WORKS.md](HOW_IT_WORKS.md#9-learning)). It does not store the word. It stores `hex(hash) → [kind, language, count, day]`, where `hash` is a 64-bit FNV-1a over a random 8-byte salt and the normalised word, at most 3,000 entries, the least recently used evicted first, and `day` is the day number of last use (granularity: days).

- Only letters, `-` and `'` are ever learned. Digits and symbols never.
- **This is not secure hashing.** FNV-1a is not a password hash, and the salt is stored in the same place as the hashes. Anyone who can read the file can test any list of candidate words against it in milliseconds. Short, common words can be guessed; a rare word typed twice may tell someone that you typed it. The store is kept small, local and capped, and can be wiped with one action. If your threat model includes someone who can read your user's preferences, turn learning off (Settings → Exceptions → Learning).
- *Reset learned words* erases all entries and replaces the salt. *Forget one word*: type it and the matching entry is removed. Learning can be switched off entirely.
- The exception words list is plain text on purpose: you can see it and edit it. *Clear exception words* empties it.

## The clipboard

Only these actions touch it: **Fix selection**, **Transliterate** and **Convert clipboard** (`⌥⇧S`, `⌥⇧T`, `⌥⇧V`).

- *Fix selection / Transliterate* read the selection through Accessibility where the app allows it. For apps that do not show their selection (browsers, Electron apps, Telegram) the app first checks that your clipboard is not flagged (if it is, it does not even press `⌘C`, because that would overwrite it), saves your clipboard in memory, presses `⌘C`, reads the copied text, converts it, writes the result flagged as transient, presses `⌘V`, and then restores your previous clipboard. The first copy is made by the *other* app, so a clipboard manager may record the selected text; Tilqos's own temporary text is flagged so well-behaved managers skip it. In terminals and code editors this path is switched off.
- *Convert clipboard* rewrites the clipboard text on purpose, and the result is an ordinary clipboard entry (not flagged), because you asked for that text to be there.
- Content flagged as **concealed**, **transient** or **auto-generated** (the nspasteboard.org markers that password managers and other utilities set), on any item, is never read, converted or overwritten. Text over **10,000 characters** is ignored and not kept. A clipboard whose contents are too large to save and put back faithfully (over 32 MiB) is not borrowed at all, and your clipboard is not restored over something you copied in the meantime. When the clipboard is flagged, the clipboard-based actions do nothing at all (on macOS you hear the system beep; on Windows it is silent).

## Logging

macOS: the unified log, subsystem `io.github.elapen.tilqos`. **Diagnostics are off by default**: then only warnings and errors are written, which are failures of the app itself (the tap could not start, a model was rejected, a replacement was refused). With diagnostics on, lifecycle and per-word lines are written too. Messages contain lengths, languages, confidence values, which replacement route was used (`accessibility` or `synthetic`), permission state and model counts. They **never contain typed characters, clipboard content or window titles**. The one value that comes from your machine and is not a number, the identifier of the app in front (a bundle id), is logged as a private value: the unified log shows it as `<private>`. Timestamps of corrections still reveal *when* you typed and in which language, which is why diagnostics are off by default. There is no switch for this in Settings yet; it is the `diagnosticsEnabled` preference. To see exactly what is logged, switch diagnostics on and stream the log:

```bash
defaults write io.github.elapen.tilqos diagnosticsEnabled -bool true     # then restart Tilqos; `defaults delete ...` switches it off again
log stream --level debug --predicate 'subsystem == "io.github.elapen.tilqos"'
```

(`TILQOS_DEBUG=1` in the environment, as `./build.sh run-direct` sets it, does the same and also echoes to stderr.)

Windows: logging is **off by default**; when you switch it on (`diagnostics` in `config.json`, or `TILQOS_DEBUG=1`) the log lives in `%LOCALAPPDATA%\Tilqos`, is rotated at 512 KiB (one `.old` copy), and follows the same content rules; executable names are not logged.

## Network

None. The macOS app contains no networking code (no `URLSession`, `NWConnection`, sockets or WebKit; check with `grep`), no update check, no telemetry and no crash reporter, and the Windows executable's dependency list contains no `net` or `crypto/tls` package. The only things the app opens are `x-apple.systempreferences:` links in System Settings and, on Windows, the folder of its own files. (Building from source can download dictionaries, and signing and notarizing talk to Apple's servers; the app does not.) Because there is no updater you will not be told about new versions: watch the [Releases page](https://github.com/elapen/tilqos/releases).

You can check for yourself: block the app in a firewall, or list its sockets with `lsof -i -a -p $(pgrep -x Tilqos)`.

## Password fields, secure input, and excluded apps

- **macOS Secure Event Input.** While any app has Secure Event Input on (native password fields, most browsers' password fields, Terminal with *Secure Keyboard Entry*), Tilqos does not read or buffer keys, drops its buffer, and disables its hotkeys ([P7](#the-promises)). This is the only password-field mechanism. It does **not** cover fields that do not enable Secure Event Input: some custom-drawn fields, Electron or Java apps, virtual-machine and remote-desktop windows.
- **Always off** (cannot be turned on): Keychain Access, 1Password (three bundle ids), Bitwarden, KeePassXC, Dashlane, Enpass, SecurityAgent and the login window. Other password managers (the Apple Passwords app, LastPass, NordPass, Proton Pass) are **not** on that list; add them in Settings → Apps.
- **Suggest only by default**: Terminal, iTerm2, Ghostty, kitty, Warp, Alacritty. Keystrokes are still held in the in-memory buffer there, so a typed `sudo` password is in memory for the buffer's lifetime, and is not written anywhere.
- **Windows:** only standard Win32 password edit controls (`ES_PASSWORD`) are recognised (the hook asks first, so keys typed into such a field are never described to the engine); password fields in browsers and custom controls are **not**. Password managers and credential prompts are always excluded, remote-desktop and VM clients are off by default, elevated windows are not seen.
- You can switch the app off per application (menu-bar → *Don't correct in …*, Settings → Apps).

## Permissions

Accessibility is a powerful permission: it lets an app read what you type and control other apps. Tilqos asks for it because it cannot work without it, asks for nothing else, and is open source so you can read what it does with it. See [PERMISSIONS.md](PERMISSIONS.md).

## Starting at login and removal

*Open at login* is **off by default** and is a standard macOS login item you can see and remove in System Settings → General → Login Items; no LaunchAgent is installed. (Windows: a value in `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`.) To remove everything, see [PERMISSIONS.md](PERMISSIONS.md#removing-everything).

## Limits of these promises

- The operating system can write crash dumps and swap memory pages out; those can contain whatever was in process memory at the time, i.e. at most the live buffer.
- Any other software with Accessibility or Input Monitoring permission (or a keyboard hook on Windows) can see everything Tilqos sees, without asking. Tilqos cannot protect you from it.
- Release builds are **ad-hoc signed and not notarized**. A download is only as trustworthy as the place you got it from: check the checksums in the release notes, or build from source.
- The engine is tuned on a hand-written corpus, so it can make mistakes: an automatic correction of a word you typed correctly is a nuisance, not a leak, but it is why undo and per-app switches exist ([ACCURACY.md](ACCURACY.md)).
- The Windows port is experimental and has not been run on real Windows by its author; treat its password-field handling in particular as weaker than the macOS one.

## How this is verified

Every promise has an implementation you can find by symbol name and tests that fail if it is broken. The tests use fake clocks and in-memory pasteboards: nothing sleeps, and no test touches your real clipboard, your preferences or macOS's secure-input state. Run them with `swift test --filter Privacy` (the `Privacy*Tests` classes in `Tests/TilqosKitTests/`) and `cd windows && go test ./...` (the `privacy_test.go` files in `windows/internal/{core,config,engine,winapp}`).

| | macOS: where, and which tests | Windows: where, and which tests |
|---|---|---|
| **P1** buffer | `StrokeBuffer` (256 strokes, `maximumIdleReset = 5`, `clamped`), `TypingEngine.noteActivity` / `idleCheck` (the timer; `Core/Scheduler.swift`), `Pipeline.lockedOrAsleep` / `wipeSessionMemory`, `EventTap` (navigation keys, shortcuts, clicks). `PrivacyBufferTests`: `testBufferIsEmptiedFiveSecondsAfterTheLastKeyWithoutAnotherKey`, `testTimerMeasuresFromTheLastKeyNotTheFirst`, `testIdleResetCannotBeStretchedBeyondFiveSeconds`, `testBufferStaysBoundedAndHoldsKeyCodesNotText`, `testTabEmptiesTheBufferLikeReturnAndEscape`, `testAppSwitchEmptiesTheBuffer`, `testLockAndSleepEmptyTheBuffer`, `testStoppingWipesTheSessionToo`, `testSecureInputEmptiesTheBufferBeforeAnythingIsRead` | `core/typing.go` (`noteActivity`, `idleCheck`), `core/buffer.go` (`MaxIdleReset`), `engine.Wipe`, lock/disconnect/suspend handling in `winapp/ui_windows.go`. `core/privacy_test.go`: `TestIdleTimerEmptiesTheBufferWithoutAnotherKey`, `TestIdleResetCannotBeStretchedBeyondFiveSeconds`, `TestBufferStaysBounded`; `engine/privacy_test.go`: `TestIdleTimerEmptiesTheEngineBufferWithoutAnotherKey`, `TestIdleTimerForgetsTheTextReadFromTheField`, `TestWipeForgetsBufferOfferAndUndo`; `config/privacy_test.go`: `TestBufferIdleResetIsLimitedToFiveSeconds` |
| **P2** undo | `UndoStack` (`defaultLifetime = 120`, one timer aimed at the oldest entry), `Pipeline.appActivated` and `wipeSessionMemory`. `PrivacyUndoTests`: `testEntryIsWipedByTheTimerAtTwoMinutes`, `testTimerWipesWithoutAnyoneAskingTheStack`, `testEachEntryHasItsOwnTwoMinutes`, `testUndoAfterExpiryDoesNothingAndDoesNotCrash`, `testClipboardUndoExpiresWithTheRest`, `testFrontmostAppChangeWipesTheUndoList`, `testLockAndSleepWipeTheUndoList`, `testQuitWipesTheUndoList` | `core/undo.go` (`DefaultUndoLifetime`, `DropExpired`), `engine` (`armUndoTimer`, `Foreground`). `core/privacy_test.go`: `TestUndoEntryExpiresAtTwoMinutes`, `TestClipboardBeforeExpiresWithTheRest`; `engine/privacy_test.go`: `TestUndoIsWipedByTheTimerAtTwoMinutes`, `TestAnotherWindowInFrontWipesTheUndoList` |
| **P3** clipboard | `ClipboardPolicy` and `NSPasteboardBoard` in `Apply/ClipboardBoard.swift` (the three nspasteboard.org markers, 10,000), `SelectionClipboard` (save everything, 32 MiB cap, restore only if nothing changed meanwhile), `Pipeline` (`usableClipboardText`). `PrivacyClipboardTests`: `testEachMarkerMakesTheClipboardOffLimits`, `testTextLimitIsTenThousandCharacters`, `testSelectionIsCopiedConvertedPastedAndTheOldClipboardComesBackInFull`, `testConcealedClipboardIsLeftAloneAndNoShortcutIsSent`, `testConcealedSelectionIsDroppedUnreadAndTheClipboardRestored`, `testOverlongSelectionIsIgnoredAndNotKept`, `testClipboardTooBigToPutBackIsNotBorrowed`, `testClipboardIsNotRestoredOverSomethingTheUserCopiedMeanwhile`, `testCommandsRefuseAConcealedClipboard` | `core/clipboardpolicy.go` (the three "exclude from monitoring / history / cloud" formats, `MaxClipboardText`), `winapp/clipguard.go`. `winapp/privacy_test.go`: `TestFlaggedClipboardIsNeverReadConvertedOrOverwritten`, `TestTextOverTenThousandCharactersIsIgnored`, `TestClipboardTooBigToPutBackIsNotBorrowed`, `TestTheTemporaryPasteIsMarkedWithTheSameFormats`; `engine/privacy_test.go`: `TestFlaggedClipboardIsNeitherReadNorConvertedNorKept`, `TestClipboardTextOverTheLimitIsIgnoredAndNotKept` |
| **P4** logging | `Core/Log.swift` (public message from numbers and fixed words, `private:` for the app identifier, diagnostics off by default). `PrivacyLogTests`: `testTypedWordNeverReachesTheLog` (types real words through the real detector and replacer and checks the captured log), `testPrivateDetailIsRedactedEverywhereWeCanSeeIt`, `testDiagnosticsAreOffByDefaultAndOnlyFailuresAreWritten`, `testNoStrayLoggingCalls`, `testEveryLoggedInterpolationHasBeenReviewed` (a new logged value fails the test until someone reviews it) | `winapp/logger.go` (off by default, creates no file until enabled), `config.DataDir`. `engine/privacy_test.go`: `TestTheLogNeverContainsTypedCopiedOrNamedThings`; `winapp/privacy_test.go`: `TestLoggerIsOffByDefaultAndCreatesNothing`, `TestEveryLoggedValueHasBeenReviewed`, `TestNoStrayOutput`; `config/privacy_test.go`: `TestDataDirIsLocalAppDataNotTheRoamingProfile`, `TestDiagnosticsAreOffByDefault` |
| **P5** storage | `Prefs.clearExceptionWords()` and the other `clear…` functions, `Learner.resetLearnedVocabulary()`, the Settings → Exceptions actions. `PrivacyStoreTests`: `testClearExceptionWordsRemovesTheWordsAndTheKey`, `testResetLearnedVocabularyWipesEntriesAndRotatesTheSalt`, `testLearnedWordsAreStoredAsSaltedHashesAndSayWhatTheyAre`, `testTheHashIsGuessableGivenTheSaltSoNoOneMayClaimOtherwise`, `testNoSourceFileCallsTheHashesOneWayOrIrreversible` (the code may not call the hashes one-way) | `config.Store.ClearExceptionWords`, `LearnerStore.ResetLearnedVocabulary`. `config/privacy_test.go`: `TestClearExceptionWordsRemovesThemFromTheFile`, `TestLearnerStoreResetWritesTheEmptyStoreAtOnce`; `core/privacy_test.go`: `TestLearnedStoreIsHashedNotTextAndTheHashIsGuessableWithTheSalt`, `TestNoSourceCallsTheHashesOneWay` |
| **P6** network | `PrivacyNetworkTests`: `testNoSourceFileUsesTheNetworkOrStartsProcesses` (scans for networking APIs), `testPackageHasNoDependencies` | `winapp/privacy_test.go`: `TestNoNetworkCode` (imports, DLL names, `go.mod`), `TestWindowsExecutableLinksNoNetworkPackage` (`go list -deps` has no `net*` or `crypto/tls`) |
| **P7** secure input | `EventTap.process` (a probe for `IsSecureEventInputEnabled`; the event is passed through before any hotkey, backspace, stroke or reset logic), `Pipeline.blockedBySecureInput` on every command that reads text. `PrivacySecureInputTests`: `testNoKeyIsReadOrHandledWhileSecureInputIsOn`, `testNothingIsLoggedWhileSecureInputIsOnOrWhenItBeginsAndEnds`, `testTypingResumesWhenSecureInputEnds`, `testCommandsThatReadTextDoNothingWhileSecureInputIsOn`, `testTheSameCommandsWorkWhenSecureInputIsOff` | Standard password controls only: `winapp/hook_windows.go` (the hook asks first), `engine.blockedByPasswordField`. `engine/privacy_test.go`: `TestPasswordFieldKeysAreNeverBuffered`, `TestCommandsDoNothingWhileAPasswordFieldIsInFront`, `TestNothingIsLoggedWhileAPasswordFieldIsInFront` |

### What is not verified

- **Windows runtime behaviour.** The hook thread, the lock/disconnect/suspend notifications, the clipboard format reads, `%LOCALAPPDATA%` path handling and the tray are compiled and vetted for amd64 and arm64 but have **never run on real Windows**. Their platform-independent cores (the engine, the clipboard rules behind an in-memory clipboard, the logger, the source audits) are tested on any OS.
- **macOS system integration.** The real `IsSecureEventInputEnabled`, the real lock/sleep notifications (`com.apple.screenIsLocked` is an undocumented but long-stable distributed notification) and the general pasteboard are exercised through probes, fakes and a private named pasteboard, not live. The real event tap needs Accessibility and is not started by the tests.
- **That every secure-input-like field enables Secure Event Input.** See the limits above.
- **Source audits are only as good as their scanners.** The log and network tests scan source files for known patterns (and check that the scanner still finds what it is meant to find); they cannot prove the absence of every possible way to log or connect.
