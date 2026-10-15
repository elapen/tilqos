import XCTest

@testable import TilqosKit

/// The lexicon reverses hunspell affix rules instead of storing every form, so its analyzer is
/// tested on a hand-made dictionary where every rule kind is visible, and then on the shipped
/// files for the words the whole app depends on.
final class LexiconTests: XCTestCase {
    // MARK: - The analyzer, on synthetic data

    private static let affixes = """
        SET UTF-8
        PFX U Y 1
        PFX U   0     un         .

        SFX S Y 3
        SFX S   y     ies        [^aeiou]y
        SFX S   0     s          [aeiou]y
        SFX S   0     s          [^y]

        SFX D N 2
        SFX D   0     d          e
        SFX D   0     ed         [^e]

        SFX L Y 2
        SFX L   ь     ю          ь
        SFX L   0     ом         [^ь]
        """

    private static let dictionary = """
        6
        happy/U
        try/SD
        toy/S
        play/SDU
        боль/L
        стол/L
        """

    private func lexicon(extra: [String] = [], core: [String] = []) -> Lexicon {
        Lexicon.build(
            lang: .en, dictionary: Self.dictionary, affixes: Self.affixes, extra: extra, core: core)
    }

    func testStemsAreWords() {
        let lexicon = lexicon()
        for word in ["happy", "try", "toy", "play", "боль", "стол"] {
            XCTAssertTrue(lexicon.contains(word), word)
        }
        XCTAssertFalse(lexicon.contains("happ"))
        XCTAssertFalse(lexicon.contains(""))
    }

    func testSuffixRulesRespectStripAndCondition() {
        let lexicon = lexicon()
        // `y` → `ies` after a consonant, plain `s` after a vowel.
        XCTAssertTrue(lexicon.contains("tries"))
        XCTAssertTrue(lexicon.contains("toys"))
        XCTAssertFalse(lexicon.contains("tryies"))
        XCTAssertFalse(lexicon.contains("toies"), "the condition [^aeiou]y must reject toy")
        // `d` after e, `ed` elsewhere.
        XCTAssertTrue(lexicon.contains("tried") == false, "try has no rule producing tried")
        XCTAssertTrue(lexicon.contains("played"))
        XCTAssertTrue(lexicon.contains("tryed"), "the literal rule output, however ugly")
        // Cyrillic with a strip: боль → болю, стол → столом, never столю.
        XCTAssertTrue(lexicon.contains("болю"))
        XCTAssertTrue(lexicon.contains("столом"))
        XCTAssertFalse(lexicon.contains("столю"))
        XCTAssertFalse(lexicon.contains("больом"))
    }

    func testFlagsAreRespected() {
        // `happy` has only U, so no suffix rule applies to it.
        XCTAssertFalse(lexicon().contains("happies"))
        XCTAssertFalse(lexicon().contains("happys"))
    }

    func testPrefixesAndCrossProduct() {
        let lexicon = lexicon()
        XCTAssertTrue(lexicon.contains("unhappy"))
        XCTAssertTrue(lexicon.contains("unplay"))
        // U and S are both cross-product rules, and play carries both.
        XCTAssertTrue(lexicon.contains("unplays"))
        // D is not cross-product, so it does not combine with the prefix.
        XCTAssertFalse(lexicon.contains("unplayed"))
        // toy has no U.
        XCTAssertFalse(lexicon.contains("untoy"))
    }

    func testNormalizationFoldsCaseYoAndApostrophes() {
        let lexicon = Lexicon.build(
            lang: .ru, dictionary: "2\nещё\ndon't", affixes: "SET UTF-8")
        XCTAssertTrue(lexicon.contains("еще"))
        XCTAssertTrue(lexicon.contains("ЕЩЁ"))
        XCTAssertTrue(lexicon.contains("don\u{2019}t"))
    }

    func testExtraAndCoreWords() {
        let lexicon = lexicon(extra: ["iphone", "# a comment", ""], core: ["try", "toy"])
        XCTAssertTrue(lexicon.contains("iPhone"))
        XCTAssertTrue(lexicon.isCore("try"))
        XCTAssertFalse(lexicon.isCore("play"))
        XCTAssertEqual(lexicon.lookup("toy").core, true)
        XCTAssertEqual(lexicon.lookup("toys").known, true)
        XCTAssertEqual(lexicon.lookup("toys").core, false)
    }

    func testSurvivesEncodeDecode() throws {
        let original = lexicon(extra: ["iphone"], core: ["try"])
        let restored = try Lexicon.decode(original.encode())
        XCTAssertEqual(restored.lang, original.lang)
        XCTAssertEqual(restored.stemCount, original.stemCount)
        XCTAssertEqual(restored.ruleCount, original.ruleCount)
        for word in ["tries", "unplays", "столом", "iphone", "happy"] {
            XCTAssertTrue(restored.contains(word), word)
        }
        XCTAssertFalse(restored.contains("unplayed"))
        XCTAssertTrue(restored.isCore("try"))
        // Byte-for-byte reproducible, so regenerating the models changes nothing.
        XCTAssertEqual(lexicon(extra: ["iphone"], core: ["try"]).encode(), original.encode())
    }

    func testGarbageIsRejected() {
        XCTAssertThrowsError(try Lexicon.decode(Data([1, 2, 3])))
        XCTAssertThrowsError(try Lexicon.decode(Data(repeating: 0, count: 64)))
        var truncated = lexicon().encode()
        truncated.removeLast(10)
        XCTAssertThrowsError(try Lexicon.decode(truncated))
    }

    // MARK: - The shipped lexicons

    private static let store: ModelStore = {
        let store = ModelStore()
        store.load(from: TrigramModelTests.modelDirectory)
        return store
    }()

    private func shipped(_ lang: Lang) throws -> Lexicon {
        guard let lexicon = Self.store.lexicon(for: lang) else {
            throw XCTSkip("No lexicon for \(lang.rawValue) — run ./generate-models.sh")
        }
        return lexicon
    }

    /// The words the original complaint was about, and the inflected forms a headword list
    /// never had.
    func testRussianInflectedFormsAreKnown() throws {
        let russian = try shipped(.ru)
        for word in ["еще", "ещё", "сделал", "сделала", "людей", "книгу", "где", "тот", "тут",
            "мне", "пришёл", "было", "тебе", "люблю", "хорошо"]
        {
            XCTAssertTrue(russian.contains(word), word)
        }
        for word in ["еще", "как", "что", "это", "где"] {
            XCTAssertTrue(russian.isCore(word), word)
        }
    }

    /// Hyphenated words are judged a level up: whole if the dictionary has them, otherwise by
    /// parts of two letters or more.
    func testHyphenatedWords() throws {
        _ = try shipped(.ru)
        let scorer = WordScorer(models: Self.store)
        XCTAssertTrue(scorer.isKnown("кто-то", lang: .ru, learned: nil))
        XCTAssertTrue(scorer.isKnown("из-за", lang: .ru, learned: nil))
        XCTAssertFalse(scorer.isKnown("к-к", lang: .ru, learned: nil), "көк typed on Russian")
        XCTAssertFalse(scorer.isKnown("-те", lang: .ru, learned: nil))
    }

    func testEnglishFormsAndTermsAreKnown() throws {
        let english = try shipped(.en)
        for word in ["the", "played", "running", "don't", "unhappy", "telegram", "iphone",
            "email", "github", "afk", "lol"]
        {
            XCTAssertTrue(english.contains(word), word)
        }
    }

    func testKazakhFormsAreKnown() throws {
        let kazakh = try shipped(.kk)
        for word in ["сәлеметсіз", "қазақ", "жақсы", "бұл", "және", "көп", "үшін", "мектептер"] {
            XCTAssertTrue(kazakh.contains(word), word)
        }
    }

    /// Layout garble is not a word in any language — the asymmetry the detector rests on.
    func testLayoutGarbleIsNotAWord() throws {
        let english = try shipped(.en)
        let russian = try shipped(.ru)
        for garble in ["ghbdtn", "rfr", "xnj", "ntcn", "ltkf", "cgfcb,j", "vyt", "ytn"] {
            XCTAssertFalse(english.contains(garble), garble)
        }
        for garble in ["руддщ", "цщкдв", "лунищфкв", "еуые", "нщг", "тще"] {
            XCTAssertFalse(russian.contains(garble), garble)
        }
    }
}
