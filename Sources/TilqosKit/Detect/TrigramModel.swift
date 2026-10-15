import Foundation

/// How well a word fits one language, judged on character trigrams.
public struct TrigramScore: Sendable, Equatable {
    /// Average cost in bits per trigram. Lower fits better.
    public var meanCost: Double
    /// Off-alphabet characters with a letter of this script on *both* sides.
    ///
    /// This, not the raw count, is the signal. A digit wedged between Cyrillic letters
    /// (`с2лем`) means Kazakh was typed on the Russian layout. Digits hanging off the end
    /// of a word (`iPhone15`) mean nothing at all, and charging for them would make every
    /// alphanumeric token look like a mistake.
    public var interiorUnknownCount: Int
    /// Every character that is not a letter of this script.
    public var unknownCount: Int
    public var length: Int

    /// Cost with interior off-alphabet characters charged for. Priced well above any
    /// improbability the trigrams alone could express, because it is a structural fact
    /// rather than a statistical hunch.
    public var penalizedCost: Double {
        meanCost + Double(interiorUnknownCount) * TrigramModel.unknownCharacterCost
    }
}

/// A character trigram language model.
///
/// Trigrams rather than a word list, for three reasons. They separate real words from
/// layout garble by a wide margin — `ghbdtn` is not merely rare English, it is
/// structurally impossible. They handle Kazakh, which is agglutinative enough that a
/// 54,000-stem list would miss most inflected forms. And they are a statistical summary
/// rather than a copy of somebody's dictionary, which keeps the licensing clean.
public struct TrigramModel: Sendable {
    /// "LSTG"
    static let magic: UInt32 = 0x4C53_5447
    static let version: UInt16 = 1
    /// Quantization step: stored value = cost in bits × 12, clamped to a byte.
    public static let costScale: Double = 12.0
    /// Charged once per character that is not a letter of the model's script.
    public static let unknownCharacterCost: Double = 6.0

    public let lang: Lang
    public let alphabet: Alphabet
    /// Flat cube, `(a * symbolCount + b) * symbolCount + c`.
    let costs: [UInt8]

    public init(lang: Lang, alphabet: Alphabet, costs: [UInt8]) {
        precondition(
            costs.count == alphabet.symbolCount * alphabet.symbolCount * alphabet.symbolCount,
            "cost table must be symbolCount cubed")
        self.lang = lang
        self.alphabet = alphabet
        self.costs = costs
    }

    // MARK: - Scoring

    /// Words shorter than this carry too little structure to judge.
    public static let minimumScorableLength = 2

    public func score(_ word: String) -> TrigramScore? {
        let lowered = word.lowercased()
        guard lowered.count >= Self.minimumScorableLength else { return nil }

        let (symbols, unknown) = alphabet.encode(lowered)
        let boundary = alphabet.boundary
        let size = alphabet.symbolCount

        // Two leading boundaries so the very first letter is conditioned on "start of
        // word", and one trailing so word endings are modelled too.
        var padded: [Int] = [boundary, boundary]
        padded.reserveCapacity(symbols.count + 3)
        padded.append(contentsOf: symbols)
        padded.append(boundary)

        var total = 0
        var count = 0
        for index in 2..<padded.count {
            let a = padded[index - 2]
            let b = padded[index - 1]
            let c = padded[index]
            total += Int(costs[(a * size + b) * size + c])
            count += 1
        }
        guard count > 0 else { return nil }

        return TrigramScore(
            meanCost: Double(total) / Double(count) / Self.costScale,
            interiorUnknownCount: interiorUnknownCount(in: lowered),
            unknownCount: unknown,
            length: lowered.count
        )
    }

    // MARK: - Incremental scoring

    /// Symbol index for a character, or nil when it is not a letter of this script.
    public func symbol(for character: Character) -> Int? {
        alphabet.index(of: character)
    }

    public var boundarySymbol: Int { alphabet.boundary }

    /// Cost in bits of `c` appearing after `a, b`.
    ///
    /// Exposed so a decoder can score one letter at a time instead of rebuilding a whole
    /// word — that is what makes beam-search transliteration cheap.
    public func cost(after a: Int, _ b: Int, _ c: Int) -> Double {
        let size = alphabet.symbolCount
        guard a < size, b < size, c < size else { return Self.unknownCharacterCost }
        return Double(costs[(a * size + b) * size + c]) / Self.costScale
    }

    /// Counts off-alphabet characters that have a letter of this script on both sides.
    private func interiorUnknownCount(in lowered: String) -> Int {
        let characters = Array(lowered)
        var count = 0
        for (position, character) in characters.enumerated() {
            guard alphabet.index(of: character) == nil else { continue }
            let letterBefore = position > 0 && alphabet.index(of: characters[position - 1]) != nil
            let letterAfter =
                position + 1 < characters.count
                && alphabet.index(of: characters[position + 1]) != nil
            if letterBefore && letterAfter { count += 1 }
        }
        return count
    }

    // MARK: - Serialization

    /// Header layout, little-endian:
    /// magic(4) version(2) kind(1) reserved(1) lang(4, NUL-padded) symbolCount(2)
    /// reserved(2) tableSize(4) then tableSize cost bytes.
    public static let headerSize = 20

    public func encode() -> Data {
        var data = Data(capacity: Self.headerSize + costs.count)
        data.appendLittleEndian(Self.magic)
        data.appendLittleEndian(Self.version)
        data.append(alphabet.kind.rawValue)
        data.append(0)
        var code = Array(lang.rawValue.utf8)
        while code.count < 4 { code.append(0) }
        data.append(contentsOf: code.prefix(4))
        data.appendLittleEndian(UInt16(alphabet.symbolCount))
        data.appendLittleEndian(UInt16(0))
        data.appendLittleEndian(UInt32(costs.count))
        data.append(contentsOf: costs)
        return data
    }

    public enum DecodeError: Error, CustomStringConvertible {
        case tooShort
        case badMagic
        case unsupportedVersion(UInt16)
        case unknownLanguage(String)
        case sizeMismatch(expected: Int, found: Int)

        public var description: String {
            switch self {
            case .tooShort: return "model file is shorter than its header"
            case .badMagic: return "not a Tilqos trigram model"
            case .unsupportedVersion(let version): return "unsupported model version \(version)"
            case .unknownLanguage(let code): return "unknown language code '\(code)'"
            case .sizeMismatch(let expected, let found):
                return "expected \(expected) cost bytes, found \(found)"
            }
        }
    }

    public static func decode(_ data: Data) throws -> TrigramModel {
        guard data.count >= headerSize else { throw DecodeError.tooShort }
        let bytes = [UInt8](data)

        guard bytes.readLittleEndian(UInt32.self, at: 0) == magic else { throw DecodeError.badMagic }
        let fileVersion = bytes.readLittleEndian(UInt16.self, at: 4)
        guard fileVersion == version else { throw DecodeError.unsupportedVersion(fileVersion) }

        guard let kind = Alphabet.Kind(rawValue: bytes[6]) else { throw DecodeError.badMagic }
        let alphabet = Alphabet.forKind(kind)

        let code = String(decoding: bytes[8..<12].prefix { $0 != 0 }, as: UTF8.self)
        guard let lang = Lang(rawValue: code) else { throw DecodeError.unknownLanguage(code) }

        let tableSize = Int(bytes.readLittleEndian(UInt32.self, at: 16))
        let expected = alphabet.symbolCount * alphabet.symbolCount * alphabet.symbolCount
        guard tableSize == expected, bytes.count == headerSize + tableSize else {
            throw DecodeError.sizeMismatch(expected: expected, found: bytes.count - headerSize)
        }

        return TrigramModel(
            lang: lang, alphabet: alphabet,
            costs: Array(bytes[headerSize..<(headerSize + tableSize)]))
    }
}

// MARK: - Little-endian helpers

extension Data {
    fileprivate mutating func appendLittleEndian(_ value: UInt16) {
        append(UInt8(truncatingIfNeeded: value))
        append(UInt8(truncatingIfNeeded: value >> 8))
    }

    fileprivate mutating func appendLittleEndian(_ value: UInt32) {
        for shift in stride(from: 0, to: 32, by: 8) {
            append(UInt8(truncatingIfNeeded: value >> UInt32(shift)))
        }
    }
}

extension Array where Element == UInt8 {
    fileprivate func readLittleEndian(_ type: UInt16.Type, at offset: Int) -> UInt16 {
        UInt16(self[offset]) | UInt16(self[offset + 1]) << 8
    }

    fileprivate func readLittleEndian(_ type: UInt32.Type, at offset: Int) -> UInt32 {
        var value: UInt32 = 0
        for index in 0..<4 { value |= UInt32(self[offset + index]) << UInt32(index * 8) }
        return value
    }
}
