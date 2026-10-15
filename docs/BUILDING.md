# Building Tilqos

Everything is built locally; there is no CI. The macOS app is plain SwiftPM with no third-party dependencies; the Windows port is Go standard library only. A normal build needs **no network**: the language models are committed.

## Requirements

| For | You need |
|---|---|
| macOS app | macOS 14 or later, a **Swift 6.0+** toolchain (Xcode 16+ or the matching Command Line Tools). `Package.swift` is `swift-tools-version:6.0`, `platforms: [.macOS(.v14)]`, built in Swift 5 language mode. Developed and tested on macOS 27 with Swift 6.4; older macOS versions are untested. |
| DMG | `hdiutil`, `codesign`, `plutil` (all part of macOS). `SetFile` (Xcode tools) is optional and only sets the volume icon. |
| Windows port | Go **1.25+**; `zip` if you want the archives from `build.sh`. Nothing is downloaded. |
| Tests | see "Tests" below: the Swift tests need the ABC, Russian and Kazakh keyboard layouts enabled. |
| Re-generating the models | the network, `curl`, and `/usr/share/dict/words` (present on macOS). Rarely needed. |

## macOS: quick start

```bash
git clone https://github.com/elapen/tilqos && cd tilqos

swift build -c release          # library, app executable, and the GenModel tool
swift test                      # 262 tests; see "Tests" for what skips

./build.sh                      # assemble and sign build/Tilqos.app
./build.sh run                  # ... and launch it through LaunchServices
./build.sh install              # ... and copy it to /Applications (quits a running Tilqos first)
./make-dmg.sh                   # build/Tilqos-<version>-macOS-<arch>.dmg, e.g. Tilqos-1.0.0-macOS-arm64.dmg
```

`build.sh` commands: `build` (default), `run`, `run-direct` (foreground, `TILQOS_DEBUG=1`, for debugging only: the permission request is credited to your shell, see [PERMISSIONS.md](PERMISSIONS.md)), `install`, `notarize`.

Environment variables (all optional):

| Variable | Meaning |
|---|---|
| `CONFIGURATION` | `release` (default) or `debug` |
| `BUILD_DIR` | where `Tilqos.app` and the DMG are written (default `./build`) |
| `SIGN_IDENTITY` | `codesign` identity; **`-` forces an ad-hoc signature**. Default: the best identity found in the keychain, falling back to ad-hoc |
| `NOTARY_PROFILE` | a `notarytool` keychain profile, for `notarize` and for `make-dmg.sh` |
| `APPLE_ID`, `TEAM_ID` | alternative to `NOTARY_PROFILE` (notarytool asks for the app-specific password each time) |
| `TILQOS_DEBUG=1` | at run time, also echo log messages to stderr |

The version of the app and of the DMG's file name comes from `CFBundleShortVersionString` in `Resources/Info.plist`. The DMG is built for the architecture of the machine that builds it (`uname -m`: `arm64` on Apple silicon, `x86_64` on Intel), and `swift build` produces a single-architecture binary. Intel builds have not been tested.

### What goes into the bundle

`Tilqos.app/Contents/MacOS/Tilqos`, `Info.plist`, `PkgInfo`, and in `Resources/`: the seven `*.bin` language files, `AppIcon.icns`, `LICENSE`, `THIRD_PARTY_NOTICES.md`, `NOTICE` (from `Resources/Models/NOTICE`) and `LICENSES/`. The DMG holds the app, an `Applications` shortcut, `LICENSE`, `THIRD_PARTY_NOTICES.md`, `LICENSES/` and a short install note.

### Signing

`Tools/signing.sh` (sourced by `build.sh` and `make-dmg.sh`) picks the identity:

1. `SIGN_IDENTITY`, if set (`-` means ad-hoc);
2. otherwise a **Developer ID Application** certificate from the keychain, then an **Apple Development** one;
3. otherwise **ad-hoc** (the script prints a note).

All builds use the hardened runtime (`--options runtime`) and the identifier `io.github.elapen.tilqos`.

**Public release builds should be made ad-hoc** (`SIGN_IDENTITY=- ./make-dmg.sh`): a signature from a certificate in your keychain embeds the certificate's common name and team identifier, which for an individual developer is their legal name (check with `codesign -dvv build/Tilqos.app`). The consequences of ad-hoc signing:

- Gatekeeper blocks the first launch on another Mac ([README](../README.md#install) explains how to open it).
- macOS ties Accessibility / Input Monitoring grants to the code identity, and an ad-hoc signature differs for every build, so users must **re-approve the permission after every update** ([PERMISSIONS.md](PERMISSIONS.md)). The same applies to you when you rebuild without a certificate.
- Ad-hoc builds cannot be notarized.

**Notarization** needs a *Developer ID Application* certificate (Xcode → Settings → Accounts → Manage Certificates → + → Developer ID Application; a paid Apple Developer Program membership is required) and notarytool credentials, stored once:

```bash
xcrun notarytool store-credentials tilqos --apple-id <YOUR_APPLE_ID> --team-id <YOUR_TEAM_ID>
NOTARY_PROFILE=tilqos ./make-dmg.sh        # sign, notarize the app, build the DMG, notarize and staple it
# or just the app:  NOTARY_PROFILE=tilqos ./build.sh notarize
```

A notarized DMG opens with a normal double click. **The notarization path is implemented but has not been exercised** by the maintainers; if you have a Developer ID certificate, trying it and reporting back is a valuable contribution.

## The language models

`Resources/Models/*.bin` are committed, so you only need this section to refresh them from newer dictionaries or to check how they were made. What they are, and under which licences, is in [THIRD_PARTY_NOTICES.md](../THIRD_PARTY_NOTICES.md).

```bash
./generate-models.sh
```

The script:

1. downloads, with `curl`, into `data/` (git-ignored): the hunspell Kazakh dictionary (`kk_KZ.dic`, `kk_KZ.aff` and its README) from `taem/hunspell-kk`, the LibreOffice Russian (`ru_RU.*`) and US English (`en_US.dic`, `en_US.aff`, README) dictionaries, and copies `/usr/share/dict/words` as `data/en_US.txt`;
2. compares four of the inputs with pinned SHA-256 sums (`kk_KZ.dic`, `kk_KZ.aff`, `ru_RU.dic`, `en_US.txt`) and **warns** (does not stop) if they differ. `ru_RU.aff`, `en_US.dic` and `en_US.aff` are not pinned, and the URLs follow upstream `master` with no commit pinned, so the output reproduces byte for byte only while upstream does not change;
3. builds `GenModel` and writes `en.bin`, `ru.bin`, `kk.bin` (trigram models), `kk-morph.bin` (suffix data) and `en-lex.bin`, `ru-lex.bin`, `kk-lex.bin` (word-form lexicons, which also compile in the hand-written lists from `wordlists/`).

Reproducibility, checked on 2026-10-16: rebuilding `en.bin`, `ru.bin`, `kk.bin`, `kk-morph.bin` and `kk-lex.bin` from the pinned inputs gives files byte-identical to the committed ones. `ru-lex.bin` and `en-lex.bin` also depend on the unpinned upstream files; the maintainers regenerated them from upstream `master` on 2026-10-15 with identical results, but that run has not been independently repeated, and a later upstream change will make the output differ.

`GenModel` can also be driven by hand:

```bash
swift build -c release --product GenModel
GEN=$(swift build -c release --product GenModel --show-bin-path)/GenModel
$GEN en  data/en_US.txt  Resources/Models/en.bin          # trigram model (en | ru | kk)
$GEN morph data/kk_KZ.dic data/kk_KZ.aff Resources/Models/kk-morph.bin
$GEN lexicon kk data/kk_KZ.dic data/kk_KZ.aff Resources/Models/kk-lex.bin \
     --extra wordlists/kk-extra.txt --core wordlists/kk-core.txt
```

Word lists in `wordlists/` are whitespace-separated words; `#` starts a comment. `*-extra.txt` holds words the dictionaries lack (brands, terms, colloquial forms); `*-core.txt` is the frequency tier that short-word decisions depend on. After editing one, re-run the script (or the single `lexicon` command above).

After changing models, rebuild the Windows port too (`windows/build.sh` copies `Resources/Models` into `windows/internal/models/embedded/`), and regenerate the golden data if behaviour changed (below).

## The icon

`./make-icon.sh` redraws `Resources/AppIcon.icns` and `Resources/icon-1024.png` from `Tools/make-icon.swift`. The icon is drawn in code rather than stored as a picture, so its geometry is reviewable in a diff and every size is rendered natively instead of resampled. The Windows `.ico` and `.syso` files are generated from `icon-1024.png` by `windows/tools/mkico`, and a test fails if they are stale.

## Tests

```bash
swift test                                     # everything
swift test --filter CorpusTests                # the accuracy report, see ACCURACY.md
TILQOS_SNAPSHOTS=$PWD/build/screenshots swift test --filter UISnapshotTests
                                               # renders every window, light and dark, to PNG
```

`swift test` runs 262 tests; 6 (the UI snapshots) are skipped unless `TILQOS_SNAPSHOTS` is set.

**Prerequisite: the ABC, Russian and Kazakh layouts must be enabled** (System Settings → Keyboard → Input Sources → Edit → +). The tests deliberately read the real system layouts rather than fixtures: if Apple changes a layout, `testRussianAndKazakhDifferOnlyOnTheNumberRow` should fail. Without the layouts, most of the suite **is skipped, not failed**, so a green run on a machine (or CI runner) without the Kazakh layout proves very little. The same applies if `Resources/Models` is missing. `EventTapTests` skips itself when Secure Event Input is active on the machine.

The tests do not touch `~/Library/Preferences`: tests that need settings storage use `ScratchDefaults`, a `UserDefaults` backed by a throw-away directory, and `ScratchDefaultsTests` fails if a test source uses a named suite, `UserDefaults()` or `.standard`.

## Windows port

The port lives in `windows/` and shares `Resources/Models` with the macOS app.

```bash
cd windows
./build.sh          # sync models, check icons, go test, go vet (amd64 + arm64), build, zip
./build.sh fast     # the same without the tests
VERSION=1.0.0 ./build.sh     # override the version; default: `var Version` in internal/winapp/app_windows.go
```

```powershell
cd windows
powershell -ExecutionPolicy Bypass -File .\build.ps1        # or .\build.ps1 -Fast
```

Output in `windows/dist/`: `Tilqos-amd64.exe`, `Tilqos-arm64.exe`, `Tilqos-<version>-Windows-x64.zip` and `Tilqos-<version>-Windows-arm64.zip` (each with `Tilqos.exe`, `LICENSE`, `THIRD_PARTY_NOTICES.md`, `NOTICE`, `LICENSES/`). Manually:

```bash
go generate ./...   # copies ../Resources/Models and the licence texts into internal/{models,licenses}/embedded/, checks the icons
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-H windowsgui -s -w" -o dist/Tilqos-amd64.exe ./cmd/tilqos
```

`go test ./...` runs 248 top-level tests on macOS, Linux and Windows. The embedded copies of the models and of the licence texts (`LICENSE`, `THIRD_PARTY_NOTICES.md`, which the executable carries so that a lone `.exe` still has them) are git-ignored; `TestEmbeddedModelsMatchSource` and `TestEmbeddedLicensesMatchSource` fail if they are missing or stale, and `go generate ./...` (or `build.sh` / `build.ps1`) refreshes them. If a branch is built before the model files exist in it, `TILQOS_MODELS_REV=<git revision> ./build.sh` (or `go run ./tools/syncmodels -rev <revision>`) takes the missing files from git; without that variable a missing file is an error and is never silently substituted.

The executables are **not signed**. Cross-building from macOS/Linux is how the port is normally built; it has not been run on real Windows by the author ([windows/README.md](../windows/README.md)).

### Regenerating the golden data (when the Swift algorithm changes)

`windows/testdata/golden/*.json` (about 11 MB, 20 files) is the Swift implementation's behaviour, dumped to JSON, against which the Go port is compared bit for bit. Never edit it by hand.

1. Port the change to the matching Go file (the table in [ARCHITECTURE.md](ARCHITECTURE.md#the-windows-port)).
2. On a **Mac with the ABC, Russian and Kazakh layouts enabled**, with `/usr/share/dict/words` present (the dump samples it) and the Swift toolchain:

   ```bash
   windows/parity/regen-golden.sh
   ```

   The script copies `windows/parity/GoldenDumpTests.swift` into `Tests/TilqosKitTests/` for one `swift test --filter GoldenDumpTests` run, removes it afterwards (even on failure), writes the JSON to a temporary directory, and replaces `testdata/golden/` only if the whole dump succeeded. `SWIFT_ROOT=<path>` dumps from another checkout of the Swift package. If you add new functions, extend the dump with their output.
3. `cd windows && go test ./...`: any divergence from Swift shows up entry by entry.

Without a Mac that has the three layouts, you can still change Go code that does not alter the algorithm; ask in an issue before changing the algorithm itself.
