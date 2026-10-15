import Foundation

/// The keystroke-level decision logic: which key ends a word, what gets evaluated when, and
/// how the buffer keeps describing the screen once a fix is in.
///
/// Free of AppKit, the event tap and the injector on purpose. `Pipeline` performs the side
/// effects this asks for; the regression corpus drives exactly the same object, so what is
/// measured is what ships.
public final class TypingEngine {
    public let buffer: StrokeBuffer
    public let detector: Detector

    /// What to do after a word ends.
    public enum Decision {
        case none
        /// Rewrite it now.
        case apply(Detector.Outcome)
        /// Show it, and let the user accept it.
        case offer(Detector.Outcome)
    }

    /// Settings that shape a decision, gathered by the caller once per evaluation.
    public struct Settings {
        public var candidateLayouts: [KeyMapTable]
        public var resolve: (String) -> KeyMapTable?
        public var minWordLength = 3
        public var exceptions: Set<String> = []
        public var autoThreshold = 0.80
        public var suggestThreshold = 0.45
        /// Automatic rewriting allowed here; otherwise even confident fixes are offered.
        public var mayApply = true
        public var learned: LearnedVocabulary?

        public init(
            candidateLayouts: [KeyMapTable], resolve: @escaping (String) -> KeyMapTable?,
            minWordLength: Int = 3, exceptions: Set<String> = [], autoThreshold: Double = 0.80,
            suggestThreshold: Double = 0.45, mayApply: Bool = true, learned: LearnedVocabulary? = nil
        ) {
            self.candidateLayouts = candidateLayouts
            self.resolve = resolve
            self.minWordLength = minWordLength
            self.exceptions = exceptions
            self.autoThreshold = autoThreshold
            self.suggestThreshold = suggestThreshold
            self.mayApply = mayApply
            self.learned = learned
        }
    }

    /// Where learning signals go. Nil in tests that measure detection alone.
    public var learner: Learner?
    /// Whether a word is in the shipped dictionaries. Only words that are not get learned as the
    /// user's vocabulary — the rest are known already.
    public var isDictionaryWord: ((String, Lang) -> Bool)?
    /// Resolves a recorded stroke's layout, to read back what is on screen.
    public var resolve: ((String) -> KeyMapTable?)?

    /// The clock and timers behind the idle reset. Nil (tests that measure detection alone)
    /// means no timer: the buffer is then only emptied by the next key, as `StrokeBuffer` does.
    public var scheduler: TimerScheduler?
    /// Called after the idle timer emptied the buffer.
    public var onIdleReset: (() -> Void)?
    /// Called whenever the buffer is emptied, whatever the reason, so whoever caches text read
    /// from the field (`Pipeline`) drops it too.
    public var onReset: (() -> Void)?

    public init(detector: Detector, buffer: StrokeBuffer = StrokeBuffer()) {
        self.detector = detector
        self.buffer = buffer
    }

    // MARK: - Idle timer

    private var idleTask: ScheduledTask?
    private var lastKeyAt: TimeInterval = 0

    /// Counts as typing: any key or Delete. Starts the timer if none is running.
    ///
    /// One timer for a whole burst of typing, not one per key: when it fires it measures the
    /// silence itself and, if keys came in since, sleeps for the remainder. The keystroke path
    /// therefore does no more than store a number.
    private func noteActivity() {
        guard let scheduler else { return }
        lastKeyAt = scheduler.now
        if idleTask == nil { scheduleIdleCheck(after: buffer.idleReset) }
    }

    private func scheduleIdleCheck(after delay: TimeInterval) {
        idleTask = scheduler?.schedule(after: delay) { [weak self] in self?.idleCheck() }
    }

    private func idleCheck() {
        idleTask = nil
        guard let scheduler else { return }
        let remaining = buffer.idleReset - (scheduler.now - lastKeyAt)
        if remaining > 0.001 {
            scheduleIdleCheck(after: remaining)
            return
        }
        reset()
        onIdleReset?()
    }

    private func cancelIdleTimer() {
        idleTask?.cancel()
        idleTask = nil
    }

    // MARK: - Keystrokes

    /// A word just ended, and whether it ended a sentence too.
    public struct WordEnd: Equatable, Sendable {
        public var sentenceEnd: Bool
    }

    /// Records a stroke. Returns non-nil when it closed a word and an evaluation should follow.
    ///
    /// A key ends a word only if it is not a letter on *any* candidate layout — see
    /// `Detector.endsWord`. The price is that `word,` on ABC is judged at the following space
    /// rather than at the comma, which nobody can see.
    @discardableResult
    public func record(
        _ stroke: KeyStroke, layout: KeyMapTable, at time: TimeInterval, candidates: [KeyMapTable]
    ) -> WordEnd? {
        lastCandidates = candidates
        noteActivity()
        if buffer.append(stroke, at: time, layoutID: layout.sourceID) {
            // Idle reset: a new session, and nothing before it is on screen any more.
            forgetSession()
        }
        guard Detector.endsWord(stroke, typedOn: layout, candidates: candidates) else { return nil }
        noteWordEnd(at: time)
        let produced = layout.text(for: stroke) ?? ""
        return WordEnd(sentenceEnd: produced.contains(where: Self.isSentenceEnd) || endsSentence(layout: layout))
    }

    /// Mirrors a Delete keypress.
    public func backspace() {
        buffer.removeLast()
        lowWater = min(lowWater, buffer.count)
        if let fix = recentFix, buffer.count < fix.wordEnd { recentFix?.erased = true }
        if !buffer.isEmpty { noteActivity() }
    }

    /// The buffer no longer describes the screen: click, Enter, app switch, secure input,
    /// lock, sleep, or five seconds of silence.
    public func reset() {
        buffer.reset()
        forgetSession()
        cancelIdleTimer()
        onReset?()
    }

    // MARK: - Learning from what the user does next

    /// The last automatic fix, watched for the user taking it back.
    private struct RecentFix {
        var originals: [String]
        /// Where the corrected words end in the buffer; deleting below this erases into them.
        var wordEnd: Int
        var time: TimeInterval
        var erased = false
    }

    /// The last finished word, waiting to see whether it is kept.
    private struct LastWord {
        var text: String
        var lang: Lang
        var end: Int
        /// The keys that produced it, to read it under the other layouts.
        var strokes: [KeyStroke]
    }

    private var recentFix: RecentFix?
    private var lastWord: LastWord?
    /// The layouts the last keystroke was judged against, for reading a kept word under each.
    private var lastCandidates: [KeyMapTable] = []

    /// Shortest word the app will learn as the user's own. Shorter strings are too often a
    /// fragment, an abbreviation or a slip to be worth remembering.
    static let vocabularyMinimumLength = 4
    /// Lowest buffer length since the last word ended: below a word's end means it was edited.
    private var lowWater = Int.max
    /// Words already counted this session, so a word repeated in one sentence counts once.
    private var countedThisSession: Set<String> = []

    /// How long after a fix erasing it and retyping the original still reads as "no".
    static let rejectionWindow: TimeInterval = 20

    private func forgetSession() {
        recentFix = nil
        lastWord = nil
        lowWater = Int.max
        countedThisSession.removeAll()
    }

    private func noteWordEnd(at time: TimeInterval) {
        guard let learner, learner.isEnabled, let current = lastTypedWord() else {
            lastWord = nil
            return
        }

        // A fix erased and the original typed again: the fix was wrong.
        if let fix = recentFix {
            if time - fix.time > Self.rejectionWindow {
                recentFix = nil
            } else if fix.erased {
                if let original = fix.originals.first(where: {
                    $0.lowercased() == current.text.lowercased()
                }) {
                    learner.rejected(typed: original)
                }
                recentFix = nil
            }
        }

        // The word before this one survived: kept. Only words nothing else can explain are
        // worth remembering — the user's names, slang and jargon.
        if let previous = lastWord, lowWater >= previous.end,
            !countedThisSession.contains(previous.text.lowercased()),
            mayLearnAsVocabulary(previous)
        {
            learner.kept(typed: previous.text, lang: previous.lang)
            countedThisSession.insert(previous.text.lowercased())
        }

        lastWord = current
        lowWater = buffer.count
    }

    /// Whether a word the user kept may be learned as their own.
    ///
    /// Only when no layout makes a dictionary word of the same keys. `yfgbcfk` is «написал» on
    /// the Russian layout: a word like that, left standing, is a mistake nobody caught — and
    /// learning it as the user's English would make the mistake permanent, since a known word
    /// is never corrected. The same check covers Russian typed on the Kazakh layout: the
    /// Russian layout renders those keys as the same letters, so `кароче` is looked up in
    /// Russian as well as Kazakh.
    private func mayLearnAsVocabulary(_ word: LastWord) -> Bool {
        guard word.text.count >= Self.vocabularyMinimumLength, let isDictionaryWord else {
            return false
        }
        if isDictionaryWord(word.text, word.lang) { return false }
        for layout in lastCandidates {
            guard let lang = layout.lang else { continue }
            let text = layout.render(word.strokes)
            if !text.isEmpty, isDictionaryWord(text, lang) { return false }
        }
        return true
    }

    /// The word just before the separator that ended it, as its own layout shows it.
    private func lastTypedWord() -> LastWord? {
        guard let resolve else { return nil }
        let recorded = buffer.recorded
        var index = recorded.count - 1
        // Skip the separator run at the end.
        while index >= 0, !isWordStroke(recorded[index], resolve) { index -= 1 }
        let end = index + 1
        var text = ""
        var lang: Lang?
        var strokes: [KeyStroke] = []
        while index >= 0, isWordStroke(recorded[index], resolve) {
            let table = resolve(recorded[index].layoutID)
            text = (table?.text(for: recorded[index].stroke) ?? "") + text
            strokes.insert(recorded[index].stroke, at: 0)
            if lang == nil { lang = table?.lang } else if lang != table?.lang { return nil }
            index -= 1
        }
        guard !text.isEmpty, let lang else { return nil }
        return LastWord(text: text, lang: lang, end: end, strokes: strokes)
    }

    private func isWordStroke(_ entry: RecordedStroke, _ resolve: (String) -> KeyMapTable?) -> Bool {
        Detector.isWordText(resolve(entry.layoutID)?.text(for: entry.stroke))
    }

    private static func isSentenceEnd(_ character: Character) -> Bool {
        ".!?…\n".contains(character)
    }

    /// `word.` then space: the period was typed before the space that closed the word.
    private func endsSentence(layout: KeyMapTable) -> Bool {
        let recorded = buffer.recorded
        guard recorded.count >= 2 else { return false }
        let previous = recorded[recorded.count - 2]
        return layout.text(for: previous.stroke)?.contains(where: Self.isSentenceEnd) ?? false
    }

    // MARK: - Deciding

    /// The last word first; at a sentence end, a wider look for a run that slipped past, which
    /// is only ever offered — silently rewriting several words at once is far too surprising.
    public func decide(
        at end: WordEnd, settings: Settings, precedingText: String? = nil
    ) -> Decision {
        guard !buffer.isEmpty else { return .none }
        let context = makeContext(settings, precedingText: precedingText)
        switch detector.evaluate(context, scope: .lastWord) {
        case .fix(let outcome):
            return settings.mayApply ? .apply(outcome) : .offer(outcome)
        case .suggest(let outcome):
            return .offer(outcome)
        case .none:
            break
        }
        guard end.sentenceEnd, let outcome = detector.evaluate(context, scope: .tail).outcome else {
            return .none
        }
        return .offer(outcome)
    }

    /// ⌥⇧F: the best reading of the last word, whatever the thresholds say.
    public func manualFix(settings: Settings) -> Detector.Outcome? {
        guard !buffer.isEmpty else { return nil }
        var context = makeContext(settings, precedingText: nil)
        context.manual = true
        context.minWordLength = 1
        return detector.evaluate(context, scope: .lastWord).outcome
    }

    public func makeContext(_ settings: Settings, precedingText: String?) -> Detector.Context {
        Detector.Context(
            recorded: buffer.recorded,
            candidateLayouts: settings.candidateLayouts,
            resolve: settings.resolve,
            minWordLength: settings.minWordLength,
            exceptions: settings.exceptions,
            autoThreshold: settings.autoThreshold,
            suggestThreshold: settings.suggestThreshold,
            precedingText: precedingText,
            learned: settings.learned)
    }

    // MARK: - After a fix

    /// Keeps the buffer describing the screen, so the next word is judged against corrected
    /// text rather than against what was there before.
    ///
    /// `automatic` fixes are watched for a while afterwards: erasing one and typing the original
    /// again is the user saying no.
    public func didApply(_ outcome: Detector.Outcome, automatic: Bool = false, at time: TimeInterval = 0) {
        guard let start = outcome.corrections.map(\.range.lowerBound).min(),
            start <= buffer.count
        else { return }
        buffer.replaceTail(
            from: start,
            with: ReplacementPlanner.rewrittenTail(
                recorded: buffer.recorded, corrections: outcome.corrections, from: start))
        // The corrected word is a dictionary word; nothing to learn from keeping it.
        lastWord = nil
        recentFix =
            automatic
            ? RecentFix(
                originals: outcome.changes.map(\.typed),
                wordEnd: outcome.wordRanges.map(\.upperBound).max() ?? buffer.count,
                time: time)
            : nil
    }

    /// The text the buffer says is on screen, for verifying against the text field.
    public func screenText(resolve: (String) -> KeyMapTable?) -> String {
        buffer.recorded.map { resolve($0.layoutID)?.text(for: $0.stroke) ?? "" }.joined()
    }
}
