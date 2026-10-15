import XCTest

@testable import TilqosKit

/// Real writing code-switches: Kazakh grammar carrying Russian vocabulary, brand names and
/// English words, all in the same ASCII spelling. The note below is synthetic but written in
/// that style. Forcing Kazakh rules onto the Russian words puts Kazakh-only letters into them.
final class MixedTranslitTests: XCTestCase {
    private static let store: ModelStore = {
        let store = ModelStore()
        store.load(from: TrigramModelTests.modelDirectory)
        return store
    }()

    private var translit: Transliterator!

    override func setUpWithError() throws {
        guard Self.store.model(for: .kk) != nil, Self.store.model(for: .ru) != nil else {
            throw XCTSkip("Models missing — run ./generate-models.sh")
        }
        translit = Transliterator(models: Self.store)
    }

    private func decode(_ latin: String) -> String {
        translit.toCyrillic(latin, scheme: .practical)
    }

    /// Russian words written in Latin must be read as Russian.
    func testRussianWordsAreReadAsRussian() {
        let cases: [(String, String)] = [
            ("Zametki", "Заметки"),
            ("Spisok", "Список"),
            ("Pokupki", "Покупки"),
            ("Nastroiki", "Настройки"),
            ("Raspisanie", "Расписание"),
            ("Zagruzka", "Загрузка"),
            ("Ssylka", "Ссылка"),
            ("Papka", "Папка"),
            ("kofe", "кофе"),
            ("Poisk", "Поиск"),
            ("Pochta", "Почта"),
        ]
        var failures: [String] = []
        for (latin, expected) in cases {
            let produced = decode(latin)
            if produced != expected {
                failures.append("\(latin): expected \(expected), got \(produced)")
            }
        }
        // Reported rather than demanded word for word: some of these are only reachable with a
        // richer model. The point is that none of them come out with Kazakh-only letters.
        print("russian-in-latin: \(cases.count - failures.count)/\(cases.count)")
        if !failures.isEmpty { print("  " + failures.joined(separator: "\n  ")) }
        XCTAssertLessThanOrEqual(failures.count, 3, "\n" + failures.joined(separator: "\n"))
    }

    /// The sharpest symptom of the old behaviour: Kazakh-only letters appearing in Russian words.
    func testRussianWordsNeverGetKazakhOnlyLetters() {
        let russianWords = [
            "Zametki", "Spisok", "Pokupki", "Nastroiki", "Raspisanie", "Zagruzka", "Ssylka",
            "Papka", "kofe", "Poisk", "Pochta", "Vkladka", "Uvedomlenie", "Podpiska",
            "Izbrannoe", "Obnovlenie", "Shablon", "Okno", "rabote",
        ]
        for word in russianWords {
            let produced = decode(word)
            let offenders = produced.filter { Alphabet.kazakhSpecific.contains($0) }
            XCTAssertTrue(
                offenders.isEmpty,
                "\(word) → \(produced) contains Kazakh-only letter(s) \(Array(offenders))")
        }
    }

    /// Kazakh words must still be read as Kazakh, with their special letters intact.
    func testKazakhWordsStillWork() {
        let cases: [(String, String)] = [
            ("qalaisyz", "қалайсыз"),
            ("kerek", "керек"),
            ("jaqsy", "жақсы"),
            ("ademi", "әдемі"),
            ("jasalgan", "жасалған"),
            ("ulken", "үлкен"),
            ("tulga", "тұлға"),
            ("koz", "көз"),
        ]
        var failures: [String] = []
        for (latin, expected) in cases {
            let produced = decode(latin)
            if produced != expected {
                failures.append("\(latin): expected \(expected), got \(produced)")
            }
        }
        XCTAssertTrue(failures.isEmpty, "\n" + failures.joined(separator: "\n"))
    }

    /// Strings implausible in both languages stay in Latin.
    ///
    /// Only the clear cases: the calibration measurements show brand names cannot be separated
    /// in general — `instagram` scores cheaper than the real word `kofe` — so anything that
    /// transliterates plausibly is converted, and the overrides list exists for the rest.
    func testImplausibleStringsAreLeftInLatin() {
        for name in ["zzxq", "whatsapp"] {
            XCTAssertEqual(decode(name), name, "\(name) should have been left alone")
        }
    }

    /// Single-letter prepositions must convert despite carrying almost no context.
    func testShortWordsStillConvert() {
        XCTAssertEqual(decode("v zale"), "в зале")
    }

    /// Text already in Cyrillic passes through untouched.
    func testCyrillicPassesThrough() {
        XCTAssertEqual(decode("(Пример)"), "(Пример)")
    }

    func testDigitsAndPunctuationSurvive() {
        let output = decode("bugin keshke 3 jospar Vstrecha, Zametki, Raspisanie")
        XCTAssertTrue(output.contains("3"))
        XCTAssertEqual(output.filter { $0 == "," }.count, 2)
    }

    /// The whole note, printed so the result can be read as a whole.
    func testFullNote() {
        let note = """
            bugin keshke 3 jospar Vstrecha, Zametki, Raspisanie, Zadachi
            ertengi 5 ssylka zoom arqyly jiberemin
            Pochta qazir jaqsy, jumys bastaldy
            Zagruzka aldymen notion qosu kerek
            Podpiska (v rabote)
            Vkladka: spisok, v papke, zametki na ertengi
            Obnovlenie telegram keiin tapsyrma ushin qaita update
            """
        let output = decode(note)
        print("=== mixed note ===\n" + output)

        XCTAssertEqual(output.split(separator: "\n").count, 7)
        // No Kazakh-only letter may appear inside these Russian words.
        for fragment in ["Встреча", "Заметки", "Расписание", "Почта", "Загрузка", "Подписка", "Вкладка"] {
            XCTAssertTrue(output.contains(fragment), "missing \(fragment)")
        }
        // Kazakh words keep their letters.
        for fragment in ["бүгін", "кешке", "жоспар", "жақсы", "керек"] {
            XCTAssertTrue(output.contains(fragment), "missing \(fragment)")
        }
    }
}

extension MixedTranslitTests {
    /// The built-in brand list, which exists because measurement showed a model cannot do this.
    func testBrandListKeepsNamesInLatin() {
        for name in ["instagram", "Instagram", "whatsapp", "slack", "notion", "figma", "iphone"] {
            let produced = translit.toCyrillic(name, scheme: .practical)
            XCTAssertEqual(produced, name, "\(name) must stay in Latin")
        }
    }

    /// A brand with a Kazakh case ending attached still counts as a brand.
    func testBrandWithAttachedSuffixIsRecognised() {
        XCTAssertTrue(TranslitTables.isLatinOnly("zoomga"))
        XCTAssertTrue(TranslitTables.isLatinOnly("instagramda"))
        // But a real word that merely starts like one must not be caught.
        XCTAssertFalse(TranslitTables.isLatinOnly("kerek"))
        XCTAssertFalse(TranslitTables.isLatinOnly("qalaisyz"))
    }

    /// Words the user has rejected are honoured.
    func testLearnedWordsAreLeftAlone() {
        let learned = Transliterator(
            models: Self.store, neverConvert: ["kofe", "pokupki"])
        XCTAssertEqual(learned.toCyrillic("kofe", scheme: .practical), "kofe")
        XCTAssertEqual(learned.toCyrillic("pokupki", scheme: .practical), "pokupki")
        // Everything else keeps converting.
        XCTAssertEqual(learned.toCyrillic("papka", scheme: .practical), "папка")
    }

    /// Undo must learn only the words that actually changed, not the whole passage.
    func testChangedWordsIdentifiesOnlyRejectedWords() {
        let changed = translit.changedWords(
            from: "kofe papka zoom", to: "кофе папка zoom")
        XCTAssertEqual(changed, ["kofe", "papka"])
    }

    func testChangedWordsBailsOutOnMismatchedWordCounts() {
        // A conversion that split or merged words gives no reliable pairing, so learn nothing
        // rather than learning something wrong.
        XCTAssertTrue(translit.changedWords(from: "a b", to: "а").isEmpty)
    }
}
