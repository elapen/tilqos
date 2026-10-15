#!/usr/bin/env bash
# Re-downloads the licence texts kept in LICENSES/ (they are already committed; this script
# documents where they came from and lets you verify them):
#
#   README_en_US.txt   upstream README of the LibreOffice en_US dictionary (SCOWL + Ispell)
#   MPL-1.1.txt        Mozilla Public License 1.1
#   LGPL-2.1.txt       GNU Lesser General Public License 2.1
#   GPL-2.0.txt        GNU General Public License 2.0
#
# README_kk_KZ.txt and README_ru_RU.txt are the verbatim upstream READMEs of the Kazakh and
# Russian dictionaries. Needs the network; not part of any build. If an URL has moved the
# script stops and says which one.
set -euo pipefail
cd "$(dirname "$0")/.."
mkdir -p LICENSES

fetch() {
  local url="$1" out="LICENSES/$2"
  printf '==> %s\n' "$2"
  curl -fsSL --retry 3 -o "$out" "$url" || { echo "error: cannot fetch $url" >&2; exit 1; }
  [ -s "$out" ] || { echo "error: $out is empty" >&2; exit 1; }
  shasum -a 256 "$out"
}

fetch "https://raw.githubusercontent.com/LibreOffice/dictionaries/master/en/README_en_US.txt" README_en_US.txt
fetch "https://raw.githubusercontent.com/spdx/license-list-data/main/text/MPL-1.1.txt" MPL-1.1.txt
fetch "https://www.gnu.org/licenses/old-licenses/lgpl-2.1.txt" LGPL-2.1.txt
fetch "https://www.gnu.org/licenses/old-licenses/gpl-2.0.txt" GPL-2.0.txt
