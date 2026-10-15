import Carbon.HIToolbox
import XCTest

@testable import TilqosKit

/// These run against the layouts macOS actually ships, not against fixtures, so they
/// fail if Apple ever changes a layout under us — which is exactly what we want,
/// because the whole Russian/Kazakh cascade rests on the shape of those tables.
final class KeyMapTableTests: XCTestCase {
    private static let abcID = "com.apple.keylayout.ABC"
    private static let russianID = "com.apple.keylayout.Russian"
    private static let kazakhID = "com.apple.keylayout.Kazakh"

    private static let manager: InputSourceManager = {
        let manager = InputSourceManager()
        manager.reloadTables()
        return manager
    }()

    private func table(_ sourceID: String) throws -> KeyMapTable {
        guard let table = Self.manager.table(forSourceID: sourceID) else {
            throw XCTSkip("Layout \(sourceID) is not enabled on this machine")
        }
        return table
    }

    // MARK: - Language detection

    func testLayoutLanguagesComeFromTextInputServices() throws {
        XCTAssertEqual(try table(Self.abcID).lang, .en)
        XCTAssertEqual(try table(Self.russianID).lang, .ru)
        XCTAssertEqual(try table(Self.kazakhID).lang, .kk)
    }

    // MARK: - The central claim

    func testRussianAndKazakhDifferOnlyOnTheNumberRow() throws {
        let russian = try table(Self.russianID)
        let kazakh = try table(Self.kazakhID)

        // kc10 and kc50 are the two keys flanking the main block; kc93 is the JIS Yen
        // key, which only exists on Japanese keyboards and mirrors kc10.
        let expected: Set<UInt16> = [10, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 50, 93]
        XCTAssertEqual(
            russian.differingKeyCodes(from: kazakh), expected,
            "Russian and Kazakh must differ only on the number row and the bracket keys"
        )
    }

    /// The claim the whole Russian/Kazakh cascade rests on, asserted directly: on every
    /// key where the two layouts disagree, the Russian side is a digit or a punctuation
    /// mark — never a letter.
    ///
    /// That is what makes Russian text immune to being typed on the Kazakh layout, and
    /// what makes a digit inside a Cyrillic word such a strong signal of the reverse
    /// mistake. (The Kazakh side of those same keys is of course Cyrillic — ә, і, ғ and
    /// friends are exactly what the keys are there for.)
    func testEveryKeyThatDiffersIsNonLetterOnTheRussianSide() throws {
        let russian = try table(Self.russianID)
        let kazakh = try table(Self.kazakhID)

        var checked = 0
        for keyCode in russian.differingKeyCodes(from: kazakh) {
            for mods in KeyMapTable.typingVariants {
                guard let text = russian.text(for: KeyStroke(keyCode: keyCode, mods: mods))
                else { continue }
                checked += 1
                XCTAssertFalse(
                    text.contains(where: \.isLetter),
                    "kc\(keyCode) differs between the layouts but yields letter '\(text)' "
                        + "in Russian — the cascade assumption is broken"
                )
            }
        }
        XCTAssertGreaterThan(checked, 0, "expected to have inspected some differing keys")
    }

    func testEveryRussianLetterSitsOnTheSameKeyInKazakh() throws {
        let russian = try table(Self.russianID)
        let kazakh = try table(Self.kazakhID)

        for letter in "абвгдеёжзийклмнопрстуфхцчшщъыьэюя" {
            guard let stroke = russian.stroke(for: letter) else {
                XCTFail("Russian layout cannot produce \(letter)")
                continue
            }
            XCTAssertEqual(
                kazakh.text(for: stroke), String(letter),
                "\(letter) must be on the same key in both Cyrillic layouts"
            )
        }
    }

    /// Consequence of the above: a purely Russian word typed on the Kazakh layout comes
    /// out correct, so there is nothing to fix and no reason to switch.
    func testRussianWordTypedOnKazakhLayoutIsUnchanged() throws {
        let russian = try table(Self.russianID)
        let kazakh = try table(Self.kazakhID)

        let word = "Астана"
        let strokes = try XCTUnwrap(russian.strokes(for: word))
        XCTAssertEqual(kazakh.render(strokes), word)
    }

    // MARK: - Latin / Cyrillic confusion

    func testCyrillicWordRenderedOnLatinLayout() throws {
        let russian = try table(Self.russianID)
        let abc = try table(Self.abcID)

        let strokes = try XCTUnwrap(russian.strokes(for: "привет"))
        XCTAssertEqual(abc.render(strokes), "ghbdtn")
    }

    func testLatinWordRenderedOnCyrillicLayout() throws {
        let russian = try table(Self.russianID)
        let abc = try table(Self.abcID)

        // Latin "o" is kc31, which carries щ on ЙЦУКЕН — not о, which lives on kc38.
        let strokes = try XCTUnwrap(abc.strokes(for: "hello"))
        XCTAssertEqual(russian.render(strokes), "руддщ")
    }

    // MARK: - Russian / Kazakh confusion

    func testKazakhWordTypedOnRussianLayoutTurnsSpecialLettersIntoDigits() throws {
        let russian = try table(Self.russianID)
        let kazakh = try table(Self.kazakhID)

        let strokes = try XCTUnwrap(kazakh.strokes(for: "сәлеметсіз"))
        XCTAssertEqual(russian.render(strokes), "с2леметс3з")
    }

    func testKazakhWordWithRepeatedSpecialLetters() throws {
        let russian = try table(Self.russianID)
        let kazakh = try table(Self.kazakhID)

        let strokes = try XCTUnwrap(kazakh.strokes(for: "қазақ"))
        XCTAssertEqual(russian.render(strokes), "0аза0")
    }

    func testDigitsTypedOnKazakhLayoutBecomeKazakhLetters() throws {
        let abc = try table(Self.abcID)
        let kazakh = try table(Self.kazakhID)

        let strokes = try XCTUnwrap(abc.strokes(for: "25"))
        XCTAssertEqual(kazakh.render(strokes), "әғ")
    }

    /// The Kazakh layout has no digits on the unshifted number row at all — they sit on
    /// the Option layer. This is why a character-to-character conversion table cannot
    /// round-trip Kazakh text, and why we store keystrokes instead.
    func testKazakhNumberRowCarriesLettersAndHidesDigitsBehindOption() throws {
        let kazakh = try table(Self.kazakhID)

        // Digit key → Kazakh letter unshifted, the digit only with Option.
        let expected: [(keyCode: UInt16, letter: String, digit: String)] = [
            (19, "ә", "2"), (20, "і", "3"), (21, "ң", "4"), (23, "ғ", "5"),
            (25, "ұ", "9"), (27, "ө", "-"), (28, "ү", "8"), (29, "қ", "0"),
        ]
        for case let (keyCode, letter, digit) in expected {
            XCTAssertEqual(
                kazakh.text(for: KeyStroke(keyCode: keyCode)), letter,
                "kc\(keyCode) should be \(letter) unshifted")
            XCTAssertEqual(
                kazakh.text(for: KeyStroke(keyCode: keyCode, mods: .option)), digit,
                "kc\(keyCode) should be \(digit) with Option")
        }
    }

    /// Digits exist on the numeric keypad under every layout, so the reverse map has to
    /// prefer the main block or every digit would resolve to a key laptops do not have.
    func testReverseMapPrefersTheMainBlockOverTheKeypad() throws {
        let abc = try table(Self.abcID)

        XCTAssertEqual(abc.stroke(for: "5")?.keyCode, 23)
        XCTAssertEqual(abc.stroke(for: "0")?.keyCode, 29)
        XCTAssertEqual(abc.stroke(for: "1")?.keyCode, 18)
    }

    /// The Option layers genuinely differ — Kazakh carries Ukrainian ґ/є/ї there. The
    /// cascade must therefore compare only the variants ordinary typing produces.
    func testOptionLayersDifferAndAreExcludedFromTheComparison() throws {
        let russian = try table(Self.russianID)
        let kazakh = try table(Self.kazakhID)

        let allVariants = (0..<ModState.variantCount).map { ModState(rawValue: UInt8($0)) }
        let differingEverywhere = russian.differingKeyCodes(from: kazakh, variants: allVariants)
        let differingWhileTyping = russian.differingKeyCodes(from: kazakh)

        XCTAssertGreaterThan(
            differingEverywhere.count, differingWhileTyping.count,
            "Option layers are expected to diverge beyond the number row")
        XCTAssertTrue(differingWhileTyping.isSubset(of: differingEverywhere))
        // A letter key: same while typing, different once Option is involved.
        XCTAssertTrue(differingEverywhere.contains(30))
        XCTAssertFalse(differingWhileTyping.contains(30))
    }

    // MARK: - Round tripping

    func testRoundTripThroughEveryLayoutPreservesStrokes() throws {
        let layouts = [
            try table(Self.abcID), try table(Self.russianID), try table(Self.kazakhID),
        ]
        for source in layouts {
            for target in layouts {
                let text = "сәлем 25 hello"
                guard let strokes = source.strokes(for: text) else { continue }
                // Rendering through the target and reading it back on the target must
                // land on the same keystrokes.
                let rendered = target.render(strokes)
                guard let recovered = target.strokes(for: rendered) else { continue }
                XCTAssertEqual(
                    recovered, strokes,
                    "\(source.sourceID) → \(target.sourceID) lost information"
                )
            }
        }
    }

    // MARK: - Table hygiene

    func testControlCharactersAreNotPartOfAnyTable() throws {
        let russian = try table(Self.russianID)
        // Return, Tab and Escape must not look like text-producing keys.
        for keyCode in [UInt16(kVK_Return), UInt16(kVK_Tab), UInt16(kVK_Escape)] {
            XCTAssertNil(russian.text(for: KeyStroke(keyCode: keyCode)))
        }
        // Space is deliberately kept — it is the word boundary.
        XCTAssertEqual(russian.text(for: KeyStroke(keyCode: UInt16(kVK_Space))), " ")
    }
}
