import XCTest

@testable import TilqosKit

/// These are the tests that decide whether the detector can work at all. If a trigram
/// model cannot tell `привет` from `руддщ`, no amount of scoring logic on top will save it.
final class TrigramModelTests: XCTestCase {
    /// Resources/Models relative to this source file.
    static let modelDirectory: URL = URL(fileURLWithPath: #filePath)
        .deletingLastPathComponent()  // TilqosKitTests
        .deletingLastPathComponent()  // Tests
        .deletingLastPathComponent()  // package root
        .appendingPathComponent("Resources/Models")

    private static let store: ModelStore = {
        let store = ModelStore()
        store.load(from: modelDirectory)
        return store
    }()

    private func model(_ lang: Lang) throws -> TrigramModel {
        guard let model = Self.store.model(for: lang) else {
            throw XCTSkip("No model for \(lang.rawValue) — run ./generate-models.sh")
        }
        return model
    }

    private func cost(_ word: String, _ lang: Lang) throws -> Double {
        let score = try XCTUnwrap(try model(lang).score(word), "\(word) is unscorable")
        return score.penalizedCost
    }

    // MARK: - Round trip

    func testModelSurvivesEncodeDecode() throws {
        let original = try model(.kk)
        let restored = try TrigramModel.decode(original.encode())

        XCTAssertEqual(restored.lang, .kk)
        XCTAssertEqual(restored.alphabet.kind, .cyrillic)
        XCTAssertEqual(restored.score("қазақ")?.meanCost, original.score("қазақ")?.meanCost)
    }

    func testGarbageIsRejected() {
        XCTAssertThrowsError(try TrigramModel.decode(Data([1, 2, 3])))
        XCTAssertThrowsError(try TrigramModel.decode(Data(repeating: 0, count: 40)))
    }

    // MARK: - Real words beat layout garble

    func testEnglishModelSeparatesWordsFromCyrillicGarble() throws {
        // "привет" typed on ABC.
        let real = try ["hello", "world", "keyboard", "language"].map { try cost($0, .en) }
        let garble = try ["ghbdtn", "hjccbz", "gjkyjcnm", "ktdjuj"].map { try cost($0, .en) }

        XCTAssertLessThan(
            real.max()!, garble.min()!,
            "every real English word must score better than every layout garble")
    }

    func testRussianModelSeparatesWordsFromLatinGarble() throws {
        // "hello", "world", "please", "switch" typed on the Russian layout.
        let real = try ["привет", "клавиатура", "переключатель", "спасибо"].map {
            try cost($0, .ru)
        }
        let garble = try ["руддщ", "цщкдв", "здефну", "ыцйксд"].map { try cost($0, .ru) }

        XCTAssertLessThan(real.max()!, garble.min()!)
    }

    func testKazakhModelSeparatesWordsFromGarble() throws {
        let real = try ["сәлеметсіз", "қазақстан", "рақмет", "мүмкіндік"].map { try cost($0, .kk) }
        let garble = try ["руддщ", "цщкдв", "ыцйксд"].map { try cost($0, .kk) }

        XCTAssertLessThan(real.max()!, garble.min()!)
    }

    // MARK: - Russian versus Kazakh

    /// Words carrying Kazakh-specific letters must prefer the Kazakh model.
    func testKazakhWordsPreferTheKazakhModel() throws {
        for word in ["сәлеметсіз", "қазақ", "мүмкіндік", "рақмет", "әңгіме"] {
            XCTAssertLessThan(
                try cost(word, .kk), try cost(word, .ru),
                "\(word) should look more Kazakh than Russian")
        }
    }

    func testRussianWordsPreferTheRussianModel() throws {
        for word in ["привет", "спасибо", "клавиатура", "который", "сегодня"] {
            XCTAssertLessThan(
                try cost(word, .ru), try cost(word, .kk),
                "\(word) should look more Russian than Kazakh")
        }
    }

    // MARK: - The digit signal

    /// A Kazakh word typed on the Russian layout keeps its Russian letters but turns its
    /// special letters into digits. The digits are what give it away.
    func testDigitInsideACyrillicWordIsPenalized() throws {
        let mistyped = try cost("с2леметс3з", .ru)
        let correct = try cost("сәлеметсіз", .kk)

        XCTAssertGreaterThan(
            mistyped, correct,
            "the mistyped form must look worse than the corrected one")

        let score = try XCTUnwrap(try model(.ru).score("с2леметс3з"))
        XCTAssertEqual(score.unknownCount, 2, "both digits should be counted as off-alphabet")
    }

    func testCorrectKazakhBeatsTheMistypedRussianRendering() throws {
        // The decision the refiner has to make, stated as a comparison.
        for (mistyped, corrected) in [("с2леметс3з", "сәлеметсіз"), ("0аза0", "қазақ")] {
            XCTAssertGreaterThan(try cost(mistyped, .ru), try cost(corrected, .kk))
        }
    }

    // MARK: - Guards on the scorer itself

    func testVeryShortWordsAreUnscorable() throws {
        let model = try model(.ru)
        XCTAssertNil(model.score("a"))
        XCTAssertNil(model.score(""))
        XCTAssertNotNil(model.score("да"))
    }

    /// Diagnostic: prints the numbers the thresholds are calibrated against.
    func testPrintSeparationMargins() throws {
        var lines: [String] = []
        for (lang, real, garble) in [
            (Lang.en, ["hello", "keyboard"], ["ghbdtn", "gjkyjcnm"]),
            (Lang.ru, ["привет", "клавиатура"], ["руддщ", "цщкдв"]),
            (Lang.kk, ["қазақстан", "сәлеметсіз"], ["руддщ", "ыцйксд"]),
        ] {
            let realCosts = try real.map { try cost($0, lang) }
            let garbleCosts = try garble.map { try cost($0, lang) }
            lines.append(
                String(
                    format: "%@: real %.2f–%.2f | garble %.2f–%.2f | margin %.2f",
                    lang.rawValue, realCosts.min()!, realCosts.max()!,
                    garbleCosts.min()!, garbleCosts.max()!,
                    garbleCosts.min()! - realCosts.max()!))
        }
        print("=== trigram separation ===\n" + lines.joined(separator: "\n"))
    }
}
