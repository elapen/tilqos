import Foundation

/// The letters a script's trigram model is built over, plus a word-boundary symbol.
///
/// One alphabet per script rather than per language: Russian and Kazakh share the same
/// 42-letter Cyrillic inventory, and using one inventory for both means their models are
/// directly comparable index for index.
public struct Alphabet: Sendable {
    public enum Kind: UInt8, Sendable {
        case latin = 0
        case cyrillic = 1
    }

    /// 26 letters plus the apostrophe, so English contractions stay one word.
    public static let latin = Alphabet(kind: .latin, characters: "abcdefghijklmnopqrstuvwxyz'")

    /// 33 Russian letters followed by the 9 Kazakh-specific ones: ә ғ қ ң ө ұ ү һ і.
    public static let cyrillic = Alphabet(
        kind: .cyrillic,
        characters: "абвгдеёжзийклмнопрстуфхцчшщъыьэюя"
            + "\u{04D9}\u{0493}\u{049B}\u{04A3}\u{04E9}\u{04B1}\u{04AF}\u{04BB}\u{0456}"
    )

    /// The nine letters Kazakh adds to the Russian inventory.
    ///
    /// Their presence is positive evidence that Cyrillic text is Kazakh rather than
    /// Russian. Their absence is not evidence of anything, since the two alphabets
    /// otherwise coincide — which is why the app falls back to per-app memory instead of
    /// guessing.
    public static let kazakhSpecific: Set<Character> = [
        "\u{04D9}", "\u{0493}", "\u{049B}", "\u{04A3}", "\u{04E9}",
        "\u{04B1}", "\u{04AF}", "\u{04BB}", "\u{0456}",
    ]

    public static func containsKazakhSpecific(_ text: String) -> Bool {
        text.lowercased().contains { kazakhSpecific.contains($0) }
    }

    public static func forKind(_ kind: Kind) -> Alphabet {
        switch kind {
        case .latin: return .latin
        case .cyrillic: return .cyrillic
        }
    }

    public static func forScript(_ script: Script) -> Alphabet {
        switch script {
        case .latin: return .latin
        case .cyrillic: return .cyrillic
        }
    }

    public let kind: Kind
    public let characters: [Character]
    private let lookup: [Character: Int]

    init(kind: Kind, characters: String) {
        self.kind = kind
        self.characters = Array(characters)
        var lookup: [Character: Int] = [:]
        for (index, character) in self.characters.enumerated() { lookup[character] = index }
        self.lookup = lookup
    }

    public var letterCount: Int { characters.count }
    /// Letters plus the boundary marker — the dimension of the trigram cube.
    public var symbolCount: Int { characters.count + 1 }
    /// Index of the word-boundary symbol; also stands in for anything off-alphabet.
    public var boundary: Int { characters.count }

    public func index(of character: Character) -> Int? {
        lookup[character]
    }

    /// Maps a word to symbol indices.
    ///
    /// Characters outside the alphabet become boundaries and are counted separately —
    /// that is what makes `с2леметс3з` score badly as Russian without being rejected
    /// outright: the digit splits the word and costs it a penalty.
    public func encode(_ word: String) -> (symbols: [Int], unknown: Int) {
        var symbols: [Int] = []
        var unknown = 0
        symbols.reserveCapacity(word.count)
        for character in word.lowercased() {
            if let index = lookup[character] {
                symbols.append(index)
            } else {
                symbols.append(boundary)
                unknown += 1
            }
        }
        return (symbols, unknown)
    }

    /// True when every character is a letter of this alphabet.
    public func covers(_ word: String) -> Bool {
        word.lowercased().allSatisfy { lookup[$0] != nil }
    }
}
