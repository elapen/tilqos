# How Tilqos decides

This is the detector pipeline in one place: what is recorded, which readings are compared, what counts as evidence, which numbers turn evidence into "fix it" or "offer it", and how the app learns. File names are in `Sources/TilqosKit/` (macOS, Swift); the Windows port in `windows/internal/core/` is a line-by-line Go port of the same algorithm, checked against the Swift implementation on golden data (see [ARCHITECTURE.md](ARCHITECTURE.md)).

```
key presses ──► event tap ──► stroke buffer ──► word ends? ──► readings under each layout ──► evidence ──► confidence
 (all apps)     (Input/)      key codes only     (Detect/)       (Detect/WordScorer)          ladder       │
                                                                                                          ▼
                                       nothing  ◄── below suggest threshold ──┤  ≥ suggest threshold: show a hint (Tab)
                                                                              │  ≥ auto threshold:    replace the word
                                                                              ▼
                                                  verify the text, replace (Apply/), switch layout, remember for undo
```

## 1. What is recorded

The tap sees key-down events, modifier changes and mouse-button-downs system-wide (`Input/EventTap.swift`). For each text-producing key it records a `RecordedStroke`: the **physical key code**, the modifiers, a timestamp and the **identifier of the layout that was active**. It never stores the characters; characters are derived when needed by rendering the key codes through a layout table (`Input/KeyMapTable.swift`, built from the system's own layouts with `UCKeyTranslate`). The buffer holds at most 256 strokes. When it is cleared and what else is kept in memory is in [PRIVACY.md](PRIVACY.md).

Why key codes: on the Kazakh layout the digits are on Option, and the same physical key is `2` on one layout and `ә` on another. A table that maps characters to characters cannot express that without loss ([KAZAKH.md](KAZAKH.md)).

## 2. When a word ends

A key ends a word only if it produces **no letter on any candidate layout**, not merely on the one that was active (`Detector.endsWord`). On the ABC layout `, . ; ' [ ]` are punctuation, but on the Russian layout they are the letters `б ю ж э х ъ`. A Russian word typed on the English layout often contains them mid-word: `,skj` is `было`, `nt,t` is `тебе`, `k.,k.` is `люблю`, `[jhjij` is `хорошо`. Ending the word at the comma used to "correct" the fragment (`[jhjij` became `[орошо`). Now the whole run is read under each layout.

The converse also matters: if a punctuation mark at the edge of the typed word looks odd in the typed layout (`to\`, `,sk`), the typed text is *not* treated as a word. `to\` is `ещё` typed on ABC, where `ё` sits on the backslash key.

Punctuation typed on the same wrong layout straight after a corrected word is converted with it: `ghbdtn^` becomes `привет,` (on the Mac Russian layout the comma is Shift+6).

The evaluation is **deferred by one run-loop turn** after the separator: the tap callback runs before the separator reaches the app, so injecting then would put the correction in front of the character that triggered it.

## 3. Readings

For the closed word, `WordScorer.reading` renders the same strokes under every candidate layout (enabled languages that have a layout installed and a model) and describes each rendering:

| Field | Meaning |
|---|---|
| `core` | the text without leading/trailing punctuation |
| `isClean` | a single word: letters, digits and at most inner hyphens or apostrophes; `nt,t` is not clean |
| `isKnown` | a real word of the language (lexicon, derived by affix rules, built-in brand list, or taught by you) |
| `isCore` | among the language's most frequent words (the frequency tier) |
| `cost` | mean trigram cost in bits per trigram, with penalties |
| `interiorJunk` | off-alphabet characters wedged between letters: `с2лем`, `к-к` |
| `hasOddEdges` | punctuation glued to the word that prose in that script does not have |

### Character trigram models (`en.bin`, `ru.bin`, `kk.bin`)

One byte per trigram: the cost in bits of a letter given the previous two, times 12, clamped to a byte; add-one smoothing; words padded with two leading boundaries and one trailing. English has a 28-symbol cube (26 letters, apostrophe, boundary) and the two Cyrillic models share one 43-symbol cube (33 Russian letters plus the nine Kazakh ones, plus boundary), so they are comparable index for index. Each interior off-alphabet character costs 6.0 bits on top.

The models are trained on dictionary **headwords**, not on inflected forms (retraining on expanded forms was tried and rejected; see section 11). That is why a frequent inflected form such as `еще` can look like garbage to the trigram model, and why the trigram model is never the first line of evidence. On the shipped models, real words cost about 3.6 to 3.8 bits per trigram (English), 2.9 to 3.9 (Russian) and 3.0 to 3.1 (Kazakh), while layout garble costs 6.8 to 7.1, 5.6 to 7.3 and 5.7 to 6.9; `TrigramModelTests.testPrintSeparationMargins` prints those ranges.

### Word-form lexicons (`en-lex.bin`, `ru-lex.bin`, `kk-lex.bin`)

Stored the way hunspell stores dictionaries: **stems with affix-flag bitmasks plus the affix rules**, not expanded forms. Lookup reverses the rules: strip a candidate ending, restore what the rule removed, check the rule's condition, and ask whether that stem exists and carries the rule's flag. Stems are 32-bit FNV-1a hashes of the lower-cased word (`ё` is folded into `е`, typographic apostrophes into `'`), so the word lists cannot be read straight back out of the files and a hash collision can only make an unknown string look known (a missed correction, the safe direction).

| File | Stems | Affix rules | Frequency tier | Size |
|---|---|---|---|---|
| `en-lex.bin` | 48,392 | 43 suffix + 7 prefix | 219 words | 296,598 B |
| `ru-lex.bin` | 139,035 | 1,581 suffix | 281 words | 882,100 B |
| `kk-lex.bin` | 53,991 | 2,593 suffix | 122 words | 412,884 B |

(Counts are read from the headers of the shipped files. Stems are distinct normalised stems, so they are fewer than the `.dic` line counts; `ru_RU.dic` has 146,269 entries and `kk_KZ.dic` 54,063.) Together with the three trigram files (`en.bin` 21,972 B, `ru.bin` and `kk.bin` 79,527 B each) and `kk-morph.bin` (221,793 B) the data is 1,994,401 bytes.

Hand-written lists in `wordlists/` are compiled in: `*-extra.txt` (brands, terms, colloquial words and chat abbreviations the dictionaries lack: `telegram`, `api`, `норм`, `lol`) and `*-core.txt` (the frequency tier). Frequency is the one thing a dictionary cannot tell you, and short words need it: `ult` → `где` is worth correcting because `где` is among the most frequent Russian words, while `afk` → `фал` is not, although `фал` is in the dictionary too.

The English lexicon is a modern SCOWL list rather than the 1934 Webster's list in `/usr/share/dict/words`, because the old list is full of words nobody types (`tot`) and each such word would hide a real layout mistake. (`en.bin`, the English trigram table, is still trained on the old list; see [THIRD_PARTY_NOTICES.md](../THIRD_PARTY_NOTICES.md).)

### Kazakh morphology (`kk-morph.bin`)

Suffix stripping against the real hunspell affix rules, three morphemes deep, used as evidence and never as a filter. Details in [KAZAKH.md](KAZAKH.md). A Kazakh candidate that decomposes (or that the lexicon already knows) gets a 0.6-bit bonus (`WordScorer.morphologyBonus`): `сәлеметсіз` parses while `с2леметс3з` cannot.

## 4. The evidence ladder

Confidence is assigned from the strongest available evidence down (`Detector.confidence`). The numbers are in code as `certain = 0.95`, `confident = 0.9`, `offerOnly = 0.6`.

1. **What you taught it outranks everything.** A word you rejected is never converted. A conversion you accepted is applied without asking, except that a conversion learned in one language's company is only *offered* among words of the other script.
2. **Is the text on screen a real word?** Then it is not rewritten automatically. `еще`, `тот`, `her` stay what they are, whatever a character model thinks of them. The one exception is a *collision* where both readings are words (`tot` vs `еще`): if the other reading is a frequent word, the typed one is not, and the surrounding words are in the other script, it is *offered* (0.6), never applied.
   *Unless* the "word" only looks like one because a punctuation mark at its edge was really a letter of the other layout (`to\`).
3. **A number is unambiguous.** `әғ` on the Kazakh layout is `25` on the number row; a numeric reading is certain (0.95) unless the typed text is itself a word, and it ignores the length floor.
4. **Structure.** A digit or hyphen between letters of a Cyrillic word (`с2лем`) is Kazakh typed on the Russian layout whatever the dictionaries say: if the other reading is a word, certain (0.95).
5. **Is the other reading a real word?** Layout garble is not a word in any language (`ghbdtn`), while its conversion is (`привет`). Brands and terms are the opposite: words in English, garbage after conversion. That asymmetry is what makes correcting safe. Then:
   - target of **4 or more letters**: certain (0.95) when the cost gap is greater than -1.0, offer-only (0.6) when it is greater than -2.0, otherwise nothing;
   - target of **3 letters**: confident (0.9) if it is a frequent word and either the gap is at least 0.8 or the context favours it, otherwise offer-only; a non-frequent three-letter word is offered only if both the gap is at least 0.8 and the context favours it;
   - **1 or 2 letters**: below the default length floor (see "Short words" below). If you lower the floor to 2, a frequent two-letter word is offered when the context favours it.
6. **Neither reading is a word.** Only the character model speaks, and only as an offer: the target must be at least 5 letters and the gap at least 1.8, and if the typed text has punctuation inside, its pieces must not be words themselves (`foo.bar` is a file name; `pfib,bcm` is `зашибись`). With a Cyrillic-junk signal the gap is mapped linearly to confidence.

The **gap** is `typed cost − target cost + context lean`, in bits per trigram. `Detector.confidence(gap:)` maps a gap to 0 to 1 linearly between 0.4 and 2.0; the shipped models separate real words from garble by roughly that range.

### Context

The scripts (Latin or Cyrillic) of the neighbouring words tilt the decision but never decide it: each extra neighbouring word in a script moves the gap by 0.35 bits (`contextWeight`), up to three. `ghbdtn` in Russian text is corrected more readily; an English word inside Russian text stays English. In the last-word path the neighbours are the words before it in the buffer; in the tail scan, two on each side.

Because the buffer is thrown away at every click, Return and pause, the first word after one has no neighbours, which is exactly when a wrong layout is most likely. So when the buffer is short (fewer than 24 strokes) and the focused text field can be read through Accessibility, the app reads up to 160 characters before the caret **once per buffer session**, immediately reduces them to the last three words, and keeps only the *scripts* of those words. It does this in memory, never logs it, and skips terminals and code editors. On the corpus this lifts the share of short Russian words fixed from 86.2 % (25 of 29) to 96.6 % (28 of 29) (`CorpusTests`, section "context from the text field").

### Short words

Three letters carry almost no statistics, so the length floor matters: words shorter than **3** letters (configurable from 2 to 8 in Settings → Correction → fine-tune) are never touched on their own, only pulled in as neighbours of a confident fix.

## 5. Thresholds and presets

A judgement's confidence is compared with two thresholds (`Prefs.autoThreshold`, `Prefs.suggestThreshold`):

| Confidence | Result |
|---|---|
| ≥ automatic threshold | replace the word (in apps whose mode is *Auto*) |
| ≥ suggestion threshold | show a hint at the cursor; accept with Tab |
| below | do nothing |

In an app whose mode is *Suggest only* (terminals by default) even a confident fix is only offered. The three presets in Settings → Correction set both thresholds:

| Preset | Fix automatically at | Offer at | Intent |
|---|---|---|---|
| **Cautious** | 0.93 | 0.55 | only obvious mistakes happen by themselves |
| **Balanced** (default) | 0.80 | 0.45 | obvious mistakes are fixed, doubtful ones are offered |
| **Bold** | 0.55 | 0.30 | also applies what would otherwise only be offered |

Because the detector's confidence comes in tiers (0.95, 0.9, 0.6), the presets sit between tiers: Cautious lets only "certain" fixes through, Bold also applies the "offer-only" tier. What each preset costs and gains on the regression corpus is in [ACCURACY.md](ACCURACY.md).

## 6. Things that are never touched

Before any of the above, the guards (`Detect/Guards.swift`) put words off limits: e-mail addresses, URLs (with a scheme, or ending in a common TLD), file paths, `snake_case`, `camelCase` and anything that looks like a call (`name(`), initialisms in capitals (`FYI`, `HTTP`, `ГОСТ`), all-digit words, a word introduced by `$` `` ` `` `#` `\` `~` `@` `%` `&`, words on your exceptions list, and (through the length floor) very short words. When in doubt, protect: a missed correction is a shrug; a mangled identifier in a code editor is the reason people uninstall utilities like this one. Apps in the *Off* mode (password managers, always; anything you add) are not evaluated at all.

## 7. Pulling neighbours in, and the wider scan

- **Backfill.** A confident fix vouches for up to four preceding unfixed words typed on the same wrong layout whose other reading is a real word: `z nt,z k.,k.` becomes `я тебя люблю`, `yt pyf.` becomes `не знаю`. Alone, `z` and `yt` are too short to touch.
- **Tail scan.** At a sentence end the detector also looks at up to 24 trailing words in two passes (first the sure fixes, then the rest, now seeing the corrected neighbours). The result is only ever **offered**: silently rewriting several words at once is too surprising.
- **Manual fix (`⌥⇧F`).** You said the word is wrong, so the only question is which reading is best: readings are ranked (a known word beats an unknown one, a frequent one beats a rare one, the trigram cost breaks ties), thresholds and the length floor are ignored, and a real word may be converted. It counts as a lesson (see section 9).

## 8. Applying a correction

`Apply/TextReplacer.swift` is built around one rule: refuse to guess. The failure it exists to prevent is the correction appearing *next to* the wrong text instead of replacing it.

1. **Before touching anything**, the text before the caret must be the text about to be deleted (the app is given up to five 8 ms looks to catch up with the last keystroke). If the app lets Tilqos read the field and it does not match (the host autocorrected, the caret moved, you pasted), **nothing is done** and the buffer is dropped. A capital the host app put on the word is carried into the correction.
2. **Accessibility route**: select exactly the replaced characters, confirm the selection, write, read back. If the app inserted without deleting, that is visible and is repaired with keystrokes, and the app is remembered as unreliable.
3. **Keystroke route**: Backspace presses, then the text as Unicode key events in chunks of at most 20 UTF-16 units (`CGEventKeyboardSetUnicodeString` carries no more), never splitting a character. Used wherever the Accessibility route is unavailable or has failed, and always for apps that draw their own text (VS Code, Cursor, the common terminals, IntelliJ, Sublime Text; the list is `Injector.alwaysSynthetic`).

A suggestion on screen is dismissed when you press Backspace: accepting it after the text changed would replace the wrong characters. Undo (`⌥⇧Z`) also checks that the corrected text is still at the caret and leaves the text alone if it is not.

After a fix, Tilqos **switches the layout** so what you type next lands in the right language. When the corrected text has no Kazakh-only letter, Russian and Kazakh are indistinguishable from the text alone; your setting (per app, always Kazakh, always Russian) and the per-app memory of the last Cyrillic layout decide.

## 9. Learning

The `Learner` (`Core/Learner.swift`) adjusts the app to you from what you do with its decisions:

| What you did | What it learns |
|---|---|
| Undid a correction with `⌥⇧Z` right away (within 8 seconds) | the word goes on the visible exceptions list |
| Erased a correction and typed the original word again (within 20 seconds) | never convert that word |
| Dismissed a hint for the same word twice with `Esc` | stop offering it |
| Accepted a hint with Tab, or fixed a word with `⌥⇧F` | convert it without asking next time (among text of the other language: only offer) |
| Typed and left a word of 4 or more letters that no dictionary or layout reading explains, in two separate sessions | it is your word (name, slang, jargon): leave it alone, and recognise it if typed on the wrong layout |

What it deliberately does **not** learn from, and why:

- **A hint you typed past, or that timed out, means nothing**; most of the time it was simply not noticed. An early version counted that as a refusal and, after two such hints, stopped correcting the word for ever.
- **A word becomes "yours" only if no other layout turns the same keys into a dictionary word.** `yfgbcfk`, left uncorrected, is a mistake nobody caught (`написал`), not an English word; learning it would stop it being corrected. The same check covers Russian typed on the Kazakh layout: those keys are looked up in the Russian dictionary too, so ordinary Russian words do not become "Kazakh vocabulary", while a slang word like `кароче` does become yours.
- Words shorter than 4 letters (as vocabulary), and anything containing digits or symbols (only letters, `-` and `'` are learned).

Storage: at most 3,000 entries, least recently used evicted first, each a salted 64-bit FNV-1a hash of the normalised word with a small state and the *day* of last use. This is **not** cryptographically irreversible; read the honest description in [PRIVACY.md](PRIVACY.md). When the rules change (`schemaVersion`), entries learned under the old rules are dropped and explicit acceptances are kept.

## 10. Transliteration

A separate path (`Translit/`), described in [KAZAKH.md](KAZAKH.md): two Latin schemes, a beam-search decoder with vowel harmony for the ambiguous direction, a second reading with Russian-in-Latin rules for mixed text, a brand list and learn-from-undo.

## 11. What was tried and not kept

These come from the author's experiments while building the original version; they were not re-run for the public release.

- **Retraining the trigram models on inflected forms.** About three more corrections on the corpus, but the model became less sharp on short words (`TrigramModelTests` stopped separating words from garble), it broke `һ` in transliteration, and four fixes turned into hints. The lexicon already covers inflected forms.
- **Sentence-position rules** (a capital Latin word mid-phrase is a brand; words that never start a sentence). No effect on the corpus: brands and names are not converted anyway because their conversion is not a word.
- **`NSSpellChecker`.** macOS only, knows English and Russian but not Kazakh (for an unsupported language every word is "correct"), and its first call costs 7 to 14 ms. File-based dictionaries do the same on all three languages and carry over to Windows unchanged.
- **A model for brand names.** Not separable from words by any threshold (see [KAZAKH.md](KAZAKH.md)); replaced by a list plus learning.

## 12. Where the numbers come from

The constants in `Detector.swift` (`certain`, `confident`, `offerOnly`, `contextWeight`, `knownAltTolerance`, `shortGarbageGap`, `unknownOfferGap`, `backfillLimit`) were set against the regression corpus, and each is documented in code with the direction in which it errs. That makes the published accuracy figures in-sample: see [ACCURACY.md](ACCURACY.md) before generalising from them.
