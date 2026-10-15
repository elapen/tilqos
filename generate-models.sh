#!/usr/bin/env bash
# Downloads the source word lists and regenerates Resources/Models/*.bin.
#
# The generated models are committed, so a normal build needs neither the network nor
# this script. Run it only to refresh the models from newer dictionaries.
set -euo pipefail
cd "$(dirname "$0")"

DATA_DIR="data"
OUT_DIR="Resources/Models"
mkdir -p "$DATA_DIR" "$OUT_DIR"

log() { printf '\033[1;34m==>\033[0m %s\n' "$*"; }

fetch() {
  local url="$1" out="$DATA_DIR/$2"
  if [ -s "$out" ]; then
    log "have $2"
    return
  fi
  log "fetching $2"
  curl -fsSL --retry 3 -o "$out" "$url"
}

# Kazakh: hunspell-kk, tri-licensed GPL-2.0+ / LGPL-2.1+ / MPL-1.1+ (we rely on MPL/LGPL).
# The affix file carries the real suffix rules and is what makes morphological validation
# possible — Kazakh is agglutinative, so a bare word list covers only a fraction of forms.
fetch "https://raw.githubusercontent.com/taem/hunspell-kk/master/kk_KZ.dic" "kk_KZ.dic"
fetch "https://raw.githubusercontent.com/taem/hunspell-kk/master/kk_KZ.aff" "kk_KZ.aff"
fetch "https://raw.githubusercontent.com/taem/hunspell-kk/master/README_kk_KZ.txt" "kk_README.txt"

# Russian: LibreOffice ru_RU by Alexander I. Lebedev, BSD-3-Clause. The affix file is what turns
# 146,269 headwords into the 1.4 million forms people actually type (`сделал`, `людей`, `еще`).
fetch "https://raw.githubusercontent.com/LibreOffice/dictionaries/master/ru_RU/ru_RU.dic" "ru_RU.dic"
fetch "https://raw.githubusercontent.com/LibreOffice/dictionaries/master/ru_RU/ru_RU.aff" "ru_RU.aff"
fetch "https://raw.githubusercontent.com/LibreOffice/dictionaries/master/ru_RU/README_ru_RU.txt" "ru_README.txt"

# English word forms: LibreOffice en_US (SCOWL, Kevin Atkinson et al.), permissive licence. Used
# for the lexicon only: a modern list with inflections and without Webster's 1934 rarities
# such as `tot` crowding out real mistakes.
fetch "https://raw.githubusercontent.com/LibreOffice/dictionaries/master/en/en_US.dic" "en_US.dic"
fetch "https://raw.githubusercontent.com/LibreOffice/dictionaries/master/en/en_US.aff" "en_US.aff"
fetch "https://raw.githubusercontent.com/LibreOffice/dictionaries/master/en/README_en_US.txt" "en_README.txt"

# English trigrams: ships with macOS. Webster's 2nd edition, public domain.
if [ ! -s "$DATA_DIR/en_US.txt" ]; then
  log "copying /usr/share/dict/words"
  cp /usr/share/dict/words "$DATA_DIR/en_US.txt"
fi

# SHA-256 of the input files the shipped Resources/Models/*.bin were generated from. These are
# checked, not enforced: a mismatch means upstream has changed (the URLs above follow
# `master`, no commit is pinned yet) or the file was edited, and the regenerated models will
# probably differ from the committed ones. ru_RU.aff, en_US.dic and en_US.aff have no pinned
# checksum yet.
sha256_of() {
  if command -v shasum >/dev/null 2>&1; then shasum -a 256 "$1" | cut -d' ' -f1
  else sha256sum "$1" | cut -d' ' -f1; fi
}
check_sha256() {
  local file="$DATA_DIR/$1" expected="$2" actual
  actual="$(sha256_of "$file")"
  if [ "$actual" != "$expected" ]; then
    printf '\033[1;33mwarning:\033[0m %s differs from the pinned copy (expected %s, got %s);\n' \
      "$1" "$expected" "$actual"
    printf '         the regenerated models may not match Resources/Models byte for byte.\n'
  fi
}
check_sha256 kk_KZ.dic 80090f69c0d098425020ab378084d05ec7a4a90155750faf73742cdde7088012
check_sha256 kk_KZ.aff 254293c1c6ae893b87ec5c1fea3b72f696fe7821a3d87740ebad86b780d6e33a
check_sha256 ru_RU.dic f6047416a0204adbecf3a451b874ec8a97ee37e2cbc714466ef04d8dbcc0d6fc
check_sha256 en_US.txt be41ad97963bf8dabedd5871d5d691596175269d540956b0f9965a885c2bbab9

log "building GenModel"
swift build -c release --product GenModel
GEN="$(swift build -c release --product GenModel --show-bin-path)/GenModel"

# Character trigram models. Deliberately still trained on headwords: retraining them on the
# expanded forms was measured and rejected (see README, "Как принимается решение").
"$GEN" en "$DATA_DIR/en_US.txt" "$OUT_DIR/en.bin"
"$GEN" ru "$DATA_DIR/ru_RU.dic" "$OUT_DIR/ru.bin"
"$GEN" kk "$DATA_DIR/kk_KZ.dic" "$OUT_DIR/kk.bin"
"$GEN" morph "$DATA_DIR/kk_KZ.dic" "$DATA_DIR/kk_KZ.aff" "$OUT_DIR/kk-morph.bin"

# Word-form lexicons: hashed stems + affix rules, plus the hand-written lists in wordlists/.
for lang in en ru kk; do
  case "$lang" in
    en) dic="$DATA_DIR/en_US.dic"; aff="$DATA_DIR/en_US.aff" ;;
    ru) dic="$DATA_DIR/ru_RU.dic"; aff="$DATA_DIR/ru_RU.aff" ;;
    kk) dic="$DATA_DIR/kk_KZ.dic"; aff="$DATA_DIR/kk_KZ.aff" ;;
  esac
  "$GEN" lexicon "$lang" "$dic" "$aff" "$OUT_DIR/$lang-lex.bin" \
    --extra "wordlists/$lang-extra.txt" --core "wordlists/$lang-core.txt"
done

log "models written to $OUT_DIR ($(du -ch "$OUT_DIR"/*.bin | tail -1 | cut -f1) total)"
