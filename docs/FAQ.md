# FAQ

## Basics

**What does the name mean?** «Тіл қос» is Kazakh for "switch the language on" (literally "add/switch on a language"). The app fixes text typed in the wrong keyboard layout and switches the layout for you.

**Which languages?** English, Russian and Kazakh, in any direction between them. Kazakh is a first-class language, not an add-on: see [KAZAKH.md](KAZAKH.md). Other languages are not supported; [open a language request](https://github.com/elapen/tilqos/issues/new?template=language_request.yml) and read the "adding a language" section of [CONTRIBUTING.md](../CONTRIBUTING.md).

**How is it different from Punto Switcher?** It does the same kind of job. It is an independent project and is **not affiliated with Yandex or Punto Switcher**. What it adds is Kazakh as an equal language (including the Kazakh letters that become digits on the Russian layout), open source under the MIT license, and a design where nothing leaves your Mac. Comparing detection quality with other products has not been done, and no claim is made.

**Which macOS versions and Macs?** The declared minimum is macOS 14; it was developed and tested on macOS 27 only, so macOS 14 to 26 are unverified. The release DMG is for Apple silicon. Intel Macs should work if you build from source, but that has not been tested.

**Is there a Windows version?** An experimental one, built from the same engine in Go. It has never been run on real Windows by its author and is unsigned. See [windows/README.md](../windows/README.md). There is no Linux version.

**How do I switch it off?** Menu-bar icon → the switch at the top, or `⌥⇧L` (which still works while the app is off). Settings → Apps switches it off per application.

**What do `EN`, `РУ` and `ҚАЗ` in the menu bar mean?** The layout that is active now. The icon is dimmed when Tilqos is switched off and becomes a warning triangle when a permission is missing or the keyboard tap has stopped.

## Privacy and permissions

**Is it a keylogger?** It has to read every key you press in order to notice a wrong layout, so it *is* in a position to be one, and that is why this project publishes its source and a precise description of what it keeps ([PRIVACY.md](PRIVACY.md)). In short: the typed-word buffer lives only in memory and is cleared after 5 seconds without typing, on app switch, on navigation keys, in password fields and on lock or sleep; typed text, clipboard content and window titles are never logged; there is no network code, no telemetry and no updater. The things that *are* saved on disk are listed honestly in that document, including the weak spot (learned words are hashes that can be guessed by someone who can read the file).

**Why does it need Accessibility?** To create the keyboard tap that can swallow the accept key, to read the text before the caret, and to replace the wrong text. Details and the optional Input Monitoring grant: [PERMISSIONS.md](PERMISSIONS.md).

**I updated Tilqos and it stopped working although the switch is on.** Expected with ad-hoc signed builds: macOS ties the permission to the code identity and the new build has a different one. Remove the old entry and grant it again; the steps and the `tccutil` commands are in [PERMISSIONS.md](PERMISSIONS.md#after-an-update-ad-hoc-builds-must-be-re-approved).

**Does it work in password fields?** While macOS's Secure Event Input is on (native password fields, most browsers' password fields, Terminal with "Secure Keyboard Entry"), Tilqos reads and logs nothing. Fields that do not enable it (some custom-drawn fields, Electron or Java apps, virtual-machine windows) are not protected by that mechanism. Password managers (Keychain Access, 1Password, Bitwarden, KeePassXC, Dashlane, Enpass) and the login window are always excluded. On Windows only the standard Win32 password edit control is recognised; password fields in browsers are not.

**Does it send anything anywhere?** No. There is no network code in the app, no telemetry, no crash reporting and no update check. Because there is no updater, watch the [Releases page](https://github.com/elapen/tilqos/releases) for new versions.

**What is stored on disk?** Settings, per-app rules, the words you added to the exceptions list (plain text), your transliteration overrides and "do not convert" words (plain text), the Cyrillic layout last used in each app, and the learned vocabulary as salted hashes. Everything is in the app's `UserDefaults` domain `io.github.elapen.tilqos` on macOS. Settings → Exceptions has *Clear exception words* and *Reset learned words*. See [PRIVACY.md](PRIVACY.md).

## Installing

**macOS refuses to open it.** The builds are ad-hoc signed and not notarized (notarization requires an Apple Developer ID certificate). See the install steps in the [README](../README.md#install): on macOS 14 right-click → *Open*; on macOS 15 and later use System Settings → Privacy & Security → *Open Anyway* after the first blocked attempt. If you would rather not trust a binary, build it from source: `./build.sh install`.

**Why not notarized?** Notarization needs an Apple Developer ID certificate. The notarization path in the build scripts exists but has not been exercised. If you can help, see [CONTRIBUTING.md](../CONTRIBUTING.md#where-help-is-most-welcome).

**Where do I find the settings?** Menu-bar icon → *Settings…* (`⌘,`). The panes are General, Correction, Languages, Transliteration, Apps, Exceptions, Shortcuts, Diagnostics and About (which repeats the privacy summary and shows the licence files). The interface language (English, Russian, Kazakh, or the system's) is chosen in General.

## Using it

**Nothing is being fixed.** Go through the checklist in [PERMISSIONS.md](PERMISSIONS.md#when-nothing-seems-to-happen): permission (for this build), the tap running, at least two of the three layouts added in System Settings → Keyboard → Input Sources, the app's mode (terminals are suggest-only by default), and a password field being focused.

**It changed a word I typed correctly.** Press `⌥⇧Z` straight away: the word goes onto the visible exceptions list and Tilqos learns not to touch it. If it keeps happening, choose the *Cautious* preset (Settings → Correction) and please open an issue with the word and the app (not your text). Correctly typed words are never rewritten automatically in the project's own corpus, but that corpus is not your typing ([ACCURACY.md](ACCURACY.md)).

**It offered a correction instead of making it.** That is the middle zone of confidence: Tab accepts, any other key dismisses it (it disappears by itself after a few seconds; Esc twice for the same word teaches it to stop offering). In terminals and code editors the accept key is `⌥Space`, because Tab is needed there.

**Why does it leave some words alone, like `ult` for `где`?** When both readings are real words (`где` on the English layout is `ult`, `the` on the Russian layout is `еру`), guessing without context would be wrong as often as right. Press `⌥⇧F` to fix the last word on request. With text in the same language in front of it, Tilqos offers the fix as a hint.

**It does not fix words shorter than 3 letters.** By design: there is too little to judge. A confident fix pulls in short neighbours (`z nt,z` becomes `я тебя`), and you can lower the length floor to 2 in Settings → Correction → fine-tune.

**It does not touch my URLs, paths, e-mail addresses and code identifiers.** Right: these are always left alone ([HOW_IT_WORKS.md](HOW_IT_WORKS.md#6-things-that-are-never-touched)).

**Does it work in VS Code, Terminal, iTerm2, Chrome, Telegram?** Terminals are suggest-only by default, with `⌥Space` to accept. Apps that draw their own text (VS Code, Cursor, terminals, IntelliJ, Sublime Text) get keystrokes instead of Accessibility edits. For selections in apps that do not show them to Accessibility (browsers, Electron apps, Telegram), "fix selection" and "transliterate" copy with ⌘C and paste back, restoring your clipboard afterwards. In practice the behaviour in each app depends on how well that app supports Accessibility; report the ones that misbehave.

**Russian or Kazakh? It switched to the wrong one.** When the corrected word has none of the nine Kazakh letters, Russian and Kazakh look the same, so Tilqos uses your setting (Settings → Languages → *Russian or Kazakh*) and, by default, the Cyrillic layout you last used in that app. A word that does contain one of the nine letters switches to Kazakh.

**Can I change the hotkeys?** Yes, Settings → Shortcuts. Defaults: `⌥⇧F` fix the last word, `⌥⇧S` fix the selection, `⌥⇧Z` undo, `⌥⇧V` convert the clipboard, `⌥⇧T` transliterate, `⌥⇧1` / `⌥⇧2` / `⌥⇧3` switch to English / Russian / Kazakh, `⌥⇧L` on/off.

**How do I report a problem without sharing what I typed?** The log contains lengths, languages, confidences and decisions, never characters. By default only failures of the app itself are logged; to include the per-word trail, switch diagnostics on (`defaults write io.github.elapen.tilqos diagnosticsEnabled -bool true` and restart Tilqos) and run `log stream --level debug --predicate 'subsystem == "io.github.elapen.tilqos"'`. Read what it prints before pasting any of it into an issue (app identifiers appear as `<private>`), and never paste text you typed.

## Removing

See [PERMISSIONS.md](PERMISSIONS.md#removing-everything): quit, delete the app, `tccutil reset All io.github.elapen.tilqos`, `defaults delete io.github.elapen.tilqos`. On Windows: exit, turn off the start-with-Windows option in the tray menu, delete the exe and the folder `%LOCALAPPDATA%\Tilqos` (see [windows/README.md](../windows/README.md)).
