#!/usr/bin/env bash
# Builds Tilqos for Windows from macOS or Linux (Go cross-compilation, no cgo).
#
#   ./build.sh           sync models, check icons, test, vet and build amd64 + arm64
#   ./build.sh fast      the same without running the tests
#
# Output in dist/:
#   Tilqos-amd64.exe, Tilqos-arm64.exe             the bare executables
#   Tilqos-<version>-Windows-x64.zip               Tilqos.exe + LICENSE + THIRD_PARTY_NOTICES.md
#                                                  + NOTICE + LICENSES/ (x64)
#   Tilqos-<version>-Windows-arm64.zip             the same for ARM64
#
# <version> defaults to the Version constant in internal/winapp/app_windows.go; override it
# with VERSION=... for a one-off build. Needs only Go 1.25+ (and `zip` for the archives);
# nothing is downloaded.
set -euo pipefail
cd "$(dirname "$0")"
here="$PWD"

MODE="${1:-full}"
case "$MODE" in
  full|fast) ;;
  *) echo "usage: $0 [fast]" >&2; exit 2 ;;
esac

log() { printf '\033[1;34m==>\033[0m %s\n' "$*"; }

VERSION="${VERSION:-$(sed -n 's/^var Version = "\(.*\)"$/\1/p' internal/winapp/app_windows.go)}"
[ -n "$VERSION" ] || { echo "error: cannot determine the version" >&2; exit 1; }
LDFLAGS="-H windowsgui -s -w -X tilqos/internal/winapp.Version=${VERSION}"

# The models are embedded from copies of ../Resources/Models, the single source of truth.
log "Syncing models from ../Resources/Models"
go run ./tools/syncmodels
go run ./tools/syncmodels -check

# The license texts are embedded too (Licenses in the tray menu; a lone .exe keeps its notices).
log "Syncing LICENSE and THIRD_PARTY_NOTICES.md from .."
go run ./tools/synclicenses
go run ./tools/synclicenses -check

# Icons are committed; regenerate only if Resources/icon-1024.png changed.
if ! go run ./tools/mkico -check >/dev/null 2>&1; then
  log "Regenerating icons from ../Resources/icon-1024.png"
  go run ./tools/mkico
fi

if [ "$MODE" = full ]; then
  log "Testing (includes parity with the Swift implementation)"
  go test ./...
fi

mkdir -p dist
for arch in amd64 arm64; do
  log "Vetting windows/$arch"
  GOOS=windows GOARCH="$arch" go vet ./...
  log "Building windows/$arch ($VERSION)"
  GOOS=windows GOARCH="$arch" CGO_ENABLED=0 \
    go build -trimpath -ldflags "$LDFLAGS" -o "dist/Tilqos-$arch.exe" ./cmd/tilqos
done

if command -v zip >/dev/null 2>&1; then
  for arch in amd64 arm64; do
    case "$arch" in amd64) label=x64 ;; *) label="$arch" ;; esac
    archive="Tilqos-$VERSION-Windows-$label.zip"
    stage="$(mktemp -d)"
    cp "dist/Tilqos-$arch.exe" "$stage/Tilqos.exe"
    for doc in LICENSE THIRD_PARTY_NOTICES.md; do
      cp "../$doc" "$stage/"
    done
    cp ../Resources/Models/NOTICE "$stage/NOTICE"
    if [ -d ../LICENSES ]; then cp -R ../LICENSES "$stage/LICENSES"; fi
    log "Packaging dist/$archive"
    rm -f "dist/$archive"
    (cd "$stage" && zip -q -X -r "$here/dist/$archive" .)
    rm -rf "$stage"
  done
else
  log "zip not found; skipping the archives (the bare executables are in dist/)"
fi

log "Done"
ls -l dist/Tilqos-*
