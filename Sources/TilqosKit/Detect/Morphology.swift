import Foundation

/// Checks whether a Kazakh word is a well-formed inflected form.
///
/// Kazakh is agglutinative: `десеңіздер` is `десе + ңіз + дер`, three morphemes deep. A
/// character model cannot express that, which is why it happily accepted `десениздер` — a
/// string that decomposes into nothing at all. Stripping suffixes against the real rules
/// settles those cases outright.
///
/// Used as evidence, never as a filter. Coverage is good but not complete — `сәлеметсіз` does
/// not decompose because its stem is missing from the word list — so a word that fails to
/// decompose is merely unsupported, not wrong.
public struct Morphology: Sendable {
    /// "LSMO"
    static let magic: UInt32 = 0x4C53_4D4F
    static let version: UInt16 = 1
    static let headerSize = 20

    /// Hashes of the base word list, sorted for binary search.
    ///
    /// Hashes rather than the words themselves: 54,000 stems cost 216 KB this way and the lookup
    /// is a binary search. (Not a secrecy measure: the dictionary is public, and a 32-bit hash can
    /// be tested against any candidate list.) A hash collision can only make the check slightly
    /// too generous, which is the safe direction for a bonus.
    let stems: [UInt32]
    /// Suffix → the letters it puts back on the stem (usually none).
    let suffixes: [String: [String]]
    let maxSuffixLength: Int

    /// How many suffixes may be peeled off. Three covers the forms that actually occur;
    /// `десе + ңіз + дер` is already at the limit.
    public static let defaultDepth = 3

    public init(stems: [UInt32], suffixes: [String: [String]]) {
        self.stems = stems.sorted()
        self.suffixes = suffixes
        self.maxSuffixLength = suffixes.keys.map(\.count).max() ?? 0
    }

    public var stemCount: Int { stems.count }
    public var suffixCount: Int { suffixes.count }

    // MARK: - Lookup

    /// FNV-1a over the lowercased UTF-8 bytes.
    public static func hash(_ word: String) -> UInt32 {
        var value: UInt32 = 0x811C_9DC5
        for byte in word.lowercased().utf8 {
            value ^= UInt32(byte)
            value = value &* 0x0100_0193
        }
        return value
    }

    public func isStem(_ word: String) -> Bool {
        let target = Self.hash(word)
        var low = 0
        var high = stems.count - 1
        while low <= high {
            let mid = (low + high) / 2
            if stems[mid] == target { return true }
            if stems[mid] < target { low = mid + 1 } else { high = mid - 1 }
        }
        return false
    }

    /// True when the word is a stem, or a stem plus up to `depth` suffixes.
    public func decomposes(_ word: String, depth: Int = Morphology.defaultDepth) -> Bool {
        decompose(Array(word.lowercased()), depth: depth) != nil
    }

    /// The stem and the suffixes peeled off it, outermost last. Nil when nothing fits.
    public func decomposition(of word: String, depth: Int = Morphology.defaultDepth)
        -> (stem: String, suffixes: [String])?
    {
        decompose(Array(word.lowercased()), depth: depth)
    }

    private func decompose(_ characters: [Character], depth: Int)
        -> (stem: String, suffixes: [String])?
    {
        let word = String(characters)
        if isStem(word) { return (word, []) }
        guard depth > 0, characters.count > 1 else { return nil }

        // Longest suffix first: a longer match is a more specific rule, and it cuts the search.
        let longest = min(maxSuffixLength, characters.count - 1)
        for length in stride(from: longest, through: 1, by: -1) {
            let tail = String(characters.suffix(length))
            guard let strippings = suffixes[tail] else { continue }
            for stripping in strippings {
                let base = String(characters.dropLast(length)) + stripping
                guard !base.isEmpty else { continue }
                if let found = decompose(Array(base), depth: depth - 1) {
                    return (found.stem, found.suffixes + [tail])
                }
            }
        }
        return nil
    }

    // MARK: - Serialization

    public func encode() -> Data {
        var data = Data()
        data.appendLE(Self.magic)
        data.appendLE(Self.version)
        data.appendLE(UInt16(0))
        data.appendLE(UInt32(stems.count))
        data.appendLE(UInt32(suffixes.count))
        data.appendLE(UInt16(maxSuffixLength))
        data.appendLE(UInt16(0))
        for stem in stems { data.appendLE(stem) }

        for (affix, strippings) in suffixes.sorted(by: { $0.key < $1.key }) {
            let affixBytes = Array(affix.utf8)
            data.append(UInt8(affixBytes.count))
            data.append(contentsOf: affixBytes)
            data.append(UInt8(strippings.count))
            for stripping in strippings {
                let bytes = Array(stripping.utf8)
                data.append(UInt8(bytes.count))
                data.append(contentsOf: bytes)
            }
        }
        return data
    }

    public enum DecodeError: Error, CustomStringConvertible {
        case tooShort
        case badMagic
        case unsupportedVersion(UInt16)
        case truncated

        public var description: String {
            switch self {
            case .tooShort: return "morphology file is shorter than its header"
            case .badMagic: return "not a Tilqos morphology file"
            case .unsupportedVersion(let version): return "unsupported morphology version \(version)"
            case .truncated: return "morphology file ends mid-record"
            }
        }
    }

    public static func decode(_ data: Data) throws -> Morphology {
        let bytes = [UInt8](data)
        guard bytes.count >= headerSize else { throw DecodeError.tooShort }
        guard bytes.readLE(UInt32.self, at: 0) == magic else { throw DecodeError.badMagic }
        let fileVersion = bytes.readLE(UInt16.self, at: 4)
        guard fileVersion == version else { throw DecodeError.unsupportedVersion(fileVersion) }

        let stemCount = Int(bytes.readLE(UInt32.self, at: 8))
        let suffixCount = Int(bytes.readLE(UInt32.self, at: 12))

        var offset = headerSize
        guard bytes.count >= offset + stemCount * 4 else { throw DecodeError.truncated }
        var stems = [UInt32]()
        stems.reserveCapacity(stemCount)
        for _ in 0..<stemCount {
            stems.append(bytes.readLE(UInt32.self, at: offset))
            offset += 4
        }

        var suffixes: [String: [String]] = [:]
        suffixes.reserveCapacity(suffixCount)
        for _ in 0..<suffixCount {
            guard offset < bytes.count else { throw DecodeError.truncated }
            let affixLength = Int(bytes[offset])
            offset += 1
            guard offset + affixLength <= bytes.count else { throw DecodeError.truncated }
            let affix = String(decoding: bytes[offset..<(offset + affixLength)], as: UTF8.self)
            offset += affixLength

            guard offset < bytes.count else { throw DecodeError.truncated }
            let strippingCount = Int(bytes[offset])
            offset += 1
            var strippings: [String] = []
            for _ in 0..<strippingCount {
                guard offset < bytes.count else { throw DecodeError.truncated }
                let length = Int(bytes[offset])
                offset += 1
                guard offset + length <= bytes.count else { throw DecodeError.truncated }
                strippings.append(
                    String(decoding: bytes[offset..<(offset + length)], as: UTF8.self))
                offset += length
            }
            suffixes[affix] = strippings
        }

        return Morphology(stems: stems, suffixes: suffixes)
    }
}

// MARK: - Little-endian helpers

extension Data {
    fileprivate mutating func appendLE(_ value: UInt16) {
        append(UInt8(truncatingIfNeeded: value))
        append(UInt8(truncatingIfNeeded: value >> 8))
    }

    fileprivate mutating func appendLE(_ value: UInt32) {
        for shift in stride(from: 0, to: 32, by: 8) {
            append(UInt8(truncatingIfNeeded: value >> UInt32(shift)))
        }
    }
}

extension Array where Element == UInt8 {
    fileprivate func readLE(_ type: UInt16.Type, at offset: Int) -> UInt16 {
        UInt16(self[offset]) | UInt16(self[offset + 1]) << 8
    }

    fileprivate func readLE(_ type: UInt32.Type, at offset: Int) -> UInt32 {
        var value: UInt32 = 0
        for index in 0..<4 { value |= UInt32(self[offset + index]) << UInt32(index * 8) }
        return value
    }
}
