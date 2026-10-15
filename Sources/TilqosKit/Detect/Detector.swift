import Foundation

/// What the user has taught the app, as the detector sees it. See `Learner`.
public protocol LearnedVocabulary {
    /// Changes whenever anything learned changes, so cached judgements can be told apart.
    var version: Int { get }
    /// Words the user keeps typing and keeping in a language — they are words, whatever the
    /// dictionary says.
    func isKnown(_ word: String, in lang: Lang) -> Bool
    /// A standing decision about converting this exact on-screen word, if the user made one.
    func decision(for typed: String) -> LearnedDecision?
}

public enum LearnedDecision: Equatable, Sendable {
    /// The user accepted this conversion, or asked for it — do it without asking next time.
    case convert(to: Lang)
    /// The user undid it, erased it, or ignored the offer repeatedly — leave the word alone.
    case keep
}

/// Turns a buffer of keystrokes into a decision: leave it, offer a fix, or apply one.
///
/// The decision rests on evidence in this order, strongest first:
///
/// 1. **Is the text on screen a real word?** If it is, it is never rewritten automatically —
///    `еще`, `тот`, `her` stay what they are, whatever a character model thinks of them.
/// 2. **Is the other reading a real word?** Layout garble is not a word in any language
///    (`ghbdtn`), while its conversion is (`привет`). Brands and terms are the opposite: known
///    in English, garbage when converted. That asymmetry is what makes correcting safe.
/// 3. **Frequency, for short words.** Three letters carry almost no statistical signal, so a
///    short conversion must land on one of the language's most frequent words (`ult` → `где`).
/// 4. **Context.** The script of the words around it tilts the decision, never decides it.
/// 5. **Structure.** A digit wedged inside a Cyrillic word (`с2лем`) is Kazakh typed on the
///    Russian layout, whatever the dictionaries say.
/// 6. **The character trigram models**, last — for words no dictionary has, and then only as
///    an offer, never an automatic change.
public final class Detector {
    /// One word the outcome changes, as the user will see it. Feeds learning.
    public struct WordChange: Sendable, Equatable {
        /// The word as it is on screen now.
        public var typed: String
        public var corrected: String
        public var typedLang: Lang
        public var targetLang: Lang
    }

    /// A correction the detector is prepared to stand behind.
    public struct Outcome: Sendable {
        public var replacement: Replacement
        public var corrections: [WordCorrection]
        public var confidence: Double
        /// The affected region as it looks now, and as it would look. For the panel.
        public var before: String
        public var after: String
        /// Language of the last corrected word — what to switch the keyboard to.
        public var targetLang: Lang
        public var targetLayoutID: String
        public var changes: [WordChange]
        /// Stroke ranges of the changed words themselves, without the punctuation re-rendered
        /// after them.
        public var wordRanges: [Range<Int>]
    }

    public enum Action: Sendable {
        case none
        /// Confident enough to offer, not to impose.
        case suggest(Outcome)
        case fix(Outcome)

        public var outcome: Outcome? {
            switch self {
            case .none: return nil
            case .suggest(let outcome), .fix(let outcome): return outcome
            }
        }

        /// The same action with its stroke indices moved, for results computed on a window.
        func shifted(by offset: Int) -> Action {
            func shift(_ outcome: Outcome) -> Outcome {
                var outcome = outcome
                outcome.corrections = outcome.corrections.map {
                    WordCorrection(
                        range: ($0.range.lowerBound + offset)..<($0.range.upperBound + offset),
                        targetLayoutID: $0.targetLayoutID)
                }
                outcome.wordRanges = outcome.wordRanges.map {
                    ($0.lowerBound + offset)..<($0.upperBound + offset)
                }
                return outcome
            }
            switch self {
            case .none: return .none
            case .suggest(let outcome): return .suggest(shift(outcome))
            case .fix(let outcome): return .fix(shift(outcome))
            }
        }
    }

    public struct Context {
        public var recorded: [RecordedStroke]
        /// Layouts the detector may propose, already filtered by the user's language choices.
        public var candidateLayouts: [KeyMapTable]
        public var resolve: (String) -> KeyMapTable?
        public var minWordLength: Int
        public var exceptions: Set<String>
        public var autoThreshold: Double
        public var suggestThreshold: Double
        /// How many trailing words the wide scan considers.
        public var maxWords: Int
        /// Text in front of the buffer, read from the text field, so the first words after a
        /// click still have context. In memory only; never logged.
        public var precedingText: String?
        public var learned: LearnedVocabulary?
        /// The user asked for a fix (⌥⇧F): pick the best reading even if the word on screen is
        /// a real word, and ignore the length floor.
        public var manual: Bool

        public init(
            recorded: [RecordedStroke],
            candidateLayouts: [KeyMapTable],
            resolve: @escaping (String) -> KeyMapTable?,
            minWordLength: Int = 3,
            exceptions: Set<String> = [],
            autoThreshold: Double = 0.80,
            suggestThreshold: Double = 0.45,
            maxWords: Int = 24,
            precedingText: String? = nil,
            learned: LearnedVocabulary? = nil,
            manual: Bool = false
        ) {
            self.recorded = recorded
            self.candidateLayouts = candidateLayouts
            self.resolve = resolve
            self.minWordLength = minWordLength
            self.exceptions = exceptions
            self.autoThreshold = autoThreshold
            self.suggestThreshold = suggestThreshold
            self.maxWords = maxWords
            self.precedingText = precedingText
            self.learned = learned
            self.manual = manual
        }
    }

    public enum Scope: Sendable {
        /// Just the word a separator has closed — the per-keystroke path.
        case lastWord
        /// The trailing run of words, for catching mistakes that already slipped past.
        case tail
    }

    // MARK: - Tuning
    //
    // Every number here was set against the regression corpus (`CorpusTests`), and each one
    // says which way it errs.

    /// Confidence that clears the default automatic threshold (0.80).
    static let certain = 0.95
    static let confident = 0.9
    /// Clears the default suggestion threshold (0.45) but not the default automatic one:
    /// shown, not imposed — unless the user lowers the automatic threshold below it.
    static let offerOnly = 0.6
    /// How much each neighbouring word in a script tilts the trigram gap, in bits. Small on
    /// purpose: context may tip a close call, but a real English word inside Russian text
    /// must stay English.
    static let contextWeight = 0.35
    /// How far a trigram gap may lean against a known conversion before it is refused.
    static let knownAltTolerance = -1.0
    /// Trigram gap that counts as "the typed text is garbage" for three-letter words.
    static let shortGarbageGap = 0.8
    /// Neither reading is a word: only offered, and only when the gap is at least this wide.
    static let unknownOfferGap = 1.8
    /// How many unfixed garbled words before a fixed one may be pulled into the same fix.
    static let backfillLimit = 4

    private let scorer: WordScorer

    public init(models: ModelStore) {
        self.scorer = WordScorer(models: models)
    }

    /// Whether the shipped dictionaries (lexicon, frequency tier, brand list) know the word —
    /// what the user teaches does not count here.
    public func isDictionaryWord(_ word: String, lang: Lang) -> Bool {
        scorer.isKnown(word, lang: lang, learned: nil)
    }

    /// Maps a trigram cost gap in bits onto 0...1.
    ///
    /// Calibrated against the separation measured on the shipped models: real words score
    /// 2.9–3.9 bits per trigram and layout garble 5.6–7.3, so a genuine mistake shows a gap of
    /// roughly 1.7–3.5 while correctly typed text sits at or below zero.
    public static func confidence(gap: Double) -> Double {
        guard gap.isFinite else { return gap > 0 ? 1 : 0 }
        return min(1, max(0, (gap - 0.4) / 1.6))
    }

    /// Whether a keystroke ends a word: it produces no letter on the layout it was typed on,
    /// *nor* on any other candidate layout.
    ///
    /// The second half is what keeps `,skj` whole. On ABC `,` is punctuation, but it is the key
    /// for `б`, and a Russian word typed on the wrong layout contains it mid-word: `было`,
    /// `тебе` (`nt,t`), `люблю` (`k.,k.`), `хорошо` (`[jhjij`). Ending the word there split it
    /// and "corrected" the fragment — `[jhjij` came out as `[орошо`.
    public static func endsWord(_ stroke: KeyStroke, typedOn layout: KeyMapTable, candidates: [KeyMapTable])
        -> Bool
    {
        guard let produced = layout.text(for: stroke), !produced.isEmpty,
            !layout.producesWordCharacter(stroke)
        else { return false }
        return !candidates.contains { $0.producesWordCharacter(stroke) }
    }

    static func isWordText(_ text: String?) -> Bool {
        guard let text, !text.isEmpty else { return false }
        return text.allSatisfy(Tokenizer.isWordCharacter)
    }

    // MARK: - Evaluation

    /// How many trailing strokes the last-word path looks at: the word, the four it may pull in,
    /// and context for them. Bounds the work however full the buffer is.
    static let lastWordWindow = 64

    public func evaluate(_ context: Context, scope: Scope) -> Action {
        guard !context.recorded.isEmpty else { return .none }
        var context = context

        // One layout per language, in a stable order.
        var languages: [Lang] = []
        var layoutFor: [Lang: KeyMapTable] = [:]
        for layout in context.candidateLayouts {
            guard let lang = layout.lang, layoutFor[lang] == nil else { continue }
            languages.append(lang)
            layoutFor[lang] = layout
        }
        guard languages.count > 1 else { return .none }
        let layouts = languages.compactMap { layoutFor[$0] }

        // Every stroke is resolved several times per evaluation; resolve each layout once.
        var cache: [String: KeyMapTable] = [:]
        for layout in context.candidateLayouts { cache[layout.sourceID] = layout }
        for id in Set(context.recorded.map(\.layoutID)) where cache[id] == nil {
            cache[id] = context.resolve(id)
        }
        let original = context.resolve
        context.resolve = { cache[$0] ?? original($0) }

        // The last-word path only needs the end of the buffer. Cut at a separator so no word is
        // split, and shift the result back afterwards.
        var offset = 0
        if scope == .lastWord, context.recorded.count > Self.lastWordWindow {
            var cut = context.recorded.count - Self.lastWordWindow
            while cut < context.recorded.count {
                let entry = context.recorded[cut]
                cut += 1
                if let typed = context.resolve(entry.layoutID),
                    Self.endsWord(entry.stroke, typedOn: typed, candidates: layouts)
                {
                    break
                }
            }
            offset = cut
            context.recorded = Array(context.recorded[cut...])
            guard !context.recorded.isEmpty else { return .none }
        }

        let slots = segment(context, layouts: layouts)
        guard !slots.isEmpty else { return .none }
        let world = World(
            context: context, languages: languages, layoutFor: layoutFor, slots: slots,
            prior: ContextPrior(offset > 0 ? nil : context.precedingText))

        let action: Action
        switch scope {
        case .lastWord: action = evaluateLast(world)
        case .tail: action = evaluateTail(world)
        }
        return offset == 0 ? action : action.shifted(by: offset)
    }

    /// Everything one evaluation works from.
    private struct World {
        let context: Context
        let languages: [Lang]
        let layoutFor: [Lang: KeyMapTable]
        let slots: [Slot]
        let prior: ContextPrior
    }

    /// A word-shaped run of strokes between separators that are separators on every layout.
    struct Slot {
        let range: Range<Int>
        /// The layout every stroke was typed on; nil when the word mixes layouts, in which case
        /// it is pinned as it is.
        let layoutID: String?
        let typedLayout: KeyMapTable?
        let typedLang: Lang?
        /// The longest run of strokes that are letters on the typed layout — the word the typed
        /// layout itself sees, without the punctuation keys around it.
        let mainRun: Range<Int>?
        /// Script of the text on screen, for context. Nil for digits and punctuation.
        let script: Script?
        /// The word as it is on screen, punctuation keys included.
        let screen: String
        let mainRunText: String?
        /// Set when the surroundings put the word off limits (URL, address, code, sigil).
        let guardReason: Guards.SkipReason?
    }

    /// The judgement on one word: the reading it should become and how sure we are.
    struct Judgement {
        var slotIndex: Int
        var typed: Reading
        var target: Reading
        var confidence: Double
    }

    // MARK: - Last word

    private func evaluateLast(_ world: World) -> Action {
        let index = world.slots.count - 1
        let support = { (script: Script) in self.support(for: script, before: index, in: world) }
        let found = judge(index, in: world, support: support)
        Log.debug(.detect, "last word: \(Self.describe(world.slots[index], skip: skipReason(world.slots[index], context: world.context), judgement: found))")
        guard let judgement = found, judgement.confidence > 0 else { return .none }

        var judgements = [judgement]
        // A confident fix vouches for its neighbours: `yt pyf.` is `не знаю`, although `yt`
        // alone was too short to touch.
        if judgement.confidence >= world.context.autoThreshold || world.context.manual {
            judgements.insert(contentsOf: backfill(before: judgement, in: world), at: 0)
        }
        return action(for: judgements, in: world)
    }

    /// One word's judgement in shapes only — languages, flags, costs — for the debug trail.
    private static func describe(_ slot: Slot, skip: Guards.SkipReason?, judgement: Judgement?) -> String {
        func reading(_ reading: Reading) -> String {
            "\(reading.lang.rawValue) len=\(reading.length) known=\(reading.isKnown) core=\(reading.isCore) "
                + "clean=\(reading.isClean) odd=\(reading.hasOddEdges) cost=\(String(format: "%.2f", reading.cost))"
        }
        var text = "typed=\(slot.typedLang?.rawValue ?? "mixed") skip=\(skip.map { "\($0)" } ?? "none")"
        if let judgement {
            text += "; typed[\(reading(judgement.typed))] target[\(reading(judgement.target))] "
                + "confidence=\(String(format: "%.2f", judgement.confidence))"
        } else {
            text += "; no judgement"
        }
        return text
    }

    /// Unfixed words right before a fixed one, typed on the same wrong layout, whose other
    /// reading is a real word. Stops at the first word that does not qualify.
    private func backfill(before anchor: Judgement, in world: World) -> [Judgement] {
        let anchorSlot = world.slots[anchor.slotIndex]
        guard let targetLayout = world.layoutFor[anchor.target.lang] else { return [] }
        var result: [Judgement] = []
        var index = anchor.slotIndex - 1
        while index >= 0, result.count < Self.backfillLimit {
            let slot = world.slots[index]
            let skip = skipReason(slot, context: world.context)
            guard slot.layoutID == anchorSlot.layoutID, skip == nil || skip == .tooShort,
                let typedLayout = slot.typedLayout, let typedLang = slot.typedLang
            else { break }
            let typed = scorer.reading(
                world.context.recorded, slot.range, layout: typedLayout, lang: typedLang,
                learned: world.context.learned, priced: false)
            let target = scorer.reading(
                world.context.recorded, slot.range, layout: targetLayout, lang: anchor.target.lang,
                learned: world.context.learned, priced: false)
            // Next to a sure fix, a rare word on the same layout that reads as a frequent word
            // of the target language is garble too: `yt` (a rare English abbreviation) before
            // `pyf.` is `не` before `знаю`. A frequent typed word is never pulled in.
            let typedIsWord = typed.isKnown && !typed.hasOddEdges
            guard target.isClean, target.text != typed.text, target.isKnown,
                target.length >= 3 || target.isCore,
                !typedIsWord || (target.isCore && !typed.isCore),
                world.context.learned?.decision(for: typed.core) != .keep
            else { break }
            result.insert(
                Judgement(slotIndex: index, typed: typed, target: target, confidence: anchor.confidence),
                at: 0)
            index -= 1
        }
        return result
    }

    // MARK: - Tail

    /// Every recent word, with context from both sides, in two passes: sure fixes first, so the
    /// weaker calls see the corrected neighbours rather than the garble.
    private func evaluateTail(_ world: World) -> Action {
        let first = max(0, world.slots.count - world.context.maxWords)
        var assigned: [Int: Judgement] = [:]

        for pass in 0..<2 {
            let floor = pass == 0 ? world.context.autoThreshold : world.context.suggestThreshold
            for index in first..<world.slots.count where assigned[index] == nil {
                let support = { (script: Script) in
                    self.support(for: script, around: index, in: world, assigned: assigned)
                }
                guard let judgement = judge(index, in: world, support: support),
                    judgement.confidence >= floor
                else { continue }
                assigned[index] = judgement
            }
        }
        let judgements = assigned.keys.sorted().compactMap { assigned[$0] }
        return action(for: judgements, in: world)
    }

    // MARK: - Judging one word

    private func judge(_ index: Int, in world: World, support: (Script) -> Int) -> Judgement? {
        let slot = world.slots[index]
        let skip = skipReason(slot, context: world.context)
        guard skip == nil || (skip == .tooShort && world.context.manual),
            let typedLayout = slot.typedLayout, let typedLang = slot.typedLang
        else { return nil }
        let context = world.context

        // The whole run first, then only the part the typed layout calls a word: `ghbdtn,` is
        // better read as `привет,` than as `приветб`.
        var options = [slot.range]
        if let run = slot.mainRun, run != slot.range { options.append(run) }

        let typedReadings = options.map {
            scorer.reading(
                context.recorded, $0, layout: typedLayout, lang: typedLang, learned: context.learned)
        }
        var best: Judgement?
        for lang in world.languages where lang != typedLang {
            guard let layout = world.layoutFor[lang] else { continue }
            for (range, typed) in zip(options, typedReadings) {
                let target = scorer.reading(
                    context.recorded, range, layout: layout, lang: lang, learned: context.learned)
                let confidence =
                    context.manual
                    ? manualConfidence(typed: typed, target: target)
                    : self.confidence(
                        typed: typed, target: target, minLength: context.minWordLength,
                        learned: context.learned, support: support)
                Log.debug(
                    .detect,
                    "  candidate \(lang.rawValue) via \(layout.sourceID.split(separator: ".").last ?? ""): "
                        + "typed[len=\(typed.length) known=\(typed.isKnown) cost=\(String(format: "%.2f", typed.cost))] "
                        + "target[len=\(target.length) known=\(target.isKnown) clean=\(target.isClean) "
                        + "same=\(target.text == typed.text) cost=\(String(format: "%.2f", target.cost))] "
                        + "confidence=\(String(format: "%.2f", confidence))")
                if confidence > (best?.confidence ?? 0) {
                    best = Judgement(
                        slotIndex: index, typed: typed, target: target, confidence: confidence)
                }
            }
        }
        return best
    }

    /// The evidence ladder from the type's documentation, as numbers.
    func confidence(
        typed: Reading, target: Reading, minLength: Int, learned: LearnedVocabulary?,
        support: (Script) -> Int
    ) -> Double {
        guard target.isClean, target.text != typed.text, !typed.core.isEmpty else { return 0 }

        let targetSupport = support(target.lang.script)
        let typedSupport = support(typed.lang.script)

        // What the user taught us outranks everything we could infer — except that a conversion
        // learned in one language's company is only offered, not imposed, amid the other's:
        // accepting `tot` → `еще` in a Russian chat says nothing about `a tot of whisky`.
        switch learned?.decision(for: typed.core) {
        case .keep?: return 0
        case .convert(let lang)? where lang == target.lang:
            return typedSupport > targetSupport ? Self.offerOnly : 1
        default: break
        }

        // The text on screen is a word as it stands — unless it only looks like one because a
        // letter of the other layout was read as punctuation around it (`to\` is `ещё`).
        let typedIsWord = typed.isKnown && !typed.hasOddEdges

        if typed.isNumeric { return 0 }
        // `әғ` → `25`: a number is unambiguous, and too short for any length floor.
        if target.isNumeric { return typedIsWord ? 0 : Self.certain }
        guard target.length >= minLength else { return 0 }
        let lean = Double(targetSupport - typedSupport) * Self.contextWeight
        let gap = typed.cost - target.cost + lean

        if typedIsWord {
            // Both readings are words — `tot` is English, and `еще` is Russian. Only frequency
            // together with context can argue for a change, and then the user decides. With no
            // context at all there is nothing to go on, so nothing is offered.
            if target.isKnown, target.isCore, !typed.isCore, targetSupport > typedSupport {
                return Self.offerOnly
            }
            return 0
        }

        // Kazakh typed on the Russian layout: its letters come out as digits and a hyphen in the
        // middle of a Cyrillic word (`с2лем`, `к-к`). That is structure, not statistics.
        let cyrillicJunk =
            typed.lang.script == .cyrillic && target.lang.script == .cyrillic
            && typed.interiorJunk > 0 && target.interiorJunk == 0

        if target.isKnown, cyrillicJunk { return Self.certain }

        if target.isKnown {
            switch target.length {
            case 4...:
                if gap > Self.knownAltTolerance { return Self.certain }
                return gap > Self.knownAltTolerance - 1 ? Self.offerOnly : 0
            case 3:
                if target.isCore {
                    return gap >= Self.shortGarbageGap || targetSupport > typedSupport
                        ? Self.confident : Self.offerOnly
                }
                return gap >= Self.shortGarbageGap && targetSupport > typedSupport ? Self.offerOnly : 0
            default:
                return target.isCore && targetSupport > typedSupport ? Self.offerOnly : 0
            }
        }

        // Neither reading is a word, but the structural signal still speaks.
        if cyrillicJunk { return Self.confidence(gap: gap) }
        // Otherwise only the character model speaks, and it may only offer. Punctuation inside
        // the typed text inflates its cost without making the other reading any more of a word,
        // so when the pieces between it are words — `foo.bar` is a file name — nothing is
        // offered. When they are not (`pfib,bcm` is `зашибись`), the gap stands.
        if target.length >= 5, gap >= Self.unknownOfferGap,
            typed.interiorJunk == 0 || !hasKnownPieces(typed)
        {
            return Self.offerOnly
        }
        return 0
    }

    /// Whether the letters between the punctuation of a typed word are words themselves.
    private func hasKnownPieces(_ reading: Reading) -> Bool {
        reading.core.split(whereSeparator: { !$0.isLetter && $0 != "'" })
            .contains { $0.count >= 2 && scorer.isKnown(String($0), lang: reading.lang, learned: nil) }
    }

    /// ⌥⇧F: the user says the word is wrong, so the only question is which reading is best.
    private func manualConfidence(typed: Reading, target: Reading) -> Double {
        guard target.isClean || target.isNumeric, target.text != typed.text else { return 0 }
        // Ranked, not thresholded: a known word beats an unknown one, a frequent one beats a
        // rare one, and the character model breaks the remaining ties.
        var score = 0.5
        if target.isKnown { score += 0.3 }
        if target.isCore { score += 0.1 }
        if target.cost.isFinite { score += max(0, 0.09 - target.cost / 100) }
        return score
    }

    // MARK: - Context

    /// Consecutive words right before `index` written in `script`, then the text in front of
    /// the buffer if the buffer runs out. Capped at three: beyond that more context adds
    /// nothing.
    private func support(for script: Script, before index: Int, in world: World) -> Int {
        var count = 0
        var position = index - 1
        while position >= 0, count < 3 {
            guard let wordScript = world.slots[position].script else {
                position -= 1
                continue
            }
            guard wordScript == script else { return count }
            count += 1
            position -= 1
        }
        guard count < 3 else { return count }
        for wordScript in world.prior.scripts.reversed() {
            guard wordScript == script, count < 3 else { break }
            count += 1
        }
        return count
    }

    /// Words within two positions on either side, as they will read once `assigned` is applied.
    private func support(
        for script: Script, around index: Int, in world: World, assigned: [Int: Judgement]
    ) -> Int {
        var count = 0
        for position in (index - 2)...(index + 2) where position != index {
            guard position >= 0, position < world.slots.count else { continue }
            let wordScript = assigned[position]?.target.lang.script ?? world.slots[position].script
            if wordScript == script { count += 1 }
        }
        if index < 2 {
            count += world.prior.scripts.suffix(2 - index).filter { $0 == script }.count
        }
        return min(count, 3)
    }

    // MARK: - Segmentation

    func segment(_ context: Context, layouts: [KeyMapTable]) -> [Slot] {
        let recorded = context.recorded
        var hard = [Bool](repeating: true, count: recorded.count)
        var typedWord = [Bool](repeating: false, count: recorded.count)
        var typedTexts = [String](repeating: "", count: recorded.count)
        for (index, entry) in recorded.enumerated() {
            guard let typed = context.resolve(entry.layoutID),
                let produced = typed.text(for: entry.stroke), !produced.isEmpty
            else { continue }
            typedTexts[index] = produced
            if typed.producesWordCharacter(entry.stroke) {
                typedWord[index] = true
                hard[index] = false
            } else {
                hard[index] = !layouts.contains { $0.producesWordCharacter(entry.stroke) }
            }
        }

        // Guards speak in the typed layout's own tokens; a slot is off limits if any of its
        // tokens is. Sigils are the exception, handled per slot below: `$PATH` is machine text,
        // but the `\` in `ghbi\k` is the `ё` of `пришёл`, in the middle of a word.
        let protectedRanges = Self.guardedRanges(typedTexts: typedTexts, typedWord: typedWord)
        func typedText(_ position: Int) -> String { typedTexts[position] }

        var slots: [Slot] = []
        var index = 0
        while index < recorded.count {
            guard !hard[index] else {
                index += 1
                continue
            }
            let start = index
            while index < recorded.count, !hard[index] { index += 1 }
            let range = start..<index

            let layoutIDs = Set(recorded[range].map(\.layoutID))
            let layoutID = layoutIDs.count == 1 ? layoutIDs.first : nil
            let typedLayout = layoutID.flatMap(context.resolve)

            // Longest run of typed-layout letters; the later one wins a tie.
            var mainRun: Range<Int>?
            var runStart: Int?
            for position in range.lowerBound...range.upperBound {
                let isWord = position < range.upperBound && typedWord[position]
                if isWord, runStart == nil { runStart = position }
                if !isWord, let begin = runStart {
                    if (mainRun?.count ?? 0) <= position - begin { mainRun = begin..<position }
                    runStart = nil
                }
            }

            let screen = range.map(typedText).joined()

            // A sigil introduces machine text only at the start of a word: right before the
            // slot, or as its first character.
            let before = start > 0 ? typedText(start - 1) : ""
            let startsWithSigil =
                before.last.map(Guards.sigils.contains) ?? false
                || screen.first.map(Guards.sigils.contains) ?? false

            slots.append(
                Slot(
                    range: range, layoutID: layoutID, typedLayout: typedLayout,
                    typedLang: typedLayout?.lang, mainRun: mainRun, script: Self.script(of: screen),
                    screen: screen,
                    mainRunText: mainRun.map { $0.map(typedText).joined() },
                    guardReason: protectedRanges.first(where: { $0.0.overlaps(range) })?.1
                        ?? (startsWithSigil ? .sigil : nil)))
        }
        return slots
    }

    /// Why a word must be left alone, if it must. Word-level checks run only for the words
    /// actually being judged — most of the buffer is only context.
    private func skipReason(_ slot: Slot, context: Context) -> Guards.SkipReason? {
        if let reason = slot.guardReason { return reason }
        let core = WordScorer.core(of: slot.screen)
        guard !core.isEmpty else { return .tooShort }
        if core.allSatisfy(\.isNumber) { return .allDigits }
        if !context.exceptions.isEmpty {
            if context.exceptions.contains(core.lowercased()) { return .exception }
            if let run = slot.mainRunText, context.exceptions.contains(run.lowercased()) {
                return .exception
            }
        }
        // FYI, HTTP, ГОСТ — initialisms are not words the models know.
        let letters = core.filter(\.isLetter)
        if letters.count > 1, letters == letters.uppercased(), letters != letters.lowercased() {
            return .acronym
        }
        return nil
    }

    /// Characters at least one guard pattern needs; see `guardedRanges`.
    private static let guardTriggers: Set<UInt32> = Set("@:./\\_(".unicodeScalars.map(\.value))

    /// Stroke ranges the guards put off limits: addresses, URLs, paths, code.
    ///
    /// Every pattern needs a character plain prose rarely has — `@ : . / \ _ (`, or a capital
    /// after a lowercase letter — so text without one skips the regular expressions entirely.
    static func guardedRanges(typedTexts: [String], typedWord: [Bool])
        -> [(Range<Int>, Guards.SkipReason)]
    {
        var needsGuards = false
        var afterLowercase = false
        scan: for text in typedTexts {
            for scalar in text.unicodeScalars {
                let value = scalar.value
                let isUpper = value >= 0x41 && value <= 0x5A
                let isLower = value >= 0x61 && value <= 0x7A
                let isDigit = value >= 0x30 && value <= 0x39
                if guardTriggers.contains(value) || (isUpper && afterLowercase) {
                    needsGuards = true
                    break scan
                }
                afterLowercase = isLower || (afterLowercase && isDigit)
            }
        }
        guard needsGuards else { return [] }

        // The typed layout's own tokens, built from the texts already rendered.
        var tokens: [Tokenizer.Token] = []
        var index = 0
        while index < typedTexts.count {
            let start = index
            let isWord = typedWord[index]
            var text = ""
            while index < typedTexts.count, typedWord[index] == isWord {
                text += typedTexts[index]
                index += 1
            }
            tokens.append(Tokenizer.Token(isWord: isWord, range: start..<index, displayed: text))
        }
        return Guards.protected(tokens: tokens)
            .filter { $0.value != .sigil }
            .map { (tokens[$0.key].range, $0.value) }
    }

    /// Script of the first letter. Plain range checks: this runs for every word on every
    /// evaluation, and the only scripts that matter here are ASCII Latin and Cyrillic.
    static func script(of text: String) -> Script? {
        for scalar in text.unicodeScalars {
            let value = scalar.value
            if value >= 0x0400 && value <= 0x04FF { return .cyrillic }
            if (value >= 0x41 && value <= 0x5A) || (value >= 0x61 && value <= 0x7A) { return .latin }
        }
        return nil
    }

    // MARK: - Building the outcome

    private func action(for judgements: [Judgement], in world: World) -> Action {
        guard let last = judgements.last else { return .none }
        let context = world.context

        var corrections = judgements.map {
            WordCorrection(range: $0.target.range, targetLayoutID: $0.target.layoutID)
        }
        // Punctuation typed on the same wrong layout right after a corrected word was meant for
        // the target layout too: `ghbdtn^` is `привет,`. Re-rendered only where both sides are
        // punctuation — a letter on either side means it was never meant as a separator — and
        // only up to a word that stays as it is.
        let correctedSlots = Set(judgements.map(\.slotIndex))
        for judgement in judgements {
            let slot = world.slots[judgement.slotIndex]
            let next = judgement.slotIndex + 1
            guard next >= world.slots.count || correctedSlots.contains(next),
                let targetLayout = world.layoutFor[judgement.target.lang]
            else { continue }
            let end = next < world.slots.count ? world.slots[next].range.lowerBound : context.recorded.count
            for position in slot.range.upperBound..<end {
                let entry = context.recorded[position]
                guard entry.layoutID == slot.layoutID,
                    let typed = context.resolve(entry.layoutID)?.text(for: entry.stroke),
                    let target = targetLayout.text(for: entry.stroke),
                    !typed.isEmpty, !target.isEmpty, typed != target,
                    !Self.isWordText(typed), !Self.isWordText(target)
                else { continue }
                corrections.append(
                    WordCorrection(range: position..<(position + 1), targetLayoutID: targetLayout.sourceID))
            }
        }
        corrections.sort { $0.range.lowerBound < $1.range.lowerBound }

        guard
            let replacement = ReplacementPlanner.plan(
                recorded: context.recorded, corrections: corrections, layout: context.resolve)
        else { return .none }

        let confidence = judgements.map(\.confidence).min() ?? 0
        let threshold = context.manual ? 0 : context.suggestThreshold
        guard confidence > 0, confidence >= threshold else { return .none }

        let (before, after) = preview(
            recorded: context.recorded, corrections: corrections, resolve: context.resolve)
        let outcome = Outcome(
            replacement: replacement,
            corrections: corrections,
            confidence: confidence,
            before: before,
            after: after,
            targetLang: last.target.lang,
            targetLayoutID: last.target.layoutID,
            changes: judgements.map {
                WordChange(
                    typed: $0.typed.core, corrected: $0.target.core, typedLang: $0.typed.lang,
                    targetLang: $0.target.lang)
            },
            wordRanges: judgements.map(\.target.range))
        return context.manual || confidence >= context.autoThreshold
            ? .fix(outcome) : .suggest(outcome)
    }

    /// The affected span, before and after, for showing the user what would change.
    private func preview(
        recorded: [RecordedStroke],
        corrections: [WordCorrection],
        resolve: (String) -> KeyMapTable?
    ) -> (before: String, after: String) {
        guard let start = corrections.map(\.range.lowerBound).min(),
            let end = corrections.map(\.range.upperBound).max()
        else { return ("", "") }

        var before = ""
        var after = ""
        for index in start..<min(end, recorded.count) {
            let entry = recorded[index]
            let asTyped = resolve(entry.layoutID)?.text(for: entry.stroke) ?? ""
            before += asTyped
            if let correction = corrections.first(where: { $0.range.contains(index) }),
                let target = resolve(correction.targetLayoutID)
            {
                after += target.text(for: entry.stroke) ?? asTyped
            } else {
                after += asTyped
            }
        }
        return (before.trimmingCharacters(in: .whitespaces), after.trimmingCharacters(in: .whitespaces))
    }
}

/// The scripts of the words just before the buffer, from the text field itself.
///
/// The buffer is thrown away at every click, Enter and pause, so the first word typed after
/// one has no neighbours — exactly when a wrong layout is most likely. The text around the
/// caret says what language the user is writing in. Only the scripts of the last few words
/// are kept; the text itself is dropped as soon as they are known.
struct ContextPrior {
    let scripts: [Script]

    init(_ text: String?) {
        guard let text, !text.isEmpty else {
            scripts = []
            return
        }
        var result: [Script] = []
        for word in text.split(whereSeparator: { !Tokenizer.isWordCharacter($0) }).suffix(3) {
            if let script = Detector.script(of: String(word)) { result.append(script) }
        }
        scripts = result
    }
}
