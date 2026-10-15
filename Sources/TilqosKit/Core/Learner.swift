import Foundation

/// What the app learns from the user, and the only place it is kept.
///
/// Four signals, each read the way a person would read it:
///
/// - **Rejection.** An automatic fix undone with ⌥⇧Z, or erased and the original typed again —
///   that word is never converted again. (Undo also adds the word to the visible exceptions list,
///   as before; this covers the erase-and-retype case the user never asked to record.)
/// - **Ignored offers.** A suggestion dismissed without accepting it, twice for the same word —
///   stop offering it.
/// - **Acceptance.** Tab on a suggestion, or ⌥⇧F on a word — convert it without asking next time.
/// - **Vocabulary.** A word no dictionary has, typed and kept in two separate sessions — it is a
///   word of that language (names, slang, jargon). That both protects it and, typed on the wrong
///   layout, makes it recognisable.
///
/// Privacy: words are stored only as salted 64-bit FNV-1a hashes, never as text, and only words
/// that took part in one of those signals. The salt is random per installation and is stored in
/// the same preferences file as the hashes. FNV-1a is a fast non-cryptographic hash, so this is
/// obfuscation and not protection: someone who can read the file can hash a list of candidate
/// words with the salt and see which of them are present (short, common words are easily
/// found). That is why the store is small (3,000 entries at most), local, switchable off, and
/// can be wiped with one call (`resetLearnedVocabulary()`). Nothing here ever leaves the Mac or
/// reaches the log.
public final class Learner: LearnedVocabulary {
    public static let shared = Learner()

    /// Posted on the main queue whenever learned data changes, for the settings window.
    public static let didChange = Notification.Name("TilqosLearningDidChange")

    /// UserDefaults keys. Kept out of `Prefs` on purpose: nothing here is a setting the user
    /// types in, and the store saves itself on its own schedule.
    enum Key {
        static let enabled = "learningEnabled"
        static let salt = "learningSalt"
        static let entries = "learnedWords"
        static let schema = "learningSchema"
    }

    /// Version of the rules stored entries were learned under. Entries from older, looser
    /// rules are dropped when loaded — see `migrate()`.
    static let schemaVersion = 2

    /// Ignored offers before a word stops being offered.
    public static let ignoreLimit = 2
    /// Sessions a dictionary-less word must be kept in before it counts as a word.
    public static let vocabularySightings = 2
    /// Entries kept at most. Past this the least recently used go first.
    public static let capacity = 3000

    /// What is known about one word, as the user typed it.
    public enum State: Equatable, Sendable {
        /// Never convert it.
        case keep
        /// Convert it straight to this language.
        case convert(Lang)
        /// Offers ignored so far.
        case ignored(Int)
        /// Seen kept, not yet enough times.
        case sighted(Lang, Int)
        /// A word of this language.
        case known(Lang)
    }

    struct Entry: Equatable {
        var state: State
        /// Days since the reference date — precise enough for eviction, too coarse to be a
        /// record of when anything was typed.
        var lastUsed: Int
    }

    private let defaults: UserDefaults
    private var entries: [UInt64: Entry] = [:]
    private var salt: [UInt8] = []
    private var saveScheduled = false
    private let now: () -> Date

    public private(set) var version = 0

    public init(defaults: UserDefaults = .standard, now: @escaping () -> Date = Date.init) {
        self.defaults = defaults
        self.now = now
        defaults.register(defaults: [Key.enabled: true])
        load()
    }

    /// Off: nothing new is learned and nothing learned is used.
    public var isEnabled: Bool {
        get { defaults.bool(forKey: Key.enabled) }
        set {
            defaults.set(newValue, forKey: Key.enabled)
            changed()
        }
    }

    // MARK: - What the detector asks

    public func isKnown(_ word: String, in lang: Lang) -> Bool {
        guard isEnabled, case .known(let known)? = entries[hash(word)]?.state else { return false }
        return known == lang
    }

    public func decision(for typed: String) -> LearnedDecision? {
        guard isEnabled, let state = entries[hash(typed)]?.state else { return nil }
        switch state {
        case .keep, .known: return .keep
        case .convert(let lang): return .convert(to: lang)
        case .ignored, .sighted: return nil
        }
    }

    // MARK: - Signals

    /// A fix was undone or erased and retyped: leave this word alone from now on.
    public func rejected(typed: String) {
        update(typed) { _ in .keep }
    }

    /// The user accepted a conversion or asked for it. Explicit, so it outranks anything
    /// learned passively.
    public func confirmed(typed: String, to lang: Lang) {
        update(typed) { _ in .convert(lang) }
    }

    /// An offer went by without being taken.
    public func ignored(typed: String) {
        update(typed) { state in
            switch state {
            case .ignored(let count)?:
                return count + 1 >= Self.ignoreLimit ? .keep : .ignored(count + 1)
            case nil, .sighted?:
                return Self.ignoreLimit <= 1 ? .keep : .ignored(1)
            case let other?:
                return other
            }
        }
    }

    /// A word the dictionaries do not have was typed and left in place.
    public func kept(typed: String, lang: Lang) {
        guard typed.count >= 3, typed.allSatisfy({ $0.isLetter || $0 == "-" || $0 == "'" }) else {
            return
        }
        update(typed) { state in
            switch state {
            case .sighted(let seenLang, let count)? where seenLang == lang:
                return count + 1 >= Self.vocabularySightings ? .known(lang) : .sighted(lang, count + 1)
            case nil, .sighted?:
                return Self.vocabularySightings <= 1 ? .known(lang) : .sighted(lang, 1)
            case let other?:
                return other
            }
        }
    }

    private func update(_ typed: String, _ transition: (State?) -> State) {
        guard isEnabled, !typed.isEmpty else { return }
        let key = hash(typed)
        let next = transition(entries[key]?.state)
        let entry = Entry(state: next, lastUsed: today)
        guard entries[key] != entry else { return }
        entries[key] = entry
        evictIfNeeded()
        changed()
    }

    // MARK: - For the settings window

    public enum Kind: String, CaseIterable, Sendable {
        /// Never converted.
        case neverConvert
        /// Converted without asking.
        case alwaysConvert
        /// The user's own words.
        case userWord
        /// Seen, not yet decided.
        case pending
    }

    /// One learned word. The word itself is not stored, so it cannot be shown — only what was
    /// learned about it.
    public struct Item: Identifiable, Equatable, Sendable {
        public let id: String
        public let kind: Kind
        public let lang: Lang?
        public let lastUsed: Date
    }

    public struct Counts: Equatable, Sendable {
        public var neverConvert = 0
        public var alwaysConvert = 0
        public var userWords = 0
        public var pending = 0
        public var total: Int { neverConvert + alwaysConvert + userWords + pending }
    }

    public var counts: Counts {
        var counts = Counts()
        for entry in entries.values {
            switch Self.kind(of: entry.state) {
            case .neverConvert: counts.neverConvert += 1
            case .alwaysConvert: counts.alwaysConvert += 1
            case .userWord: counts.userWords += 1
            case .pending: counts.pending += 1
            }
        }
        return counts
    }

    /// Newest first.
    public func items() -> [Item] {
        entries.map { key, entry in
            Item(
                id: String(format: "%016llx", key), kind: Self.kind(of: entry.state),
                lang: Self.lang(of: entry.state),
                lastUsed: Date(timeIntervalSinceReferenceDate: Double(entry.lastUsed) * 86_400))
        }
        .sorted { $0.lastUsed != $1.lastUsed ? $0.lastUsed > $1.lastUsed : $0.id < $1.id }
    }

    public func forget(id: String) {
        guard let key = UInt64(id, radix: 16), entries.removeValue(forKey: key) != nil else { return }
        changed()
    }

    /// Forgets whatever was learned about a word the user types in, as typed or as converted.
    @discardableResult
    public func forget(word: String) -> Bool {
        guard entries.removeValue(forKey: hash(word)) != nil else { return false }
        changed()
        return true
    }

    /// Deletes everything learned: every entry, and the salt, which is replaced so that no hash
    /// computed before the wipe matches anything learned after it. Written to disk at once, not
    /// two seconds from now.
    public func resetLearnedVocabulary() {
        entries.removeAll()
        salt = Self.makeSalt()
        defaults.set(Data(salt), forKey: Key.salt)
        changed()
        flush()
    }

    /// The older name of `resetLearnedVocabulary()`.
    public func forgetAll() {
        resetLearnedVocabulary()
    }

    static func kind(of state: State) -> Kind {
        switch state {
        case .keep: return .neverConvert
        case .convert: return .alwaysConvert
        case .known: return .userWord
        case .ignored, .sighted: return .pending
        }
    }

    static func lang(of state: State) -> Lang? {
        switch state {
        case .convert(let lang), .known(let lang), .sighted(let lang, _): return lang
        case .keep, .ignored: return nil
        }
    }

    // MARK: - Hashing

    /// Salted FNV-1a over the normalized word: the same word is the same entry whatever its
    /// case or `ё`, and no entry is stored as text. (Guessable all the same; see the type's
    /// documentation.)
    func hash(_ word: String) -> UInt64 {
        var value: UInt64 = 0xCBF2_9CE4_8422_2325
        for byte in salt {
            value = (value ^ UInt64(byte)) &* 0x0000_0100_0000_01B3
        }
        for scalar in Lexicon.normalize(word) {
            for byte in UTF8.encode(scalar)! {
                value = (value ^ UInt64(byte)) &* 0x0000_0100_0000_01B3
            }
        }
        return value
    }

    private static func makeSalt() -> [UInt8] {
        var generator = SystemRandomNumberGenerator()
        return (0..<8).map { _ in UInt8.random(in: .min ... .max, using: &generator) }
    }

    private var today: Int {
        Int(now().timeIntervalSinceReferenceDate / 86_400)
    }

    private func evictIfNeeded() {
        guard entries.count > Self.capacity else { return }
        let excess = entries.count - Self.capacity
        for key in entries.sorted(by: { $0.value.lastUsed < $1.value.lastUsed }).prefix(excess)
            .map(\.key)
        {
            entries.removeValue(forKey: key)
        }
    }

    // MARK: - Persistence

    /// Stored as one dictionary: hex hash → `[kind, language, count, day]`. Integers only.
    private func load() {
        if let data = defaults.data(forKey: Key.salt), data.count == 8 {
            salt = [UInt8](data)
        } else {
            salt = Self.makeSalt()
            defaults.set(Data(salt), forKey: Key.salt)
        }
        if let stored = defaults.dictionary(forKey: Key.entries) as? [String: [Int]] {
            for (hex, fields) in stored {
                guard let key = UInt64(hex, radix: 16), fields.count == 4,
                    let state = Self.decode(kind: fields[0], lang: fields[1], count: fields[2])
                else { continue }
                entries[key] = Entry(state: state, lastUsed: fields[3])
            }
        }
        if defaults.integer(forKey: Key.schema) < Self.schemaVersion { migrate() }
    }

    /// Drops what the first version learned from signals that meant nothing.
    ///
    /// It learned "never convert" from offers the user merely typed past, and learned garbled
    /// words as the user's own vocabulary whenever a fix had not happened — and in the app the
    /// dictionaries that should have told a word from a slip were not even loaded, so ordinary
    /// Russian typed on the Kazakh layout was learned wholesale. None of that is evidence. What
    /// the user accepted explicitly — Tab on an offer, ⌥⇧F — is, and it stays.
    private func migrate() {
        let before = entries.count
        entries = entries.filter {
            if case .convert = $0.value.state { return true }
            return false
        }
        defaults.set(Self.schemaVersion, forKey: Key.schema)
        flush()
        if before > 0 {
            Log.info(.detect, "learning: kept \(entries.count) of \(before) entries under the new rules")
        }
    }

    /// Writes now. Normally saving is coalesced; tests and shutdown call this directly.
    public func flush() {
        saveScheduled = false
        var stored: [String: [Int]] = [:]
        for (key, entry) in entries {
            let (kind, lang, count) = Self.encode(entry.state)
            stored[String(format: "%016llx", key)] = [kind, lang, count, entry.lastUsed]
        }
        defaults.set(stored, forKey: Key.entries)
    }

    private func changed() {
        version += 1
        if !saveScheduled {
            saveScheduled = true
            // Coalesced: a burst of typing is one write, not one per word.
            DispatchQueue.main.asyncAfter(deadline: .now() + 2) { [weak self] in
                guard let self, self.saveScheduled else { return }
                self.flush()
            }
        }
        NotificationCenter.default.post(name: Self.didChange, object: self)
    }

    private static func encode(_ state: State) -> (Int, Int, Int) {
        func index(_ lang: Lang) -> Int { Lang.allCases.firstIndex(of: lang) ?? -1 }
        switch state {
        case .keep: return (0, -1, 0)
        case .convert(let lang): return (1, index(lang), 0)
        case .ignored(let count): return (2, -1, count)
        case .sighted(let lang, let count): return (3, index(lang), count)
        case .known(let lang): return (4, index(lang), 0)
        }
    }

    private static func decode(kind: Int, lang: Int, count: Int) -> State? {
        let language = Lang.allCases.indices.contains(lang) ? Lang.allCases[lang] : nil
        switch kind {
        case 0: return .keep
        case 1: return language.map(State.convert)
        case 2: return .ignored(count)
        case 3: return language.map { .sighted($0, count) }
        case 4: return language.map(State.known)
        default: return nil
        }
    }
}
