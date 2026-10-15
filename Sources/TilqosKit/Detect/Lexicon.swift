import Foundation

/// Which words exist in a language, every inflected form included.
///
/// The detector's worst failure is rewriting a word that was typed correctly — `еще` became
/// `tot` because a character model trained on dictionary headwords had never seen `щ` between
/// two `е`, while `tot` looked like ordinary English. No amount of trigram tuning fixes that: a
/// three-letter word carries almost no statistical signal. What fixes it is knowing that `еще`
/// is a word and `ult` is not.
///
/// Stored the way hunspell stores it — stems with affix flags, plus the affix rules — rather
/// than as expanded forms. Russian alone expands to 1.4 million forms (5.7 MB even as 32-bit
/// hashes); as stems it is 146,269 entries at 6 bytes each. Lookup reverses the rules: strip a
/// candidate ending, restore what the rule removed, check the condition, and ask whether that
/// stem exists and carries the rule's flag. That makes the check exact with respect to the
/// grammar in the affix file instead of merely generous.
///
/// Stems are 32-bit FNV-1a hashes, so the word list cannot be read back out of the file. A
/// collision can only make an unknown string look known, which costs a missed correction —
/// the safe direction.
public struct Lexicon: Sendable {
    /// "LSLX"
    static let magic: UInt32 = 0x4C53_4C58
    static let version: UInt16 = 1
    static let headerSize = 36

    public let lang: Lang
    /// Affix-flag bitmasks, interned: stems overwhelmingly share a handful of flag sets.
    let flagSets: [UInt64]
    /// Sorted, for binary search.
    let stemHashes: [UInt32]
    /// Parallel to `stemHashes`: index into `flagSets`.
    let stemFlagSets: [UInt16]
    let suffixes: [AffixRule]
    let prefixes: [AffixRule]
    /// The most frequent words of the language, hashed and sorted.
    ///
    /// Frequency is the one thing a dictionary cannot say, and short words need it: `ult` → `где`
    /// is worth correcting because `где` is among the hundred commonest Russian words, while
    /// `afk` → `фал` is not, although `фал` is in the dictionary too.
    let core: [UInt32]

    private let suffixIndex: [UInt32: [Int]]
    private let prefixIndex: [UInt32: [Int]]
    private let maxSuffixLength: Int
    private let maxPrefixLength: Int

    init(
        lang: Lang, flagSets: [UInt64], stemHashes: [UInt32], stemFlagSets: [UInt16],
        suffixes: [AffixRule], prefixes: [AffixRule], core: [UInt32]
    ) {
        self.lang = lang
        self.flagSets = flagSets
        self.stemHashes = stemHashes
        self.stemFlagSets = stemFlagSets
        self.suffixes = suffixes
        self.prefixes = prefixes
        self.core = core

        var suffixIndex: [UInt32: [Int]] = [:]
        for (index, rule) in suffixes.enumerated() {
            suffixIndex[Self.hash(rule.affix[...]), default: []].append(index)
        }
        var prefixIndex: [UInt32: [Int]] = [:]
        for (index, rule) in prefixes.enumerated() {
            prefixIndex[Self.hash(rule.affix[...]), default: []].append(index)
        }
        self.suffixIndex = suffixIndex
        self.prefixIndex = prefixIndex
        self.maxSuffixLength = suffixes.map(\.affix.count).max() ?? 0
        self.maxPrefixLength = prefixes.map(\.affix.count).max() ?? 0
    }

    public var stemCount: Int { stemHashes.count }
    public var ruleCount: Int { suffixes.count + prefixes.count }
    public var coreCount: Int { core.count }

    // MARK: - Normalization and hashing

    /// Lowercase, `ё` folded into `е`, typographic apostrophe folded into ASCII.
    ///
    /// People type `еще` for `ещё` far more often than not, and the dictionary spells it both
    /// ways inconsistently, so the two are one word here. Applied identically at build and at
    /// lookup time, which is all that matters for a hash.
    public static func normalize(_ word: String) -> [Unicode.Scalar] {
        var result: [Unicode.Scalar] = []
        result.reserveCapacity(word.unicodeScalars.count)
        for scalar in word.lowercased().unicodeScalars {
            switch scalar {
            case "ё": result.append("е")
            case "\u{2019}": result.append("'")
            default: result.append(scalar)
            }
        }
        return result
    }

    /// FNV-1a over the UTF-8 encoding. Portable on purpose: any other implementation (the
    /// Windows port) gets identical hashes from `[]byte(s)`.
    static func hash<S: Sequence>(_ scalars: S) -> UInt32 where S.Element == Unicode.Scalar {
        var value = offsetBasis
        for scalar in scalars { value = step(value, scalar.value) }
        return value
    }

    static let offsetBasis: UInt32 = 0x811C_9DC5

    @inline(__always)
    static func step(_ value: UInt32, _ scalar: Unicode.Scalar) -> UInt32 {
        step(value, scalar.value)
    }

    /// Feeds one scalar's UTF-8 bytes into a running FNV-1a hash, so a hash can be extended
    /// without building the string it describes.
    @inline(__always)
    static func step(_ value: UInt32, _ code: UInt32) -> UInt32 {
        var value = value
        @inline(__always) func mix(_ byte: UInt32) { value = (value ^ byte) &* 0x0100_0193 }
        switch code {
        case 0..<0x80:
            mix(code)
        case 0x80..<0x800:
            mix(0xC0 | code >> 6)
            mix(0x80 | code & 0x3F)
        case 0x800..<0x10000:
            mix(0xE0 | code >> 12)
            mix(0x80 | code >> 6 & 0x3F)
            mix(0x80 | code & 0x3F)
        default:
            mix(0xF0 | code >> 18)
            mix(0x80 | code >> 12 & 0x3F)
            mix(0x80 | code >> 6 & 0x3F)
            mix(0x80 | code & 0x3F)
        }
        return value
    }

    // MARK: - Lookup

    /// True when the word is a dictionary form, or one the affix rules derive from one.
    public func contains(_ word: String) -> Bool {
        lookup(word).known
    }

    /// True for the language's most frequent words.
    public func isCore(_ word: String) -> Bool {
        Self.binarySearch(core, Self.hash(Self.normalize(word))) != nil
    }

    /// Both answers from one normalization: whether the word exists, and whether it is among
    /// the most frequent. The detector always wants both.
    public func lookup(_ word: String) -> (known: Bool, core: Bool) {
        let normalized = Self.normalize(word)
        guard !normalized.isEmpty else { return (false, false) }
        let wordHash = Self.hash(normalized)
        let isCore = Self.binarySearch(core, wordHash) != nil
        if isCore || flags(ofHash: wordHash) != nil { return (true, isCore) }
        return (contains(normalized: normalized), false)
    }

    func contains(normalized word: [Unicode.Scalar]) -> Bool {
        guard !word.isEmpty else { return false }
        if flags(ofHash: Self.hash(word)) != nil { return true }
        if matchesSuffix(word.map(\.value), crossWith: nil) { return true }
        return matchesPrefix(word)
    }

    private func flags(ofHash stemHash: UInt32) -> UInt64? {
        guard let index = Self.binarySearch(stemHashes, stemHash) else { return nil }
        return flagSets[Int(stemFlagSets[index])]
    }

    /// Reverses one suffix rule. With `crossWith`, only cross-product rules count and the stem
    /// must also carry the prefix flag — the word had a prefix stripped already.
    ///
    /// The candidate stem is never built: its hash is the running hash of the word's first part
    /// continued over the rule's strip string, and the condition is checked across the same
    /// virtual concatenation. This is the hot loop — Russian has 1,581 rules — so it works on
    /// raw scalar values with no generic code in the way.
    private func matchesSuffix(_ word: [UInt32], crossWith prefixFlag: UInt8?) -> Bool {
        let count = word.count
        guard count > 0 else { return false }
        var prefixHashes = [UInt32](repeating: Self.offsetBasis, count: count + 1)
        for index in 0..<count { prefixHashes[index + 1] = Self.step(prefixHashes[index], word[index]) }

        for length in 0...min(maxSuffixLength, count) {
            let base = count - length
            var endingHash = Self.offsetBasis
            for index in base..<count { endingHash = Self.step(endingHash, word[index]) }
            guard let candidates = suffixIndex[endingHash] else { continue }
            candidateLoop: for index in candidates {
                let rule = suffixes[index]
                if prefixFlag != nil && !rule.cross { continue }
                let affix = rule.affixValues
                guard affix.count == length else { continue }
                for offset in 0..<length where affix[offset] != word[base + offset] {
                    continue candidateLoop
                }
                let strip = rule.stripValues
                guard base + strip.count > 0, rule.matchesEnd(word, base: base, strip: strip)
                else { continue }
                var stemHash = prefixHashes[base]
                for value in strip { stemHash = Self.step(stemHash, value) }
                guard let mask = flags(ofHash: stemHash), mask & (1 << UInt64(rule.flag)) != 0
                else { continue }
                if let prefixFlag, mask & (1 << UInt64(prefixFlag)) == 0 { continue }
                return true
            }
        }
        return false
    }

    /// Prefix rules — English only, and far fewer of them, so this path builds stems plainly.
    private func matchesPrefix(_ word: [Unicode.Scalar]) -> Bool {
        guard maxPrefixLength > 0 else { return false }
        for length in 1...min(maxPrefixLength, word.count) {
            let start = word[0..<length]
            guard let candidates = prefixIndex[Self.hash(start)] else { continue }
            for index in candidates {
                let rule = prefixes[index]
                guard rule.affix.elementsEqual(start) else { continue }
                var stem = rule.strip
                stem.append(contentsOf: word[length...])
                guard !stem.isEmpty, rule.matchesStart(of: stem) else { continue }
                if let mask = flags(ofHash: Self.hash(stem)), mask & (1 << UInt64(rule.flag)) != 0 {
                    return true
                }
                if rule.cross, matchesSuffix(stem.map(\.value), crossWith: rule.flag) { return true }
            }
        }
        return false
    }

    static func binarySearch(_ sorted: [UInt32], _ target: UInt32) -> Int? {
        var low = 0
        var high = sorted.count - 1
        while low <= high {
            let mid = (low + high) >> 1
            let value = sorted[mid]
            if value == target { return mid }
            if value < target { low = mid + 1 } else { high = mid - 1 }
        }
        return nil
    }
}

// MARK: - Affix rules

/// One hunspell `SFX`/`PFX` line: remove `strip` from the stem, add `affix`, provided the stem
/// matches `condition` (at its end for suffixes, its start for prefixes).
struct AffixRule: Sendable, Equatable {
    /// Bit index of the flag in the stems' masks.
    var flag: UInt8
    /// Whether the rule combines with rules of the other kind (hunspell's cross product).
    var cross: Bool
    var strip: [Unicode.Scalar]
    var affix: [Unicode.Scalar]
    /// The condition as written, e.g. `[^aeiou]y`. Kept for serialization.
    var condition: String
    let pattern: [CharacterClass]
    /// `strip` and `affix` as raw scalar values, for the lookup loop.
    let stripValues: [UInt32]
    let affixValues: [UInt32]

    init(flag: UInt8, cross: Bool, strip: [Unicode.Scalar], affix: [Unicode.Scalar], condition: String) {
        self.flag = flag
        self.cross = cross
        self.strip = strip
        self.affix = affix
        self.condition = condition
        self.pattern = CharacterClass.compile(condition)
        self.stripValues = strip.map(\.value)
        self.affixValues = affix.map(\.value)
    }

    static func == (lhs: AffixRule, rhs: AffixRule) -> Bool {
        lhs.flag == rhs.flag && lhs.cross == rhs.cross && lhs.strip == rhs.strip
            && lhs.affix == rhs.affix && lhs.condition == rhs.condition
    }

    /// Checks the condition against the end of `word[0..<base] + strip` without concatenating
    /// them.
    func matchesEnd(_ word: [UInt32], base: Int, strip: [UInt32]) -> Bool {
        let total = base + strip.count
        guard pattern.count <= total else { return false }
        let offset = total - pattern.count
        for index in 0..<pattern.count {
            let position = offset + index
            let value = position < base ? word[position] : strip[position - base]
            if !pattern[index].matches(value) { return false }
        }
        return true
    }

    func matchesStart(of stem: [Unicode.Scalar]) -> Bool {
        guard pattern.count <= stem.count else { return pattern.isEmpty }
        for index in 0..<pattern.count where !pattern[index].matches(stem[index].value) {
            return false
        }
        return true
    }
}

/// One position of a hunspell condition: `.`, a letter, `[abc]` or `[^abc]`.
enum CharacterClass: Sendable, Equatable {
    case any
    case one(UInt32)
    case oneOf([UInt32])
    case noneOf([UInt32])

    /// Condition sets are a handful of letters, so a linear scan beats hashing.
    func matches(_ value: UInt32) -> Bool {
        switch self {
        case .any: return true
        case .one(let expected): return value == expected
        case .oneOf(let set): return set.contains(value)
        case .noneOf(let set): return !set.contains(value)
        }
    }

    /// `.` alone means "no condition" and compiles to nothing.
    static func compile(_ condition: String) -> [CharacterClass] {
        guard condition != "." else { return [] }
        let scalars = Lexicon.normalize(condition).map(\.value)
        let open = UInt32(UInt8(ascii: "[")), close = UInt32(UInt8(ascii: "]"))
        let caret = UInt32(UInt8(ascii: "^")), dot = UInt32(UInt8(ascii: "."))
        var result: [CharacterClass] = []
        var index = 0
        while index < scalars.count {
            let scalar = scalars[index]
            if scalar == open {
                var negated = false
                var members: [UInt32] = []
                index += 1
                if index < scalars.count, scalars[index] == caret {
                    negated = true
                    index += 1
                }
                while index < scalars.count, scalars[index] != close {
                    members.append(scalars[index])
                    index += 1
                }
                result.append(negated ? .noneOf(members) : .oneOf(members))
            } else if scalar == dot {
                result.append(.any)
            } else {
                result.append(.one(scalar))
            }
            index += 1
        }
        return result
    }
}

// MARK: - Building from hunspell sources

extension Lexicon {
    /// Builds a lexicon from a hunspell dictionary and affix file.
    ///
    /// `extra` are whole words added as bare stems — brands, terms and colloquial forms the
    /// dictionary lacks. `core` is the frequency tier (see `core`).
    public static func build(
        lang: Lang, dictionary: String, affixes: String, extra: [String] = [],
        core: [String] = []
    ) -> Lexicon {
        var flagBits: [Character: UInt8] = [:]
        func bit(for flag: Character) -> UInt8 {
            if let existing = flagBits[flag] { return existing }
            let next = UInt8(flagBits.count)
            precondition(next < 64, "more than 64 affix flags")
            flagBits[flag] = next
            return next
        }

        var suffixes: [AffixRule] = []
        var prefixes: [AffixRule] = []
        var compoundOnly: Character?

        let affixLines = Self.lines(affixes)
        var lineIndex = 0
        while lineIndex < affixLines.count {
            let fields = affixLines[lineIndex].split(whereSeparator: \.isWhitespace).map(String.init)
            lineIndex += 1
            if fields.count >= 2, fields[0] == "ONLYINCOMPOUND" { compoundOnly = fields[1].first }
            guard fields.count >= 4, fields[0] == "SFX" || fields[0] == "PFX",
                let ruleCount = Int(fields[3]), let flag = fields[1].first
            else { continue }
            let isSuffix = fields[0] == "SFX"
            let cross = fields[2] == "Y"
            let flagBit = bit(for: flag)
            for _ in 0..<ruleCount where lineIndex < affixLines.count {
                let rule = affixLines[lineIndex].split(whereSeparator: \.isWhitespace).map(String.init)
                lineIndex += 1
                guard rule.count >= 4 else { continue }
                let strip = rule[2] == "0" ? "" : rule[2]
                var affix = String(rule[3].split(separator: "/", maxSplits: 1).first ?? "")
                if affix == "0" { affix = "" }
                let condition = rule.count >= 5 ? rule[4] : "."
                let parsed = AffixRule(
                    flag: flagBit, cross: cross, strip: normalize(strip), affix: normalize(affix),
                    condition: condition)
                if isSuffix { suffixes.append(parsed) } else { prefixes.append(parsed) }
            }
        }

        var masks: [UInt32: UInt64] = [:]
        let dictionaryLines = Self.lines(dictionary)
        for (position, line) in dictionaryLines.enumerated() {
            // The first line of a .dic file is the entry count.
            if position == 0, Int(line.trimmingCharacters(in: .whitespaces)) != nil { continue }
            let entry = line.split(whereSeparator: { $0 == "\t" || $0 == " " }).first.map(String.init) ?? ""
            guard !entry.isEmpty else { continue }
            let parts = entry.split(separator: "/", maxSplits: 1, omittingEmptySubsequences: false)
            let word = String(parts[0])
            // Ordinal fragments like `1th` exist only to build compounds; digits are never a
            // word the detector needs to know.
            guard !word.isEmpty, !word.contains(where: \.isNumber) else { continue }
            let flags = parts.count > 1 ? String(parts[1]) : ""
            if let compoundOnly, flags.contains(compoundOnly) { continue }
            var mask: UInt64 = 0
            // Flags with no rules behind them change nothing, so they are not given a bit.
            for flag in flags {
                if let known = flagBits[flag] { mask |= 1 << UInt64(known) }
            }
            masks[hash(normalize(word)), default: 0] |= mask
        }
        for word in extra {
            let trimmed = word.trimmingCharacters(in: .whitespaces)
            guard !trimmed.isEmpty, !trimmed.hasPrefix("#") else { continue }
            masks[hash(normalize(trimmed)), default: 0] |= 0
        }

        // Sorted, so the file is byte-for-byte reproducible: dictionary order in Swift changes
        // from one run to the next.
        let flagSets = Set(masks.values).sorted()
        precondition(flagSets.count <= Int(UInt16.max), "too many distinct flag sets")
        var flagSetIndex: [UInt64: UInt16] = [:]
        for (index, mask) in flagSets.enumerated() { flagSetIndex[mask] = UInt16(index) }
        let stems = masks.map { ($0.key, flagSetIndex[$0.value]!) }.sorted { $0.0 < $1.0 }

        let coreHashes = Array(
            Set(
                core.map { $0.trimmingCharacters(in: .whitespaces) }
                    .filter { !$0.isEmpty && !$0.hasPrefix("#") }
                    .map { hash(normalize($0)) })
        ).sorted()

        return Lexicon(
            lang: lang, flagSets: flagSets, stemHashes: stems.map(\.0),
            stemFlagSets: stems.map(\.1), suffixes: suffixes, prefixes: prefixes, core: coreHashes)
    }

    /// Splits on any newline and drops a UTF-8 BOM. A CRLF pair is a single `Character` in
    /// Swift, so splitting on "\n" alone silently matches nothing in the Kazakh files.
    static func lines(_ text: String) -> [String] {
        text.split(whereSeparator: \.isNewline).map {
            $0.hasPrefix("\u{FEFF}") ? String($0.dropFirst()) : String($0)
        }
    }
}

// MARK: - Serialization

extension Lexicon {
    /// Layout, little-endian:
    /// magic(4) version(2) reserved(2) lang(4, NUL-padded) stemCount(4) flagSetCount(4)
    /// suffixCount(4) prefixCount(4) coreCount(4) reserved(4)
    /// then flagSets (u64 each), stemHashes (u32 each), stemFlagSets (u16 each), suffix rules,
    /// prefix rules, core hashes (u32 each). A rule is flag(1) cross(1) and three
    /// length-prefixed UTF-8 strings: strip, affix, condition.
    public func encode() -> Data {
        var data = Data()
        data.appendLE32(Self.magic)
        data.appendLE16(Self.version)
        data.appendLE16(0)
        var code = Array(lang.rawValue.utf8)
        while code.count < 4 { code.append(0) }
        data.append(contentsOf: code.prefix(4))
        data.appendLE32(UInt32(stemHashes.count))
        data.appendLE32(UInt32(flagSets.count))
        data.appendLE32(UInt32(suffixes.count))
        data.appendLE32(UInt32(prefixes.count))
        data.appendLE32(UInt32(core.count))
        data.appendLE32(0)
        for mask in flagSets {
            data.appendLE32(UInt32(truncatingIfNeeded: mask))
            data.appendLE32(UInt32(truncatingIfNeeded: mask >> 32))
        }
        for stemHash in stemHashes { data.appendLE32(stemHash) }
        for index in stemFlagSets { data.appendLE16(index) }
        for rule in suffixes + prefixes {
            data.append(rule.flag)
            data.append(rule.cross ? 1 : 0)
            for text in [String(String.UnicodeScalarView(rule.strip)),
                String(String.UnicodeScalarView(rule.affix)), rule.condition]
            {
                let bytes = Array(text.utf8)
                data.append(UInt8(bytes.count))
                data.append(contentsOf: bytes)
            }
        }
        for coreHash in core { data.appendLE32(coreHash) }
        return data
    }

    public enum DecodeError: Error, CustomStringConvertible {
        case tooShort
        case badMagic
        case unsupportedVersion(UInt16)
        case unknownLanguage(String)
        case truncated

        public var description: String {
            switch self {
            case .tooShort: return "lexicon file is shorter than its header"
            case .badMagic: return "not a Tilqos lexicon"
            case .unsupportedVersion(let version): return "unsupported lexicon version \(version)"
            case .unknownLanguage(let code): return "unknown language code '\(code)'"
            case .truncated: return "lexicon file ends mid-record"
            }
        }
    }

    public static func decode(_ data: Data) throws -> Lexicon {
        let bytes = [UInt8](data)
        guard bytes.count >= headerSize else { throw DecodeError.tooShort }
        var reader = ByteReader(bytes: bytes)
        guard try reader.u32() == magic else { throw DecodeError.badMagic }
        let fileVersion = try reader.u16()
        guard fileVersion == version else { throw DecodeError.unsupportedVersion(fileVersion) }
        _ = try reader.u16()
        let code = String(decoding: try reader.bytes(4).prefix { $0 != 0 }, as: UTF8.self)
        guard let lang = Lang(rawValue: code) else { throw DecodeError.unknownLanguage(code) }
        let stemCount = Int(try reader.u32())
        let flagSetCount = Int(try reader.u32())
        let suffixCount = Int(try reader.u32())
        let prefixCount = Int(try reader.u32())
        let coreCount = Int(try reader.u32())
        _ = try reader.u32()

        var flagSets: [UInt64] = []
        flagSets.reserveCapacity(flagSetCount)
        for _ in 0..<flagSetCount {
            let low = UInt64(try reader.u32())
            let high = UInt64(try reader.u32())
            flagSets.append(low | high << 32)
        }
        var stemHashes: [UInt32] = []
        stemHashes.reserveCapacity(stemCount)
        for _ in 0..<stemCount { stemHashes.append(try reader.u32()) }
        var stemFlagSets: [UInt16] = []
        stemFlagSets.reserveCapacity(stemCount)
        for _ in 0..<stemCount {
            let index = try reader.u16()
            guard Int(index) < flagSetCount else { throw DecodeError.truncated }
            stemFlagSets.append(index)
        }

        func rule() throws -> AffixRule {
            let flag = try reader.u8()
            let cross = try reader.u8() != 0
            let strip = try reader.string()
            let affix = try reader.string()
            let condition = try reader.string()
            return AffixRule(
                flag: flag, cross: cross, strip: Array(strip.unicodeScalars),
                affix: Array(affix.unicodeScalars), condition: condition)
        }
        var suffixes: [AffixRule] = []
        for _ in 0..<suffixCount { suffixes.append(try rule()) }
        var prefixes: [AffixRule] = []
        for _ in 0..<prefixCount { prefixes.append(try rule()) }
        var core: [UInt32] = []
        for _ in 0..<coreCount { core.append(try reader.u32()) }

        return Lexicon(
            lang: lang, flagSets: flagSets, stemHashes: stemHashes, stemFlagSets: stemFlagSets,
            suffixes: suffixes, prefixes: prefixes, core: core)
    }
}

/// Bounds-checked little-endian reads.
struct ByteReader {
    let bytes: [UInt8]
    var offset = 0

    mutating func u8() throws -> UInt8 {
        guard offset < bytes.count else { throw Lexicon.DecodeError.truncated }
        defer { offset += 1 }
        return bytes[offset]
    }

    mutating func u16() throws -> UInt16 {
        guard offset + 2 <= bytes.count else { throw Lexicon.DecodeError.truncated }
        defer { offset += 2 }
        return UInt16(bytes[offset]) | UInt16(bytes[offset + 1]) << 8
    }

    mutating func u32() throws -> UInt32 {
        guard offset + 4 <= bytes.count else { throw Lexicon.DecodeError.truncated }
        var value: UInt32 = 0
        for index in 0..<4 { value |= UInt32(bytes[offset + index]) << UInt32(index * 8) }
        offset += 4
        return value
    }

    mutating func bytes(_ count: Int) throws -> ArraySlice<UInt8> {
        guard offset + count <= bytes.count else { throw Lexicon.DecodeError.truncated }
        defer { offset += count }
        return bytes[offset..<(offset + count)]
    }

    mutating func string() throws -> String {
        let length = Int(try u8())
        return String(decoding: try bytes(length), as: UTF8.self)
    }
}

extension Data {
    mutating func appendLE16(_ value: UInt16) {
        append(UInt8(truncatingIfNeeded: value))
        append(UInt8(truncatingIfNeeded: value >> 8))
    }

    mutating func appendLE32(_ value: UInt32) {
        for shift in stride(from: 0, to: 32, by: 8) {
            append(UInt8(truncatingIfNeeded: value >> UInt32(shift)))
        }
    }
}
