import Foundation

/// A correction that was applied, with everything needed to take it back.
public struct AppliedFix: Sendable {
    /// What kind of change this was, because undoing them teaches different things: undoing a
    /// layout fix means "never correct this word", undoing a transliteration means "never
    /// convert this word".
    public enum Kind: String, Sendable {
        case layoutFix
        case transliteration
    }

    public var kind: Kind = .layoutFix
    /// Clipboard contents before the change, when the change was made to the clipboard rather
    /// than to a text field.
    public var clipboardBefore: String?
    public var replacement: Replacement
    public var bundleID: String?
    public var time: Date
    /// The word as it looked on screen before the fix. Feeds the "never correct this
    /// word" action and the learning-from-undo path.
    public var originalWord: String
    public var correctedWord: String
    /// Layout active before we switched, so undo restores that too.
    public var previousLayoutID: String?
    public var strategy: Injector.Strategy
    /// Each changed word as it was typed. A fix can span several words (`z nt,z` → `я тебя`),
    /// and undoing it rejects each of them, not the span as one string.
    public var typedWords: [String]

    public init(
        kind: Kind = .layoutFix,
        clipboardBefore: String? = nil,
        replacement: Replacement,
        bundleID: String?,
        time: Date = Date(),
        originalWord: String,
        correctedWord: String,
        previousLayoutID: String?,
        strategy: Injector.Strategy,
        typedWords: [String] = []
    ) {
        self.kind = kind
        self.clipboardBefore = clipboardBefore
        self.replacement = replacement
        self.bundleID = bundleID
        self.time = time
        self.originalWord = originalWord
        self.correctedWord = correctedWord
        self.previousLayoutID = previousLayoutID
        self.strategy = strategy
        self.typedWords = typedWords
    }
}

/// Recent corrections, newest last.
///
/// Bounded on purpose: this is a safety net for "that was wrong, put it back", not an
/// edit history, and it holds text the user typed. So it is also short-lived: an entry is
/// wiped `lifetime` seconds (two minutes) after the correction, by a timer rather than by the
/// next call, and the whole stack is wiped by `clear()` on lock, sleep, quit and when another
/// app comes to the front.
public final class UndoStack {
    /// How long a correction can still be taken back.
    public static let defaultLifetime: TimeInterval = 120

    public let limit: Int
    public let lifetime: TimeInterval
    private var entries: [AppliedFix] = []
    private let scheduler: TimerScheduler?
    private let now: () -> Date
    private var expiryTask: ScheduledTask?

    /// Called after the timer dropped entries, so the menu can stop offering "Undo".
    public var onExpire: (() -> Void)?

    public init(
        limit: Int = 20, lifetime: TimeInterval = UndoStack.defaultLifetime,
        scheduler: TimerScheduler? = nil, now: @escaping () -> Date = Date.init
    ) {
        self.limit = limit
        self.lifetime = lifetime
        self.scheduler = scheduler
        self.now = now
    }

    /// The newest fix that has not expired.
    public var last: AppliedFix? {
        dropExpired()
        return entries.last
    }
    public var isEmpty: Bool { count == 0 }
    public var count: Int {
        dropExpired()
        return entries.count
    }

    public func push(_ fix: AppliedFix) {
        dropExpired()
        entries.append(fix)
        if entries.count > limit {
            entries.removeFirst(entries.count - limit)
        }
        armTimer()
    }

    public func popLast() -> AppliedFix? {
        dropExpired()
        let fix = entries.popLast()
        if entries.isEmpty { cancelTimer() }
        return fix
    }

    /// True when the newest fix is recent enough that undoing it should be read as "you
    /// got that wrong" and turned into a lasting exception.
    public func lastIsFresh(within window: TimeInterval = 8, now: Date = Date()) -> Bool {
        guard let last else { return false }
        return now.timeIntervalSince(last.time) <= window
    }

    /// Forgets everything at once.
    public func clear() {
        entries.removeAll()
        cancelTimer()
    }

    // MARK: - Expiry

    private func dropExpired() {
        let cutoff = now().addingTimeInterval(-lifetime)
        // `<=`: a fix made exactly `lifetime` ago is gone, so "two minutes" is a ceiling.
        entries.removeAll { $0.time <= cutoff }
    }

    /// One timer, aimed at the oldest entry. When it fires everything due goes, and it aims at
    /// the next one.
    private func armTimer() {
        guard let scheduler, expiryTask == nil, let oldest = entries.first else { return }
        let due = oldest.time.addingTimeInterval(lifetime).timeIntervalSince(now())
        expiryTask = scheduler.schedule(after: max(0, due)) { [weak self] in
            guard let self else { return }
            self.expiryTask = nil
            let before = self.entries.count
            self.dropExpired()
            self.armTimer()
            if self.entries.count != before { self.onExpire?() }
        }
    }

    private func cancelTimer() {
        expiryTask?.cancel()
        expiryTask = nil
    }
}
