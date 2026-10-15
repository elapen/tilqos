# Accuracy

Short version: on the project's own regression corpus Tilqos rewrites **0 of 1,858** correctly typed words and fixes **96.4 %** of words typed on the wrong layout by itself. Those numbers are real and you can reproduce them with the commands below, but they are **in-sample**: the corpus was written by the author and the detector's constants were tuned against it. Read "What these numbers are not" before you generalise from them.

## What is measured

`Tests/TilqosKitTests/Corpus.swift` (hand-written) and `CorpusTests.swift` (the simulation). The simulated user presses the keys for the text they *intend*, on whatever layout happens to be active, and, like the real app, the active layout switches when a fix is applied, so the rest of a sentence arrives correctly. Suggestions are ignored by the simulated user and counted separately. Two directions are measured on purpose, because they fail differently:

- **Must stay.** Correctly typed text. Any change is a false positive, the failure people uninstall tools like this over (`еще` becoming `tot`).
- **Must convert.** Text typed on the wrong layout. A miss is a false negative, a shrug: you press `⌥⇧F`.

| Group | Size |
|---|---|
| Frequent words typed correctly (Russian with common inflections, English, Kazakh) | 448 + 474 + 202 |
| Slang, abbreviations, names no dictionary has (Russian, English) | 34 + 33 |
| Synthetic typos of frequent words (a typo is not a layout mistake) | 300 + 300 |
| "Collisions": a word that is also a word on the other layout, typed correctly (`рук`, `her`, `the`) | 12 + 18 |
| Words with punctuation glued on, typed correctly | 25 + 12 |
| **Correctly typed words in total** | **1,858** |
| Correctly typed sentences (Russian, English, Kazakh) | 72 + 40 + 20 = 132 |
| Words typed on the wrong layout (Russian on English, English on Russian, Kazakh on Russian, Kazakh on English) | 88 + 83 + 53 + 25 = 249 |
| Sentences typed on the wrong layout (the same four directions) | 72 + 40 + 20 + 20 = 152 |
| Mixed-script and context cases (brands inside Russian sentences, garble inside Russian sentences) | 34 |

## Results

Default settings (Balanced preset: automatic at 0.80, offer at 0.45), as printed by `CorpusTests`:

| | Result |
|---|---|
| Correctly typed words changed automatically | **0 of 1,858** (0 offers) |
| Correctly typed sentences changed | **0 of 132** (0 offers) |
| Wrong-layout words fixed automatically | **240 of 249 (96.4 %)**; another 6 (2.4 %) offered as hints; 3 missed; 0 converted to the wrong word |
| Wrong-layout sentences converted completely and correctly | **152 of 152** |
| Mixed-script and context cases | **34 of 34** |

Wrong-layout words by direction:

| Direction | Fixed automatically | Offered | Missed |
|---|---|---|---|
| Russian typed on English | 84 of 88 (95.5 %) | 3 | 1 (`где`) |
| English typed on Russian | 79 of 83 (95.2 %) | 2 | 2 (`the`, `but`) |
| Kazakh typed on Russian (the Kazakh letters come out as digits and `=` / `-`) | 53 of 53 (100 %) | 0 | 0 |
| Kazakh typed on English | 24 of 25 (96.0 %) | 1 | 0 |

The remaining misses are *doubles*, where both readings are words. `где` typed on the English layout gives `ult`, which English word lists accept; `the` typed on the Russian layout gives `еру`, a Russian word form. Without context, guessing would be wrong as often as right, so Tilqos does not guess; `⌥⇧F` fixes them on request, and with text in the same language in the field they are offered as a hint (see below). The hardest group is Russian words whose English-layout rendering is itself an English word (`рук` gives `her`, `руку` gives `here`, `еру` gives `the`): 5 of 12 are fixed in the plain run and 6 of 12 with Russian context, and none is ever converted wrongly.

Words that *no* dictionary has (the "rare" group, e.g. `ништяк`, `зашибись`) can only be caught by the character model, which is only allowed to offer: 6 of 9 fixed, 2 offered, 1 missed.

### Effect of context from the text field

When the buffer is short, the app reads the scripts of the last three words in the focused field ([HOW_IT_WORKS.md](HOW_IT_WORKS.md#context)). Same corpus, with Russian text before the word:

| | Empty field | Russian text before |
|---|---|---|
| Short Russian words typed on English, fixed | 25 of 29 (86.2 %) | 28 of 29 (96.6 %) |
| Russian typed on English, fixed | 84 of 88 (95.5 %) | 87 of 88 (98.9 %) |
| English typed on Russian, fixed (English text before) | 79 of 83 (95.2 %) | 81 of 83 (97.6 %) |
| English frequent words changed after Russian text | 0 of 474 | 0 of 474 (and 0 offers) |

### Presets

Both thresholds matter, and the presets trade help against surprise (`CorpusTests.testPresets`, 224 wrong-layout words: Russian on English, English on Russian, Kazakh on Russian):

| Preset (automatic / offer) | Correct words rewritten | Mistakes fixed automatically | Offered instead |
|---|---|---|---|
| Cautious (0.93 / 0.55) | 0 | 171 of 224 (76.3 %) | 50 |
| Balanced (0.80 / 0.45), default | 0 | 216 of 224 (96.4 %) | 5 |
| Bold (0.55 / 0.30) | 0 | 221 of 224 (98.7 %) | 0 |

"Bold" does not rewrite any correct word in this corpus either, but its zero is not asserted by a test, and bolder means more of the *doubtful* cases are applied without asking. Choose by how much you dislike surprises.

### Transliteration

| Check | Result |
|---|---|
| Practical Latin to Cyrillic Kazakh, words (`TransliteratorTests.testPracticalDecodingAccuracy`) | 44 of 44 (the test only requires 75 %) |
| Russian words written in Latin inside Kazakh text kept as Russian (`MixedTranslitTests`) | 11 of 11 |
| Brand list, vowel harmony, mixed note | covered by `MixedTranslitTests` (13 tests) |

44 and 11 are tiny samples written by the author while tuning. They are regression tests, not accuracy estimates.

## What these numbers are not

- **They are not a held-out benchmark.** The corpus is hand-written by the author; the decision constants (`certain`, `confident`, `offerOnly`, `contextWeight`, ...) were set against it, as the comments in `Detector.swift` say. "0 correct words rewritten" is also a hard gate in the test suite (`XCTAssertEqual(tally.changed, 0)`), so it cannot get worse without failing the build; that makes it a guarantee about the corpus, not a measurement of your text.
- **The corpus is everyday vocabulary.** Jargon, code, names, slang, and heavily mixed chat will do worse than this. The guards (URLs, paths, identifiers, e-mail, acronyms, words after `$ # @ \`) are there because those are the cases that hurt, but they are pattern rules, not proof.
- **The Kazakh part is small.** 202 correctly typed Kazakh words, 20 Kazakh sentences typed correctly, 53 + 25 Kazakh words and 20 + 20 sentences typed on the wrong layout. It was written by one person. The practical-Latin sample is 44 words. If you write Kazakh professionally, expect to find misses; please report them (the *language* issue form is for exactly this).
- **The typos are synthetic.** `Corpus.typos(of:limit:)` generates misspellings of the frequent words; real typing has more variety.
- **It measures the detector, not the app in the wild.** The simulation drives the real `TypingEngine` and detector, with real macOS layouts, but not the event tap, Accessibility, or a real text field. Replacement logic is tested against a fake field that misbehaves in the ways real apps do (`TextReplacerTests`), not against the real applications.
- **No other product was measured.** There is no comparison here, and none is claimed.

## Reproducing

You need a Mac with the **ABC, Russian and Kazakh** keyboard layouts enabled (System Settings → Keyboard → Input Sources). Without them most Swift tests are **skipped silently** (they report as passed with a skip), so a green run on a machine without the Kazakh layout proves little.

```bash
git clone https://github.com/elapen/tilqos && cd tilqos

swift test --filter CorpusTests          # prints the report quoted above
swift test --filter TransliteratorTests  # the 44-word sample
swift test                               # everything: 262 tests, 6 skipped without TILQOS_SNAPSHOTS=<dir>

cd windows
go test ./internal/core -run Corpus -v   # same corpus through the Go port, and equality with the Swift run
go test ./...                            # 248 top-level tests
go test ./internal/winapp -run Corpus -v # the corpus on hand-written approximations of Windows layouts
```

The Swift suite does not write to your user defaults or `~/Library/Preferences`; tests that need settings storage use a throw-away directory (`ScratchDefaults`), and one test fails if a test source uses a named `UserDefaults` suite or `.standard`.

### The Go port gives the same answers

The Go tests do not just re-run a corpus; they compare against data dumped from the Swift implementation (`windows/testdata/golden/*.json`, regenerated with `windows/parity/regen-golden.sh`, see [BUILDING.md](BUILDING.md)):

- the corpus: 8,971 runs, every screen, every fix and every hint equal to Swift;
- 7,476 detector sessions: 0 mismatches (3,301 fix, 3,904 none, 271 suggest);
- 835 learning scenarios: 0 differences, hash for hash;
- plus word readings, lexicon lookups, trigram costs to the bit, Unicode semantics, guards, confidence values, the text converter and transliteration.

That proves the port computes what the Swift code computes. It does **not** prove anything about Windows itself, which is a different question (see [windows/README.md](../windows/README.md)): the Windows corpus run (0 of 1,857 correct words changed, 240 of 249 fixed) uses *hand-written approximations* of the US, Russian and Kazakh Windows layouts, because the real tables come from `ToUnicodeEx` at run time. One corpus word (with « ») cannot be typed on those fixtures and is skipped.

## Speed and memory

The suite contains micro-benchmarks (`PerformanceTests`): keystroke append, last-word evaluation, an 11-word tail scan, rebuilding all layout tables. They print timings and fail only on very loose bounds (20 µs, 1 ms, 10 ms and 100 ms). Tilqos does the heavy work after a word ends, off the keystroke callback, and the whole language data is about 2 MB, but **no CPU or memory figures are published here, because nothing in the repository measures them**. If you profile it, please share your numbers.

## Measuring it better

Better evidence would be a corpus nobody tuned against: real sentences from people who type in all three languages, with the layout mistakes they actually make. Contributions are welcome (see [CONTRIBUTING.md](../CONTRIBUTING.md)); please contribute only text you are free to publish, with no private information.
