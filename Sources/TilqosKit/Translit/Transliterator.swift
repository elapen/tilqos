import Foundation

/// Converts Kazakh between Cyrillic and Latin.
///
/// The two directions are not equally hard. Cyrillic to Latin is a substitution. Latin to
/// Cyrillic, in the practical ASCII spelling, is genuinely ambiguous: `u` could be ұ, ү or у,
/// `a` could be а or ә, `n` could be н or ң. So that direction is decoded rather than
/// mapped — a beam search over the Kazakh trigram model, constrained by vowel harmony.
public struct Transliterator {
    /// How many partial readings to keep while decoding a word. Twelve is comfortably past
    /// the point where a wider beam changes the answer.
    private let beamWidth = 12
    /// Charged when a word mixes front and back vowels. Native Kazakh words never do, so this
    /// single constraint resolves most of the vowel ambiguity on its own.
    private let harmonyPenalty = 2.2
    /// Charged for choosing һ, which is correct in only a handful of words.
    private let hehPenalty = 4.0
    /// Charged for choosing ң over н. Both are written `n`, but н is far more common, and a
    /// character model on its own over-predicts ң in the endings `-ын` / `-мын`.
    ///
    /// Measured: lowering this to 0.5 broke `bolamyn` → боламын without recovering
    /// `desenizder` → десеңіздер, so 1.0 is the better trade.
    private let engPenalty = 1.0
    /// Charged for г at the start of a word. Native Kazakh words begin with ғ; initial г
    /// belongs to loanwords, so `goi` is ғой rather than гөй.
    private let initialGPenalty = 1.6

    /// Subtracted from a Kazakh reading that decomposes into real morphemes.
    ///
    /// Deliberately large relative to the calibration bias: a valid decomposition is hard
    /// evidence, not a statistical lean. Calibrated in `TranslitCalibrationTests`.
    private let morphologyBonus = 1.0

    private let models: ModelStore
    private let morphology: Morphology?
    /// User corrections, consulted before anything else.
    private let overrides: [String: String]
    /// Words the user has rejected, kept in Latin.
    private let neverConvert: Set<String>

    public init(
        models: ModelStore = .shared,
        overrides: [String: String] = [:],
        neverConvert: Set<String> = []
    ) {
        self.models = models
        self.morphology = models.kazakhMorphology()
        self.overrides = overrides
        self.neverConvert = neverConvert
    }

    /// Words in `text` that the decoder would rewrite, as their Latin spellings.
    ///
    /// Used to learn from an undo: whichever words changed are the ones the user rejected.
    public func changedWords(from before: String, to after: String) -> [String] {
        func words(_ text: String) -> [String] {
            text.split(whereSeparator: { !$0.isLetter && $0 != "'" }).map(String.init)
        }
        let source = words(before)
        let result = words(after)
        guard source.count == result.count else { return [] }
        return zip(source, result).filter { $0.0 != $0.1 }.map(\.0)
    }

    // MARK: - Direction

    public enum Direction: Sendable {
        case toLatin
        case toCyrillic
    }

    /// Guesses which way the user wants to go by looking at what the text already is.
    public static func direction(for text: String) -> Direction {
        var cyrillic = 0
        var latin = 0
        for character in text.lowercased() where character.isLetter {
            if TranslitTables.cyrillicToPractical[character] != nil {
                cyrillic += 1
            } else if character.isASCII || TranslitTables.officialToCyrillic[character] != nil {
                latin += 1
            }
        }
        return cyrillic > latin ? .toLatin : .toCyrillic
    }

    public func convert(_ text: String, scheme: TranslitScheme, direction: Direction) -> String {
        switch direction {
        case .toLatin: return toLatin(text, scheme: scheme)
        case .toCyrillic: return toCyrillic(text, scheme: scheme)
        }
    }

    // MARK: - Cyrillic to Latin

    public func toLatin(_ text: String, scheme: TranslitScheme) -> String {
        let table =
            scheme == .official2021
            ? TranslitTables.cyrillicToOfficial : TranslitTables.cyrillicToPractical

        var result = ""
        result.reserveCapacity(text.count)
        for character in text {
            let lower = Character(character.lowercased())
            guard let mapped = table[lower] else {
                result.append(character)
                continue
            }
            // Restore the case of the original letter across however many Latin letters it
            // became, so Ш becomes Sh rather than SH or sh.
            result += character.isUppercase ? capitalizeFirst(mapped) : mapped
        }
        return result
    }

    private func capitalizeFirst(_ text: String) -> String {
        guard let first = text.first else { return text }
        return first.uppercased() + text.dropFirst()
    }

    // MARK: - Latin to Cyrillic

    public func toCyrillic(_ text: String, scheme: TranslitScheme) -> String {
        var result = ""
        var word = ""
        result.reserveCapacity(text.count)

        // Work word by word: harmony and the language model are properties of a word, not of
        // a whole passage.
        for character in text {
            if character.isLetter || character == "'" || character == "\u{2019}" {
                word.append(character)
            } else {
                if !word.isEmpty {
                    result += convertWord(word, scheme: scheme)
                    word = ""
                }
                result.append(character)
            }
        }
        if !word.isEmpty { result += convertWord(word, scheme: scheme) }
        return result
    }

    private func convertWord(_ word: String, scheme: TranslitScheme) -> String {
        let lower = word.lowercased()
        let converted =
            scheme == .official2021 ? decodeOfficial(lower) : decodePractical(lower)
        return restoreCase(of: word, onto: converted)
    }

    /// Applies the original word's capitalisation. Length can change (`sh` → `ш`), so this
    /// works at word level rather than per character.
    private func restoreCase(of original: String, onto converted: String) -> String {
        let letters = original.filter(\.isLetter)
        guard let first = original.first, first.isUppercase else { return converted }
        // ALL CAPS stays all caps; a single leading capital stays a single leading capital.
        if letters.count > 1, letters.allSatisfy(\.isUppercase) {
            return converted.uppercased()
        }
        return capitalizeFirst(converted)
    }

    // MARK: - Official decoding

    /// Deterministic apart from `i`, which the standard uses for both и and й.
    private func decodeOfficial(_ word: String) -> String {
        let characters = Array(word)
        var result = ""
        for (position, character) in characters.enumerated() {
            guard let mapped = TranslitTables.officialToCyrillic[character] else {
                result.append(character)
                continue
            }
            if character == "i" {
                // и becomes й between a vowel and a consonant, which is where Kazakh
                // diphthongs put it: `ai` → ай, `oi` → ой.
                let previous = position > 0 ? characters[position - 1] : nil
                let next = position + 1 < characters.count ? characters[position + 1] : nil
                let previousIsVowel =
                    previous.flatMap { TranslitTables.officialToCyrillic[$0] }
                    .map(TranslitTables.isVowel) ?? false
                let nextIsVowel =
                    next.flatMap { TranslitTables.officialToCyrillic[$0] }
                    .map(TranslitTables.isVowel) ?? false
                result.append(previousIsVowel && !nextIsVowel ? "й" : "и")
                continue
            }
            result.append(mapped)
        }
        return applyHehExceptions(result)
    }

    // MARK: - Practical decoding

    /// One partial reading of a word.
    private struct Beam {
        var text: String
        var cost: Double
        /// Last two trigram symbols, so the next letter can be scored in constant time.
        var previous: Int
        var beforePrevious: Int
        var sawFrontVowel: Bool
        var sawBackVowel: Bool
        /// Scored transitions, for normalising the cost by length.
        var steps: Int
    }

    /// One way of reading a Latin word: which language, which letter options, which rules.
    private struct Variant {
        let lang: Lang
        let options: [Character: [Character]]
        let digraphs: [String: [Character]]
        /// Vowel harmony and the ң/г rules apply to Kazakh only.
        let kazakhRules: Bool
    }

    private static let kazakhVariant = Variant(
        lang: .kk, options: TranslitTables.practicalToCyrillic,
        digraphs: TranslitTables.practicalDigraphs, kazakhRules: true)

    private static let russianVariant = Variant(
        lang: .ru, options: TranslitTables.practicalRussianToCyrillic,
        digraphs: TranslitTables.practicalRussianDigraphs, kazakhRules: false)

    /// Added to the Kazakh reading before comparing.
    ///
    /// The two models are not calibrated against each other: Kazakh is trained on 54,063 stems
    /// and Russian on 146,269 words, so the Kazakh model is systematically more confident and
    /// every Russian word leans slightly Kazakh. Measured over labelled words
    /// (`TranslitCalibrationTests`), the Kazakh-minus-Russian margin runs −0.85…+2.09 for
    /// Russian words and −3.08…+0.58 for Kazakh ones. 0.4 sits inside that gap: it moves the
    /// misread Russian words across without dragging any Kazakh word with them — the closest,
    /// `kerek` at −0.81, still lands at −0.41.
    private let kazakhBias = 0.4

    /// Above this cost per symbol a word is implausible in both languages and is left in Latin.
    ///
    /// Set from the same measurements: real words top out around 4.6 (`kofe`, `video`) and this
    /// clears them. It catches `whatsapp` (5.3), but deliberately does **not**
    /// pretend to recognise brand names in general — `instagram` scores 3.96, cheaper than
    /// real words, so no threshold can separate them. Names that transliterate
    /// plausibly do get converted; that is what the overrides list is for.
    private let leaveAloneThreshold = 5.0

    /// One- and two-letter words skip the threshold entirely. They carry too little context to
    /// score well, and they are almost always prepositions — `v` must become `в`.
    private let shortWordLength = 2

    private func decodePractical(_ word: String) -> String {
        // The user's own corrections win, then the built-in frequency list; a character model
        // has no way to settle either.
        if let pinned = overrides[word] { return pinned }
        // Brands and anything the user has rejected stay exactly as typed.
        if TranslitTables.isLatinOnly(word, extra: neverConvert) { return word }
        if let known = TranslitTables.practicalWordOverrides[word] { return known }

        // Read the word as Kazakh and as Russian, then keep whichever fits better. Real writing
        // code-switches constantly — a Kazakh sentence carries `Zametki`, `spisok`, `papka` —
        // and forcing Kazakh rules onto those puts Kazakh-only letters into them.
        // `q` settles it outright. In this spelling q is always қ, and Russian written in Latin
        // has no use for the letter — it spells к as `k`. So a word containing q is Kazakh, and
        // the calibration bias must not be allowed to pull it away.
        let isDefinitelyKazakh = word.contains("q")

        let readings = [Self.kazakhVariant, Self.russianVariant].compactMap {
            variant -> Reading? in
            guard var reading = decode(word, with: variant) else { return nil }
            if variant.kazakhRules && !isDefinitelyKazakh { reading.cost += kazakhBias }
            return reading
        }
        guard let best = readings.min(by: { $0.cost < $1.cost }) else {
            return fallbackDecode(word)
        }
        // Neither language claims it: almost certainly a name or a foreign word.
        if word.count > shortWordLength, best.cost > leaveAloneThreshold { return word }
        return best.text
    }

    /// Both readings of a word with their costs. Used to calibrate the language bias against
    /// labelled words rather than by guesswork.
    func readings(of word: String) -> [(lang: Lang, text: String, cost: Double)] {
        [Self.kazakhVariant, Self.russianVariant].compactMap { variant in
            guard let reading = decode(word.lowercased(), with: variant) else { return nil }
            return (variant.lang, reading.text, reading.cost)
        }
    }

    struct Reading {
        var text: String
        /// Mean bits per symbol. Comparable between the two models because Russian and Kazakh
        /// share one 42-letter Cyrillic alphabet, so their tables are indexed identically.
        var cost: Double
    }

    private func decode(_ word: String, with variant: Variant) -> Reading? {
        guard let model = models.model(for: variant.lang) else { return nil }

        let units = split(word, with: variant)
        let boundary = model.boundarySymbol
        var beams = [
            Beam(
                text: "", cost: 0, previous: boundary, beforePrevious: boundary,
                sawFrontVowel: false, sawBackVowel: false, steps: 0)
        ]

        for unit in units {
            let options = unit.options
            var next: [Beam] = []
            next.reserveCapacity(beams.count * options.count)

            for beam in beams {
                let atWordStart = beam.text.isEmpty
                for option in options {
                    // ң never begins a Kazakh word, so that reading is not merely unlikely.
                    if variant.kazakhRules && option == "ң" && atWordStart { continue }

                    guard let symbol = model.symbol(for: option) else {
                        // Not a Cyrillic letter — carry it through without scoring it.
                        var extended = beam
                        extended.text.append(option)
                        next.append(extended)
                        continue
                    }

                    var extended = beam
                    extended.cost += model.cost(
                        after: beam.beforePrevious, beam.previous, symbol)
                    extended.text.append(option)
                    extended.beforePrevious = beam.previous
                    extended.previous = symbol
                    extended.steps += 1

                    if variant.kazakhRules {
                        if TranslitTables.frontVowels.contains(option) {
                            if beam.sawBackVowel { extended.cost += harmonyPenalty }
                            extended.sawFrontVowel = true
                        } else if TranslitTables.backVowels.contains(option) {
                            if beam.sawFrontVowel { extended.cost += harmonyPenalty }
                            extended.sawBackVowel = true
                        } else if TranslitTables.frontConsonants.contains(option) {
                            if beam.sawBackVowel { extended.cost += harmonyPenalty }
                        } else if TranslitTables.backConsonants.contains(option) {
                            if beam.sawFrontVowel { extended.cost += harmonyPenalty }
                        }

                        if option == "һ" { extended.cost += hehPenalty }
                        if option == "ң" { extended.cost += engPenalty }
                        if option == "г" && atWordStart { extended.cost += initialGPenalty }
                    }
                    next.append(extended)
                }
            }

            beams = Array(next.sorted { $0.cost < $1.cost }.prefix(beamWidth))
        }

        // Word endings carry a lot of information in an agglutinative language, so close the
        // word properly rather than stopping mid-context.
        for index in beams.indices {
            beams[index].cost += model.cost(
                after: beams[index].beforePrevious, beams[index].previous, boundary)
            beams[index].steps += 1
        }

        // Re-rank on morphology. A reading that decomposes into real morphemes is far stronger
        // evidence than any trigram score, and it is the only thing that separates `десеңіздер`
        // (десе + ңіз + дер) from `десениздер`, which decomposes into nothing.
        //
        // Applied after the beam search rather than inside it, because a decomposition is a
        // property of the finished word, not of a prefix.
        var candidates = beams.compactMap { beam -> Reading? in
            guard beam.steps > 0 else { return nil }
            let text = variant.kazakhRules ? applyHehExceptions(beam.text) : beam.text
            // Normalised, so a long word is not judged worse for being long.
            var cost = beam.cost / Double(beam.steps)
            if variant.kazakhRules,
                let depth = morphology?.decomposition(of: text)?.suffixes.count
            {
                // Weighted by how many suffixes it took. A shallow decomposition is stronger
                // evidence, and it suppresses accidental chains: `екенмің` only decomposes as
                // `екен + м + ің`, three morphemes deep, while the correct `екенмін` is
                // `екен + мін`.
                cost -= morphologyBonus / (1.0 + 0.6 * Double(max(0, depth - 1)))
            }
            return Reading(text: text, cost: cost)
        }
        candidates.sort { $0.cost < $1.cost }
        return candidates.first
    }

    /// Rewrites х to һ in the few words that genuinely take it. Cheaper and far more accurate
    /// than hoping a character model picks a letter that appears a dozen times in the language.
    private func applyHehExceptions(_ word: String) -> String {
        guard word.contains("х") || word.contains("һ") else { return word }
        let normalized = word.replacingOccurrences(of: "һ", with: "х")
        for candidate in TranslitTables.hehWords {
            if candidate.replacingOccurrences(of: "һ", with: "х") == normalized {
                return candidate
            }
        }
        return normalized
    }

    private func fallbackDecode(_ word: String) -> String {
        var result = ""
        for unit in split(word, with: Self.kazakhVariant) {
            result.append(unit.options.first ?? Character(unit.source))
        }
        return result
    }

    /// One input letter or digraph, with everything it could stand for.
    private struct Unit {
        var source: String
        var options: [Character]
    }

    /// Splits a Latin word into units, matching digraphs before single letters so `sh` is one
    /// unit rather than an `s` followed by an `h`.
    private func split(_ word: String, with variant: Variant) -> [Unit] {
        let characters = Array(word)
        var units: [Unit] = []
        var index = 0

        while index < characters.count {
            var matched = false
            // Longest digraph first: shch before sh.
            for length in stride(from: 4, through: 2, by: -1) where index + length <= characters.count {
                let candidate = String(characters[index..<(index + length)])
                if let options = variant.digraphs[candidate] {
                    units.append(Unit(source: candidate, options: options))
                    index += length
                    matched = true
                    break
                }
            }
            if matched { continue }

            let character = characters[index]
            let options = variant.options[character] ?? [character]
            units.append(Unit(source: String(character), options: options))
            index += 1
        }
        return units
    }
}
