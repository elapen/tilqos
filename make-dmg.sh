#!/usr/bin/env bash
# Builds a distributable disk image.
#
#   ./make-dmg.sh
#
# The result is build/Tilqos-<version>-macOS-<arch>.dmg (for example
# Tilqos-1.0.0-macOS-arm64.dmg) with the app, an Applications shortcut and install
# instructions in English, Russian and Kazakh (Install.txt). Signed with whatever identity
# build.sh picks (ad-hoc when the keychain has none).
#
# Environment (all optional): BUILD_DIR (default ./build), SIGN_IDENTITY, NOTARY_PROFILE or
# APPLE_ID + TEAM_ID. See Tools/signing.sh.
#
# Gatekeeper: unless the app is signed with a "Developer ID Application" certificate AND
# notarized, another Mac will refuse to open it on a double click. The instructions inside
# the image explain the workaround, and setting NOTARY_PROFILE (or APPLE_ID + TEAM_ID) with
# a Developer ID certificate in the keychain removes the need for it.
set -euo pipefail
cd "$(dirname "$0")"
# shellcheck source=Tools/signing.sh
. Tools/signing.sh

APP_NAME="Tilqos"
VOLUME_NAME="Tilqos"
BUILD_DIR="${BUILD_DIR:-build}"
STAGE="$BUILD_DIR/dmg-stage"

log() { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
die() { printf '\033[1;31merror:\033[0m %s\n' "$*" >&2; exit 1; }

resolve_signing_identity
resolve_notary_args

# Notarizing needs a Developer ID; an Apple Development certificate (or an ad-hoc
# signature) is rejected.
NOTARIZE=0
if [ ${#NOTARY_ARGS[@]} -gt 0 ]; then
  is_developer_id \
    || die "notarization needs a 'Developer ID Application' certificate; found '$SIGN_IDENTITY'. Unset NOTARY_PROFILE / APPLE_ID to build without notarizing."
  NOTARIZE=1
fi

VERSION="$(plutil -extract CFBundleShortVersionString raw Resources/Info.plist)"
ARCH="$(uname -m)"   # arm64 on Apple silicon, x86_64 on Intel: the app is built for this host
DMG="$BUILD_DIR/$APP_NAME-$VERSION-macOS-$ARCH.dmg"
TEMP_DMG="$BUILD_DIR/$APP_NAME-rw.dmg"

mkdir -p "$BUILD_DIR"
log "Building and signing the app"
BUILD_DIR="$BUILD_DIR" SIGN_IDENTITY="$SIGN_IDENTITY" ./build.sh >/dev/null   # notes and errors go to stderr
[ -d "$BUILD_DIR/$APP_NAME.app" ] || die "app bundle missing"

# The app is notarized and stapled before packaging, so its ticket travels with it once the
# user drags it out of the image. The image is notarized separately below, which is what
# Gatekeeper checks on the first open.
if [ "$NOTARIZE" = 1 ]; then
  log "Notarizing the app"
  ditto -c -k --keepParent "$BUILD_DIR/$APP_NAME.app" "$BUILD_DIR/$APP_NAME-app.zip"
  xcrun notarytool submit "$BUILD_DIR/$APP_NAME-app.zip" "${NOTARY_ARGS[@]}" --wait
  xcrun stapler staple "$BUILD_DIR/$APP_NAME.app"
  rm -f "$BUILD_DIR/$APP_NAME-app.zip"
fi

log "Staging disk image contents"
rm -rf "$STAGE" "$DMG" "$TEMP_DMG"
mkdir -p "$STAGE"
cp -R "$BUILD_DIR/$APP_NAME.app" "$STAGE/"
ln -s /Applications "$STAGE/Applications"
cp LICENSE THIRD_PARTY_NOTICES.md "$STAGE/"
if [ -d LICENSES ]; then cp -R LICENSES "$STAGE/LICENSES"; fi
cp Resources/AppIcon.icns "$STAGE/.VolumeIcon.icns"

# Install instructions in the three languages of the app (English, Russian, Kazakh), in one
# file so the volume stays tidy. The texts live in Resources/Installer; step 2 (first launch)
# is different for a notarized image, hence the [[NOTARIZED]] / [[NOT_NOTARIZED]] markers.
if [ "$NOTARIZE" = 1 ]; then KEEP=NOTARIZED; else KEEP=NOT_NOTARIZED; fi
for lang in en ru kk; do
  [ -f "Resources/Installer/Install.$lang.txt" ] || die "Resources/Installer/Install.$lang.txt is missing"
done
{
  for lang in en ru kk; do
    awk -v keep="$KEEP" '
      /^\[\[NOTARIZED\]\]$/     { skip = (keep != "NOTARIZED"); next }
      /^\[\[NOT_NOTARIZED\]\]$/ { skip = (keep != "NOT_NOTARIZED"); next }
      /^\[\[\/(NOT_)?NOTARIZED\]\]$/ { skip = 0; next }
      !skip { print }
    ' "Resources/Installer/Install.$lang.txt"
    if [ "$lang" != kk ]; then
      printf '\n\n================================================================\n\n\n'
    fi
  done
} > "$STAGE/Install.txt"

# A read-write image first, so the volume can be given a custom icon; the compressed
# read-only image is produced from it afterwards.
log "Creating image"
SIZE_KB=$(( $(du -sk "$STAGE" | cut -f1) + 20000 ))
hdiutil create -volname "$VOLUME_NAME" -srcfolder "$STAGE" -ov \
  -format UDRW -fs HFS+ -size "${SIZE_KB}k" "$TEMP_DMG" >/dev/null

MOUNT_POINT="$(mktemp -d)"
hdiutil attach "$TEMP_DMG" -mountpoint "$MOUNT_POINT" -nobrowse -quiet

# The custom-icon flag needs SetFile, which ships with Xcode. Purely cosmetic, so a missing
# tool must not fail the build.
if command -v SetFile >/dev/null 2>&1; then
  SetFile -a C "$MOUNT_POINT" && log "Volume icon set"
else
  log "SetFile unavailable; skipping volume icon"
fi

hdiutil detach "$MOUNT_POINT" -quiet
rmdir "$MOUNT_POINT" 2>/dev/null || true

log "Compressing"
hdiutil convert "$TEMP_DMG" -format UDZO -imagekey zlib-level=9 -o "$DMG" >/dev/null
rm -f "$TEMP_DMG"
rm -rf "$STAGE"

log "Signing the image"
if [ "$NOTARIZE" = 1 ]; then
  codesign --force --timestamp --sign "$SIGN_IDENTITY" "$DMG"
else
  codesign --force --sign "$SIGN_IDENTITY" "$DMG"
fi
codesign --verify --verbose=1 "$DMG"

if [ "$NOTARIZE" = 1 ]; then
  log "Notarizing the image"
  xcrun notarytool submit "$DMG" "${NOTARY_ARGS[@]}" --wait
  xcrun stapler staple "$DMG"
  log "Notarized and stapled — opens on any Mac with a normal double click"
fi

log "Built $DMG ($(du -h "$DMG" | cut -f1))"
if [ "$NOTARIZE" != 1 ]; then
  printf '\033[1;33mnote:\033[0m not notarized — on another Mac use right click -> Open on the\n'
  printf '      first launch (the instructions inside the image explain it).\n'
fi
