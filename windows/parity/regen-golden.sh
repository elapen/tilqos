#!/usr/bin/env bash
# Regenerates windows/testdata/golden/*.json from the Swift implementation.
#
# Needs macOS with the ABC, Russian and Kazakh keyboard layouts enabled (the dump reads the
# real layouts, exactly like the Swift test suite does) and the Swift toolchain.
#
# The dump lives here, outside every SwiftPM target, so the Swift tree is never modified:
# it is copied into Tests/TilqosKitTests/ for one `swift test` run and always removed
# afterwards, even if the run fails. The JSON is written to a temporary directory and only
# replaces testdata/golden once the whole dump has succeeded, so a failed run leaves the
# old data intact and a successful one leaves no stale files behind.
#
# SWIFT_ROOT points at another checkout of the Swift package (for example a `git archive`
# of the commit being ported); by default it is the repository this windows/ lives in.
#
# Run it after porting a change of the Swift algorithm, then `go test ./...` in windows/.
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
golden="$(cd "$here/../testdata/golden" && pwd)"
root="${SWIFT_ROOT:-$(cd "$here/../.." && pwd)}"
root="$(cd "$root" && pwd)"
target="$root/Tests/TilqosKitTests/GoldenDumpTests.swift"

if [ -e "$target" ]; then
  echo "error: $target already exists; refusing to overwrite it" >&2
  exit 1
fi

out="$(mktemp -d "${TMPDIR:-/tmp}/tilqos-golden.XXXXXX")"
cleanup() {
  rm -f "$target"
  rm -rf "$out"
}
trap cleanup EXIT

cp "$here/GoldenDumpTests.swift" "$target"
(cd "$root" && GOLDEN_OUT="$out" swift test --filter GoldenDumpTests)

if ! ls "$out"/*.json >/dev/null 2>&1; then
  echo "error: the dump wrote nothing" >&2
  exit 1
fi
rm -f "$golden"/*.json
cp "$out"/*.json "$golden"/
echo "golden data written to $golden"
