#!/usr/bin/env bash
# Assembles Tilqos.app from the SPM build, signs it, and optionally installs or notarizes it.
#
#   ./build.sh              build + sign
#   ./build.sh run          build + sign + launch via LaunchServices
#   ./build.sh run-direct   launch in the foreground for debugging (see the TCC note below)
#   ./build.sh install      build + sign + copy into /Applications
#   ./build.sh notarize     build + sign + submit to Apple (needs a Developer ID certificate)
#
# Environment (all optional):
#   CONFIGURATION   release (default) or debug
#   BUILD_DIR       where Tilqos.app is assembled (default: ./build)
#   SIGN_IDENTITY   codesign identity; "-" forces an ad-hoc signature. Auto-detected from
#                   the keychain by default, and ad-hoc when the keychain has none.
#   NOTARY_PROFILE  notarytool keychain profile, or APPLE_ID + TEAM_ID, for `notarize`.
#                   See Tools/signing.sh.
set -euo pipefail
cd "$(dirname "$0")"
# shellcheck source=Tools/signing.sh
. Tools/signing.sh

APP_NAME="Tilqos"
BUNDLE_ID="io.github.elapen.tilqos"
CONFIGURATION="${CONFIGURATION:-release}"
BUILD_DIR="${BUILD_DIR:-build}"
APP="$BUILD_DIR/$APP_NAME.app"
COMMAND="${1:-build}"

log() { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
die() { printf '\033[1;31merror:\033[0m %s\n' "$*" >&2; exit 1; }

case "$COMMAND" in
  build|run|run-direct|install|notarize) ;;
  *) die "unknown command '$COMMAND' (build | run | run-direct | install | notarize)" ;;
esac

# macOS ties granted Input Monitoring / Accessibility permissions to the code identity, so
# signing with a stable certificate is what keeps them alive across rebuilds. An
# "Apple Development" certificate is enough for that; notarizing needs a
# "Developer ID Application" one. Without any certificate the app is signed ad-hoc, which
# works but needs the permissions re-approved after every rebuild.
resolve_signing_identity
resolve_notary_args

if [ "$COMMAND" = notarize ]; then
  [ ${#NOTARY_ARGS[@]} -gt 0 ] \
    || die "set NOTARY_PROFILE (a notarytool keychain profile) or APPLE_ID and TEAM_ID"
  is_developer_id \
    || die "notarization requires a 'Developer ID Application' certificate; SIGN_IDENTITY is '$SIGN_IDENTITY'"
fi

build_binary() {
  log "Building ($CONFIGURATION)"
  swift build -c "$CONFIGURATION" --product "$APP_NAME"
  BIN_PATH="$(swift build -c "$CONFIGURATION" --product "$APP_NAME" --show-bin-path)/$APP_NAME"
  [ -x "$BIN_PATH" ] || die "binary not found at $BIN_PATH"
}

assemble_bundle() {
  log "Assembling $APP"
  rm -rf "$APP"
  mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
  cp "$BIN_PATH" "$APP/Contents/MacOS/$APP_NAME"
  cp Resources/Info.plist "$APP/Contents/Info.plist"
  if compgen -G "Resources/Models/*.bin" >/dev/null; then
    cp Resources/Models/*.bin "$APP/Contents/Resources/"
  fi
  if [ -f Resources/AppIcon.icns ]; then
    cp Resources/AppIcon.icns "$APP/Contents/Resources/"
  fi
  printf 'APPL????' > "$APP/Contents/PkgInfo"

  # The interface texts (English, Russian, Kazakh) and the localized Info.plist values. The app
  # reads Localizable.strings itself (see L10n), so a broken file would silently show keys.
  compgen -G "Resources/*.lproj" >/dev/null || die "Resources/*.lproj (the interface texts) is missing"
  for lproj in Resources/*.lproj; do
    plutil -lint "$lproj"/*.strings >/dev/null || die "invalid strings file in $lproj"
    cp -R "$lproj" "$APP/Contents/Resources/"
  done

  # The licences travel with the app: the MIT text, the third-party notices the language
  # data legally requires, and the notice that sits next to the models.
  [ -f LICENSE ] && [ -f THIRD_PARTY_NOTICES.md ] || die "LICENSE or THIRD_PARTY_NOTICES.md is missing"
  cp LICENSE THIRD_PARTY_NOTICES.md "$APP/Contents/Resources/"
  cp Resources/Models/NOTICE "$APP/Contents/Resources/NOTICE"
  if [ -d LICENSES ]; then cp -R LICENSES "$APP/Contents/Resources/LICENSES"; fi
}

sign_bundle() {
  if [ "$ADHOC" = 1 ]; then
    log "Signing ad-hoc"
    print_adhoc_note
  else
    log "Signing as: $SIGN_IDENTITY"
  fi
  local extra=()
  # A secure timestamp is mandatory for notarization and needs network access, so
  # it is only requested when we are actually going to notarize.
  if [ "$ADHOC" = 0 ] && [ ${#NOTARY_ARGS[@]} -gt 0 ]; then extra+=(--timestamp); fi
  codesign --force --options runtime ${extra[@]+"${extra[@]}"} \
    --identifier "$BUNDLE_ID" --sign "$SIGN_IDENTITY" "$APP"
  codesign --verify --strict --verbose=2 "$APP"
}

build_binary
assemble_bundle
sign_bundle
log "Built $APP"

case "$COMMAND" in
  run)
    # Through LaunchServices, and this matters more than it looks. TCC attributes a
    # permission request to the *responsible process* — the top-level ancestor — so an app
    # started from a shell gets its request credited to Terminal instead of to itself, and
    # never appears in the Input Monitoring or Accessibility lists at all. `open` makes
    # launchd the parent, so the app is responsible for itself.
    log "Launching"
    pkill -x "$APP_NAME" 2>/dev/null || true
    open -a "$APP"
    log "Running (log: log stream --predicate 'subsystem == \"$BUNDLE_ID\"')"
    ;;
  run-direct)
    # Debug only: stderr goes to the terminal, but see the TCC caveat above — permission
    # prompts from this launch are credited to the shell, not to the app.
    log "Launching directly (TCC will credit the shell, not the app)"
    pkill -x "$APP_NAME" 2>/dev/null || true
    TILQOS_DEBUG=1 "$APP/Contents/MacOS/$APP_NAME"
    ;;
  install)
    log "Installing to /Applications"
    pkill -x "$APP_NAME" 2>/dev/null || true
    rm -rf "/Applications/$APP_NAME.app"
    cp -R "$APP" "/Applications/$APP_NAME.app"
    log "Installed /Applications/$APP_NAME.app"
    ;;
  notarize)
    ZIP="$BUILD_DIR/$APP_NAME.zip"
    log "Submitting to Apple"
    ditto -c -k --keepParent "$APP" "$ZIP"
    xcrun notarytool submit "$ZIP" "${NOTARY_ARGS[@]}" --wait
    xcrun stapler staple "$APP"
    log "Notarized and stapled"
    ;;
esac
