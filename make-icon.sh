#!/usr/bin/env bash
# Regenerates Resources/AppIcon.icns and Resources/icon-1024.png from Tools/make-icon.swift.
#
# The icon is drawn in code rather than kept as a binary asset, so its geometry stays
# reviewable and every size is rendered from the same source instead of being resampled.
set -euo pipefail
cd "$(dirname "$0")"

ICONSET="Resources/AppIcon.iconset"

printf '\033[1;34m==>\033[0m Rendering sizes\n'
rm -rf "$ICONSET"
swift Tools/make-icon.swift "$ICONSET"

printf '\033[1;34m==>\033[0m Packing icns\n'
iconutil -c icns "$ICONSET" -o Resources/AppIcon.icns

printf '\033[1;34m==>\033[0m Wrote Resources/AppIcon.icns (%s)\n' \
  "$(du -h Resources/AppIcon.icns | cut -f1)"
