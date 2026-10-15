import Foundation

/// A run of keystrokes read under one layout, with everything the detector weighs.
public struct Reading: Sendable {
    public var lang: Lang
    public var layoutID: String
    /// Strokes this reading covers.
    public var range: Range<Int>
    /// Everything the strokes produce under this layout, punctuation included.
    public var text: String
    /// `text` without leading and trailing punctuation — the word itself.
    public var core: String
    /// Characters in `core` — `i'm` is three long, like `его`.
    public var length: Int
    /// Punctuation glued to the word that text in this script does not have: a leading comma,
    /// a trailing backslash or bracket. `to\` is not `to` followed by punctuation; it is `ещё`
    /// typed on ABC, where `ё` sits on the backslash key.
    public var hasOddEdges: Bool
    /// The core is a single word: nothing but letters, digits, and at most inner hyphens or
    /// apostrophes. A reading with a comma in the middle (`nt,t`) is not something a person
    /// types in that language.
    public var isClean: Bool
    public var isNumeric: Bool
    /// A real word of the language: in the lexicon, derived from it, or taught by the user.
    public var isKnown: Bool
    /// Among the language's most frequent words.
    public var isCore: Bool
    /// Trigram cost in bits per symbol, penalties included. `.infinity` when unscorable.
    public var cost: Double
    /// Off-alphabet characters wedged between letters — `с2лем`, `nt,t`.
    public var interiorJunk: Int
}

/// Renders a keystroke run under every candidate layout and prices each reading.
public struct WordScorer {
    /// Cost assigned to an all-digit reading.
    ///
    /// Digits are not letters of any script, so no trigram model can say anything about
    /// `25`. Without this, the reverse Kazakh mistake — typing `25` while the Kazakh
    /// layout is active and getting `әғ` — could never be detected, because the correct
    /// reading would score as unknown garbage while the wrong one scored as real letters.
    /// The value sits below any real word's cost, since a bare number is unambiguous.
    public static let numericCost: Double = 2.0

    /// Subtracted from a Kazakh candidate that is a well-formed inflected word.
    ///
    /// Sharpens the Russian-versus-Kazakh decision in particular: `сәлеметсіз` decomposing while
    /// `с2леметс3з` cannot is much harder evidence than the trigram difference between them.
    /// Smaller than the transliterator's bonus, because here it competes with a confidence
    /// threshold that was calibrated on trigram costs alone.
    public static let morphologyBonus: Double = 0.6

    /// The only one-letter words each language has. A single letter is a word only if it is
    /// one of these — whatever else a dictionary lists (letter names, abbreviations).
    static let singleLetterWords: [Lang: Set<String>] = [
        .en: ["a", "i"],
        .ru: ["я", "а", "и", "в", "с", "к", "у", "о"],
        .kk: ["а", "е", "о"],
    ]

    private let models: ModelStore

    public init(models: ModelStore) {
        self.models = models
    }

    // The data is read from the store on every use, never copied at init. The app builds its
    // detector at launch while the models are still loading in the background, and a copy taken
    // then stayed empty for good: every word was "unknown", no fix was ever more than an offer,
    // and Kazakh morphology never applied at all. `ModelStore` answers in well under a
    // microsecond, so reading it each time costs nothing that matters.

    private var morphology: Morphology? { models.kazakhMorphology() }

    private func lexicon(for lang: Lang) -> Lexicon? { models.lexicon(for: lang) }

    public static func isNumeric(_ text: String) -> Bool {
        !text.isEmpty && text.allSatisfy(\.isNumber)
    }

    // MARK: - Readings

    /// Reads `range` of the recorded strokes under `layout`.
    ///
    /// `priced: false` skips the trigram cost (left at `.infinity`) for callers that decide on
    /// the lexicon alone — pulling neighbours into a fix does, and it runs on every one of them.
    public func reading(
        _ recorded: [RecordedStroke], _ range: Range<Int>, layout: KeyMapTable, lang: Lang,
        learned: LearnedVocabulary?, priced: Bool = true
    ) -> Reading {
        var text = ""
        for index in range {
            if let produced = layout.text(for: recorded[index].stroke) { text += produced }
        }
        let analysis = analyze(text, lang: lang, learned: learned, priced: priced)
        return Reading(
            lang: lang, layoutID: layout.sourceID, range: range, text: text, core: analysis.core,
            length: analysis.length, hasOddEdges: analysis.hasOddEdges, isClean: analysis.isClean,
            isNumeric: analysis.isNumeric, isKnown: analysis.isKnown, isCore: analysis.isCore,
            cost: analysis.cost, interiorJunk: analysis.interiorJunk)
    }

    /// Everything about a reading that depends on its text alone.
    struct Analysis {
        var core: String
        var length: Int
        var hasOddEdges: Bool
        var isClean: Bool
        var isNumeric: Bool
        var isKnown: Bool
        var isCore: Bool
        var cost: Double
        var interiorJunk: Int
        var priced: Bool
    }

    /// Memoized: every word end re-reads the words before it, so the same few texts come back
    /// again and again. Bounded, and keyed by the learned vocabulary's version so a word the
    /// user teaches takes effect at once, and by the models' generation so nothing judged before
    /// the dictionaries loaded survives them.
    private func analyze(_ text: String, lang: Lang, learned: LearnedVocabulary?, priced: Bool)
        -> Analysis
    {
        let key = MemoKey(
            lang: lang, version: learned?.version ?? -1, generation: models.generation, text: text)
        if let cached = memo.value(for: key), cached.priced || !priced { return cached }

        let core = Self.core(of: text)
        let numeric = Self.isNumeric(core)
        let (known, frequent) =
            numeric ? (true, false) : lookup(core, lang: lang, learned: learned)
        let cost =
            numeric
            ? Self.numericCost : priced ? trigramCost(core, lang: lang, known: known).cost : .infinity
        let analysis = Analysis(
            core: core, length: core.count, hasOddEdges: Self.hasOddEdges(text),
            isClean: Self.isClean(core), isNumeric: numeric, isKnown: known, isCore: frequent,
            cost: cost, interiorJunk: Self.interiorJunk(core), priced: priced || numeric)
        memo.store(analysis, for: key)
        return analysis
    }

    private let memo = Memo()

    struct MemoKey: Hashable {
        var lang: Lang
        var version: Int
        /// The models' load generation: a judgement made before the dictionaries arrived must
        /// not outlive their arrival.
        var generation: Int
        var text: String
    }

    /// A small shared cache. The detector runs on the main thread, but a lock keeps it honest
    /// wherever else it is used.
    final class Memo: @unchecked Sendable {
        private var entries: [MemoKey: Analysis] = [:]
        private let lock = NSLock()
        static let capacity = 2048

        func value(for key: MemoKey) -> Analysis? {
            lock.lock()
            defer { lock.unlock() }
            return entries[key]
        }

        func store(_ analysis: Analysis, for key: MemoKey) {
            lock.lock()
            defer { lock.unlock() }
            if entries.count >= Self.capacity { entries.removeAll(keepingCapacity: true) }
            entries[key] = analysis
        }
    }

    /// Known and frequent in one pass over the lexicon.
    private func lookup(_ word: String, lang: Lang, learned: LearnedVocabulary?) -> (Bool, Bool) {
        guard !word.isEmpty else { return (false, false) }
        if word.count == 1 {
            let isWord = Self.singleLetterWords[lang]?.contains(word.lowercased()) ?? false
            return (isWord, isWord)
        }
        if let lexicon = lexicon(for: lang) {
            let (known, frequent) = lexicon.lookup(word)
            if known { return (true, frequent) }
        }
        return (isKnown(word, lang: lang, learned: learned), false)
    }

    /// Characters inside a word that no word of the language has there: a digit or a symbol
    /// between two letters, or a hyphen next to a lone letter (`к-к` is `көк` typed on the
    /// Russian layout, where `ө` sits on the hyphen key). A hyphen between real parts, as in
    /// `кто-то`, and an apostrophe, as in `don't`, are not junk.
    static func interiorJunk(_ core: String) -> Int {
        let characters = Array(core)
        guard characters.count > 2 else { return 0 }
        var junk = 0
        for index in 1..<(characters.count - 1) {
            let character = characters[index]
            guard !character.isLetter, character != "'", character != "\u{2019}" else { continue }
            guard characters[index - 1].isLetter, characters[index + 1].isLetter else { continue }
            if character == "-" {
                let left = characters[..<index].reversed().prefix(while: \.isLetter).count
                let right = characters[(index + 1)...].prefix(while: \.isLetter).count
                if left >= 2 && right >= 2 { continue }
            }
            junk += 1
        }
        return junk
    }

    /// The word with surrounding punctuation removed.
    static func core(of text: String) -> String {
        guard let first = text.firstIndex(where: Tokenizer.isWordCharacter),
            let last = text.lastIndex(where: Tokenizer.isWordCharacter)
        else { return "" }
        return String(text[first...last])
    }

    /// Punctuation that ordinary prose puts right before or right after a word.
    static let normalLeading: Set<Character> = ["(", "[", "«", "\"", "'", "“", "‘", "„", "¿", "¡"]
    static let normalTrailing: Set<Character> = [
        ",", ".", "!", "?", ";", ":", ")", "]", "»", "\"", "'", "”", "’", "…",
    ]

    static func hasOddEdges(_ text: String) -> Bool {
        guard let first = text.firstIndex(where: Tokenizer.isWordCharacter),
            let last = text.lastIndex(where: Tokenizer.isWordCharacter)
        else { return false }
        return !text[..<first].allSatisfy(normalLeading.contains)
            || !text[text.index(after: last)...].allSatisfy(normalTrailing.contains)
    }

    static func isClean(_ core: String) -> Bool {
        guard !core.isEmpty else { return false }
        return core.allSatisfy { Tokenizer.isWordCharacter($0) || $0 == "-" }
            && !core.contains("--")
    }

    /// Whether the word exists. A hyphenated word counts when the dictionary has it whole
    /// (`x-ray`), or when every part is a word of two letters or more (`кто-то`) — `к-к` is not a
    /// word because `к` is.
    public func isKnown(_ word: String, lang: Lang, learned: LearnedVocabulary?) -> Bool {
        guard !word.isEmpty else { return false }
        if isKnownWhole(word, lang: lang, learned: learned) { return true }
        let parts = word.split(separator: "-", omittingEmptySubsequences: false)
        guard parts.count > 1, !parts.contains(where: { $0.count < 2 }) else { return false }
        return parts.allSatisfy { isKnownWhole(String($0), lang: lang, learned: learned) }
    }

    private func isKnownWhole(_ word: String, lang: Lang, learned: LearnedVocabulary?) -> Bool {
        if word.count == 1 {
            return Self.singleLetterWords[lang]?.contains(word.lowercased()) ?? false
        }
        if let lexicon = lexicon(for: lang), lexicon.contains(word) || lexicon.isCore(word) {
            return true
        }
        if lang == .en, TranslitTables.latinOnlyWords.contains(word.lowercased()) { return true }
        return learned?.isKnown(word, in: lang) ?? false
    }

    public func isCore(_ word: String, lang: Lang) -> Bool {
        if word.count == 1 {
            return Self.singleLetterWords[lang]?.contains(word.lowercased()) ?? false
        }
        return lexicon(for: lang)?.isCore(word) ?? false
    }

    public var hasLexicons: Bool { Lang.allCases.contains { lexicon(for: $0) != nil } }

    /// Mean trigram cost, hyphenated parts scored separately so the hyphen itself is not
    /// charged as junk. Returns the count of interior junk characters alongside.
    ///
    /// A Kazakh word the lexicon already knows gets the morphology bonus without the search:
    /// a known form is a stem or a stem plus a suffix, which is what the search would find.
    func trigramCost(_ word: String, lang: Lang, known: Bool = false) -> (cost: Double, junk: Int) {
        guard let model = models.model(for: lang) else { return (.infinity, 0) }
        var total = 0.0
        var weight = 0.0
        var junk = 0
        for part in word.split(separator: "-") {
            guard let score = model.score(String(part)) else { continue }
            let length = Double(score.length)
            total += score.penalizedCost * length
            weight += length
            junk += score.interiorUnknownCount
        }
        guard weight > 0 else { return (.infinity, junk) }
        var cost = total / weight
        if lang == .kk, known || morphology?.decomposes(word) == true {
            cost -= Self.morphologyBonus
        }
        return (cost, junk)
    }
}
