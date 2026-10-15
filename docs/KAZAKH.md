# Kazakh in Tilqos

Kazakh is a first-class language in Tilqos, not an afterthought bolted onto an English/Russian switcher. This page explains the facts about keyboard layouts that the design rests on, what they mean for detection, and how the Latin-to-Cyrillic transliteration works. Everything here is checked by tests that read the *real* macOS layouts (not fixtures); the test names are given so you can look.

> Kazakh readers: the same facts, in Kazakh, are in [README.kk.md](../README.kk.md). Corrections to the Kazakh text are very welcome; see [CONTRIBUTING.md](../CONTRIBUTING.md#kazakh-review).

## 1. The alphabet and the keyboard

Cyrillic Kazakh has 42 letters: the 33 letters of the Russian alphabet plus nine more:

| Letter | Sound (rough) | Latin letter in the official 2021 scheme |
|---|---|---|
| ә Ә | a as in *cat* | ä |
| ғ Ғ | voiced uvular, like a soft *gh* | ğ |
| қ Қ | back *k* | q |
| ң Ң | *ng* | ñ |
| ө Ө | ö | ö |
| ұ Ұ | back *u* | ū |
| ү Ү | front *ü* | ü |
| һ Һ | *h* (rare, mostly loanwords) | h |
| і І | short *i* | ı (dotless) |

On the Mac, the system **Kazakh** layout puts these nine letters on the number row, where the Russian layout has digits. Everything else is in the same place. The table below was produced by reading the three layouts with `UCKeyTranslate` on a Mac with the ABC, Russian and Kazakh layouts enabled (unshifted / with Shift):

| Key (macOS keycode) | ABC | Russian | Kazakh |
|---|---|---|---|
| `2` (kc 19) | `2` `@` | `2` `"` | **ә** **Ә** |
| `3` (kc 20) | `3` `#` | `3` `№` | **і** **І** |
| `4` (kc 21) | `4` `$` | `4` `%` | **ң** **Ң** |
| `5` (kc 23) | `5` `%` | `5` `:` | **ғ** **Ғ** |
| `=` (kc 24) | `=` `+` | `=` `+` | **һ** **Һ** |
| `9` (kc 25) | `9` `(` | `9` `(` | **ұ** **Ұ** |
| `-` (kc 27) | `-` `_` | `-` `_` | **ө** **Ө** |
| `8` (kc 28) | `8` `*` | `8` `;` | **ү** **Ү** |
| `0` (kc 29) | `0` `)` | `0` `)` | **қ** **Қ** |
| `1` (kc 18) | `1` `!` | `1` `!` | `"` `!` |
| `6` (kc 22) | `6` `^` | `6` `,` | `,` `;` |
| `7` (kc 26) | `7` `&` | `7` `.` | `.` `:` |
| `§` / `` ` `` (kc 10, kc 50) | `§` `±` / `` ` `` `~` | `>` `<` / `]` `[` | `(` `)` / `[` `]` |

On the Kazakh layout the digits are not on the number row at all: they are on **Option** (`⌥2` types `2`).

Two facts follow, and the whole design leans on them:

1. **The Russian and Kazakh layouts put all 33 Russian letters on the same keys.** Checked directly: every one of `абвгдеёжзийклмнопрстуфхцчшщъыьэюя` is found on the same physical key in both layouts (`testEveryRussianLetterSitsOnTheSameKeyInKazakh`).
2. **The two layouts differ only on the 12 number-row keys (keycodes 18 to 29) plus three edge keys (10, 50 and 93, the last being the JIS Yen key)** (`testRussianAndKazakhDifferOnlyOnTheNumberRow`). On every one of those keys the Russian side is a digit or a punctuation mark, never a letter (`testEveryKeyThatDiffersIsNonLetterOnTheRussianSide`).

If Apple ever changes either layout these tests fail, which is the point of reading real layouts.

## 2. What that means for detection

| You meant to type | Active layout | You see | Tilqos |
|---|---|---|---|
| Russian `привет` | English (ABC) | `ghbdtn` | fixes it (the usual case) |
| Kazakh `сәлеметсіз` | Russian | `с2леметс3з` | fixes it; the digits wedged inside a Cyrillic word are the giveaway |
| Kazakh `қазақ` | Russian | `0аза0` | fixes it |
| Kazakh `қалайсыз` | English (ABC) | `0fkfqcsp` | fixes it |
| the number `25` | Kazakh | `әғ` | fixes it (a number is unambiguous, whatever the length) |
| Russian `Астана` | Kazakh | `Астана` | **nothing to fix**: the text is already right |

The strings in the third column are what the real macOS layouts produce for the same key presses.

Consequences:

- **Russian text typed on the Kazakh layout is never "wrong".** It looks identical, so there is nothing to correct and no reason to switch layout. This removes the largest source of false positives in a Russian/Kazakh/English switcher.
- **A digit or hyphen in the middle of a Cyrillic word is strong evidence of Kazakh typed on the Russian layout.** `с2лем`, `к-к` (`көк` typed on the Russian layout, where `ө` sits on the hyphen key). The detector treats this as *structure*, ahead of what the character statistics say.
- **Russian vs Kazakh cannot always be decided from the letters.** A word spelled only with letters both languages share looks the same in both layouts. When the corrected text has none of the nine Kazakh letters, Tilqos decides which Cyrillic layout to switch to by your setting (Settings → Languages → *Russian or Kazakh*: decide per app, always Kazakh, always Russian) and, by default, by **remembering which Cyrillic layout you last used in that app**. It switches to Kazakh on its own only when a word contains one of the nine letters.

## 3. Why the buffer stores keys, not characters

On the Kazakh layout the digits are on Option; on the Russian layout they are bare. A table that maps "character typed" to "character that should have been typed" cannot represent "I pressed the key that gives `ә` on one layout and `2` on another" without loss. Tilqos records the **physical key code, the modifiers and the identifier of the layout that was active** for each keystroke, and renders the same keystrokes under every enabled layout when it needs to compare readings. Characters are always derived, never stored. (See also [HOW_IT_WORKS.md](HOW_IT_WORKS.md) and [PRIVACY.md](PRIVACY.md): the buffer holds key codes, lives in memory only, and is short-lived.)

## 4. Kazakh word forms: morphology

Kazakh is agglutinative: `десеңіздер` is `десе` + `ңіз` + `дер`. A character-trigram model cannot express that, and used to accept `десениздер`, a string that does not decompose at all.

Tilqos strips suffixes against the **real rules from the hunspell Kazakh affix file**: 54,063 stems and 360 distinct suffixes (including the 18 personal endings that the source file lacks; see [THIRD_PARTY_NOTICES.md](../THIRD_PARTY_NOTICES.md)), at most three morphemes deep, stored in a 216 KB file (`Resources/Models/kk-morph.bin`). The lexicon (`kk-lex.bin`) separately applies the affix rules in reverse to check whether a typed form is a dictionary form at all.

```
десеңіздер  = десе + ңіз + дер     decomposes
десениздер                         does not decompose
кофені      = кофе + ні            decomposes
```

This is used as *evidence*, not as a filter: coverage is good but not complete (for instance `сәлеметсіз` has no stem in the word list), so a word that fails to decompose is *unsupported*, not *wrong*. The bonus is weighted by depth: a one-morpheme parse counts for more than a three-morpheme one, which separates `екенмін` (`екен` + `мін`) from the accidental chain `екенмің` (`екен` + `м` + `ің`).

## 5. Latin to Cyrillic Kazakh (transliteration)

Hotkey `⌥⇧T`, or menu → *Transliterate*. The direction (to Cyrillic or to Latin) is worked out from the text itself, and there are two Latin schemes (Settings → Transliteration):

| Scheme | What it is |
|---|---|
| **Official 2021** | 31 letters with diacritics: `ä ğ ñ ö ū ü ş` and the dotted/dotless `i` / `ı` pair. One Cyrillic letter to one Latin letter, so it round-trips almost exactly. |
| **Practical (ASCII)** | What people type in chats, with no diacritics: `qalaisyz`, `menin ishimde`. Lossy by construction. |

```
сәлеметсіз бе, қалайсыз?   ->   sälemetsız be, qalaisyz?    (official 2021)
сәлеметсіз бе, қалайсыз?   ->   salemetsiz be, qalaisyz?    (practical)
```

### The hard direction: practical Latin to Cyrillic

Cyrillic to Latin is a substitution. Latin to Cyrillic in the practical scheme is **ambiguous**: `a` is `а` or `ә`, `o` is `о` or `ө`, `u` is `ұ`, `ү` or `у`, `n` is `н` or `ң`, `g` is `г` or `ғ`, `h` is `х` or `һ`. A table cannot decide that, so Tilqos **decodes** each word with a beam search (width 12) over the Kazakh trigram model, with the constraints of the language built in:

- **Vowel harmony.** In a native Kazakh word the vowels come from one row. A word that mixes front and back vowels is penalised (2.2 bits), which settles most vowel ambiguity on its own:

  ```
  ulken  ->  үлкен      front row: ү
  tulga  ->  тұлға      back row:  ұ
  koz    ->  көз        front:     ө
  bolyp  ->  болып      back:      о
  magan  ->  маған      back, so g -> ғ
  ```
- **Consonant harmony.** `ғ` and `қ` go with back vowels, `г` and `к` with front ones.
- **Three rules that statistics do not learn well:** `ң` never starts a word, native words do not start with `г` (only loanwords do; penalty 1.6), and `һ` occurs in a handful of words (penalty 4.0). Choosing `ң` over `н` costs 1.0, because a character model alone over-predicts `ң` in `-ын` / `-мын` endings.
- **Morphology.** A reading that decomposes into real morphemes gets a 1.0-bit bonus.
- A list of the most frequent short words (`ғой`, `да`, `не`, `бұл`) is fixed by hand: three letters carry too little statistical information.

On the project's own sample of 44 Kazakh words the decoder gets all 44 right (`TransliteratorTests.testPracticalDecodingAccuracy`, which only requires 75 %). That sample was written by the author and used while tuning, so treat it as a regression test, not as a measure of real-world accuracy. Some endings are genuinely ambiguous (`jatqanyn` is both `жатқанын` and `жатқаның`); Settings → Transliteration lets you pin a spelling.

### Mixed text: Kazakh with Russian and English words

Real notes are bilingual: Kazakh grammar with Russian words that are *also* typed in Latin (`Zametki`, `Spisok`). Applying Kazakh rules to those produces `Зәметкі`. So each word is read **twice**, once with the Kazakh model and once with the Russian model, and the better reading wins. The two models are not calibrated against each other: the Kazakh one is trained on 54,063 words and the Russian one on 146,269, so the Kazakh model is systematically more confident. The Kazakh reading's cost is therefore raised by 0.4 bits before the comparison (`kazakhBias` in `Transliterator.swift`). That constant was chosen empirically from the margins that `TranslitCalibrationTests.testPrintCalibration` prints for 20 labelled Russian and 20 labelled Kazakh words; it is a compromise, not a clean separation, and the cases it cannot settle are settled by other signals: the letter `q` (in this scheme `q` is always `қ`, and Russian words in Latin do not use it), the brand list, morphology, vowel harmony and your own rejections.

```
bugin keshke 3 jospar Vstrecha, Zametki, Raspisanie, Zadachi
    ->
бүгін кешке 3 жоспар Встреча, Заметки, Расписание, Задачи

Vkladka: spisok, v papke, zametki na ertengi
    ->
Вкладка: список, в папке, заметки на ертеңгі
```

(Both lines are taken from `MixedTranslitTests.testFullNote`.)

### Brands are a list, not a model

Tried and rejected: no model separates brand names from words. No cost threshold works: the best Cyrillic reading of `instagram` costs 3.96 bits per symbol, *less* than the real Russian word `kofe` (4.62), and a third, English, model did not help either (`TranslitCalibrationTests.testPrintEnglishModelSeparation` prints the numbers). Instead there are two mechanisms: a **built-in list** of popular brands and apps (including case endings, so `instagramda` is recognised), and **learning from rejections**: if you undo a transliteration with `⌥⇧Z`, the words that changed are remembered as "do not convert" (Settings → Transliteration shows and edits the list).

## 6. Limits, honestly

- The Kazakh part of the evaluation corpus is small and was written by one person: see [ACCURACY.md](ACCURACY.md).
- The UI and these documents in Kazakh need review by native speakers. Please open an issue (or a pull request) with corrections.
- Kazakh typed on a layout other than ABC/Russian/Kazakh (for example a third-party Kazakh Latin layout) is not modelled.
- Names, slang and loanwords that are in none of the dictionaries are handled by statistics only, and statistics only ever produce an *offer*, never an automatic fix. Learning (words you keep in two separate sessions) makes the app recognise your own vocabulary over time.
