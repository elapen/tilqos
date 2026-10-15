# Permissions

Tilqos watches the keyboard system-wide, so macOS makes you grant it access explicitly. This page says which permissions it asks for, why, how to grant and reset them, and why you have to do it again after an update of an ad-hoc signed build. For what the app does with what it sees, read [PRIVACY.md](PRIVACY.md).

## What Tilqos asks for

| Permission (System Settings → Privacy & Security) | Needed? | What it is used for |
|---|---|---|
| **Accessibility** | **Yes, in practice** | Creates the keyboard event tap that can *swallow* the key that accepts a hint (so Tab is not also typed), reads the caret and the text before it in the focused field, replaces the wrong text through the Accessibility API, and posts the synthetic key events used where that API does not work. It also covers reading the keyboard, so it is the only grant you normally need. |
| **Input Monitoring** | Optional fallback | Lets the app *read* keystrokes. With only this grant Tilqos can still detect mistakes and show hints, but it cannot apply them. If Accessibility is granted, macOS may not even list Tilqos under Input Monitoring, and that is normal. The app asks for Accessibility first and, by design, does not prompt for Input Monitoring on top of it. |

Not requested: Screen Recording, Full Disk Access, Automation / Apple Events, Contacts, Location, Camera, Microphone, network access. The app has no entitlements file (hardened runtime is on, App Sandbox is not) and no `NS…UsageDescription` strings. It does not use the network at all.

Accessibility is a powerful permission: any app that has it can read what you type and control other apps. Read the source (`Sources/TilqosKit/Input/EventTap.swift` and `Sources/TilqosKit/Apply/`) before you grant it, or build it yourself ([BUILDING.md](BUILDING.md)).

## Granting it

1. Install and open Tilqos from Finder, the Dock, Spotlight or `open -a Tilqos`. (See "Launch it normally" below.)
2. The welcome window asks for Accessibility and opens System Settings at **Privacy & Security → Accessibility**. Switch **Tilqos** on. Authenticate if macOS asks.
3. The menu-bar icon changes from a warning triangle to the current layout (`EN`, `РУ` or `ҚАЗ`), and the first line of its menu says that keyboard monitoring is working. The window notices the grant by itself; you do not have to restart the app.

Tilqos only appears in the list **after it has asked**; checking a permission's state does not create the row. That is why the welcome window requests it as soon as it opens (and why the "Allow…" button asks again).

Settings → Diagnostics shows the state of both permissions, whether the keyboard tap is running (with a *Restart* button if macOS stopped it), the loaded language models, the current layout, and the command for watching the log.

### Launch it normally

macOS credits a permission request to the **responsible process**, the top-level ancestor. An app started from a shell hands the request to the terminal, and then never shows up in the Accessibility or Input Monitoring lists. Start Tilqos from Finder, the Dock, Spotlight, a login item or `open -a`. The repository's `./build.sh run` uses `open` for this reason; `./build.sh run-direct` exists for debugging only and warns about it.

## After an update: ad-hoc builds must be re-approved

macOS ties a granted permission to the app's **code identity**. Release builds of Tilqos are **ad-hoc signed** (they carry no certificate; see [BUILDING.md](BUILDING.md#signing)), and an ad-hoc signature is derived from the binary itself, so it is different for every build. After you replace Tilqos with a newer version, macOS treats it as a different app: the old entry in the list may still look switched on, but it no longer applies to the new binary and Tilqos silently stops seeing the keyboard. The same happens after you rebuild from source without a signing certificate.

To fix it:

1. Quit Tilqos (menu → Quit).
2. System Settings → Privacy & Security → Accessibility. Switch the old **Tilqos** entry off and on; if it still does not work, select it and press **−** to remove it.
3. Open Tilqos again and grant the permission when asked.

Or reset from Terminal and let the app ask again:

```bash
tccutil reset Accessibility io.github.elapen.tilqos
tccutil reset ListenEvent   io.github.elapen.tilqos     # Input Monitoring, if you had granted it
```

(`tccutil reset All io.github.elapen.tilqos` resets everything macOS holds for the app.) If you build from source and have an "Apple Development" certificate in your keychain (a free Apple ID is enough in Xcode → Settings → Accounts → Manage Certificates), `build.sh` uses it automatically and the identity is stable across rebuilds, so the grants survive.

## Gatekeeper is a separate matter

Gatekeeper decides whether macOS lets you *open* an app that is not notarized; the Accessibility permission decides what the app may *do*. The first-launch steps for unnotarized builds are in the [README](../README.md#install). Clearing one does not clear the other.

## When nothing seems to happen

Almost every permission problem looks the same ("nothing is fixed"), so check in this order:

1. Is the icon a warning triangle? Open the menu: the top row says what is missing. Settings → Diagnostics shows each permission.
2. Is Tilqos switched on in System Settings → Privacy & Security → Accessibility **for this build**? After an update, redo the steps above.
3. Is the tap running? Diagnostics → *Restart*.
4. Are at least **two** of the English, Russian and Kazakh layouts added in System Settings → Keyboard → Input Sources? With a single layout there is nothing to switch to. Settings → Languages shows which layouts Tilqos found.
5. Is the app set to *Off* or *Suggest only* for the app you are typing in? (Menu → *Don't correct in …*, Settings → Apps.) Terminals are suggest-only by default; password managers are always off.
6. Is a password field (or another app that enables **Secure Event Input**) focused? Tilqos deliberately sees nothing then ([PRIVACY.md](PRIVACY.md)).

For the log (lengths, languages, decisions, never characters): diagnostics are off by default and then only failures of the app are written. To see the whole trail, run `defaults write io.github.elapen.tilqos diagnosticsEnabled -bool true` and restart Tilqos, then `log stream --level debug --predicate 'subsystem == "io.github.elapen.tilqos"'`; switch it off again with `defaults delete io.github.elapen.tilqos diagnosticsEnabled`.

## Removing everything

Switch off *Open at login* in Settings (or remove it in System Settings → General → Login Items), quit Tilqos, drag the app to the Trash, then:

```bash
tccutil reset All io.github.elapen.tilqos
defaults delete io.github.elapen.tilqos        # settings, exceptions, learned words
```

There is no installer and no background service; nothing else is left behind.

## Windows

- **No administrator rights** are needed and none are requested; there are no permission prompts.
- Tilqos installs low-level keyboard and mouse hooks and a foreground-window event hook for your user session, and sends input with `SendInput`. It reads and writes text only in standard Windows **Edit** and **RichEdit** controls (through `EM_GETSEL` / `WM_GETTEXT` / `EM_REPLACESEL`); other applications get synthetic keystrokes.
- **Windows of elevated programs are not seen** (UIPI): Windows does not deliver keystrokes from an elevated window to a normal process or accept its input. Tilqos does nothing there. Running Tilqos itself as administrator removes the limit, but is not recommended.
- The executable is **unsigned**: SmartScreen warns on first launch ("More info → Run anyway"), and security software may flag an unsigned program with a global keyboard hook.
- The port is experimental and has never been run on real Windows by its author; see [windows/README.md](../windows/README.md).
